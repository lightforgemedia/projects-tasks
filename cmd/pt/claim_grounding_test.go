package main

import (
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdClaimBlocksWithoutGroundingForStrictTemplate(t *testing.T) {
	loadedHooks = nil
	_, store := setupStoreEnv(t)
	t.Setenv("USER", "tester")

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task A", Template: "backend_endpoint", Role: "dev", Artifact: "code:cmd/pt/main.go", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	err := cmdClaim([]string{"pt-1"})
	if err == nil {
		t.Fatalf("expected claim to fail, got nil")
	}
	if !strings.Contains(err.Error(), "missing grounding pack") {
		t.Fatalf("expected grounding error, got: %v", err)
	}
}
