package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectDoDDisplayPath_DefaultIsRepoRelative(t *testing.T) {
	td := t.TempDir()
	dbPath := filepath.Join(td, ".pt", "db.json")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir .pt: %v", err)
	}

	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(td); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv("PT_DB", dbPath)
	t.Setenv("PT_NO_DISCOVER", "1")
	t.Setenv("PT_PROJECT_DOD", "")

	if got := projectDoDDisplayPath(false); got != "PROJECT_DOD.md" {
		t.Fatalf("expected repo-relative display path, got %q", got)
	}
	if got := projectDoDDisplayPath(true); got != filepath.Join(td, "PROJECT_DOD.md") {
		t.Fatalf("expected absolute display path in verbose mode, got %q", got)
	}
}

func TestProjectDoDDisplayPath_EnvOverrideHidesPathUnlessVerbose(t *testing.T) {
	td := t.TempDir()
	override := filepath.Join(td, "custom-dod.md")
	t.Setenv("PT_PROJECT_DOD", override)

	if got := projectDoDDisplayPath(false); got != "PT_PROJECT_DOD" {
		t.Fatalf("expected env var name, got %q", got)
	}
	if got := projectDoDDisplayPath(true); got != override {
		t.Fatalf("expected full env var path in verbose mode, got %q", got)
	}
}
