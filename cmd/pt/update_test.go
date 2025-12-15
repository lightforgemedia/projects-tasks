package main

import (
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdUpdateFlagOrdering(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Flags before id
	if err := cmdUpdate([]string{"--next-hint", "next", "pt-1"}); err != nil {
		t.Fatalf("update (--next-hint next pt-1): %v", err)
	}

	// Flags after id
	if err := cmdUpdate([]string{"pt-1", "--next-hint", "next2"}); err != nil {
		t.Fatalf("update (pt-1 --next-hint next2): %v", err)
	}
}
