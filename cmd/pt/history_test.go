package main

import (
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdHistory(t *testing.T) {
	path, store := setupStoreEnv(t)
	_, _ = store.Sync(t.Context(), pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	})
	if err := cmdHistory([]string{"pt-1"}); err != nil {
		t.Fatalf("history err: %v", err)
	}
	// Ensure history is persisted
	store2 := pt.NewStoreClient(path, "pt")
	events, err := store2.History(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("history fetch err: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected at least one history event")
	}
}
