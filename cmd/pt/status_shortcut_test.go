package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestStatusShortcutForwardsWorkflowFlag(t *testing.T) {
	t.Setenv("PT_WORKFLOW", "")
	_, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	wfPath := filepath.Join(t.TempDir(), "wf.toml")
	wf := `
name = "wf"

[phase_assignment.by_template]
backend_endpoint = "build"

[[phases]]
id = "build"
name = "Build"
order = 1
`
	if err := os.WriteFile(wfPath, []byte(wf), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	// Capture stdout so we can assert output was produced.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := run([]string{"pt", "status", "--workflow", wfPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("status shortcut failed: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "Workflow") {
		t.Fatalf("expected workflow status output, got:\n%s", out)
	}
}
