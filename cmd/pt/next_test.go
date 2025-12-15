package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

