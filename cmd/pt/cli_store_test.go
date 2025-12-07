package main

import (
	"os"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdSyncStore(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := t.TempDir() + "/manifest.json"
	data := `{"title":"T","tasks":[{"title":"A","role":"dev","template":"backend_endpoint","dod":{"manual":"check"}},{"title":"B","role":"dev","template":"backend_endpoint","deps":["A"],"dod":{"manual":"check"}}]}`
	if err := os.WriteFile(manifest, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync([]string{manifest}); err != nil {
		t.Fatalf("cmdSync err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issues, err := store.Ready(t.Context(), "dev", 10)
	if err != nil || len(issues) != 1 {
		t.Fatalf("expected one ready issue, got %d err=%v", len(issues), err)
	}
}

func TestCmdClaimAndReleaseStore(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "A", Template: "backend_endpoint", Role: "dev", DoD: pt.DefinitionOfDone{Manual: "check"}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := cmdClaim([]string{"--as", "bob", "pt-1"}); err != nil {
		t.Fatalf("claim err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ := store.GetTask(t.Context(), "pt-1")
	if issue.Status != "in_progress" || issue.Assignee != "bob" {
		t.Fatalf("claim state mismatch: %+v", issue)
	}
	if err := cmdRelease([]string{"--as", "bob", "pt-1"}); err != nil {
		t.Fatalf("release err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ = store.GetTask(t.Context(), "pt-1")
	if issue.Status != "open" {
		t.Fatalf("expected open after release, got %s", issue.Status)
	}
}
