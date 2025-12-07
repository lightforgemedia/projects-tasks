package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

type testRunnerStep struct {
	expect []string
	out    string
	err    error
}

type testRunner struct {
	t      *testing.T
	steps  []testRunnerStep
	cursor int
}

func (r *testRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	if r.cursor >= len(r.steps) {
		r.t.Fatalf("unexpected command: %v", args)
	}
	step := r.steps[r.cursor]
	r.cursor++
	got := strings.TrimSpace(fmt.Sprint(args))
	want := strings.TrimSpace(fmt.Sprint(step.expect))
	if got != want {
		r.t.Fatalf("command mismatch at step %d: got %q want %q", r.cursor, got, want)
	}
	return []byte(step.out), step.err
}

func TestReadyBlockers(t *testing.T) {
	depsJSON := `[{"id":"dep1","status":"open"},{"id":"dep2","status":"closed"}]`
	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: depsJSON},
		},
	}
	client := pt.NewBDClient(runner)
	ctx := context.Background()
	blk := readyBlockers(ctx, client, pt.Issue{ID: "ISSUE-1"})
	if len(blk) != 1 || !strings.Contains(blk[0], "dep1") {
		t.Fatalf("expected blocker list, got %+v", blk)
	}
}

func TestReadyBlockersIgnoresBadJSON(t *testing.T) {
	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "dep", "list", "ISSUE-1", "--json"}, out: "Malformed response"},
		},
	}
	client := pt.NewBDClient(runner)
	ctx := context.Background()
	blk := readyBlockers(ctx, client, pt.Issue{ID: "ISSUE-1"})
	if blk != nil {
		t.Fatalf("expected nil blockers on bad JSON, got %+v", blk)
	}
}

func TestSplitManualSteps(t *testing.T) {
	steps := splitManualSteps("first\n\n second ")
	if len(steps) != 2 || steps[0] != "first" || steps[1] != "second" {
		t.Fatalf("unexpected steps: %+v", steps)
	}
}
