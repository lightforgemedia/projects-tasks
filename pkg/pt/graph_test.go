package pt

import (
	"strings"
	"testing"
)

func TestRenderManifestTree(t *testing.T) {
	dod := DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "check", Criteria: []string{"observed ok"}}
	m := Manifest{
		Title: "Test Phase",
		Tasks: []Task{
			{Title: "Root", Role: "arch", Template: "backend_endpoint", Artifact: "spec:root", DoD: dod},
			{Title: "Child A", Role: "dev", Template: "backend_endpoint", Artifact: "spec:a", Deps: []string{"Root"}, DoD: dod},
			{Title: "Child B", Role: "dev", Template: "backend_endpoint", Artifact: "spec:b", Deps: []string{"Root"}, DoD: dod},
			{Title: "Grandchild", Role: "qa", Template: "backend_endpoint", Artifact: "spec:c", Deps: []string{"Child A"}, DoD: dod},
		},
	}

	got := RenderManifestTree(m)

	// Check for key structural elements
	expectedLines := []string{
		"Phase: Test Phase",
		"└── [arch] Root",
		"    ├── [dev] Child A",
		"    │   └── [qa] Grandchild",
		"    └── [dev] Child B",
	}

	for _, line := range expectedLines {
		if !strings.Contains(got, line) {
			t.Errorf("expected output to contain %q, got:\n%s", line, got)
		}
	}
}

func TestRenderManifestTreeCyclesAreMarked(t *testing.T) {
	dod := DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "check", Criteria: []string{"observed ok"}}
	m := Manifest{
		Title: "Cycle Phase",
		Tasks: []Task{
			{Title: "A", Role: "dev", Template: "backend_endpoint", Artifact: "spec:a", Deps: []string{"B"}, DoD: dod},
			{Title: "B", Role: "dev", Template: "backend_endpoint", Artifact: "spec:b", Deps: []string{"A"}, DoD: dod},
		},
	}

	got := RenderManifestTree(m)
	if !strings.Contains(got, "(cycle)") {
		t.Fatalf("expected cycle marker, got:\n%s", got)
	}
}
