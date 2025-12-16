package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestReviewWriteCreatesFileAndLinksComment(t *testing.T) {
	td := t.TempDir()
	db := filepath.Join(td, ".pt", "db.json")
	store := pt.NewStoreClient(db, "pt")

	ctx := t.Context()
	id, err := store.AddTask(ctx, pt.Task{
		Title:    "Review target",
		Template: "discovery",
		Role:     "planner",
		Artifact: "doc:review",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "n/a", Criteria: []string{"ok"}},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	// Use explicit --db so the review file is written under the temp project root.
	if err := cmdReview([]string{"write", id, "--db", db, "--kind", "pre", "--phase", "prove", "--desc", "kickoff"}); err != nil {
		t.Fatalf("review write: %v", err)
	}

	reviewsDir := filepath.Join(td, ".pt", "reviews")
	files, err := filepath.Glob(filepath.Join(reviewsDir, "*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 review file, got %d (%v)", len(files), files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read review: %v", err)
	}
	if !strings.Contains(string(raw), "Review (pre)") {
		t.Fatalf("expected review content to include kind, got:\n%s", string(raw))
	}

	// Ensure the task has a linking comment.
	// Re-open store to observe persisted comments (StoreClient is in-memory per instance).
	store2 := pt.NewStoreClient(db, "pt")
	comments, err := store2.Comments(ctx, id)
	if err != nil {
		t.Fatalf("comments: %v", err)
	}
	found := false
	for _, c := range comments {
		if strings.Contains(c, "review-file:") && strings.Contains(c, "review-kind: pre") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected review link comment, got: %v", comments)
	}

	// Review check should fail until the template TODOs are replaced.
	if err := cmdReview([]string{"check", id, "--db", db, "--kind", "pre"}); err == nil {
		t.Fatalf("expected review check to fail when file contains TODO placeholders")
	}
	raw2, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read review: %v", err)
	}
	filled := strings.ReplaceAll(string(raw2), "- TODO", "- done")
	if err := os.WriteFile(files[0], []byte(filled), 0o644); err != nil {
		t.Fatalf("write filled review: %v", err)
	}
	if err := cmdReview([]string{"check", id, "--db", db, "--kind", "pre"}); err != nil {
		t.Fatalf("review check after fill: %v", err)
	}
}

func TestReviewWriteDryRunDoesNotWrite(t *testing.T) {
	td := t.TempDir()
	db := filepath.Join(td, ".pt", "db.json")
	store := pt.NewStoreClient(db, "pt")

	ctx := t.Context()
	id, err := store.AddTask(ctx, pt.Task{
		Title:    "Review target",
		Template: "discovery",
		Role:     "planner",
		Artifact: "doc:review",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "n/a", Criteria: []string{"ok"}},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	if err := cmdReview([]string{"write", id, "--db", db, "--kind", "post", "--dry-run"}); err != nil {
		t.Fatalf("review write dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(td, ".pt", "reviews")); err == nil {
		t.Fatalf("expected no reviews dir on dry-run")
	}
}
