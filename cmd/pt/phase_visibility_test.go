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

func TestShowDisplaysPhaseWithWorkflowOverride(t *testing.T) {
	t.Setenv("PT_WORKFLOW", "")
	_, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Risk Task", Template: "spike", Role: "dev", Artifact: "doc:risk", MaxHours: 1, DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "Build Task", Template: "backend_endpoint", Role: "dev", Artifact: "spec:build", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
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
	if err := os.WriteFile(wfPath, []byte(wf), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := cmdShow([]string{"--workflow", wfPath, "pt-1"})
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "Phase: Risk") {
		t.Fatalf("expected phase name in output, got:\n%s", out)
	}
}

func TestListFiltersByPhaseWithWorkflowOverride(t *testing.T) {
	t.Setenv("PT_WORKFLOW", "")
	_, store := setupStoreEnv(t)

	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Risk Task", Template: "spike", Role: "dev", Artifact: "doc:risk", MaxHours: 1, DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "Build Task", Template: "backend_endpoint", Role: "dev", Artifact: "spec:build", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
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
	if err := os.WriteFile(wfPath, []byte(wf), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := cmdList([]string{"--workflow", wfPath, "--phase", "risk", "--porcelain"})
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "pt-1\t") {
		t.Fatalf("expected pt-1 in risk phase list, got:\n%s", out)
	}
	if strings.Contains(out, "pt-2\t") {
		t.Fatalf("did not expect pt-2 in risk phase list, got:\n%s", out)
	}
}
