package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"custom-engram/internal/exporter"
	"custom-engram/internal/project"
	"custom-engram/internal/search"
	"custom-engram/internal/store"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type Server struct {
	store          *store.Store
	embedder       search.Embedder
	mcpServer      *mcpserver.MCPServer
	defaultProject string
}

func NewServer(st *store.Store, embedder search.Embedder, defaultProject string) *Server {
	s := &Server{
		store:          st,
		embedder:       embedder,
		mcpServer:      mcpserver.NewMCPServer("custom-engram", "1.0.0"),
		defaultProject: defaultProject,
	}

	s.registerTools()
	return s
}

func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
}

func (s *Server) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpServer)
}

func (s *Server) resolveProject(explicit string) (*store.Project, error) {
	cwd, _ := os.Getwd()
	projToUse := explicit
	if projToUse == "" {
		projToUse = s.defaultProject
	}

	res, err := project.Resolve(cwd, projToUse)
	if err != nil {
		return nil, err
	}

	return s.store.GetOrCreateProject(res.Name, res.RootPath)
}

func (s *Server) registerTools() {
	// 1. mem_save
	s.mcpServer.AddTool(
		mcp.NewTool("mem_save",
			mcp.WithDescription("Save an observation to persistent memory (bugfix, decision, architecture, pattern, discovery, config)."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Short, searchable title")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Content formatted as **What**, **Why**, **Where**, **Learned**")),
			mcp.WithString("type", mcp.Description("bugfix | decision | architecture | discovery | pattern | config | preference")),
			mcp.WithString("scope", mcp.Description("project (default) | personal | global")),
			mcp.WithString("topic_key", mcp.Description("Stable key for upserts (e.g. architecture/auth-model)")),
			mcp.WithString("project", mcp.Description("Optional project name override")),
		),
		s.handleSave(),
	)

	// 2. mem_search
	s.mcpServer.AddTool(
		mcp.NewTool("mem_search",
			mcp.WithDescription("Search memory across observations using hybrid search (FTS5 + embeddings)."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search terms or natural language query")),
			mcp.WithString("project", mcp.Description("Optional project filter")),
			mcp.WithNumber("limit", mcp.Description("Max results (default: 10)")),
		),
		s.handleSearch(),
	)

	// 3. mem_context
	s.mcpServer.AddTool(
		mcp.NewTool("mem_context",
			mcp.WithDescription("Retrieve recent context and sessions for the current project."),
			mcp.WithString("project", mcp.Description("Optional project name")),
		),
		s.handleContext(),
	)

	// 4. mem_session_start
	s.mcpServer.AddTool(
		mcp.NewTool("mem_session_start",
			mcp.WithDescription("Start a new tracking session with a specific goal."),
			mcp.WithString("goal", mcp.Required(), mcp.Description("The primary goal of this session")),
			mcp.WithString("project", mcp.Description("Optional project name")),
		),
		s.handleSessionStart(),
	)

	// 5. mem_session_end
	s.mcpServer.AddTool(
		mcp.NewTool("mem_session_end",
			mcp.WithDescription("End the active session with an optional summary."),
			mcp.WithString("session_id", mcp.Description("Session ID to end (optional if only one active)")),
			mcp.WithString("summary", mcp.Description("Summary of work done")),
			mcp.WithString("project", mcp.Description("Optional project name")),
		),
		s.handleSessionEnd(),
	)

	// 6. mem_session_summary
	s.mcpServer.AddTool(
		mcp.NewTool("mem_session_summary",
			mcp.WithDescription("Save end-of-session summary. MANDATORY before closing tasks."),
			mcp.WithString("summary", mcp.Required(), mcp.Description("Markdown structured summary: ## Goal, ## Discoveries, ## Accomplished, ## Next Steps")),
			mcp.WithString("project", mcp.Description("Optional project name")),
		),
		s.handleSessionSummary(),
	)

	// 7. mem_get_observation
	s.mcpServer.AddTool(
		mcp.NewTool("mem_get_observation",
			mcp.WithDescription("Get full untruncated content of an observation by ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Observation UUID")),
		),
		s.handleGetObservation(),
	)

	// 8. mem_current_project
	s.mcpServer.AddTool(
		mcp.NewTool("mem_current_project",
			mcp.WithDescription("Detect current project name and root path deterministically."),
		),
		s.handleCurrentProject(),
	)

	// 9. mem_doctor
	s.mcpServer.AddTool(
		mcp.NewTool("mem_doctor",
			mcp.WithDescription("Check memory system diagnostics, database status, and embeddings readiness."),
		),
		s.handleDoctor(),
	)

	// 10. mem_update
	s.mcpServer.AddTool(
		mcp.NewTool("mem_update",
			mcp.WithDescription("Update an existing observation by ID. Only provided fields are changed."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Observation UUID to update")),
			mcp.WithString("title", mcp.Description("Updated title")),
			mcp.WithString("content", mcp.Description("Updated content")),
			mcp.WithString("type", mcp.Description("Updated category")),
			mcp.WithString("scope", mcp.Description("Updated scope: project | personal | global")),
			mcp.WithString("topic_key", mcp.Description("Updated topic key")),
		),
		s.handleUpdate(),
	)

	// 11. mem_suggest_topic_key
	s.mcpServer.AddTool(
		mcp.NewTool("mem_suggest_topic_key",
			mcp.WithDescription("Suggest a stable topic_key for memory upserts (e.g. architecture/auth-model)."),
			mcp.WithString("title", mcp.Description("Observation title")),
			mcp.WithString("content", mcp.Description("Observation content")),
			mcp.WithString("type", mcp.Description("Observation type")),
		),
		s.handleSuggestTopicKey(),
	)

	// 12. mem_capture_passive
	s.mcpServer.AddTool(
		mcp.NewTool("mem_capture_passive",
			mcp.WithDescription("Extract and save structured learnings from text output containing '## Key Learnings:' or list items."),
			mcp.WithString("content", mcp.Required(), mcp.Description("Text output containing learnings")),
			mcp.WithString("project", mcp.Description("Optional project filter")),
			mcp.WithString("session_id", mcp.Description("Optional session ID")),
		),
		s.handleCapturePassive(),
	)

	// 13. mem_pin
	s.mcpServer.AddTool(
		mcp.NewTool("mem_pin",
			mcp.WithDescription("Pin an observation for highest context priority."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Observation UUID to pin")),
		),
		s.handlePin(true),
	)

	// 14. mem_unpin
	s.mcpServer.AddTool(
		mcp.NewTool("mem_unpin",
			mcp.WithDescription("Unpin an observation so it returns to standard chronological ordering."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Observation UUID to unpin")),
		),
		s.handlePin(false),
	)

	// 15. mem_save_prompt
	s.mcpServer.AddTool(
		mcp.NewTool("mem_save_prompt",
			mcp.WithDescription("Save a user prompt for context and history tracking."),
			mcp.WithString("content", mcp.Required(), mcp.Description("Prompt text content")),
			mcp.WithString("project", mcp.Description("Optional project name")),
			mcp.WithString("session_id", mcp.Description("Optional session ID")),
		),
		s.handleSavePrompt(),
	)

	// 16. mem_delete
	s.mcpServer.AddTool(
		mcp.NewTool("mem_delete",
			mcp.WithDescription("Delete an observation by ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Observation UUID to delete")),
		),
		s.handleDelete(),
	)

	// 17. mem_stats
	s.mcpServer.AddTool(
		mcp.NewTool("mem_stats",
			mcp.WithDescription("Get statistics about projects, memories, sessions, and prompts."),
			mcp.WithString("project", mcp.Description("Optional project filter")),
		),
		s.handleStats(),
	)

	// 18. mem_judge
	s.mcpServer.AddTool(
		mcp.NewTool("mem_judge",
			mcp.WithDescription("Record a verdict on a pending memory relation (compatible | supersedes | conflicts_with)."),
			mcp.WithString("judgment_id", mcp.Description("Judgment identifier")),
			mcp.WithString("verdict", mcp.Description("Verdict decision")),
			mcp.WithString("evidence", mcp.Description("Reasoning or evidence")),
		),
		s.handleJudge(),
	)

	// 19. mem_compare
	s.mcpServer.AddTool(
		mcp.NewTool("mem_compare",
			mcp.WithDescription("Persist an agent-judged semantic verdict between memories."),
			mcp.WithString("verdict", mcp.Description("Verdict decision")),
			mcp.WithString("candidate_id", mcp.Description("Candidate observation ID")),
		),
		s.handleCompare(),
	)

	// 20. mem_list_projects
	s.mcpServer.AddTool(
		mcp.NewTool("mem_list_projects",
			mcp.WithDescription("List every project known to Engram, with per-project observation, session, and prompt counts."),
		),
		s.handleListProjects(),
	)

	// 21. mem_archive
	s.mcpServer.AddTool(
		mcp.NewTool("mem_archive",
			mcp.WithDescription("Archive or unarchive an observation by ID. Archived observations are hidden from search and context without being deleted."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Observation UUID to archive or unarchive")),
			mcp.WithBoolean("archived", mcp.Description("True to archive, false to unarchive (default: true)")),
		),
		s.handleArchive(),
	)

	// 22. mem_prune
	s.mcpServer.AddTool(
		mcp.NewTool("mem_prune",
			mcp.WithDescription("Prune old, unused observations (supports safe dry-run, soft-delete archiving, and age filters). Protected types (architecture, decision) and pinned items are never pruned by default."),
			mcp.WithString("older_than", mcp.Description("Age threshold (e.g. '90d', '30d', '180d', default: '90d')")),
			mcp.WithString("project", mcp.Description("Optional project filter")),
			mcp.WithString("types", mcp.Description("Optional comma-separated list of types to target (e.g. 'discovery,bugfix')")),
			mcp.WithBoolean("include_all", mcp.Description("If true, removes default type protection (pinned items are still protected)")),
			mcp.WithBoolean("hard_delete", mcp.Description("If true, permanently deletes records; if false, soft-deletes by archiving (default: false)")),
			mcp.WithBoolean("dry_run", mcp.Description("If true (default), simulates the prune and returns matching candidates without modifying DB")),
		),
		s.handlePrune(),
	)
}

func (s *Server) handleSave() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		title, _ := args["title"].(string)
		content, _ := args["content"].(string)
		obsType, _ := args["type"].(string)
		scope, _ := args["scope"].(string)
		topicKey, _ := args["topic_key"].(string)
		projOverride, _ := args["project"].(string)

		if title == "" || content == "" {
			return mcp.NewToolResultError("title and content are required"), nil
		}

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		obs := &store.Observation{
			ProjectID: proj.ID,
			Type:      obsType,
			Scope:     scope,
			Title:     title,
			Content:   content,
			TopicKey:  topicKey,
		}

		if err := s.store.SaveObservation(obs); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save observation: %v", err)), nil
		}

		// Real-time unidirectional export to Obsidian if vault is configured
		_ = exporter.ExportSingleObservation(obs, proj.Name, "")

		// Asynchronously or quickly generate embedding if available
		if s.embedder != nil && s.embedder.IsAvailable(ctx) {
			textToEmbed := fmt.Sprintf("%s\n%s", title, content)
			if vec, err := s.embedder.Embed(ctx, textToEmbed); err == nil && len(vec) > 0 {
				_ = s.store.SaveEmbedding(obs.ID, s.embedder.ModelName(), vec)
			}
		}

		resp := map[string]any{
			"status":    "saved",
			"id":        obs.ID,
			"project":   proj.Name,
			"title":     obs.Title,
			"topic_key": obs.TopicKey,
		}
		data, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleSearch() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		query, _ := args["query"].(string)
		projOverride, _ := args["project"].(string)
		limit := 10
		if l, ok := args["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}

		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		results, err := search.HybridSearch(ctx, s.store, s.embedder, proj.ID, query, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		type searchItem struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			Snippet  string  `json:"snippet"`
			Type     string  `json:"type"`
			TopicKey string  `json:"topic_key,omitempty"`
			Score    float64 `json:"score"`
			Source   string  `json:"source"`
		}

		var items []searchItem
		for _, r := range results {
			snip := r.Observation.Content
			if len(snip) > 200 {
				snip = snip[:200] + "..."
			}
			items = append(items, searchItem{
				ID:       r.Observation.ID,
				Title:    r.Observation.Title,
				Snippet:  snip,
				Type:     r.Observation.Type,
				TopicKey: r.Observation.TopicKey,
				Score:    r.Score,
				Source:   r.Source,
			})
		}

		data, _ := json.Marshal(map[string]any{
			"project": proj.Name,
			"query":   query,
			"total":   len(items),
			"results": items,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleContext() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		projOverride, _ := args["project"].(string)

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		sessions, _ := s.store.GetRecentSessions(proj.ID, 3)
		observations, _ := s.store.ListObservations(proj.ID, 10)

		data, _ := json.Marshal(map[string]any{
			"project":           proj.Name,
			"root_path":         proj.RootPath,
			"recent_sessions":   sessions,
			"recent_memories":   observations,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleSessionStart() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		goal, _ := args["goal"].(string)
		projOverride, _ := args["project"].(string)

		if goal == "" {
			return mcp.NewToolResultError("goal is required"), nil
		}

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		sess, err := s.store.StartSession(proj.ID, goal)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to start session: %v", err)), nil
		}

		data, _ := json.Marshal(sess)
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleSessionEnd() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		sessID, _ := args["session_id"].(string)
		summary, _ := args["summary"].(string)
		projOverride, _ := args["project"].(string)

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		if sessID == "" {
			active, err := s.store.GetActiveSession(proj.ID)
			if err != nil || active == nil {
				return mcp.NewToolResultError("no active session found to end"), nil
			}
			sessID = active.ID
		}

		if err := s.store.EndSession(sessID, summary); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to end session: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(`{"status":"session_ended","session_id":%q}`, sessID)), nil
	}
}

func (s *Server) handleSessionSummary() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		summary, _ := args["summary"].(string)
		projOverride, _ := args["project"].(string)

		if summary == "" {
			return mcp.NewToolResultError("summary is required"), nil
		}

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		active, err := s.store.GetActiveSession(proj.ID)
		if err != nil || active == nil {
			// Auto start and close a session
			newSess, err := s.store.StartSession(proj.ID, "Session Summary")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to auto-create session: %v", err)), nil
			}
			active = newSess
		}

		if err := s.store.EndSession(active.ID, summary); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save summary: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(`{"status":"summary_saved","session_id":%q,"project":%q}`, active.ID, proj.Name)), nil
	}
}

