package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdFeedbackCreatesTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := cmdFeedback([]string{"--desc", "ce ui"}); err != nil {
		t.Fatalf("cmdFeedback err: %v", err)
	}

	dir := filepath.Join(home, ".pt", "feedback")
	files, err := filepath.Glob(filepath.Join(dir, "*.feedback.md"))
	if err != nil {
		t.Fatalf("glob err: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 feedback file, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read feedback err: %v", err)
	}
	content := string(data)
	for _, want := range []string{"Summary:", "Findings:", "Gaps/Risks:", "Recommendations:", "Status:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("feedback template missing %q section", want)
		}
	}
	if !strings.Contains(files[0], "ce-ui.feedback.md") {
		t.Fatalf("slug not applied to filename: %s", files[0])
	}
}

func TestCmdFeedbackDryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := cmdFeedback([]string{"--desc", "demo", "--dry-run"}); err != nil {
		t.Fatalf("cmdFeedback dry-run err: %v", err)
	}
	dir := filepath.Join(home, ".pt", "feedback")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected no feedback directory on dry-run, got %v", err)
	}
}
