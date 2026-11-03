package exporter

import (
	"os"
	"path/filepath"
	"testing"

	"custom-engram/internal/store"
)

func TestExportToObsidian(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("Store error: %v", err)
	}
	defer st.Close()

	proj, err := st.GetOrCreateProject("test-obsidian", "C:/test")
	if err != nil {
		t.Fatalf("Project error: %v", err)
	}

	obs := &store.Observation{
		ProjectID: proj.ID,
		Title:     "Note 1",
		Content:   "This is content for note 1",
		TopicKey:  "testing",
	}
	_ = st.SaveObservation(obs)

	sess, _ := st.StartSession(proj.ID, "Testing sessions")
	_ = st.EndSession(sess.ID, "Session done")

	tempVault, err := os.MkdirTemp("", "vault-test-*")
	if err != nil {
		t.Fatalf("Temp dir error: %v", err)
	}
	defer os.RemoveAll(tempVault)

	if err := ExportToObsidian(st, proj, tempVault); err != nil {
		t.Fatalf("ExportToObsidian failed: %v", err)
	}

	// Verify file was written
	obsPath := filepath.Join(tempVault, proj.Name, "Observations", "Note 1.md")
	if _, err := os.Stat(obsPath); err != nil {
		t.Errorf("Expected observation note file at %s, error: %v", obsPath, err)
	}

	// Verify JSON export
	jsonPath := filepath.Join(tempVault, "backup.json")
	if err := ExportToJSON(st, proj, jsonPath); err != nil {
		t.Fatalf("ExportToJSON failed: %v", err)
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("Expected json file at %s", jsonPath)
	}
}