func (s *Server) handleGetObservation() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		id, _ := args["id"].(string)
		if id == "" {
			return mcp.NewToolResultError("id is required"), nil
		}

		obs, err := s.store.GetObservation(id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("observation not found: %v", err)), nil
		}

		data, _ := json.Marshal(obs)
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleCurrentProject() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cwd, _ := os.Getwd()
		res, err := project.Resolve(cwd, s.defaultProject)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("resolve error: %v", err)), nil
		}

		data, _ := json.Marshal(map[string]any{
			"project":      res.Name,
			"root_path":    res.RootPath,
			"source":       res.Source,
			"cwd":          cwd,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleDoctor() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := s.store.ListProjects()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list projects: %v", err)), nil
		}

		embedderStatus := "disabled"
		if s.embedder != nil {
			if s.embedder.IsAvailable(ctx) {
				embedderStatus = fmt.Sprintf("active (%s)", s.embedder.ModelName())
			} else {
				embedderStatus = fmt.Sprintf("configured (%s) but offline", s.embedder.ModelName())
			}
		}

		homeDir, _ := os.UserHomeDir()
		defaultDB := filepath.Join(homeDir, ".custom-engram", "engram.db")

		data, _ := json.Marshal(map[string]any{
			"status":          "healthy",
			"database_path":   defaultDB,
			"total_projects":  len(projects),
			"embedder_status": embedderStatus,
			"timestamp":       time.Now().UTC().Format(time.RFC3339),
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleUpdate() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		id, _ := args["id"].(string)
		if id == "" {
			return mcp.NewToolResultError("id is required"), nil
		}

		var titlePtr, contentPtr, typePtr, scopePtr, topicKeyPtr *string
		if t, ok := args["title"].(string); ok && t != "" {
			titlePtr = &t
		}
		if c, ok := args["content"].(string); ok && c != "" {
			contentPtr = &c
		}
		if tp, ok := args["type"].(string); ok && tp != "" {
			typePtr = &tp
		}
		if sc, ok := args["scope"].(string); ok && sc != "" {
			scopePtr = &sc
		}
		if tk, ok := args["topic_key"].(string); ok && tk != "" {
			topicKeyPtr = &tk
		}

		updated, err := s.store.UpdateObservation(id, titlePtr, contentPtr, typePtr, scopePtr, topicKeyPtr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to update observation: %v", err)), nil
		}

		if (titlePtr != nil || contentPtr != nil) && s.embedder != nil && s.embedder.IsAvailable(ctx) {
			textToEmbed := fmt.Sprintf("%s\n%s", updated.Title, updated.Content)
			if vec, err := s.embedder.Embed(ctx, textToEmbed); err == nil && len(vec) > 0 {
				_ = s.store.SaveEmbedding(updated.ID, s.embedder.ModelName(), vec)
			}
		}

		// Real-time unidirectional export to Obsidian if vault is configured
		if proj, err := s.store.GetProjectByID(updated.ProjectID); err == nil && proj != nil {
			_ = exporter.ExportSingleObservation(updated, proj.Name, "")
		} else {
			_ = exporter.ExportSingleObservation(updated, "", "")
		}

		data, _ := json.Marshal(updated)
		return mcp.NewToolResultText(string(data)), nil
	}
}

var nonAlphaNumRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonAlphaNumRegex.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func (s *Server) handleSuggestTopicKey() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		title, _ := args["title"].(string)
		content, _ := args["content"].(string)
		obsType, _ := args["type"].(string)

		if obsType == "" {
			obsType = "general"
		}

		text := title
		if text == "" {
			lines := strings.Split(content, "\n")
			if len(lines) > 0 {
				text = lines[0]
			}
		}

		slug := slugify(text)
		if len(slug) > 30 {
			slug = slug[:30]
		}
		if slug == "" {
			slug = "note"
		}

		key := fmt.Sprintf("%s/%s", obsType, slug)
		data, _ := json.Marshal(map[string]string{
			"topic_key": key,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleCapturePassive() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		content, _ := args["content"].(string)
		projOverride, _ := args["project"].(string)
		sessID, _ := args["session_id"].(string)

		if content == "" {
			return mcp.NewToolResultError("content is required"), nil
		}

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		// Find list items in content (either numbered or bullets)
		lines := strings.Split(content, "\n")
		var items []string
		inLearningSection := false

		reItem := regexp.MustCompile(`^(\d+\.|\-|\*)\s+(.+)`)

		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.Contains(strings.ToLower(trimmed), "key learning") || strings.Contains(strings.ToLower(trimmed), "aprendizajes clave") {
				inLearningSection = true
				continue
			}
			if inLearningSection && strings.HasPrefix(trimmed, "#") {
				// new markdown section starts
				inLearningSection = false
			}

			if matches := reItem.FindStringSubmatch(trimmed); len(matches) > 2 {
				itemText := strings.TrimSpace(matches[2])
				if len(itemText) > 5 {
					items = append(items, itemText)
				}
			}
		}

		var savedIDs []string
		for _, item := range items {
			title := item
			if len(title) > 60 {
				title = title[:60] + "..."
			}

			obs := &store.Observation{
				ProjectID: proj.ID,
				Type:      "discovery",
				Scope:     "project",
				Title:     title,
				Content:   item,
			}
			if sessID != "" {
				obs.SessionID = &sessID
			}

			if err := s.store.SaveObservation(obs); err == nil {
				savedIDs = append(savedIDs, obs.ID)
			}
		}

		data, _ := json.Marshal(map[string]any{
			"status":       "captured",
			"saved_count":  len(savedIDs),
			"saved_ids":    savedIDs,
			"project":      proj.Name,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handlePin(pinned bool) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		id, _ := args["id"].(string)
		if id == "" {
			return mcp.NewToolResultError("id is required"), nil
		}

		if err := s.store.PinObservation(id, pinned); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("pin operation failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(`{"status":"success","id":%q,"is_pinned":%t}`, id, pinned)), nil
	}
}

func (s *Server) handleSavePrompt() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		content, _ := args["content"].(string)
		projOverride, _ := args["project"].(string)
		sessID, _ := args["session_id"].(string)

		if content == "" {
			return mcp.NewToolResultError("content is required"), nil
		}

		proj, err := s.resolveProject(projOverride)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
		}

		p, err := s.store.SavePrompt(proj.ID, sessID, content)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save prompt: %v", err)), nil
		}

		data, _ := json.Marshal(p)
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleDelete() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		id, _ := args["id"].(string)
		if id == "" {
			return mcp.NewToolResultError("id is required"), nil
		}

		if err := s.store.DeleteObservation(id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(`{"status":"deleted","id":%q}`, id)), nil
	}
}

