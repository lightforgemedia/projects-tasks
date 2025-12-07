package pt

import (
	"strings"
	"testing"
)

func TestRenderManifestTree(t *testing.T) {
	m := Manifest{
		Title: "Test Phase",
		Tasks: []Task{
			{Title: "Root", Role: "arch"},
			{Title: "Child A", Role: "dev", Deps: []string{"Root"}},
			{Title: "Child B", Role: "dev", Deps: []string{"Root"}},
			{Title: "Grandchild", Role: "qa", Deps: []string{"Child A"}},
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
	m := Manifest{
		Title: "Cycle Phase",
		Tasks: []Task{
			{Title: "A", Role: "dev", Deps: []string{"B"}},
			{Title: "B", Role: "dev", Deps: []string{"A"}},
		},
	}

	got := RenderManifestTree(m)
	if !strings.Contains(got, "(cycle)") {
		t.Fatalf("expected cycle marker, got:\n%s", got)
	}
}
