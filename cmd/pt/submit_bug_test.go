package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitBugWritesReportUnderHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := cmdSubmitBug([]string{
		"--label=ce ui",
		"--description=pt workflow status shows blocked when tasks are ready",
		"--found_in=projects/codexacp-client (pt workflow status)",
		"--repro=cd projects/codexacp-client && pt workflow status",
	}); err != nil {
		t.Fatalf("submit-bug err: %v", err)
	}

	dir := filepath.Join(home, ".pt", "bugs")
	files, err := filepath.Glob(filepath.Join(dir, "*", "bug-*.md"))
	if err != nil {
		t.Fatalf("glob err: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 bug report file, got %d (%v)", len(files), files)
	}
	if !strings.Contains(files[0], "bug-ce-ui.md") {
		t.Fatalf("expected slugged filename, got %s", files[0])
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read bug report: %v", err)
	}
	content := string(raw)
	for _, want := range []string{"# PT Bug Report", "Label: ce ui", "Found In:", "## Description", "## Reproduction"} {
		if !strings.Contains(content, want) {
			t.Fatalf("bug report missing %q\n%s", want, content)
		}
	}
}

func TestSubmitBugRequiresFields(t *testing.T) {
	if err := cmdSubmitBug([]string{"--label=x"}); err == nil {
		t.Fatalf("expected error for missing required flags")
	}
}

