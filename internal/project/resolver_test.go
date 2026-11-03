package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolver_Explicit(t *testing.T) {
	res, err := Resolve("C:/anywhere", "my-custom-project")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if res.Name != "my-custom-project" {
		t.Errorf("Expected 'my-custom-project', got '%s'", res.Name)
	}
	if res.Source != "explicit" {
		t.Errorf("Expected source 'explicit', got '%s'", res.Source)
	}
}

func TestResolver_ConfigFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "engram-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir := filepath.Join(tempDir, "pkg", "core")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subDir: %v", err)
	}

	// Init config at tempDir root
	if err := InitConfigFile(tempDir, "test-repo-configured"); err != nil {
		t.Fatalf("Failed to init config: %v", err)
	}

	// Resolve from deep subDir
	res, err := Resolve(subDir, "")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if res.Name != "test-repo-configured" {
		t.Errorf("Expected 'test-repo-configured', got '%s'", res.Name)
	}
	if res.Source != "config" {
		t.Errorf("Expected source 'config', got '%s'", res.Source)
	}
}
