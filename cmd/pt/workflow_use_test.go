package main

import (
	"os"
	"path/filepath"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestWorkflowUse_WritesSelectionAndResolvesWhenMultipleWorkflows(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".pt"), 0o755); err != nil {
		t.Fatalf("mkdir .pt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}

	// Store path anchors the "project root".
	dbPath := filepath.Join(projectDir, ".pt", "db.json")
	t.Setenv("PT_BACKEND", "store")
	t.Setenv("PT_DB", dbPath)
	t.Setenv("PT_SKIP_HOOKS", "1")
	t.Setenv("PT_WORKFLOW", "")

	// Two workflows exist -> ambiguity unless selected.
	wfA := filepath.Join(projectDir, "workflows", "a.toml")
	wfB := filepath.Join(projectDir, "workflows", "b.toml")
	content := `
name = "wf"
[phase_assignment]
label_prefix = "phase:"
default_phase = "build"
[[phases]]
id = "build"
name = "Build"
order = 1
`
	if err := os.WriteFile(wfA, []byte(content), 0o644); err != nil {
		t.Fatalf("write wfA: %v", err)
	}
	if err := os.WriteFile(wfB, []byte(content), 0o644); err != nil {
		t.Fatalf("write wfB: %v", err)
	}

	// Seed a tiny store so discovery has a stable root.
	store := pt.NewStoreClient(dbPath, "pt")
	if _, err := store.Sync(t.Context(), pt.Manifest{Tasks: []pt.Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "code:a", DoD: pt.DefinitionOfDone{Manual: "m", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}}}}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Without selection, it's ambiguous.
	if _, err := findWorkflowFileFor(dbPath); err == nil {
		t.Fatalf("expected ambiguity error")
	}

	// Select workflows/a.toml.
	if err := cmdWorkflow([]string{"use", "workflows/a.toml", "--db", dbPath}); err != nil {
		t.Fatalf("workflow use: %v", err)
	}

	got, err := findWorkflowFileFor(dbPath)
	if err != nil {
		t.Fatalf("find workflow: %v", err)
	}
	if got != wfA {
		t.Fatalf("workflow=%q, want %q", got, wfA)
	}

	// Selection file is written in project .pt.
	sel := filepath.Join(projectDir, ".pt", "workflow.toml")
	if _, err := os.Stat(sel); err != nil {
		t.Fatalf("selection file missing: %v", err)
	}
}