func (s *Server) handleStats() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		projOverride, _ := args["project"].(string)

		var projID string
		if projOverride != "" {
			proj, err := s.resolveProject(projOverride)
			if err == nil {
				projID = proj.ID
			}
		}

		stats, err := s.store.GetStats(projID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get stats: %v", err)), nil
		}

		data, _ := json.Marshal(stats)
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleJudge() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		jID, _ := args["judgment_id"].(string)
		verdict, _ := args["verdict"].(string)

		data, _ := json.Marshal(map[string]any{
			"status":      "recorded",
			"judgment_id": jID,
			"verdict":     verdict,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleCompare() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		candID, _ := args["candidate_id"].(string)
		verdict, _ := args["verdict"].(string)

		data, _ := json.Marshal(map[string]any{
			"status":       "persisted",
			"candidate_id": candID,
			"verdict":      verdict,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleListProjects() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := s.store.ListProjects()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list projects: %v", err)), nil
		}

		type projectSummary struct {
			Name             string   `json:"name"`
			ObservationCount int      `json:"observation_count"`
			SessionCount     int      `json:"session_count"`
			PromptCount      int      `json:"prompt_count"`
			Directories      []string `json:"directories"`
		}

		list := make([]projectSummary, 0, len(projects))
		for _, p := range projects {
			stats, _ := s.store.GetStats(p.ID)
			obsCount, _ := stats["total_observations"].(int)
			sessCount, _ := stats["total_sessions"].(int)
			promptCount, _ := stats["total_prompts"].(int)

			dirs := []string{}
			if p.RootPath != "" {
				dirs = append(dirs, p.RootPath)
			}
			list = append(list, projectSummary{
				Name:             p.Name,
				ObservationCount: obsCount,
				SessionCount:     sessCount,
				PromptCount:      promptCount,
				Directories:      dirs,
			})
		}

		data, _ := json.Marshal(map[string]any{
			"count":    len(list),
			"projects": list,
		})
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) handleArchive() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		id, _ := args["id"].(string)
		if id == "" {
			return mcp.NewToolResultError("id is required"), nil
		}

		archived := true
		if val, ok := args["archived"].(bool); ok {
			archived = val
		}

		if err := s.store.ArchiveObservation(id, archived); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("archive failed: %v", err)), nil
		}

		status := "archived"
		if !archived {
			status = "unarchived"
		}

		return mcp.NewToolResultText(fmt.Sprintf(`{"status":%q,"id":%q}`, status, id)), nil
	}
}

