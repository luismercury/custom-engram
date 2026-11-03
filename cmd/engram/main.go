package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"custom-engram/internal/exporter"
	"custom-engram/internal/mcp"
	"custom-engram/internal/project"
	"custom-engram/internal/search"
	"custom-engram/internal/store"
)

const Version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "mcp":
		runMCP(args)
	case "save":
		runSave(args)
	case "search":
		runSearch(args)
	case "context":
		runContext(args)
	case "export":
		runExport(args)
	case "init":
		runInit(args)
	case "doctor":
		runDoctor(args)
	case "prune":
		runPrune(args)
	case "archive":
		runArchive(args)
	case "version", "--version", "-v":
		fmt.Printf("custom-engram version %s\n", Version)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'custom-engram help' for usage.\n", command)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`Custom Engram — Persistent Memory for AI Agents (v%s)

Usage:
  custom-engram <command> [arguments]

Commands:
  mcp                  Start stdio Model Context Protocol (MCP) server
  save <title> <msg>   Save a memory observation to persistent memory
  search <query>       Search memories across project using hybrid search (FTS5 + embeddings)
  context              Show recent context and session history
  export               Export memories to Obsidian markdown vault or JSON
  init [name]          Initialize .engram/config.json in current directory
  doctor               Check database diagnostics, project count, and embeddings
  prune                Archive or delete old, unused memories (supports --dry-run)
  archive [id]         Archive, unarchive, or list archived observations
  version              Show version info
  help                 Show this help

Flags (available on relevant subcommands):
  --project <name>     Explicit project name override
  --type <type>        Observation category (decision, architecture, bugfix, pattern, discovery)
  --scope <scope>      Memory scope (project, personal, global)
  --topic-key <key>    Key for evolving updates (e.g. arch/db-schema)
  --older-than <dur>   Age threshold for prune (e.g. 90d, 30d, 180d, 1y)
  --dry-run            Simulate prune without modifying database
  --hard-delete        Permanently delete instead of archiving during prune
  --unarchive          Restore an archived memory in archive command
  --obsidian <path>    Vault directory path for export
  --json <path>        File path for JSON backup export
