package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestWorkflowCheck_EngineV2Parity(t *testing.T) {
	_, store := setupStoreEnv(t)
	t.Setenv("PT_SKIP_HOOKS", "1")

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Risk", Template: "discovery", Role: "dev", Artifact: "doc:risk", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
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
discovery = "risk"
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
	if err := os.WriteFile(wfPath, []byte(wf), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	run := func(t *testing.T, engine string) string {
		t.Helper()
		t.Setenv("PT_ENGINE", engine)
		t.Setenv("PT_WORKFLOW", wfPath)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		_ = cmdWorkflow([]string{"check", "--db", os.Getenv("PT_DB"), "--workflow", wfPath, "--task", "pt-2"})
		w.Close()
		os.Stdout = old

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}

	outV1 := run(t, "")
	outV2 := run(t, "v2")
	if outV1 != outV2 {
		t.Fatalf("workflow check output mismatch v1 vs v2\nv1=%q\nv2=%q", outV1, outV2)
	}
}

