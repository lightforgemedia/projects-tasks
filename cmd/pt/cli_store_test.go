package main

import (
	"os"
	"path/filepath"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdSyncStore(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := t.TempDir() + "/manifest.json"
	data := `{"title":"T","tasks":[{"title":"A","role":"dev","template":"backend_endpoint","artifact":"spec:a","dod":{"manual":"check","tests":["go test ./..."],"criteria":["observed go test pass"]}},{"title":"B","role":"dev","template":"backend_endpoint","artifact":"spec:b","deps":["A"],"dod":{"manual":"check","tests":["go test ./..."],"criteria":["observed go test pass"]}}]}`
	if err := os.WriteFile(manifest, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync([]string{manifest}); err != nil {
		t.Fatalf("cmdSync err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issues, err := store.Ready(t.Context(), "dev", 10)
	if err != nil || len(issues) != 2 {
		t.Fatalf("expected two open issues, got %d err=%v", len(issues), err)
	}
}

func TestCmdClaimAndReleaseStore(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}}},
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

func TestCmdValidateStoresComment(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}
	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("validate err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	if len(store.CommentsFor("pt-1")) == 0 {
		t.Fatalf("expected comment stored")
	}
}

func TestCmdAddCommentListSnapshot(t *testing.T) {
	path, store := setupStoreEnv(t)
	if err := cmdAdd([]string{"New Task", "--role", "dev", "--template", "backend_endpoint", "--artifact", "spec:new", "--manual", "check", "--tests", "echo ok", "--criteria", "observed ok"}); err != nil {
		t.Fatalf("add err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	iss, _, err := store.GetTask(t.Context(), "pt-1")
	if err != nil || iss.Title != "New Task" {
		t.Fatalf("task not created: %+v err=%v", iss, err)
	}
	if err := cmdComment([]string{"pt-1", "note"}); err != nil {
		t.Fatalf("comment err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	if len(store.CommentsFor("pt-1")) == 0 {
		t.Fatalf("expected comment stored")
	}
	if err := cmdList([]string{"--status=open"}); err != nil {
		t.Fatalf("list err: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "snap.json")
	if err := cmdSnapshot([]string{"--out", outPath}); err != nil {
		t.Fatalf("snapshot err: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
}

func TestCmdClaimWithDraft(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "DraftTask", Template: "backend_endpoint", Role: "dev", Artifact: "spec:draft", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Claim with --draft flag
	if err := cmdClaim([]string{"--as", "alice", "--draft", "pt-1"}); err != nil {
		t.Fatalf("claim --draft err: %v", err)
	}

	// Verify state:draft label is present
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ := store.GetTask(t.Context(), "pt-1")
	hasDraft := false
	for _, l := range issue.Labels {
		if l == "state:draft" {
			hasDraft = true
			break
		}
	}
	if !hasDraft {
		t.Fatalf("expected state:draft label after claim --draft, got labels: %v", issue.Labels)
	}
}
