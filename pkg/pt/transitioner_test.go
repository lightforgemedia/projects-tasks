package pt

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func issueWithStatus(id, status string, meta TaskMeta) string {
	metaBytes, _ := json.Marshal(meta)
	issue := Issue{
		ID:          id,
		Title:       "T",
		Status:      status,
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	payload, _ := json.Marshal([]Issue{issue})
	return string(payload)
}

func TestTransitionerClaimBlocksInvalidState(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "needs_review", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "[]"},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Claim(ctx, "proj-1", "alice"); err == nil {
		t.Fatalf("expected error when claiming needs_review")
	}
	if runner.current != len(runner.script) {
		t.Fatalf("unexpected commands executed: %d", runner.current)
	}
}

func TestTransitionerClaimHappyPath(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "open", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: `[{"id":"dep-1","status":"closed"}]`},
			{expect: []string{"bd", "update", "proj-1", "--status", "in_progress", "--assignee", "alice"}, out: ""},
			{expect: []string{"bd", "update", "proj-1", "--add-label", "state:claimed"}, out: ""},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Claim(ctx, "proj-1", "alice"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if runner.current != len(runner.script) {
		t.Fatalf("not all commands executed")
	}
}

func TestTransitionerClaimBlockedByDeps(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "open", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: `[{"id":"dep-1","status":"open"}]`},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Claim(ctx, "proj-1", "alice"); err == nil {
		t.Fatalf("expected blocked claim due to open dependency")
	}
	if runner.current != len(runner.script) {
		t.Fatalf("unexpected commands executed: %d", runner.current)
	}
}

func TestTransitionerClaimIgnoresBadDepJSON(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "open", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "Malformed dep response"},
			{expect: []string{"bd", "update", "proj-1", "--status", "in_progress", "--assignee", "alice"}, out: ""},
			{expect: []string{"bd", "update", "proj-1", "--add-label", "state:claimed"}, out: ""},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Claim(ctx, "proj-1", "alice"); err != nil {
		t.Fatalf("claim should ignore bad dep json: %v", err)
	}
	if runner.current != len(runner.script) {
		t.Fatalf("unexpected commands executed: %d", runner.current)
	}
}

func issueWithStatusAndAssignee(id, status, assignee string, meta TaskMeta) string {
	metaBytes, _ := json.Marshal(meta)
	issue := Issue{
		ID:          id,
		Title:       "T",
		Status:      status,
		Assignee:    assignee,
		Description: fmt.Sprintf("desc\n<!-- pt-meta: %s -->", string(metaBytes)),
	}
	payload, _ := json.Marshal([]Issue{issue})
	return string(payload)
}

func TestTransitionerApproveHappyPath(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "needs_review", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "[]"},
			{expect: []string{"bd", "update", "proj-1", "--status", "closed"}, out: ""},
			{expect: []string{"bd", "update", "proj-1", "--remove-label", "state:needs_review", "--remove-label", "state:claimed"}, out: ""},
			{expect: []string{"bd", "comment", "proj-1", "Approved and closed"}, out: ""},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Approve(ctx, "proj-1"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if runner.current != len(runner.script) {
		t.Fatalf("not all commands executed")
	}
}

func TestTransitionerRejectHappyPath(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "needs_review", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "[]"},
			{expect: []string{"bd", "update", "proj-1", "--status", "in_progress"}, out: ""},
			{expect: []string{"bd", "update", "proj-1", "--remove-label", "state:needs_review"}, out: ""},
			{expect: []string{"bd", "comment", "proj-1", "Rejected: fix it"}, out: ""},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Reject(ctx, "proj-1", "fix it"); err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if runner.current != len(runner.script) {
		t.Fatalf("not all commands executed")
	}
}

func TestTransitionerReleaseHappyPath(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatusAndAssignee("proj-1", "in_progress", "alice", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "[]"},
			{expect: []string{"bd", "update", "proj-1", "--status", "open"}, out: ""},
			{expect: []string{"bd", "update", "proj-1", "--remove-label", "state:claimed", "--remove-label", "state:needs_review"}, out: ""},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Release(ctx, "proj-1", "alice"); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if runner.current != len(runner.script) {
		t.Fatalf("not all commands executed")
	}
}

func TestTransitionerUnknownStatus(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "proj-1", "--json"}, out: issueWithStatus("proj-1", "weird_status", TaskMeta{})},
			{expect: []string{"bd", "dep", "list", "proj-1", "--json"}, out: "[]"},
		},
	}
	trans := Transitioner{Client: NewBDClient(runner)}
	ctx := context.Background()
	if err := trans.Claim(ctx, "proj-1", "alice"); err == nil {
		t.Fatalf("expected error for unknown status")
	}
}
