package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestWorkflowHelpHookSnippetUsesSupportedHookConfig(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = cmdWorkflow([]string{"help"})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	// Ensure the snippet matches PT hook config + placeholder conventions.
	if !strings.Contains(out, "[[hook]]") {
		t.Fatalf("expected [[hook]] in workflow help output, got:\n%s", out)
	}
	if !strings.Contains(out, "on_fail = \"fail\"") {
		t.Fatalf("expected on_fail=fail in workflow help output, got:\n%s", out)
	}
	if !strings.Contains(out, "--task={{id}}") {
		t.Fatalf("expected {{id}} placeholder in workflow help output, got:\n%s", out)
	}
}

func TestWorkflowStatusDoesNotBlockOnEmptyEarlierPhases(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "closed", ""); err != nil {
		t.Fatalf("close err: %v", err)
	}

	td := t.TempDir()
	wfPath := td + "/wf.toml"
	// Note: phase 1 has 0 tasks (risk), but should not block later phases.
	content := `
name = "wf"

[phase_assignment]
label_prefix = "phase:"

[phase_assignment.by_template]
backend_endpoint = "backend"

[[phases]]
id = "risk"
name = "Risk"
order = 1
[phases.gate]
type = "soft"
condition = "all_closed"

[[phases]]
id = "backend"
name = "Backend"
order = 2
`
	if err := os.WriteFile(wfPath, []byte(content), 0644); err != nil {
		t.Fatalf("write wf err: %v", err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdWorkflowStatus([]string{"--db", os.Getenv("PT_DB"), "--workflow", wfPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("workflow status err: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if strings.Contains(out, "waiting for phase:risk") {
		t.Fatalf("did not expect backend to be blocked by empty risk phase, got:\n%s", out)
	}
	// Ensure the backend phase is rendered and includes the task.
	if !strings.Contains(out, "Backend") || !strings.Contains(out, "pt-1") {
		t.Fatalf("expected backend phase with task, got:\n%s", out)
	}
}

func TestWorkflowStatusDoesNotMarkCompletePhaseBlocked(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "closed", ""); err != nil {
		t.Fatalf("close err: %v", err)
	}

	td := t.TempDir()
	wfPath := td + "/wf.toml"
	content := `
name = "wf"

[phase_assignment]
label_prefix = "phase:"

[phase_assignment.by_template]
backend_endpoint = "backend"

[[phases]]
id = "backend"
name = "Backend"
order = 1

[phases.gate]
type = "hard"
condition = "has_comment:user-approved"
block_message = "Need approval"
`
	if err := os.WriteFile(wfPath, []byte(content), 0644); err != nil {
		t.Fatalf("write wf err: %v", err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdWorkflowStatus([]string{"--db", os.Getenv("PT_DB"), "--workflow", wfPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("workflow status err: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()

	if strings.Contains(out, "BLOCKED") {
		t.Fatalf("did not expect a completed phase to be marked BLOCKED, got:\n%s", out)
	}
}
