package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"projects-tasks/pkg/pt"
)

func TestParseContextInitArgs_AllowsFlagsAfterID(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		id    string
		role  string
		isErr bool
	}{
		{"flagAfterID", []string{"proj-1", "--role", "builder"}, "proj-1", "builder", false},
		{"flagBeforeID", []string{"--role", "builder", "proj-1"}, "proj-1", "builder", false},
		{"flagEquals", []string{"proj-1", "--role=builder"}, "proj-1", "builder", false},
		{"noRole", []string{"proj-1"}, "proj-1", "", false},
		{"missingID", []string{"--role", "builder"}, "", "", true},
		{"unknownFlag", []string{"proj-1", "--foo"}, "", "", true},
		{"extraID", []string{"proj-1", "proj-2"}, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, role, err := parseContextInitArgs(tt.args)
			if tt.isErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.isErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tt.id || role != tt.role {
					t.Fatalf("got id=%q role=%q, want id=%q role=%q", id, role, tt.id, tt.role)
				}
			}
		})
	}
}

func TestCmdValidateMarksNeedsReview(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Test Task", Template: "backend_endpoint", Role: "builder", Artifact: "spec:test", DoD: pt.DefinitionOfDone{Manual: "verify output", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	// Move to in_progress before validate
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}

	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("cmdValidate failed: %v", err)
	}
	store2 := pt.NewStoreClient(path, "pt")
	issue, _, _ := store2.GetTask(t.Context(), "pt-1")
	if issue.Status != "needs_review" {
		t.Fatalf("expected needs_review, got %s", issue.Status)
	}
}

func TestCmdValidateManualYesSkipsPrompt(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Test Task 2", Template: "backend_endpoint", Role: "builder", Artifact: "spec:test2", DoD: pt.DefinitionOfDone{Manual: "Step one\nStep two", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}
	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("cmdValidate failed: %v", err)
	}
	store2 := pt.NewStoreClient(path, "pt")
	issue, _, _ := store2.GetTask(t.Context(), "pt-1")
	if issue.Status != "needs_review" {
		t.Fatalf("expected needs_review, got %s", issue.Status)
	}
}

func TestCmdValidateFlagOrdering(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Test Task", Template: "backend_endpoint", Role: "builder", Artifact: "spec:test", DoD: pt.DefinitionOfDone{Manual: "verify output", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	// Move to in_progress before validate
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}

	// Flags before ID (baseline)
	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("cmdValidate (--yes pt-1) failed: %v", err)
	}

	// Reset task back to in_progress
	store2 := pt.NewStoreClient(path, "pt")
	if err := store2.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update2 err: %v", err)
	}

	// Flags after ID (target behavior)
	if err := cmdValidate([]string{"pt-1", "--yes"}); err != nil {
		t.Fatalf("cmdValidate (pt-1 --yes) failed: %v", err)
	}

	store3 := pt.NewStoreClient(path, "pt")
	issue, _, _ := store3.GetTask(t.Context(), "pt-1")
	if issue.Status != "needs_review" {
		t.Fatalf("expected needs_review, got %s", issue.Status)
	}
}

func TestCmdContextErrors(t *testing.T) {
	if err := cmdContext([]string{}); err == nil {
		t.Error("expected error for missing subcommand")
	}
	if err := cmdContext([]string{"unknown"}); err == nil {
		t.Error("expected error for unknown subcommand")
	}
}

func TestCmdContextHelp(t *testing.T) {
	if err := cmdContext([]string{"--help"}); err != nil {
		t.Fatalf("expected no error for context --help, got: %v", err)
	}
	if err := cmdContext([]string{"help"}); err != nil {
		t.Fatalf("expected no error for context help, got: %v", err)
	}
}

func TestCmdContextPrime(t *testing.T) {
	_, _ = setupStoreEnv(t)

	// Add some tasks
	if err := cmdAdd([]string{"Task A", "--role", "dev", "--template", "backend_endpoint", "--artifact", "spec:a", "--manual", "check", "--tests", "echo ok", "--criteria", "verified"}); err != nil {
		t.Fatalf("add task A: %v", err)
	}
	if err := cmdAdd([]string{"Task B", "--role", "dev", "--template", "backend_endpoint", "--artifact", "spec:b", "--manual", "check", "--tests", "echo ok", "--criteria", "verified"}); err != nil {
		t.Fatalf("add task B: %v", err)
	}

	// Claim one task
	if err := cmdClaim([]string{"--as", "alice", "pt-1"}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Test context prime runs without error
	if err := cmdContextPrime([]string{}); err != nil {
		t.Fatalf("context prime: %v", err)
	}

	// Test JSON output
	if err := cmdContextPrime([]string{"--json"}); err != nil {
		t.Fatalf("context prime --json: %v", err)
	}
}

func TestCmdContextInitErrors(t *testing.T) {
	if err := cmdContextInit([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
}

func TestCmdContextValidateErrors(t *testing.T) {
	if err := cmdContextValidate([]string{}); err == nil {
		t.Error("expected error for missing file")
	}
	// Create dummy file
	f, _ := os.CreateTemp(t.TempDir(), "ctx.json")
	f.Close()

	// Missing contract flag
	if err := cmdContextValidate([]string{f.Name()}); err == nil {
		t.Error("expected error for missing contract flag or role")
	}
}

func TestCmdContextValidateDefaultsContractFromRole(t *testing.T) {
	tmp := t.TempDir()
	payloadPath := fmt.Sprintf("%s/payload.json", tmp)
	now := time.Now().UTC().Format(time.RFC3339)
	// Top-level role field should drive contract path selection.
	payload := `{
  "role":"builder",
  "goal":{"prompt":"write a detailed useful implementation that exceeds twenty chars"},
  "scope":{"files":["main.go"]},
  "success":{"criteria":["go test ./..."]},
  "provenance":{"inputs":[{"field":"goal.prompt","source":"pt:123"}],"issued_at":"` + now + `"}
}`
	if err := os.WriteFile(payloadPath, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}
	// Should succeed using contracts/builder.toml
	if err := cmdContextValidate([]string{payloadPath}); err != nil {
		t.Fatalf("expected success with inferred contract, got %v", err)
	}
}

func TestCmdContextInitEmitsRoleAndValidateWorksWithoutContractFlag(t *testing.T) {
	t.Setenv("PT_WORKFLOW", "")
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Ctx Task", Template: "backend_endpoint", Role: "builder", Artifact: "spec:ctx", DoD: pt.DefinitionOfDone{Manual: "verify", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Capture stdout from context init.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := cmdContextInit([]string{"pt-1"}); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("context init err: %v", err)
	}

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	payload := buf.Bytes()

	if !bytes.Contains(payload, []byte(`"role": "builder"`)) {
		t.Fatalf("expected role to be present in payload, got:\n%s", buf.String())
	}

	// Validate without --contract (role should allow inference).
	ctxPath := filepath.Join(t.TempDir(), "context.json")
	if err := os.WriteFile(ctxPath, payload, 0644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := cmdContextValidate([]string{ctxPath}); err != nil {
		t.Fatalf("expected validate success without --contract, got: %v\npayload=%s", err, buf.String())
	}
}
