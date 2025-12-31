package main

import (
	"os"
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

func TestCmdUpdateGroundingPack(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if err := cmdUpdate([]string{"pt-1", "--grounding-files", "cmd/pt/main.go,pkg/pt/types.go", "--grounding-symbols", "cmdClaim,TaskMeta"}); err != nil {
		t.Fatalf("update grounding: %v", err)
	}

	store2 := pt.NewStoreClient(os.Getenv("PT_DB"), "pt")
	_, meta, err := store2.GetTask(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if meta.Grounding == nil || len(meta.Grounding.Files) != 2 || len(meta.Grounding.Symbols) != 2 {
		t.Fatalf("expected grounding files+symbols set, got: %#v", meta.Grounding)
	}

	if err := cmdUpdate([]string{"pt-1", "--grounding-files", "-"}); err != nil {
		t.Fatalf("clear grounding files: %v", err)
	}
	store2 = pt.NewStoreClient(os.Getenv("PT_DB"), "pt")
	_, meta, err = store2.GetTask(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("get task (2): %v", err)
	}
	if meta.Grounding == nil || len(meta.Grounding.Files) != 0 || len(meta.Grounding.Symbols) != 2 {
		t.Fatalf("expected grounding files cleared but symbols preserved, got: %#v", meta.Grounding)
	}
}
