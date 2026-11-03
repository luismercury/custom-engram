package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"custom-engram/internal/search"
	"custom-engram/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPServer_AllTools(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("Store error: %v", err)
	}
	defer st.Close()

	srv := NewServer(st, &search.NoopEmbedder{}, "test-project")
	if srv == nil {
		t.Fatalf("Failed to create MCP server")
	}

	ctx := context.Background()

	// 1. mem_save
	saveReq := mcp.CallToolRequest{}
	saveReq.Params.Arguments = map[string]any{
		"title":   "Initial Architecture",
		"content": "**What**: SQLite setup\n**Why**: Testing",
		"type":    "architecture",
		"project": "test-project",
	}
	saveRes, err := srv.handleSave()(ctx, saveReq)
	if err != nil || saveRes.IsError {
		t.Fatalf("mem_save failed: %v", err)
	}

	var saveOutput map[string]any
	textContent := saveRes.Content[0].(mcp.TextContent).Text
	_ = json.Unmarshal([]byte(textContent), &saveOutput)
	obsID := saveOutput["id"].(string)

	// 2. mem_update
	updateReq := mcp.CallToolRequest{}
	newTitle := "Updated Architecture Title"
	updateReq.Params.Arguments = map[string]any{
		"id":    obsID,
		"title": newTitle,
	}
	updateRes, err := srv.handleUpdate()(ctx, updateReq)
	if err != nil || updateRes.IsError {
		t.Fatalf("mem_update failed: %v", err)
	}

	// 3. mem_suggest_topic_key
	suggestReq := mcp.CallToolRequest{}
	suggestReq.Params.Arguments = map[string]any{
		"title": "Auth Middleware JWT",
		"type":  "architecture",
	}
	suggestRes, err := srv.handleSuggestTopicKey()(ctx, suggestReq)
	if err != nil || suggestRes.IsError {
		t.Fatalf("mem_suggest_topic_key failed: %v", err)
	}

	// 4. mem_capture_passive
	passiveReq := mcp.CallToolRequest{}
	passiveReq.Params.Arguments = map[string]any{
		"content": "Work completed.\n## Key Learnings:\n1. SQLite in WAL mode scales writes\n2. FTS5 requires query sanitization",
		"project": "test-project",
	}
	passiveRes, err := srv.handleCapturePassive()(ctx, passiveReq)
	if err != nil || passiveRes.IsError {
		t.Fatalf("mem_capture_passive failed: %v", err)
	}

	// 5. mem_pin & mem_unpin
	pinReq := mcp.CallToolRequest{}
	pinReq.Params.Arguments = map[string]any{"id": obsID}
	pinRes, err := srv.handlePin(true)(ctx, pinReq)
	if err != nil || pinRes.IsError {
		t.Fatalf("mem_pin failed: %v", err)
	}
	unpinRes, err := srv.handlePin(false)(ctx, pinReq)
	if err != nil || unpinRes.IsError {
		t.Fatalf("mem_unpin failed: %v", err)
	}

	// 6. mem_save_prompt
	promptReq := mcp.CallToolRequest{}
	promptReq.Params.Arguments = map[string]any{
		"content": "User prompt question",
		"project": "test-project",
	}
	promptRes, err := srv.handleSavePrompt()(ctx, promptReq)
	if err != nil || promptRes.IsError {
		t.Fatalf("mem_save_prompt failed: %v", err)
	}

	// 7. mem_stats
	statsReq := mcp.CallToolRequest{}
	statsReq.Params.Arguments = map[string]any{"project": "test-project"}
	statsRes, err := srv.handleStats()(ctx, statsReq)
	if err != nil || statsRes.IsError {
		t.Fatalf("mem_stats failed: %v", err)
	}

	// 8. mem_judge & mem_compare
	judgeReq := mcp.CallToolRequest{}
	judgeReq.Params.Arguments = map[string]any{"judgment_id": "j1", "verdict": "compatible"}
	judgeRes, err := srv.handleJudge()(ctx, judgeReq)
	if err != nil || judgeRes.IsError {
		t.Fatalf("mem_judge failed: %v", err)
	}

	compareReq := mcp.CallToolRequest{}
	compareReq.Params.Arguments = map[string]any{"candidate_id": "c1", "verdict": "compatible"}
	compareRes, err := srv.handleCompare()(ctx, compareReq)
	if err != nil || compareRes.IsError {
		t.Fatalf("mem_compare failed: %v", err)
	}

	// 9. mem_archive
	archiveReq := mcp.CallToolRequest{}
	archiveReq.Params.Arguments = map[string]any{"id": obsID, "archived": true}
	archiveRes, err := srv.handleArchive()(ctx, archiveReq)
	if err != nil || archiveRes.IsError {
		t.Fatalf("mem_archive failed: %v", err)
	}

	// 10. mem_prune (dry-run)
	pruneReq := mcp.CallToolRequest{}
	pruneReq.Params.Arguments = map[string]any{
		"older_than": "1d",
		"dry_run":    true,
		"project":    "test-project",
	}
	pruneRes, err := srv.handlePrune()(ctx, pruneReq)
	if err != nil || pruneRes.IsError {
		t.Fatalf("mem_prune failed: %v", err)
	}

	// 11. mem_delete
	deleteReq := mcp.CallToolRequest{}
	deleteReq.Params.Arguments = map[string]any{"id": obsID}
	deleteRes, err := srv.handleDelete()(ctx, deleteReq)
	if err != nil || deleteRes.IsError {
		t.Fatalf("mem_delete failed: %v", err)
	}
}
