package pt

import (
	"context"
	"os"
	"testing"
)

func TestStoreSyncAndReady(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	manifest := Manifest{
		Title: "Test",
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", DoD: DefinitionOfDone{}},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Deps: []string{"A"}, DoD: DefinitionOfDone{}},
		},
	}
	ctx := context.Background()
	ids, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	ready, err := client.Ready(ctx, "dev", 10)
	if err != nil {
		t.Fatalf("ready err: %v", err)
	}
	// Ready now returns all open tasks (blocked or not).
	if len(ready) != 2 {
		t.Fatalf("expected 2 open tasks, got %d", len(ready))
	}
	// Mark A done to unblock B (logic check for transitioner, but here just updating state)
	if err := client.UpdateIssue(ctx, ids["A"], "closed", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}
	ready, _ = client.Ready(ctx, "dev", 10)
	// Still 2 tasks? No, A is closed. So only B (open) should be returned.
	if len(ready) != 1 || ready[0].Title != "B" {
		t.Fatalf("expected B ready after A closed, got %+v", ready)
	}
	// Ensure persistence
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("expected db file: %v", err)
	}
}

func TestStoreNextHint(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	manifest := Manifest{
		Title: "Hints",
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", NextHint: "Do B next", DoD: DefinitionOfDone{Manual: "check"}},
		},
	}
	ctx := context.Background()
	ids, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	iss, meta, err := client.GetTask(ctx, ids["A"])
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if iss.NextHint != "Do B next" || meta.NextHint != "Do B next" {
		t.Fatalf("next_hint not stored: issue=%q meta=%q", iss.NextHint, meta.NextHint)
	}
}
