package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"custom-engram/internal/store"
)

var safeFilenameRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\s]+`)

// ExportToObsidian exports observations and sessions for a project into an Obsidian-compatible markdown vault.
func ExportToObsidian(st *store.Store, project *store.Project, vaultPath string) error {
	projectDir := filepath.Join(vaultPath, project.Name)
	obsDir := filepath.Join(projectDir, "Observations")
	sessDir := filepath.Join(projectDir, "Sessions")

	if err := os.MkdirAll(obsDir, 0755); err != nil {
		return fmt.Errorf("failed to create obs dir: %w", err)
	}
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return fmt.Errorf("failed to create sess dir: %w", err)
	}

	// 1. Export Observations
	observations, err := st.ListObservations(project.ID, 1000)
	if err != nil {
		return fmt.Errorf("failed to list observations: %w", err)
	}

	for _, obs := range observations {
		cleanTitle := safeFilenameRegex.ReplaceAllString(obs.Title, "")
		cleanTitle = strings.TrimSpace(cleanTitle)
		if cleanTitle == "" {
			cleanTitle = obs.ID
		}
		if len(cleanTitle) > 60 {
			cleanTitle = cleanTitle[:60]
		}
		filename := fmt.Sprintf("%s.md", cleanTitle)
		filePath := filepath.Join(obsDir, filename)

		var sb strings.Builder
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("id: %s\n", obs.ID))
		sb.WriteString(fmt.Sprintf("title: %q\n", obs.Title))
		sb.WriteString(fmt.Sprintf("type: %s\n", obs.Type))
		sb.WriteString(fmt.Sprintf("scope: %s\n", obs.Scope))
		if obs.TopicKey != "" {
			sb.WriteString(fmt.Sprintf("topic_key: %s\n", obs.TopicKey))
		}
		sb.WriteString(fmt.Sprintf("created_at: %s\n", obs.CreatedAt.Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("updated_at: %s\n", obs.UpdatedAt.Format(time.RFC3339)))
		sb.WriteString("tags:\n")
		sb.WriteString("  - engram\n")
		sb.WriteString("  - observation\n")
		sb.WriteString(fmt.Sprintf("  - %s\n", obs.Type))
		sb.WriteString("---\n\n")

		sb.WriteString(fmt.Sprintf("# %s\n\n", obs.Title))
		if obs.TopicKey != "" {
			sb.WriteString(fmt.Sprintf("Topic: [[%s]]\n\n", obs.TopicKey))
		}
		sb.WriteString(obs.Content)
		sb.WriteString("\n")

		if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filePath, err)
		}
	}

	// 2. Export Sessions
	sessions, err := st.GetRecentSessions(project.ID, 500)
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	for _, sess := range sessions {
		dateStr := sess.CreatedAt.Format("2006-01-02_15-04")
		filename := fmt.Sprintf("%s_Session_%s.md", dateStr, sess.ID[:8])
		filePath := filepath.Join(sessDir, filename)

		var sb strings.Builder
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("id: %s\n", sess.ID))
		sb.WriteString(fmt.Sprintf("status: %s\n", sess.Status))
		sb.WriteString(fmt.Sprintf("created_at: %s\n", sess.CreatedAt.Format(time.RFC3339)))
		sb.WriteString("tags:\n")
		sb.WriteString("  - engram\n")
		sb.WriteString("  - session\n")
		sb.WriteString("---\n\n")

		sb.WriteString(fmt.Sprintf("# Session: %s\n\n", dateStr))
		if sess.Goal != "" {
			sb.WriteString("## Goal\n\n")
			sb.WriteString(sess.Goal)
			sb.WriteString("\n\n")
		}
		if sess.Summary != "" {
			sb.WriteString("## Summary\n\n")
			sb.WriteString(sess.Summary)
			sb.WriteString("\n\n")
		}

		if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filePath, err)
		}
	}

	return nil
}

type ExportData struct {
	Project      store.Project       `json:"project"`
	Observations []store.Observation `json:"observations"`
	Sessions     []store.Session     `json:"sessions"`
	ExportedAt   time.Time           `json:"exported_at"`
}

// ExportToJSON exports project data to a single JSON backup file.
func ExportToJSON(st *store.Store, project *store.Project, targetFile string) error {
	obs, err := st.ListObservations(project.ID, 5000)
	if err != nil {
		return err
	}
	sessions, err := st.GetRecentSessions(project.ID, 1000)
	if err != nil {
		return err
	}

	data := ExportData{
		Project:      *project,
		Observations: obs,
		Sessions:     sessions,
		ExportedAt:   time.Now().UTC(),
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(targetFile, bytes, 0644)
}
