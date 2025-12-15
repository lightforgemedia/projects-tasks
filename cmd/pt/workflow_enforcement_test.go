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

func TestCmdClaimEnforcesWorkflowHardGate(t *testing.T) {
	path, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Spike", Template: "spike", Role: "dev", Artifact: "doc:spike", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "Build", Template: "backend_endpoint", Role: "dev", Artifact: "code:build", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
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
[phases.gate]
type = "hard"
condition = "all_closed"
block_message = "Complete risk tasks first"

[[phases]]
id = "build"
name = "Build"
order = 2
`
	if err := os.WriteFile(wfPath, []byte(wf), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	t.Setenv("PT_WORKFLOW", wfPath)

	// pt-2 is in build phase; should be blocked until risk phase is closed.
	if err := cmdClaim([]string{"--as", "alice", "pt-2"}); err == nil {
		t.Fatalf("expected hard gate to block claim")
	}

	// Close the risk task and retry.
	store = pt.NewStoreClient(path, "pt")
	if err := store.UpdateIssue(t.Context(), "pt-1", "closed", ""); err != nil {
		t.Fatalf("close risk: %v", err)
	}
	if err := cmdClaim([]string{"--as", "alice", "pt-2"}); err != nil {
		t.Fatalf("claim after risk closed: %v", err)
	}
}

func TestCmdClaimSoftGateRequiresOverrideAndRecordsComment(t *testing.T) {
	path, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Spike", Template: "spike", Role: "dev", Artifact: "doc:spike", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "Build", Template: "backend_endpoint", Role: "dev", Artifact: "code:build", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
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
[phases.gate]
type = "soft"
condition = "all_closed"
reminder_before = "Risk not complete"

[[phases]]
id = "build"
name = "Build"
order = 2
`
	if err := os.WriteFile(wfPath, []byte(wf), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	t.Setenv("PT_WORKFLOW", wfPath)

	// Without override, claim should be blocked.
	if err := cmdClaim([]string{"--as", "alice", "pt-2"}); err == nil {
		t.Fatalf("expected soft gate to block claim without override")
	}

	// With override, claim should proceed and record an override comment.
	if err := cmdClaim([]string{"--as", "alice", "--override-soft", "proceeding to unblock parallel work", "pt-2"}); err != nil {
		t.Fatalf("claim with override: %v", err)
	}

	store = pt.NewStoreClient(path, "pt")
	comments := store.CommentsFor("pt-2")
	found := false
	for _, c := range comments {
		if strings.Contains(c, "gate-override: risk") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected gate-override comment on pt-2, got: %v", comments)
	}
}

func TestCmdReadyDefaultsToCurrentPhaseWhenWorkflowExists(t *testing.T) {
	_, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Spike", Template: "spike", Role: "dev", Artifact: "doc:spike", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "Build", Template: "backend_endpoint", Role: "dev", Artifact: "code:build", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
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
	if err := os.WriteFile(wfPath, []byte(wf), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	t.Setenv("PT_WORKFLOW", wfPath)

	// Capture stdout for --json output.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdReady([]string{"--json"})

	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("ready: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal ready json: %v\nraw=%s", err, buf.String())
	}
	if len(out) != 1 {
		t.Fatalf("expected only current-phase task, got %d: %v", len(out), out)
	}
	if out[0]["id"] != "pt-1" {
		t.Fatalf("expected pt-1 in current phase, got %v", out[0]["id"])
	}

	// With --all-phases, both tasks are shown.
	old = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w

	err = cmdReady([]string{"--json", "--all-phases"})

	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("ready --all-phases: %v", err)
	}
	buf.Reset()
	_, _ = io.Copy(&buf, r)
	out = nil
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal ready json (all): %v\nraw=%s", err, buf.String())
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 tasks with --all-phases, got %d: %v", len(out), out)
	}
}

