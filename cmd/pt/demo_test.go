package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestDemoRunExecutesCommandsFromLinkedReview(t *testing.T) {
	td := t.TempDir()
	db := filepath.Join(td, ".pt", "db.json")
	store := pt.NewStoreClient(db, "pt")
	t.Setenv("PT_BACKEND", "store")
	t.Setenv("PT_DB", db)

	ctx := t.Context()
	id, err := store.AddTask(ctx, pt.Task{
		Title:    "Demo target",
		Template: "discovery",
		Role:     "planner",
		Artifact: "demo:outputs/demo/",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "n/a", Criteria: []string{"ok"}},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	reviewPath := filepath.Join(td, ".pt", "reviews", "demo.md")
	if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
		t.Fatalf("mkdir reviews: %v", err)
	}
	review := "# Review (demo)\n\n" +
		"## Demo Commands (pt demo run)\n\n" +
		"```sh\n" +
		"echo \"hello-demo\"\n" +
		"```\n"
	if err := os.WriteFile(reviewPath, []byte(review), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	if err := store.AddComment(ctx, id, "review-file: "+reviewPath+"\nreview-kind: demo\nreview-phase: signoff"); err != nil {
		t.Fatalf("link review: %v", err)
	}

	outDir := filepath.Join(td, "outputs", "demo")
	if err := cmdDemo([]string{"run", id, "--db", db, "--out", outDir, "--timeout", "30s"}); err != nil {
		t.Fatalf("demo run: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("readdir out: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected demo logs in %s", outDir)
	}
	found := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(outDir, e.Name()))
		if rerr != nil {
			continue
		}
		if strings.Contains(string(raw), "hello-demo") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected command output in logs under %s", outDir)
	}
}

func TestDemoRunBlocksRMOutsideProjectRootByDefault(t *testing.T) {
	td := t.TempDir()
	db := filepath.Join(td, ".pt", "db.json")
	store := pt.NewStoreClient(db, "pt")
	t.Setenv("PT_BACKEND", "store")
	t.Setenv("PT_DB", db)

	ctx := t.Context()
	id, err := store.AddTask(ctx, pt.Task{
		Title:    "Demo rm guard",
		Template: "discovery",
		Role:     "planner",
		Artifact: "demo:outputs/demo/",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "n/a", Criteria: []string{"ok"}},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	reviewPath := filepath.Join(td, ".pt", "reviews", "demo-rm.md")
	if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
		t.Fatalf("mkdir reviews: %v", err)
	}
	review := "# Review (demo)\n\n" +
		"## Demo Commands (pt demo run)\n\n" +
		"```sh\n" +
		"rm -rf ..\n" +
		"```\n"
	if err := os.WriteFile(reviewPath, []byte(review), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	if err := store.AddComment(ctx, id, "review-file: "+reviewPath+"\nreview-kind: demo\nreview-phase: signoff"); err != nil {
		t.Fatalf("link review: %v", err)
	}

	outDir := filepath.Join(td, "outputs", "demo")
	err = cmdDemo([]string{"run", id, "--db", db, "--out", outDir, "--timeout", "30s"})
	if err == nil {
		t.Fatalf("expected rm-guard to block dangerous rm command")
	}
}
