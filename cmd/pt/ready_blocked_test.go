package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestReadySkipsManuallyBlockedTasksByDefault(t *testing.T) {
	t.Setenv("PT_WORKFLOW", "")

	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.SetBlocked(t.Context(), "pt-2", "out of focus", "agent"); err != nil {
		t.Fatalf("set blocked err: %v", err)
	}

	capture := func(args []string) string {
		t.Helper()
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		err := cmdReady(args)
		w.Close()
		os.Stdout = old
		if err != nil {
			t.Fatalf("ready err: %v", err)
		}
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}

	outRaw := capture([]string{"--json"})
	var out []map[string]any
	if err := json.Unmarshal([]byte(outRaw), &out); err != nil {
		t.Fatalf("unmarshal ready json: %v\nraw=%s", err, outRaw)
	}
	if len(out) != 1 {
		t.Fatalf("expected blocked task hidden by default, got %d: %v", len(out), out)
	}
	if out[0]["id"] != "pt-1" {
		t.Fatalf("expected pt-1 to remain, got %v", out[0]["id"])
	}

	outRaw = capture([]string{"--json", "--include-blocked"})
	out = nil
	if err := json.Unmarshal([]byte(outRaw), &out); err != nil {
		t.Fatalf("unmarshal ready json (include-blocked): %v\nraw=%s", err, outRaw)
	}
	if len(out) != 2 {
		t.Fatalf("expected blocked task included with --include-blocked, got %d: %v", len(out), out)
	}
	var blockedFound bool
	for _, row := range out {
		if row["id"] == "pt-2" {
			blockedFound = true
			if row["blocked"] != true {
				t.Fatalf("expected pt-2 blocked=true, got %v", row["blocked"])
			}
			if row["block_reason"] != "out of focus" {
				t.Fatalf("expected pt-2 block_reason to be preserved, got %v", row["block_reason"])
			}
		}
	}
	if !blockedFound {
		t.Fatalf("expected pt-2 in output with --include-blocked, got: %v", out)
	}
}

func TestReadyAppliesLimitAfterFilteringManuallyBlocked(t *testing.T) {
	t.Setenv("PT_WORKFLOW", "")

	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.SetBlocked(t.Context(), "pt-1", "out of focus", "agent"); err != nil {
		t.Fatalf("set blocked err: %v", err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := cmdReady([]string{"--json", "--sort=title", "--limit=1"})
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("ready err: %v", err)
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal ready json: %v\nraw=%s", err, buf.String())
	}
	if len(out) != 1 {
		t.Fatalf("expected limit applied after filtering, got %d: %v", len(out), out)
	}
	if out[0]["id"] != "pt-2" {
		t.Fatalf("expected pt-2 (pt-1 is blocked), got %v", out[0]["id"])
	}
}
