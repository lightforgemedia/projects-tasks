package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestWorktreeStatus_MissingWorktreePrintsRemediation(t *testing.T) {
	td := t.TempDir()
	dbPath := filepath.Join(td, "db.json")
	client := pt.NewStoreClient(dbPath, "pt")
	ctx := context.Background()

	id, err := client.AddTask(ctx, pt.Task{
		Title:    "Test worktree status remediation",
		Template: "bug_fix",
		Role:     "builder",
		Artifact: "doc:artifact.md",
		DoD: pt.DefinitionOfDone{
			Tests:    []string{"go test ./..."},
			Manual:   "n/a",
			Criteria: []string{"prints remediation when worktree missing"},
		},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	missingPath := filepath.Join(td, "missing-worktree-dir")
	if err := client.SetWorktree(ctx, id, pt.WorktreeInfo{Path: missingPath, Branch: "b", CreatedAt: "now"}); err != nil {
		t.Fatalf("set worktree: %v", err)
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	runErr := cmdWorktreeStatus([]string{"--db", dbPath})
	_ = w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("cmdWorktreeStatus: %v", runErr)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "[MISSING]") {
		t.Fatalf("expected [MISSING] marker, got: %q", out)
	}
	if !strings.Contains(out, "Remediation: pt worktree done "+id) {
		t.Fatalf("expected remediation line, got: %q", out)
	}
}
