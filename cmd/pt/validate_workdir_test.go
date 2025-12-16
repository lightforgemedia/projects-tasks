package main

import (
	"os"
	"path/filepath"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdValidateRunsDoDFromProjectRootWhenStoreInDotPT(t *testing.T) {
	t.Setenv("PT_SKIP_HOOKS", "1")
	t.Setenv("PT_WORKFLOW", "")

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	storePath := filepath.Join(projectDir, ".pt", "db.json")
	t.Setenv("PT_DB", storePath)
	t.Setenv("PT_PREFIX", "pt")

	store := pt.NewStoreClient(storePath, "pt")
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{
				Title:    "Validate workdir uses project root",
				Template: "bug_fix",
				Role:     "dev",
				Artifact: "code:cmd/pt/main.go",
				DoD: pt.DefinitionOfDone{
					Manual:   "marker exists",
					Tests:    []string{"test -f marker.txt"},
					Criteria: []string{"runs from project root"},
				},
			},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}

	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("cmdValidate failed: %v", err)
	}

	store2 := pt.NewStoreClient(storePath, "pt")
	issue, _, err := store2.GetTask(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if issue.Status != "needs_review" {
		t.Fatalf("expected needs_review, got %s", issue.Status)
	}
}