`, Version)
}

func getDBPath() string {
	if dir := os.Getenv("ENGRAM_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "engram.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "engram.db"
	}
	return filepath.Join(home, ".custom-engram", "engram.db")
}

func getStore() (*store.Store, error) {
	return store.New(getDBPath())
}

func runMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	projectFlag := fs.String("project", "", "Default project name")
	_ = fs.String("tools", "", "Tools subset filter (compatibility)")
	_ = fs.Parse(args)

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	ctx := context.Background()
	embedder := search.DetectEmbedder(ctx)

	srv := mcp.NewServer(st, embedder, *projectFlag)
	if err := srv.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlagsAndPositional(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags []string
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, arg)
		}
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return pos, nil
}

func runSave(args []string) {
	fs := flag.NewFlagSet("save", flag.ExitOnError)
	typeFlag := fs.String("type", "discovery", "Observation type")
	scopeFlag := fs.String("scope", "project", "Observation scope")
	topicKeyFlag := fs.String("topic-key", "", "Stable topic key for upserts")
	projectFlag := fs.String("project", "", "Project name override")
	tail, _ := parseFlagsAndPositional(fs, args)

	if len(tail) < 1 {
		fmt.Println("Usage: custom-engram save <title> [content] [--type TYPE] [--project PROJECT] [--topic-key KEY]")
		return
	}

	title := tail[0]
	content := title
	if len(tail) > 1 {
		content = strings.Join(tail[1:], " ")
	}

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	cwd, _ := os.Getwd()
	res, err := project.Resolve(cwd, *projectFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Project error: %v\n", err)
		os.Exit(1)
	}

	proj, err := st.GetOrCreateProject(res.Name, res.RootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting project: %v\n", err)
		os.Exit(1)
	}

	obs := &store.Observation{
		ProjectID: proj.ID,
		Type:      *typeFlag,
		Scope:     *scopeFlag,
		Title:     title,
		Content:   content,
		TopicKey:  *topicKeyFlag,
	}

	if err := st.SaveObservation(obs); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving memory: %v\n", err)
		os.Exit(1)
	}

	// Real-time unidirectional export to Obsidian if vault is configured
	_ = exporter.ExportSingleObservation(obs, proj.Name, "")

	// Try embedding
	ctx := context.Background()
	embedder := search.DetectEmbedder(ctx)
	if embedder.IsAvailable(ctx) {
		textToEmbed := fmt.Sprintf("%s\n%s", title, content)
		if vec, err := embedder.Embed(ctx, textToEmbed); err == nil && len(vec) > 0 {
			_ = st.SaveEmbedding(obs.ID, embedder.ModelName(), vec)
		}
	}

	fmt.Printf("Memory saved successfully! [ID: %s] [Project: %s]\n", obs.ID, proj.Name)
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	projectFlag := fs.String("project", "", "Project filter")
	limitFlag := fs.Int("limit", 10, "Max results")
	tail, _ := parseFlagsAndPositional(fs, args)

	if len(tail) < 1 {
		fmt.Println("Usage: custom-engram search <query> [--project PROJECT] [--limit N]")
		return
	}
	query := strings.Join(tail, " ")

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	cwd, _ := os.Getwd()
	res, err := project.Resolve(cwd, *projectFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Project resolution error: %v\n", err)
		os.Exit(1)
	}

	proj, err := st.GetOrCreateProject(res.Name, res.RootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	embedder := search.DetectEmbedder(ctx)

	results, err := search.HybridSearch(ctx, st, embedder, proj.ID, query, *limitFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Search results for %q in project %s (%d found):\n\n", query, proj.Name, len(results))
	for i, r := range results {
		fmt.Printf("[%d] %s (%s) [Score: %.3f | Source: %s]\n", i+1, r.Observation.Title, r.Observation.Type, r.Score, r.Source)
		if r.Observation.TopicKey != "" {
			fmt.Printf("    Topic: %s\n", r.Observation.TopicKey)
		}
		lines := strings.Split(r.Observation.Content, "\n")
		firstLine := lines[0]
		if len(firstLine) > 100 {
			firstLine = firstLine[:100] + "..."
		}
		fmt.Printf("    %s\n\n", firstLine)
	}
}

func runContext(args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	projectFlag := fs.String("project", "", "Project filter")
	_ = fs.Parse(args)

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	cwd, _ := os.Getwd()
	res, err := project.Resolve(cwd, *projectFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Project error: %v\n", err)
		os.Exit(1)
	}

	proj, err := st.GetOrCreateProject(res.Name, res.RootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Project error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Context for Project: %s (Root: %s)\n", proj.Name, proj.RootPath)
	fmt.Println(strings.Repeat("-", 50))

	sessions, _ := st.GetRecentSessions(proj.ID, 3)
	fmt.Printf("Recent Sessions (%d):\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("  • [%s] Status: %s | Goal: %s\n", s.CreatedAt.Format("2006-01-02 15:04"), s.Status, s.Goal)
		if s.Summary != "" {
			fmt.Printf("    Summary: %s\n", s.Summary)
		}
	}

	fmt.Println()
	observations, _ := st.ListObservations(proj.ID, 5)
	fmt.Printf("Recent Observations (%d):\n", len(observations))
	for _, o := range observations {
		fmt.Printf("  • [%s] %s (%s)\n", o.Type, o.Title, o.CreatedAt.Format("2006-01-02 15:04"))
	}
}

func runExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	obsidianFlag := fs.String("obsidian", "", "Target Obsidian vault directory")
	jsonFlag := fs.String("json", "", "Target JSON backup file")
	projectFlag := fs.String("project", "", "Project filter")
	allFlag := fs.Bool("all", false, "Export all projects")
	_ = fs.Parse(args)

	// Support positional syntax: "export obsidian <dir>" or "export json <path>"
	remaining := fs.Args()
	for _, a := range remaining {
		if a == "--all" || a == "-all" {
			*allFlag = true
		}
	}
	if *obsidianFlag == "" && *jsonFlag == "" && len(remaining) >= 2 {
		targetType := strings.ToLower(remaining[0])
		targetPath := remaining[1]
		if targetType == "obsidian" {
			*obsidianFlag = targetPath
		} else if targetType == "json" {
			*jsonFlag = targetPath
		}
	} else if *obsidianFlag == "" && *jsonFlag == "" && len(remaining) == 1 {
		*obsidianFlag = remaining[0]
	}

	if *obsidianFlag == "" && *jsonFlag == "" {
		fmt.Println("Please specify either: export obsidian <dir> (or --obsidian <dir>) / export json <file>")
		return
	}

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	var projectsToExport []*store.Project

	if *allFlag {
		allProjs, err := st.ListProjects()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list projects: %v\n", err)
			os.Exit(1)
		}
		for i := range allProjs {
			projectsToExport = append(projectsToExport, &allProjs[i])
		}
	} else if *projectFlag != "" {
		p, err := st.GetProjectByName(*projectFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Project %q not found: %v\n", *projectFlag, err)
			os.Exit(1)
		}
		projectsToExport = append(projectsToExport, p)
	} else {
		cwd, _ := os.Getwd()
		res, err := project.Resolve(cwd, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Project error: %v\n", err)
			os.Exit(1)
		}
		proj, err := st.GetOrCreateProject(res.Name, res.RootPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		projectsToExport = append(projectsToExport, proj)
	}

	for _, proj := range projectsToExport {
		if *obsidianFlag != "" {
			if err := exporter.ExportToObsidian(st, proj, *obsidianFlag); err != nil {
				fmt.Fprintf(os.Stderr, "Obsidian export failed for %s: %v\n", proj.Name, err)
				os.Exit(1)
			}
			fmt.Printf("Exported memories for project %q to Obsidian vault at %s\n", proj.Name, *obsidianFlag)
		}

		if *jsonFlag != "" {
			if err := exporter.ExportToJSON(st, proj, *jsonFlag); err != nil {
				fmt.Fprintf(os.Stderr, "JSON export failed for %s: %v\n", proj.Name, err)
				os.Exit(1)
			}
			fmt.Printf("Exported memories for project %q to JSON at %s\n", proj.Name, *jsonFlag)
		}
	}
}

func runInit(args []string) {
	cwd, _ := os.Getwd()
	projName := ""
	if len(args) > 0 {
		projName = args[0]
	} else {
		projName = filepath.Base(cwd)
	}

	if err := project.InitConfigFile(cwd, projName); err != nil {
		fmt.Fprintf(os.Stderr, "Init failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Initialized .engram/config.json with project name %q in %s\n", projName, cwd)
}

func runDoctor(args []string) {
	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Doctor check: DB connection failed: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	projects, err := st.ListProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Doctor check: Failed to list projects: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	embedder := search.DetectEmbedder(ctx)
	embedStatus := "Not detected / Fallback to pure FTS5"
	if embedder.IsAvailable(ctx) {
		embedStatus = fmt.Sprintf("Active (%s)", embedder.ModelName())
	}

	fmt.Printf("Custom Engram Diagnostics (v%s)\n", Version)
	fmt.Printf("Database Path:  %s\n", getDBPath())
	fmt.Printf("Status:         OK\n")
	fmt.Printf("Total Projects: %d\n", len(projects))
	for _, p := range projects {
		obs, _ := st.ListObservations(p.ID, 1000)
		fmt.Printf("  • %-25s (%d memories)\n", p.Name, len(obs))
	}
	fmt.Printf("Embeddings:     %s\n", embedStatus)
}

func runPrune(args []string) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	projectFlag := fs.String("project", "", "Project filter")
	olderThanFlag := fs.String("older-than", "90d", "Age threshold (e.g. 90d, 30d, 180d, 1y)")
	typesFlag := fs.String("types", "", "Comma-separated list of observation types (e.g. discovery,bugfix)")
	includeAllFlag := fs.Bool("all", false, "Include protected types (architecture, decision). Pinned items are still kept safe.")
	hardDeleteFlag := fs.Bool("hard-delete", false, "Permanently delete records instead of soft-deleting (archiving)")
	dryRunFlag := fs.Bool("dry-run", false, "Simulate without making any changes")
	_ = fs.Parse(args)

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	var projID string
	var projName string
	if *projectFlag != "" {
		cwd, _ := os.Getwd()
		res, err := project.Resolve(cwd, *projectFlag)
		if err == nil {
			proj, err := st.GetOrCreateProject(res.Name, res.RootPath)
			if err == nil {
				projID = proj.ID
				projName = proj.Name
			}
		}
	}

	duration, err := store.ParseAge(*olderThanFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid older-than duration %q: %v\n", *olderThanFlag, err)
		os.Exit(1)
	}

	var types []string
	if *typesFlag != "" {
		for _, t := range strings.Split(*typesFlag, ",") {
			if tr := strings.TrimSpace(t); tr != "" {
				types = append(types, tr)
			}
		}
	}

	res, err := st.Prune(store.PruneOptions{
		ProjectID:  projID,
		OlderThan:  duration,
		Types:      types,
		IncludeAll: *includeAllFlag,
		HardDelete: *hardDeleteFlag,
		DryRun:     *dryRunFlag,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Prune failed: %v\n", err)
		os.Exit(1)
	}

	scopeMsg := "all projects"
	if projName != "" {
		scopeMsg = fmt.Sprintf("project %s", projName)
	}

	actionDesc := "archived"
	if *hardDeleteFlag {
		actionDesc = "permanently deleted"
	}

	if *dryRunFlag {
		fmt.Printf("[DRY-RUN] Prune simulation for %s (older than %s):\n", scopeMsg, *olderThanFlag)
		fmt.Printf("Candidates found: %d (would be %s)\n\n", res.MatchedCount, actionDesc)
		for i, o := range res.Observations {
			fmt.Printf("  [%d] [%s] %s (Type: %s, Last updated: %s)\n", i+1, o.ID[:8], o.Title, o.Type, o.UpdatedAt.Format("2006-01-02"))
		}
		if res.MatchedCount > 0 {
			fmt.Printf("\nRun again without --dry-run to apply this cleanup.\n")
		}
	} else {
		fmt.Printf("Prune completed for %s (older than %s):\n", scopeMsg, *olderThanFlag)
		fmt.Printf("Total %s: %d\n", actionDesc, res.AffectedCount)
		for i, o := range res.Observations {
			fmt.Printf("  [%d] [%s] %s\n", i+1, o.ID[:8], o.Title)
		}
	}
}

func runArchive(args []string) {
	fs := flag.NewFlagSet("archive", flag.ExitOnError)
	unarchiveFlag := fs.Bool("unarchive", false, "Restore/unarchive an observation")
	listFlag := fs.Bool("list", false, "List currently archived observations")
	projectFlag := fs.String("project", "", "Project filter")
	_ = fs.Parse(args)

	st, err := getStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	if *listFlag {
		var projID string
		if *projectFlag != "" {
			cwd, _ := os.Getwd()
			res, err := project.Resolve(cwd, *projectFlag)
			if err == nil {
				proj, err := st.GetOrCreateProject(res.Name, res.RootPath)
				if err == nil {
					projID = proj.ID
				}
			}
		}
		archivedList, err := st.ListArchivedObservations(projID, 50)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list archived: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Archived Observations (%d):\n", len(archivedList))
		for _, o := range archivedList {
			fmt.Printf("  • [%s] %s (%s, Updated: %s)\n", o.ID[:8], o.Title, o.Type, o.UpdatedAt.Format("2006-01-02"))
		}
		return
	}

	remaining := fs.Args()
	if len(remaining) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: custom-engram archive <observation-id> [--unarchive] or custom-engram archive --list\n")
		os.Exit(1)
	}

	id := remaining[0]
	shouldArchive := !*unarchiveFlag
	if err := st.ArchiveObservation(id, shouldArchive); err != nil {
		fmt.Fprintf(os.Stderr, "Archive failed: %v\n", err)
		os.Exit(1)
	}

	if shouldArchive {
		fmt.Printf("Observation %s successfully archived.\n", id)
	} else {
		fmt.Printf("Observation %s successfully unarchived (restored).\n", id)
	}
}
