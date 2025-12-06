package pt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type scriptedRunner struct {
	t       *testing.T
	script  []scriptStep
	current int
}

type scriptStep struct {
	expect []string
	out    string
	err    error
}

func (r *scriptedRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	if r.current >= len(r.script) {
		r.t.Fatalf("unexpected command: %v", args)
	}
	step := r.script[r.current]
	r.current++
	got := strings.Join(args, " ")
	want := strings.Join(step.expect, " ")
	if got != want {
		r.t.Fatalf("command mismatch at step %d: got %q want %q", r.current, got, want)
	}
	return []byte(step.out), step.err
}

func TestSyncCreatesIssuesAndDeps(t *testing.T) {
	manifest := Manifest{
		Title: "Login",
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", DoD: DefinitionOfDone{Manual: "check"}},
			{Title: "B", Template: "frontend_component", Role: "dev", Deps: []string{"A"}, DoD: DefinitionOfDone{Manual: "check"}},
		},
	}
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "list", "--json", "--title", "A", "--limit", "1"}, out: "[]"},
			{expect: []string{"bd", "create", "--type", "task", "--title", "A", "--labels", "role:dev,template:backend_endpoint", "--description", "Template: backend_endpoint\nRole: dev\nDoD: manual=check\n<!-- pt-meta: {\"template\":\"backend_endpoint\",\"role\":\"dev\",\"dod\":{\"manual\":\"check\"}} -->"}, out: "✓ Created issue: proj-1"},
			{expect: []string{"bd", "list", "--json", "--title", "B", "--limit", "1"}, out: "[]"},
			{expect: []string{"bd", "create", "--type", "task", "--title", "B", "--labels", "role:dev,template:frontend_component", "--description", "Template: frontend_component\nRole: dev\nDoD: manual=check\n<!-- pt-meta: {\"template\":\"frontend_component\",\"role\":\"dev\",\"dod\":{\"manual\":\"check\"}} -->"}, out: "✓ Created issue: proj-2"},
			{expect: []string{"bd", "dep", "add", "proj-2", "proj-1", "--type", "blocks"}, out: ""},
		},
	}

	client := NewBDClient(runner)
	ctx, cancel := ContextWithTimeout(context.Background(), 0)
	defer cancel()

	got, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if got["A"] != "proj-1" || got["B"] != "proj-2" {
		t.Fatalf("unexpected id mapping: %+v", got)
	}
}

func TestSyncIdempotentFindsExisting(t *testing.T) {
	manifest := Manifest{
		Title: "Login",
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", DoD: DefinitionOfDone{Manual: "check"}},
		},
	}
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "list", "--json", "--title", "A", "--limit", "1"}, out: `[{"id":"proj-1","title":"A"}]`},
		},
	}
	client := NewBDClient(runner)
	ctx, cancel := ContextWithTimeout(context.Background(), 0)
	defer cancel()

	got, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if got["A"] != "proj-1" {
		t.Fatalf("expected existing id proj-1, got %+v", got)
	}
}

func TestReadyFiltersByRole(t *testing.T) {
	issues := []Issue{
		{ID: "1", Title: "A", Status: "open"},
		{ID: "2", Title: "B", Status: "open"},
	}
	payload, _ := json.Marshal(issues)
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "ready", "--json", "--label", "role:dev", "--limit", "5"}, out: string(payload)},
		},
	}
	client := NewBDClient(runner)
	ctx, cancel := ContextWithTimeout(context.Background(), 0)
	defer cancel()

	got, err := client.Ready(ctx, "dev", 5)
	if err != nil {
		t.Fatalf("Ready error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "1" {
		t.Fatalf("unexpected issues: %+v", got)
	}
}

func TestDependencyAlreadyExistsIgnored(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "dep", "add", "a", "b", "--type", "blocks"}, out: "already depends", err: errors.New("already depends")},
		},
	}
	client := NewBDClient(runner)
	ctx := context.Background()
	if err := client.addDependency(ctx, "a", "b"); err != nil {
		t.Fatalf("expected already depends to be ignored, got %v", err)
	}
}

func TestExtractIssueID(t *testing.T) {
	id, err := extractIssueID("✓ Created issue: projects-1")
	if err != nil || id != "projects-1" {
		t.Fatalf("unexpected parse %v %v", id, err)
	}
}

func TestUpdateIssueAndComment(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "update", "id1", "--status", "in_progress", "--assignee", "alice"}, out: ""},
			{expect: []string{"bd", "comment", "id1", "Looks good"}, out: ""},
			{expect: []string{"bd", "update", "id1", "--add-label", "state:needs_review", "--add-label", "template:backend_endpoint"}, out: ""},
			{expect: []string{"bd", "update", "id1", "--remove-label", "state:needs_review"}, out: ""},
		},
	}
	client := NewBDClient(runner)
	ctx := context.Background()
	if err := client.UpdateIssue(ctx, "id1", "in_progress", "alice"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := client.AddComment(ctx, "id1", "Looks good"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if err := client.AddLabels(ctx, "id1", "state:needs_review", "template:backend_endpoint"); err != nil {
		t.Fatalf("add labels: %v", err)
	}
	if err := client.RemoveLabels(ctx, "id1", "state:needs_review"); err != nil {
		t.Fatalf("remove labels: %v", err)
	}
}

func TestGetTaskParsesMeta(t *testing.T) {
	meta := TaskMeta{
		Template: "backend_endpoint",
		Role:     "dev",
		DoD: DefinitionOfDone{
			Tests:         []string{"go test ./..."},
			ValidationCmd: "echo ok",
			Manual:        "check",
			OnFailure:     "block",
		},
	}
	metaBytes, _ := json.Marshal(meta)
	desc := fmt.Sprintf("something\n<!-- pt-meta: %s -->", string(metaBytes))
	issue := Issue{
		ID:          "id1",
		Title:       "A",
		Description: desc,
	}
	payload, _ := json.Marshal([]Issue{issue})
	runner := &scriptedRunner{
		t: t,
		script: []scriptStep{
			{expect: []string{"bd", "show", "id1", "--json"}, out: string(payload)},
		},
	}
	client := NewBDClient(runner)
	ctx := context.Background()
	_, gotMeta, err := client.GetTask(ctx, "id1")
	if err != nil {
		t.Fatalf("GetTask err: %v", err)
	}
	if gotMeta.Template != meta.Template || gotMeta.Role != meta.Role || gotMeta.DoD.Manual != meta.DoD.Manual {
		t.Fatalf("metadata mismatch: %+v vs %+v", gotMeta, meta)
	}
}
