package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	meta := pt.TaskMeta{
		Role: "builder",
		DoD:  pt.DefinitionOfDone{},
	}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "proj-1",
		Title:       "Test Task",
		Status:      "in_progress",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	payload, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: string(payload)},
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: string(payload)},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "[]"},
			{expect: []string{"bd", "update", "proj-1", "--status", "needs_review"}, out: ""},
			{expect: []string{"bd", "update", "proj-1", "--add-label", "state:needs_review"}, out: ""},
			{expect: []string{"bd", "comment", "proj-1", "Validation passed; ready for review"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	if err := cmdValidate([]string{"proj-1"}); err != nil {
		t.Fatalf("cmdValidate failed: %v", err)
	}

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdValidateManualYesSkipsPrompt(t *testing.T) {
	// Manual steps present; --yes should auto-confirm and include steps in comment.
	meta := pt.TaskMeta{
		Role: "builder",
		DoD:  pt.DefinitionOfDone{Manual: "Step one\nStep two"},
	}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "proj-2",
		Title:       "Test Task",
		Status:      "in_progress",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	payload, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "proj-2", "--json"}, out: string(payload)},
			{expect: []string{"bd", "show", "proj-2", "--json"}, out: string(payload)},
			{expect: []string{"bd", "dep", "list", "proj-2", "--json"}, out: "[]"},
			{expect: []string{"bd", "update", "proj-2", "--status", "needs_review"}, out: ""},
			{expect: []string{"bd", "update", "proj-2", "--add-label", "state:needs_review"}, out: ""},
			{expect: []string{"bd", "comment", "proj-2", "Validation passed; manual steps confirmed: Step one; Step two"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	if err := cmdValidate([]string{"--yes", "proj-2"}); err != nil {
		t.Fatalf("cmdValidate failed: %v", err)
	}
	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
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
  "provenance":{"inputs":[{"field":"goal.prompt","source":"bd:123"}],"issued_at":"` + now + `"}
}`
	if err := os.WriteFile(payloadPath, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}
	// Should succeed using contracts/builder.toml
	if err := cmdContextValidate([]string{payloadPath}); err != nil {
		t.Fatalf("expected success with inferred contract, got %v", err)
	}
}
