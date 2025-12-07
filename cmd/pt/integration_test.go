package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestCmdSync(t *testing.T) {
	// Create temp manifest
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifestContent := `{
  "title": "Integration Test",
  "owner": "test",
  "tasks": [
    {
      "title": "Task 1",
      "role": "dev",
      "template": "backend_endpoint",
      "dod": { "manual": "check" }
    },
    {
      "title": "Task 2",
      "role": "dev",
      "template": "backend_endpoint",
      "deps": ["Task 1"],
      "dod": { "manual": "check" }
    }
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Expected meta JSON (compact, sorted keys)
	metaJSON := `{"template":"backend_endpoint","role":"dev","dod":{"manual":"check"}}`
	desc := fmt.Sprintf("Template: backend_endpoint\nRole: dev\nDoD: manual=check\n<!-- pt-meta: %s -->", metaJSON)

	// Mock responses
	// 1. findIssueByTitle (Task 1) -> empty (create)
	// 2. create Task 1
	// 3. findIssueByTitle (Task 2) -> empty (create)
	// 4. create Task 2
	// 5. dep add Task 2 -> Task 1

	// NOTE: The order of ensuring issues is iteration order of manifest.Tasks (slice), so it's deterministic.
	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			// Task 1
			{expect: []string{"bd", "list", "--json", "--title", "Task 1", "--limit", "1"}, out: "[]"},
			{expect: []string{"bd", "create", "--type", "task", "--title", "Task 1", "--labels", "role:dev,template:backend_endpoint", "--description", desc}, out: "Created issue: ISSUE-1"},
			// Task 2
			{expect: []string{"bd", "list", "--json", "--title", "Task 2", "--limit", "1"}, out: "[]"},
			{expect: []string{"bd", "create", "--type", "task", "--title", "Task 2", "--labels", "role:dev,template:backend_endpoint", "--description", desc}, out: "Created issue: ISSUE-2"},
			// Deps: Task 2 -> Task 1
			{expect: []string{"bd", "dep", "add", "ISSUE-2", "ISSUE-1", "--type", "blocks"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdSync([]string{manifestPath})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdReady(t *testing.T) {
	// Mock output for bd ready
	issues := []pt.Issue{
		{ID: "ISSUE-1", Title: "Task 1", Status: "open", IssueType: "task"},
		{ID: "ISSUE-2", Title: "Task 2", Status: "in_progress", IssueType: "task"}, // Should be filtered out by cmdReady logic
	}
	outBytes, _ := json.Marshal(issues)

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "ready", "--json", "--label", "role:dev", "--limit", "5"}, out: string(outBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: "[]"},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	// Capture stdout?
	// cmdReady prints to stdout. For now we just ensure it runs without error and calls the command.
	cmdReady([]string{"--role=dev", "--limit=5"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdReadyVerboseShowsDeps(t *testing.T) {
	issues := []pt.Issue{
		{ID: "ISSUE-1", Title: "Task 1", Status: "open", IssueType: "task"},
	}
	outBytes, _ := json.Marshal(issues)

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "ready", "--json", "--limit", "10"}, out: string(outBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: `[{"id":"dep1","status":"open"}]`},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdReady([]string{"--verbose"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdClaim(t *testing.T) {
	t.Setenv("USER", "testuser")

	// Prepare issue data for GetTask
	meta := pt.TaskMeta{Role: "dev", DoD: pt.DefinitionOfDone{}}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "ISSUE-1",
		Title:       "Task 1",
		Status:      "open",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	issueBytes, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			// GetTask
			{expect: []string{"bd", "show", "ISSUE-1", "--json"}, out: string(issueBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: "[]"},
			// UpdateIssue (status/assignee)
			{expect: []string{"bd", "update", "ISSUE-1", "--status", "in_progress", "--assignee", "testuser"}, out: ""},
			// AddLabels
			{expect: []string{"bd", "update", "ISSUE-1", "--add-label", "state:claimed"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdClaim([]string{"ISSUE-1"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdClaimOverrideAssignee(t *testing.T) {
	meta := pt.TaskMeta{Role: "dev", DoD: pt.DefinitionOfDone{}}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "ISSUE-2",
		Title:       "Task 2",
		Status:      "open",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	issueBytes, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "ISSUE-2", "--json"}, out: string(issueBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-2", "--json"}, out: "[]"},
			{expect: []string{"bd", "update", "ISSUE-2", "--status", "in_progress", "--assignee", "override"}, out: ""},
			{expect: []string{"bd", "update", "ISSUE-2", "--add-label", "state:claimed"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdClaim([]string{"--as", "override", "ISSUE-2"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdRelease(t *testing.T) {
	t.Setenv("USER", "testuser")

	// Prepare issue data
	meta := pt.TaskMeta{Role: "dev", DoD: pt.DefinitionOfDone{}}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "ISSUE-1",
		Title:       "Task 1",
		Status:      "in_progress",
		Assignee:    "testuser",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	issueBytes, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			// GetTask
			{expect: []string{"bd", "show", "ISSUE-1", "--json"}, out: string(issueBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: "[]"},
			// UpdateIssue (status open, unassign)
			// cmdRelease passes "open" and empty assignee (implied by not passing it? No, wait)
			// UpdateIssue signature: (ctx, id, status, assignee)
			// In cmdRelease: trans.Release calls client.UpdateIssue(ctx, issue.ID, "open", "")
			// client.UpdateIssue checks if assignee != "" to append flag.
			{expect: []string{"bd", "update", "ISSUE-1", "--status", "open"}, out: ""},
			// RemoveLabels
			{expect: []string{"bd", "update", "ISSUE-1", "--remove-label", "state:claimed", "--remove-label", "state:needs_review"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdRelease([]string{"ISSUE-1"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdApprove(t *testing.T) {
	// Prepare issue data
	meta := pt.TaskMeta{Role: "dev", DoD: pt.DefinitionOfDone{}}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "ISSUE-1",
		Title:       "Task 1",
		Status:      "needs_review",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	issueBytes, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			// GetTask (Approve)
			{expect: []string{"bd", "show", "ISSUE-1", "--json"}, out: string(issueBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: "[]"},
			// UpdateIssue (closed)
			{expect: []string{"bd", "update", "ISSUE-1", "--status", "closed"}, out: ""},
			// RemoveLabels
			{expect: []string{"bd", "update", "ISSUE-1", "--remove-label", "state:needs_review", "--remove-label", "state:claimed"}, out: ""},
			// AddComment
			{expect: []string{"bd", "comment", "ISSUE-1", "Approved and closed"}, out: ""},
			// Ready (show next tasks)
			{expect: []string{"bd", "ready", "--json", "--limit", "5"}, out: "[]"},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdApprove([]string{"ISSUE-1"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}

func TestCmdReject(t *testing.T) {
	// Prepare issue data
	meta := pt.TaskMeta{Role: "dev", DoD: pt.DefinitionOfDone{}}
	metaBytes, _ := json.Marshal(meta)
	issue := pt.Issue{
		ID:          "ISSUE-1",
		Title:       "Task 1",
		Status:      "needs_review",
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	issueBytes, _ := json.Marshal([]pt.Issue{issue})

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			// GetTask
			{expect: []string{"bd", "show", "ISSUE-1", "--json"}, out: string(issueBytes)},
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: "[]"},
			// UpdateIssue (in_progress)
			{expect: []string{"bd", "update", "ISSUE-1", "--status", "in_progress"}, out: ""},
			// RemoveLabels
			{expect: []string{"bd", "update", "ISSUE-1", "--remove-label", "state:needs_review"}, out: ""},
			// AddComment
			{expect: []string{"bd", "comment", "ISSUE-1", "Rejected: bad code"}, out: ""},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	cmdReject([]string{"--reason", "bad code", "ISSUE-1"})

	if runner.cursor != len(runner.steps) {
		t.Fatalf("not all commands consumed, got %d want %d", runner.cursor, len(runner.steps))
	}
}
