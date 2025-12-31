package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestUXPreflightWritesCanonicalAndSetsMeta(t *testing.T) {
	loadedHooks = nil
	_, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{
				Title:    "UX Task",
				Template: "frontend_component",
				Role:     "dev",
				Artifact: "ui:Widget",
				UX:       &pt.UXConfig{Type: "web"},
				DoD:      pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}},
			},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if err := cmdUX([]string{"preflight", "pt-1"}); err != nil {
		t.Fatalf("ux preflight: %v", err)
	}

	store2 := pt.NewStoreClient(os.Getenv("PT_DB"), "pt")
	_, meta, err := store2.GetTask(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if meta.UXState == nil || !meta.UXState.PreflightDone || meta.UXState.PreflightFile == "" {
		t.Fatalf("expected preflight meta set, got: %#v", meta.UXState)
	}

	reviewsDir := filepath.Join(filepath.Dir(os.Getenv("PT_DB")), ".pt", "reviews")
	if _, err := os.Stat(filepath.Join(reviewsDir, "UX-PREFLIGHT-pt-1.md")); err != nil {
		t.Fatalf("expected preflight file to exist: %v", err)
	}
}

func TestCmdClaimHardBlocksDiscoveryDocUXWithoutPreflight(t *testing.T) {
	loadedHooks = nil
	_, store := setupStoreEnv(t)
	t.Setenv("USER", "tester")

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{
				Title:    "Discovery UX",
				Template: "discovery",
				Role:     "dev",
				Artifact: "doc:web-spec.md",
				UX:       &pt.UXConfig{Type: "web"},
				DoD:      pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}},
			},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	err := cmdClaim([]string{"pt-1"})
	if err == nil {
		t.Fatalf("expected claim to fail, got nil")
	}
	if !strings.Contains(err.Error(), "ux-preflight") {
		t.Fatalf("expected ux-preflight error, got: %v", err)
	}
}
