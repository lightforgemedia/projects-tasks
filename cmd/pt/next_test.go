package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdNextModes(t *testing.T) {
	t.Run("REVIEW when needs_review exists", func(t *testing.T) {
		_, store := setupStoreEnv(t)
		manifest := pt.Manifest{
			Tasks: []pt.Task{
				{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			},
		}
		if _, err := store.Sync(t.Context(), manifest); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if err := store.UpdateIssue(t.Context(), "pt-1", "needs_review", ""); err != nil {
			t.Fatalf("update: %v", err)
		}

		out := runCmdNextJSON(t, []string{"--json"})
		if out["mode"] != "REVIEW" {
			t.Fatalf("mode=%v, want REVIEW", out["mode"])
		}
	})

	t.Run("WORK selects current-phase open task when workflow exists", func(t *testing.T) {
		_, store := setupStoreEnv(t)
		manifest := pt.Manifest{
			Tasks: []pt.Task{
				{Title: "Risk", Template: "spike", Role: "dev", Artifact: "doc:risk", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
				{Title: "Build", Template: "backend_endpoint", Role: "dev", Artifact: "code:build", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			},
		}
		if _, err := store.Sync(t.Context(), manifest); err != nil {
			t.Fatalf("sync: %v", err)
		}

		wfPath := filepath.Join(t.TempDir(), "wf.toml")
		wf := `
name = "wf"

[phase_assignment.by_template]
spike = "risk"
backend_endpoint = "build"

[[phases]]
id = "risk"
name = "Risk"
order = 1

[[phases]]
id = "build"
name = "Build"
order = 2
`
		if err := os.WriteFile(wfPath, []byte(wf), 0o644); err != nil {
			t.Fatalf("write wf: %v", err)
		}
		t.Setenv("PT_WORKFLOW", wfPath)

		out := runCmdNextJSON(t, []string{"--json"})
		if out["mode"] != "WORK" {
			t.Fatalf("mode=%v, want WORK", out["mode"])
		}
		current := out["current_phase"].(map[string]any)
		if current["id"] != "risk" {
			t.Fatalf("current_phase.id=%v, want risk", current["id"])
		}
	})

	t.Run("workflow selection warning when multiple workflows exist and none selected", func(t *testing.T) {
		_, store := setupStoreEnv(t)
		manifest := pt.Manifest{
			Tasks: []pt.Task{
				{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			},
		}
		if _, err := store.Sync(t.Context(), manifest); err != nil {
			t.Fatalf("sync: %v", err)
		}

		// Create two workflow files and ensure PT_WORKFLOW is not set.
		wd := t.TempDir()
		if err := os.MkdirAll(filepath.Join(wd, "workflows"), 0o755); err != nil {
			t.Fatalf("mkdir workflows: %v", err)
		}
		if err := os.WriteFile(filepath.Join(wd, "workflows", "a.toml"), []byte("name=\"a\"\n[[phases]]\nid=\"p\"\nname=\"p\"\norder=1\n"), 0o644); err != nil {
			t.Fatalf("write wf a: %v", err)
		}
		if err := os.WriteFile(filepath.Join(wd, "workflows", "b.toml"), []byte("name=\"b\"\n[[phases]]\nid=\"p\"\nname=\"p\"\norder=1\n"), 0o644); err != nil {
			t.Fatalf("write wf b: %v", err)
		}

		oldWd, _ := os.Getwd()
		_ = os.Chdir(wd)
		t.Cleanup(func() { _ = os.Chdir(oldWd) })
		t.Setenv("PT_WORKFLOW", "")

		out := runCmdNextJSON(t, []string{"--json"})
		why, _ := out["why"].([]any)
		if len(why) == 0 {
			t.Fatalf("expected why messages")
		}
		found := false
		for _, v := range why {
			if s, ok := v.(string); ok && strings.Contains(s, "workflow not selected") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected workflow selection warning in why, got: %v", why)
		}
	})

	t.Run("WORK does not embed local username in claim recommendation", func(t *testing.T) {
		_, store := setupStoreEnv(t)
		manifest := pt.Manifest{
			Tasks: []pt.Task{
				{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			},
		}
		if _, err := store.Sync(t.Context(), manifest); err != nil {
			t.Fatalf("sync: %v", err)
		}

		t.Setenv("USER", "local-user-should-not-appear")
		t.Setenv("PT_WORKFLOW", "")

		out := runCmdNextJSON(t, []string{"--json"})
		if out["mode"] != "WORK" {
			t.Fatalf("mode=%v, want WORK", out["mode"])
		}
		recs, ok := out["recommended"].([]any)
		if !ok || len(recs) == 0 {
			t.Fatalf("expected recommendations, got: %v", out["recommended"])
		}
		foundClaim := false
		for _, r := range recs {
			m, _ := r.(map[string]any)
			cmd, _ := m["cmd"].(string)
			if strings.HasPrefix(cmd, "pt claim ") {
				foundClaim = true
				if strings.Contains(cmd, "local-user-should-not-appear") {
					t.Fatalf("expected claim recommendation to not include local username, got: %q", cmd)
				}
				if !strings.Contains(cmd, "--as=$USER") {
					t.Fatalf("expected claim recommendation to use $USER placeholder, got: %q", cmd)
				}
			}
		}
		if !foundClaim {
			t.Fatalf("expected a pt claim recommendation, got: %v", recs)
		}
	})
}

func TestNextDoesNotEmbedUsername(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync: %v", err)
	}

	t.Setenv("USER", "local-user-should-not-appear")
	t.Setenv("PT_WORKFLOW", "")

	out := runCmdNextJSON(t, []string{"--json"})
	if out["mode"] != "WORK" {
		t.Fatalf("mode=%v, want WORK", out["mode"])
	}
	recs, ok := out["recommended"].([]any)
	if !ok || len(recs) == 0 {
		t.Fatalf("expected recommendations, got: %v", out["recommended"])
	}
	foundClaim := false
	for _, r := range recs {
		m, _ := r.(map[string]any)
		cmd, _ := m["cmd"].(string)
		if strings.HasPrefix(cmd, "pt claim ") {
			foundClaim = true
			if strings.Contains(cmd, "local-user-should-not-appear") {
				t.Fatalf("expected claim recommendation to not include local username, got: %q", cmd)
			}
			if !strings.Contains(cmd, "--as=$USER") {
				t.Fatalf("expected claim recommendation to use $USER placeholder, got: %q", cmd)
			}
		}
	}
	if !foundClaim {
		t.Fatalf("expected a pt claim recommendation, got: %v", recs)
	}
}

func runCmdNextJSON(t *testing.T, args []string) map[string]any {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdNext(args)

	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("cmdNext: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, buf.String())
	}
	return out
}
