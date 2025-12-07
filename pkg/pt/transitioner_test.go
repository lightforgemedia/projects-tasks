package pt

import (
	"context"
	"testing"
)

func newTestStore(t *testing.T) *StoreClient {
	t.Helper()
	return NewStoreClient(t.TempDir()+"/pt.db.json", "pt")
}

func seedSimpleManifest(t *testing.T, store *StoreClient) map[string]string {
	t.Helper()
	manifest := Manifest{
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", DoD: DefinitionOfDone{}},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Deps: []string{"A"}, DoD: DefinitionOfDone{}},
		},
	}
	ids, err := store.Sync(context.Background(), manifest)
	if err != nil {
		t.Fatalf("sync err: %v", err)
	}
	return ids
}

func TestTransitionerClaimBlockedByDepsStore(t *testing.T) {
	store := newTestStore(t)
	ids := seedSimpleManifest(t, store)
	trans := Transitioner{Client: store}
	if err := trans.Claim(context.Background(), ids["B"], "alice"); err == nil {
		t.Fatalf("expected claim blocked by deps")
	}
}

func TestTransitionerClaimHappyPathStore(t *testing.T) {
	store := newTestStore(t)
	ids := seedSimpleManifest(t, store)
	_ = store.UpdateIssue(context.Background(), ids["A"], "closed", "")
	trans := Transitioner{Client: store}
	if err := trans.Claim(context.Background(), ids["B"], "alice"); err != nil {
		t.Fatalf("claim err: %v", err)
	}
	iss, _, _ := store.GetTask(context.Background(), ids["B"])
	if iss.Status != "in_progress" || iss.Assignee != "alice" {
		t.Fatalf("unexpected claim state: %+v", iss)
	}
}

func TestTransitionerSubmitForReviewStore(t *testing.T) {
	store := newTestStore(t)
	ids := seedSimpleManifest(t, store)
	_ = store.UpdateIssue(context.Background(), ids["A"], "in_progress", "")
	trans := Transitioner{Client: store}
	if err := trans.SubmitForReview(context.Background(), ids["A"], "ok"); err != nil {
		t.Fatalf("submit err: %v", err)
	}
	iss, _, _ := store.GetTask(context.Background(), ids["A"])
	if iss.Status != "needs_review" {
		t.Fatalf("expected needs_review, got %s", iss.Status)
	}
}

func TestTransitionerApproveStore(t *testing.T) {
	store := newTestStore(t)
	ids := seedSimpleManifest(t, store)
	_ = store.UpdateIssue(context.Background(), ids["A"], "needs_review", "")
	trans := Transitioner{Client: store}
	if err := trans.Approve(context.Background(), ids["A"]); err != nil {
		t.Fatalf("approve err: %v", err)
	}
	iss, _, _ := store.GetTask(context.Background(), ids["A"])
	if iss.Status != "closed" {
		t.Fatalf("expected closed, got %s", iss.Status)
	}
}

func TestTransitionerRejectStore(t *testing.T) {
	store := newTestStore(t)
	ids := seedSimpleManifest(t, store)
	_ = store.UpdateIssue(context.Background(), ids["A"], "needs_review", "")
	trans := Transitioner{Client: store}
	if err := trans.Reject(context.Background(), ids["A"], "fix"); err != nil {
		t.Fatalf("reject err: %v", err)
	}
	iss, _, _ := store.GetTask(context.Background(), ids["A"])
	if iss.Status != "in_progress" {
		t.Fatalf("expected in_progress, got %s", iss.Status)
	}
}
