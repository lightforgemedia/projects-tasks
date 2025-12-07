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
	if len(ready) != 1 {
		t.Fatalf("expected only root ready, got %d", len(ready))
	}
	// Mark A done to unblock B
	if err := client.UpdateIssue(ctx, ids["A"], "closed", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}
	ready, _ = client.Ready(ctx, "dev", 10)
	if len(ready) != 1 || ready[0].Title != "B" {
		t.Fatalf("expected B ready after A closed, got %+v", ready)
	}
	// Ensure persistence
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("expected db file: %v", err)
	}
}
