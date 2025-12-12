package pt

import (
	"path/filepath"
	"testing"
)

// TestPhasesManifestsParse ensures shipped manifests stay valid as the schema evolves.
func TestPhasesManifestsParse(t *testing.T) {
	paths, err := filepath.Glob("../../phases/*.toml")
	if err != nil {
		t.Fatalf("glob phases: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no manifests found in phases/")
	}
	for _, p := range paths {
		if _, err := ParseManifest(p); err != nil {
			t.Fatalf("manifest %s failed to parse: %v", p, err)
		}
	}
}