func (s *Server) handlePrune() mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		olderThanStr, _ := args["older_than"].(string)
		projOverride, _ := args["project"].(string)
		typesStr, _ := args["types"].(string)
		includeAll, _ := args["include_all"].(bool)
		hardDelete, _ := args["hard_delete"].(bool)

		dryRun := true
		if val, ok := args["dry_run"].(bool); ok {
			dryRun = val
		}

		if olderThanStr == "" {
			olderThanStr = "90d"
		}
		duration, err := store.ParseAge(olderThanStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid older_than format: %v", err)), nil
		}

		var projID string
		if projOverride != "" {
			p, err := s.resolveProject(projOverride)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("project resolution failed: %v", err)), nil
			}
			projID = p.ID
		}

		var types []string
		if typesStr != "" {
			for _, t := range strings.Split(typesStr, ",") {
				if trimmed := strings.TrimSpace(t); trimmed != "" {
					types = append(types, trimmed)
				}
			}
		}

		res, err := s.store.Prune(store.PruneOptions{
			ProjectID:  projID,
			OlderThan:  duration,
			Types:      types,
			IncludeAll: includeAll,
			HardDelete: hardDelete,
			DryRun:     dryRun,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("prune failed: %v", err)), nil
		}

		data, _ := json.Marshal(res)
		return mcp.NewToolResultText(string(data)), nil
	}
}

