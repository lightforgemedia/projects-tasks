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

func TestCmdSyncStore(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := t.TempDir() + "/manifest.json"
	data := `{"title":"T","tasks":[{"title":"A","role":"dev","template":"backend_endpoint","artifact":"spec:a","dod":{"manual":"check","tests":["go test ./..."],"criteria":["observed go test pass"]}},{"title":"B","role":"dev","template":"backend_endpoint","artifact":"spec:b","deps":["A"],"dod":{"manual":"check","tests":["go test ./..."],"criteria":["observed go test pass"]}}]}`
	if err := os.WriteFile(manifest, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSync([]string{manifest}); err != nil {
		t.Fatalf("cmdSync err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issues, err := store.Ready(t.Context(), "dev", 10)
	if err != nil || len(issues) != 2 {
		t.Fatalf("expected two open issues, got %d err=%v", len(issues), err)
	}
}

func TestCmdClaimAndReleaseStore(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := cmdClaim([]string{"--as", "bob", "pt-1"}); err != nil {
		t.Fatalf("claim err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ := store.GetTask(t.Context(), "pt-1")
	if issue.Status != "in_progress" || issue.Assignee != "bob" {
		t.Fatalf("claim state mismatch: %+v", issue)
	}
	if err := cmdRelease([]string{"--as", "bob", "pt-1"}); err != nil {
		t.Fatalf("release err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ = store.GetTask(t.Context(), "pt-1")
	if issue.Status != "open" {
		t.Fatalf("expected open after release, got %s", issue.Status)
	}
}

func TestCmdValidateStoresComment(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}
	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("validate err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	if len(store.CommentsFor("pt-1")) == 0 {
		t.Fatalf("expected comment stored")
	}
}

func TestCmdAddCommentListSnapshot(t *testing.T) {
	path, store := setupStoreEnv(t)
	if err := cmdAdd([]string{"New Task", "--role", "dev", "--template", "backend_endpoint", "--artifact", "spec:new", "--manual", "check", "--tests", "echo ok", "--criteria", "observed ok", "--no-handoff-seed"}); err != nil {
		t.Fatalf("add err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	iss, _, err := store.GetTask(t.Context(), "pt-1")
	if err != nil || iss.Title != "New Task" {
		t.Fatalf("task not created: %+v err=%v", iss, err)
	}
	if err := cmdComment([]string{"pt-1", "note"}); err != nil {
		t.Fatalf("comment err: %v", err)
	}
	store = pt.NewStoreClient(path, "pt")
	if len(store.CommentsFor("pt-1")) == 0 {
		t.Fatalf("expected comment stored")
	}
	if err := cmdList([]string{"--status=open"}); err != nil {
		t.Fatalf("list err: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "snap.json")
	if err := cmdSnapshot([]string{"--out", outPath}); err != nil {
		t.Fatalf("snapshot err: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
}

func TestCmdClaimWithDraft(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "DraftTask", Template: "backend_endpoint", Role: "dev", Artifact: "spec:draft", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"observed ok"}}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Claim with --draft flag
	if err := cmdClaim([]string{"--as", "alice", "--draft", "pt-1"}); err != nil {
		t.Fatalf("claim --draft err: %v", err)
	}

	// Verify state:draft label is present
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ := store.GetTask(t.Context(), "pt-1")
	hasDraft := false
	for _, l := range issue.Labels {
		if l == "state:draft" {
			hasDraft = true
			break
		}
	}
	if !hasDraft {
		t.Fatalf("expected state:draft label after claim --draft, got labels: %v", issue.Labels)
	}
}

func TestValidateWorktree(t *testing.T) {
	path, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{{Title: "WorktreeValidate", Template: "backend_endpoint", Role: "dev", Artifact: "spec:wt", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"pwd"}, Criteria: []string{"runs in worktree"}}}},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Claim task
	if err := store.UpdateIssue(t.Context(), "pt-1", "in_progress", "bob"); err != nil {
		t.Fatalf("claim err: %v", err)
	}

	// Create a worktree directory (simulated, no git)
	worktreeDir := t.TempDir()
	wtInfo := pt.WorktreeInfo{
		Path:      worktreeDir,
		Branch:    "test-branch",
		CreatedAt: "2025-01-01T00:00:00Z",
	}
	if err := store.SetWorktree(t.Context(), "pt-1", wtInfo); err != nil {
		t.Fatalf("set worktree err: %v", err)
	}

	// Reload client for test
	_ = pt.NewStoreClient(path, "pt")

	// Run validate - should detect worktree and change to it
	if err := cmdValidate([]string{"--yes", "pt-1"}); err != nil {
		t.Fatalf("validate with worktree err: %v", err)
	}

	// Verify task is now needs_review
	store = pt.NewStoreClient(path, "pt")
	issue, _, _ := store.GetTask(t.Context(), "pt-1")
	if issue.Status != "needs_review" {
		t.Fatalf("expected needs_review after validate, got %s", issue.Status)
	}
}

func TestSyncGeneratesReviews(t *testing.T) {
	path, _ := setupStoreEnv(t)
	manifest := t.TempDir() + "/manifest.json"
	data := `{
		"title":"ReviewTest",
		"tasks":[
			{"title":"TaskA","role":"dev","template":"backend_endpoint","artifact":"spec:a",
			 "dod":{"manual":"check","tests":["echo ok"],"criteria":["observed ok"]}}
		]
	}`
	if err := os.WriteFile(manifest, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	// Sync with --generate-reviews
	if err := cmdSync([]string{"--generate-reviews", manifest}); err != nil {
		t.Fatalf("cmdSync with --generate-reviews: %v", err)
	}

	store := pt.NewStoreClient(path, "pt")

	// Find the implementation task
	implIssue, _, err := store.GetTask(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("get impl task: %v", err)
	}
	if implIssue.Title != "TaskA" {
		t.Fatalf("expected TaskA, got %s", implIssue.Title)
	}

	// Find the review task
	reviewIssue, _, err := store.GetTask(t.Context(), "pt-2")
	if err != nil {
		t.Fatalf("get review task: %v", err)
	}
	if reviewIssue.Title != "[Review] TaskA" {
		t.Fatalf("expected [Review] TaskA, got %s", reviewIssue.Title)
	}

	// Verify review task has role=planner
	meta, err := pt.ParseTaskMeta(reviewIssue.Description)
	if err != nil {
		t.Fatalf("parse review meta: %v", err)
	}
	if meta.Role != "planner" {
		t.Errorf("expected role planner, got %s", meta.Role)
	}

	// Verify impl depends on review
	deps, _ := store.Dependencies(t.Context(), "pt-1")
	found := false
	for _, d := range deps {
		if d.ID == "pt-2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("implementation task should depend on review task")
	}
}

func TestContextPrimeAgent(t *testing.T) {
	_, _ = setupStoreEnv(t)

	// Create a manifest with project metadata
	manifestDir := t.TempDir()
	manifestContent := `
title = "Test Project"

[project]
summary = "A test project for context prime"
structure = ["cmd", "pkg", "docs"]

[[tasks]]
template = "backend_endpoint"
title = "Task With Context"
role = "dev"
artifact = "code:feature.go"
context = "This task adds a new feature"
inputs = ["pkg/feature.go"]
scope = "IN: add feature. OUT: refactor"
[tasks.dod]
tests = ["echo ok"]
manual = "verify"
criteria = ["works"]
`
	manifestPath := filepath.Join(manifestDir, "manifest.toml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Sync the manifest
	if err := cmdSync([]string{manifestPath}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Run context prime with manifest - capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdContextPrime([]string{"--manifest=" + manifestPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("context prime: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify project summary is shown
	if !strings.Contains(output, "A test project for context prime") {
		t.Error("expected project summary in output")
	}

	// Verify structure is shown
	if !strings.Contains(output, "cmd, pkg, docs") {
		t.Error("expected project structure in output")
	}

	// Verify task context is shown for ready tasks
	if !strings.Contains(output, "This task adds a new feature") {
		t.Error("expected task context in output")
	}

	// Verify inputs are shown
	if !strings.Contains(output, "pkg/feature.go") {
		t.Error("expected task inputs in output")
	}
}

func TestHelpTaskAuthoring(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdHelp([]string{"task-authoring"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("help task-authoring: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify key sections are present
	if !strings.Contains(output, "# Task Authoring Guide") {
		t.Error("expected Task Authoring Guide header")
	}
	if !strings.Contains(output, "Required Fields") {
		t.Error("expected Required Fields section")
	}
	if !strings.Contains(output, "Handoff Fields") {
		t.Error("expected Handoff Fields section")
	}
	if !strings.Contains(output, "Good vs Bad Examples") {
		t.Error("expected examples section")
	}
	if !strings.Contains(output, "Task Template") {
		t.Error("expected template section")
	}
	if !strings.Contains(output, "context") && !strings.Contains(output, "inputs") {
		t.Error("expected handoff field documentation")
	}
}

func TestCmdApproveShowsOnlyReadyWork(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", Deps: []string{"A"}, DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "C", Template: "backend_endpoint", Role: "dev", Artifact: "spec:c", Deps: []string{"B"}, DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}
	if err := store.UpdateIssue(t.Context(), "pt-1", "needs_review", ""); err != nil {
		t.Fatalf("set needs_review err: %v", err)
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdApprove([]string{"pt-1"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("approve err: %v", err)
	}
	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "Ready Work:") {
		t.Fatalf("expected Ready Work header, got:\n%s", out)
	}
	if !strings.Contains(out, "pt-2") {
		t.Fatalf("expected pt-2 to be suggested as ready work, got:\n%s", out)
	}
	if strings.Contains(out, "pt-3") {
		t.Fatalf("did not expect pt-3 (still blocked) in ready work, got:\n%s", out)
	}
}

func TestContextPrimeReportsOverriddenDBPath(t *testing.T) {
	td := t.TempDir()
	dbPath := filepath.Join(td, "override.db.json")
	store := pt.NewStoreClient(dbPath, "pt")
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Capture stdout (JSON output)
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdContextPrime([]string{"--db", dbPath, "--json"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("context prime err: %v", err)
	}
	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, `"store_path":`) {
		t.Fatalf("expected store_path field, got:\n%s", out)
	}
	if !strings.Contains(out, dbPath) {
		t.Fatalf("expected store_path to include overridden db path %q, got:\n%s", dbPath, out)
	}
}

func TestPorcelainListOutput(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task One", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
			{Title: "Task Two", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdList([]string{"--porcelain"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("list --porcelain err: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	// Should be TSV format: id\tstatus\tassignee\ttitle
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), out)
	}

	// Check first line has correct TSV format
	parts := strings.Split(lines[0], "\t")
	if len(parts) != 4 {
		t.Fatalf("expected 4 TSV columns, got %d: %s", len(parts), lines[0])
	}
	if parts[1] != "open" {
		t.Fatalf("expected status 'open', got %s", parts[1])
	}
	if parts[2] != "-" {
		t.Fatalf("expected unassigned '-', got %s", parts[2])
	}
}

func TestPorcelainShowOutput(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task One", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdShow([]string{"--porcelain", "pt-1"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("show --porcelain err: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	// Should be JSON
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected JSON output starting with '{', got: %s", out)
	}
	if !strings.Contains(out, `"id":`) {
		t.Fatalf("expected 'id' field in JSON, got: %s", out)
	}
	if !strings.Contains(out, `"Task One"`) {
		t.Fatalf("expected title in JSON, got: %s", out)
	}
}

func TestShellCdNoWorktree(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task One", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Should fail - no worktree exists
	err := cmdCd([]string{"pt-1"})
	if err == nil {
		t.Fatal("expected error when no worktree exists")
	}
	if !strings.Contains(err.Error(), "no worktree") {
		t.Fatalf("expected 'no worktree' error, got: %v", err)
	}
}

func TestShellCdWithWorktree(t *testing.T) {
	_, store := setupStoreEnv(t)
	manifest := pt.Manifest{
		Tasks: []pt.Task{
			{Title: "Task One", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"ok"}}},
		},
	}
	if _, err := store.Sync(t.Context(), manifest); err != nil {
		t.Fatalf("sync err: %v", err)
	}

	// Manually set a worktree
	wtPath := filepath.Join(t.TempDir(), "worktree-test")
	if err := store.SetWorktree(t.Context(), "pt-1", pt.WorktreeInfo{
		TaskID:    "pt-1",
		Path:      wtPath,
		Branch:    "test-branch",
		CreatedAt: "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("set worktree err: %v", err)
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdCd([]string{"pt-1"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cd err: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := strings.TrimSpace(buf.String())

	if out != wtPath {
		t.Fatalf("expected path %q, got %q", wtPath, out)
	}
}

func TestShellEnv(t *testing.T) {
	setupStoreEnv(t)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmdEnv([]string{})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("env err: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "PT_DB=") {
		t.Fatalf("expected PT_DB in output, got: %s", out)
	}
	if !strings.Contains(out, "export PT_DB") {
		t.Fatalf("expected 'export PT_DB' in output, got: %s", out)
	}
}

func TestHelpOutput(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	usage()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	// UC1: New user can quickly see available commands
	// Check that QUICK START section exists and comes first
	if !strings.Contains(out, "QUICK START") {
		t.Error("help should have QUICK START section")
	}
	quickStartIdx := strings.Index(out, "QUICK START")
	commonWorkflowIdx := strings.Index(out, "COMMON WORKFLOW")
	if quickStartIdx > commonWorkflowIdx {
		t.Error("QUICK START should come before COMMON WORKFLOW")
	}

	// Check essential commands are visible
	essentialCmds := []string{"sync", "ready", "claim", "validate", "approve"}
	for _, cmd := range essentialCmds {
		if !strings.Contains(out, cmd) {
			t.Errorf("help should mention %q command", cmd)
		}
	}

	// UC2: Power user can find specific command details
	// Check that sections are organized logically
	sections := []string{"VIEW & SEARCH", "CREATE & UPDATE", "WORKFLOW GUIDANCE", "WORKTREES", "ADVANCED"}
	for _, section := range sections {
		if !strings.Contains(out, section) {
			t.Errorf("help should have %q section", section)
		}
	}
}

func TestCheckpointLabelDetection(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   bool
	}{
		{"no labels", []string{}, false},
		{"other labels", []string{"role:dev", "template:bug_fix"}, false},
		{"checkpoint required", []string{"role:dev", "checkpoint:required"}, true},
		{"checkpoint shorthand", []string{"checkpoint"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iss := pt.Issue{Labels: tc.labels}
			got := hasCheckpointLabel(iss)
			if got != tc.want {
				t.Errorf("hasCheckpointLabel(%v) = %v, want %v", tc.labels, got, tc.want)
			}
		})
	}
}

func TestMockCommands(t *testing.T) {
	path, store := setupStoreEnv(t)
	ctx := t.Context()

	// Create a spike task
	spikeManifest := pt.Manifest{
		Tasks: []pt.Task{{
			Title:    "Spike: API Options",
			Template: "spike",
			Role:     "dev",
			Artifact: "doc:findings.md",
			MaxHours: 2,
			DoD:      pt.DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"done"}},
		}},
	}
	if _, err := store.Sync(ctx, spikeManifest); err != nil {
		t.Fatalf("sync spike: %v", err)
	}

	// Create an integration task
	integrationManifest := pt.Manifest{
		Tasks: []pt.Task{{
			Title:    "Implement real API",
			Template: "backend_endpoint",
			Role:     "dev",
			Artifact: "code:api.go",
			DoD:      pt.DefinitionOfDone{Manual: "check", Tests: []string{"go test"}, Criteria: []string{"api works"}},
		}},
	}
	if _, err := store.Sync(ctx, integrationManifest); err != nil {
		t.Fatalf("sync integration: %v", err)
	}

	// Register a mock
	mock, err := store.RegisterMock(ctx, "Fake API client", "pkg/api/client.go:42", "pt-1", "pt-2")
	if err != nil {
		t.Fatalf("register mock: %v", err)
	}
	if mock.ID == "" {
		t.Error("mock ID should not be empty")
	}

	// List mocks
	mocks := store.ListMocks(ctx, false)
	if len(mocks) != 1 {
		t.Errorf("expected 1 mock, got %d", len(mocks))
	}

	// Check for orphans (integration task is not closed)
	orphans := store.CheckMocks(ctx)
	if len(orphans) != 1 {
		t.Errorf("expected 1 orphan, got %d", len(orphans))
	}

	// Close the integration task
	store = pt.NewStoreClient(path, "pt")
	if err := store.UpdateIssue(ctx, "pt-2", "in_progress", "tester"); err != nil {
		t.Fatalf("claim integration: %v", err)
	}
	if err := store.UpdateIssue(ctx, "pt-2", "needs_review", "tester"); err != nil {
		t.Fatalf("transition to review: %v", err)
	}
	if err := store.UpdateIssue(ctx, "pt-2", "closed", "tester"); err != nil {
		t.Fatalf("transition to closed: %v", err)
	}

	// Check again - should have no orphans now
	orphans = store.CheckMocks(ctx)
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphans after closing integration, got %d", len(orphans))
	}

	// Retire the mock
	if err := store.RetireMock(ctx, mock.ID); err != nil {
		t.Fatalf("retire mock: %v", err)
	}

	// List should be empty (without --all)
	mocks = store.ListMocks(ctx, false)
	if len(mocks) != 0 {
		t.Errorf("expected 0 active mocks after retire, got %d", len(mocks))
	}

	// List with --all should show retired
	mocks = store.ListMocks(ctx, true)
	if len(mocks) != 1 {
		t.Errorf("expected 1 mock with --all, got %d", len(mocks))
	}
}
