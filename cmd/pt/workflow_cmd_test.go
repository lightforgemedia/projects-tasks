package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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

func TestFindWorkflowFileResolvesPTWORKFLOWRelativeToRepoRoot(t *testing.T) {
	// Simulate running from a subdirectory (like cmd/pt) while PT_WORKFLOW is set to a repo-relative path.
	// This matches how hooks run go test ./... and how users may invoke pt from different dirs.
	root, err := gitRepoRoot("")
	if err != nil || root == "" {
		t.Fatalf("expected git repo root, err=%v root=%q", err, root)
	}

	oldWd, _ := os.Getwd()
	subdir := filepath.Join(root, "cmd", "pt")
	if err := os.Chdir(subdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	t.Setenv("PT_WORKFLOW", "workflows/risk-first.toml")
	got, err := findWorkflowFile()
	if err != nil {
		t.Fatalf("findWorkflowFile: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("workflows", "risk-first.toml")) {
		t.Fatalf("unexpected workflow path: %q", got)
	}
}

func TestWorkflowStatusSurfacesDependencyBlockersAndSuggestedNextSkipsBlocked(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", Deps: []string{"A"}, DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	td := t.TempDir()
	wfPath := td + "/wf.toml"
	content := `
name = "wf"

[phase_assignment]
label_prefix = "phase:"

[phase_assignment.by_template]
backend_endpoint = "build"

[[phases]]
id = "build"
name = "Build"
order = 1
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

	var line1, line2 string
	for _, line := range strings.Split(out, "\n") {
		// Match the task rows, not the "Suggested next" footer.
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[1] {
		case "pt-1":
			line1 = line
		case "pt-2":
			line2 = line
		}
	}
	if line1 == "" || line2 == "" {
		t.Fatalf("expected lines for pt-1 and pt-2, got:\n%s", out)
	}

	// A is unblocked and should be marked READY.
	if !strings.Contains(line1, "← READY") {
		t.Fatalf("expected pt-1 to be marked READY, got line: %q\nfull:\n%s", line1, out)
	}

	// B depends on A, so should be shown as blocked by deps and not READY.
	if !strings.Contains(line2, "[blocked") {
		t.Fatalf("expected pt-2 to show a blocked indicator, got line: %q\nfull:\n%s", line2, out)
	}
	if strings.Contains(line2, "← READY") {
		t.Fatalf("did not expect pt-2 to be marked READY, got line: %q\nfull:\n%s", line2, out)
	}

	// Suggested next should skip deps-blocked tasks.
	if !strings.Contains(out, "Suggested next:  pt claim pt-1") {
		t.Fatalf("expected suggested next to target pt-1, got:\n%s", out)
	}
}
