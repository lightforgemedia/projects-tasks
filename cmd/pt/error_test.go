package main

import (
	"errors"
	"os"
	"testing"
)

func TestCmdSyncErrors(t *testing.T) {
	// 1. Missing Argument
	if err := cmdSync([]string{}); err == nil {
		t.Error("expected error for missing manifest argument")
	}

	// 2. Invalid Manifest Path
	if err := cmdSync([]string{"nonexistent.json"}); err == nil {
		t.Error("expected error for invalid manifest path")
	}

	// 3. BD Failure
	// Need a manifest file first
	// To avoid disk I/O dependency in error test if possible, but ParseManifest reads file.
	// So create a dummy file.
	f, _ := os.CreateTemp(t.TempDir(), "*.json")
	f.Write([]byte(`{"title":"T","tasks":[]}`))
	f.Close()

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			// sync calls ensureIssue -> findIssueByTitle (if tasks exist)
			// But here tasks is empty, so it might just return.
			// Let's add a task to trigger bd call.
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()
	// For empty tasks, it returns nil.

	// Let's try a task that fails creation.
	f2, _ := os.CreateTemp(t.TempDir(), "*.json")
	f2.Write([]byte(`{"title":"T","tasks":[{"title":"A","role":"dev","template":"refactor","dod":{"manual":"check"}}]}`))
	f2.Close()

	runner = &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "list", "--json", "--title", "A", "--limit", "1"}, err: errors.New("bd failed")},
		},
	}
	bdRunner = runner
	if err := cmdSync([]string{f2.Name()}); err == nil {
		t.Error("expected error when bd fails")
	}
}

func TestCmdClaimErrors(t *testing.T) {
	if err := cmdClaim([]string{}); err == nil {
		t.Error("expected error for missing id")
	}

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "ID", "--json"}, err: errors.New("bd failed")},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	if err := cmdClaim([]string{"ID"}); err == nil {
		t.Error("expected error when bd fails")
	}
}

func TestCmdReleaseErrors(t *testing.T) {
	if err := cmdRelease([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
    
    // Test Transitioner.Release failure (e.g. GetTask fails)
    runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "ID", "--json"}, err: errors.New("bd failed")},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()
    
    if err := cmdRelease([]string{"ID"}); err == nil {
		t.Error("expected error when bd fails")
	}
}

func TestCmdRejectErrors(t *testing.T) {
	if err := cmdReject([]string{}); err == nil {
		t.Error("expected error for missing id")
	}

	if err := cmdReject([]string{"ID"}); err == nil {
		t.Error("expected error for missing reason")
	}
    
    // Test Transitioner.Reject failure
    runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "ID", "--json"}, err: errors.New("bd failed")},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()
    
    if err := cmdReject([]string{"ID", "--reason", "bad"}); err == nil {
		t.Error("expected error when bd fails")
	}
}

func TestCmdValidateErrors(t *testing.T) {
	if err := cmdValidate([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
	
	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "ID", "--json"}, err: errors.New("bd failed")},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	if err := cmdValidate([]string{"ID"}); err == nil {
		t.Error("expected error when bd fails")
	}
}

func TestCmdApproveErrors(t *testing.T) {
	if err := cmdApprove([]string{}); err == nil {
		t.Error("expected error for missing id")
	}

	runner := &testRunner{
		t: t,
		steps: []testRunnerStep{
			{expect: []string{"bd", "show", "ID", "--json"}, err: errors.New("bd failed")},
		},
	}
	bdRunner = runner
	defer func() { bdRunner = nil }()

	if err := cmdApprove([]string{"ID"}); err == nil {
		t.Error("expected error when bd fails")
	}
}
