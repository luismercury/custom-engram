package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Project string `json:"project"`
	Root    string `json:"root,omitempty"`
}

type ResolvedProject struct {
	Name     string
	RootPath string
	Source   string // "explicit", "config", "git", "fallback"
}

// Resolve identifies the project name and root directory deterministically.
func Resolve(cwd, explicitProject string) (*ResolvedProject, error) {
	// 1. Explicit project provided via CLI flag or API parameter
	if explicitProject = strings.TrimSpace(explicitProject); explicitProject != "" {
		return &ResolvedProject{
			Name:     explicitProject,
			RootPath: cwd,
			Source:   "explicit",
		}, nil
	}

	// 2. Environment variable override
	if envProj := strings.TrimSpace(os.Getenv("ENGRAM_PROJECT")); envProj != "" {
		return &ResolvedProject{
			Name:     envProj,
			RootPath: cwd,
			Source:   "env",
		}, nil
	}

	// Clean CWD
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		absCwd = cwd
	}

	// 3. Search upwards for .engram/config.json
	if cfgProj, cfgRoot := findConfig(absCwd); cfgProj != "" {
		return &ResolvedProject{
			Name:     cfgProj,
			RootPath: cfgRoot,
			Source:   "config",
		}, nil
	}

	// 4. Search upwards for .git directory
	if gitRoot := findGitRoot(absCwd); gitRoot != "" {
		projName := filepath.Base(gitRoot)
		// Clean project name
		projName = strings.TrimSpace(projName)
		if projName != "" && projName != "." && projName != "/" && projName != "\\" {
			return &ResolvedProject{
				Name:     projName,
				RootPath: gitRoot,
				Source:   "git",
			}, nil
		}
	}

	// 5. Fallback without crashing:
	// If cwd is the user's home directory, default to "global" to prevent multi-repo ambiguity crashes.
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" && strings.EqualFold(filepath.Clean(absCwd), filepath.Clean(homeDir)) {
		return &ResolvedProject{
			Name:     "global",
			RootPath: absCwd,
			Source:   "fallback_home",
		}, nil
	}

	// Fallback to directory basename
	base := filepath.Base(absCwd)
	if base == "" || base == "." || base == "/" || base == "\\" {
		base = "default"
	}

	return &ResolvedProject{
		Name:     base,
		RootPath: absCwd,
		Source:   "fallback_dir",
	}, nil
}

// findConfig walks up from startDir looking for .engram/config.json.
func findConfig(startDir string) (projectName, rootDir string) {
	curr := startDir
	for {
		cfgPath := filepath.Join(curr, ".engram", "config.json")
		if data, err := os.ReadFile(cfgPath); err == nil {
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err == nil && cfg.Project != "" {
				root := cfg.Root
				if root == "" {
					root = curr
				}
				return cfg.Project, root
			}
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return "", ""
}

// findGitRoot walks up from startDir looking for a .git directory or file.
func findGitRoot(startDir string) string {
	curr := startDir
	for {
		gitPath := filepath.Join(curr, ".git")
		if fi, err := os.Stat(gitPath); err == nil {
			// .git can be a directory or a worktree file
			if fi.IsDir() || !fi.IsDir() {
				return curr
			}
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

// InitConfigFile writes a default .engram/config.json in the specified directory.
func InitConfigFile(dir, projectName string) error {
	engramDir := filepath.Join(dir, ".engram")
	if err := os.MkdirAll(engramDir, 0755); err != nil {
		return fmt.Errorf("failed to create .engram directory: %w", err)
	}

	cfg := Config{
		Project: projectName,
		Root:    dir,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(engramDir, "config.json")
	return os.WriteFile(cfgPath, data, 0644)
}
