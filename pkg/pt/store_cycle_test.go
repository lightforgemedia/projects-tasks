package pt

import "testing"

func TestStoreSyncFailsOnCycle(t *testing.T) {
	store := NewStoreClient(t.TempDir()+"/pt.db.json", "pt")
	manifest := Manifest{
		Title: "cycle",
		Owner: "test",
		Tasks: []Task{
			{Template: "backend_endpoint", Title: "A", Role: "dev", Artifact: "spec:a", Deps: []string{"B"}, DoD: DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "m", Criteria: []string{"c"}}},
			{Template: "backend_endpoint", Title: "B", Role: "dev", Artifact: "spec:b", Deps: []string{"A"}, DoD: DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "m", Criteria: []string{"c"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err == nil {
		t.Fatalf("expected sync to fail on cycle")
	}
}

