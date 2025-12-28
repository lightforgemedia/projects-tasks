package pt

import (
	"context"
	"os"
	"testing"
)

func TestStoreSyncAndReady(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	manifest := Manifest{
		Title: "Test",
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: minimalDoD},
			{Title: "B", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", Deps: []string{"A"}, DoD: minimalDoD},
		},
	}
	ctx := context.Background()
	ids, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	ready, err := client.Ready(ctx, "dev", 10)
	if err != nil {
		t.Fatalf("ready err: %v", err)
	}
	// Ready now returns all open tasks (blocked or not).
	if len(ready) != 2 {
		t.Fatalf("expected 2 open tasks, got %d", len(ready))
	}
	// Mark A done to unblock B (logic check for transitioner, but here just updating state)
	if err := client.UpdateIssue(ctx, ids["A"], "closed", ""); err != nil {
		t.Fatalf("update err: %v", err)
	}
	ready, _ = client.Ready(ctx, "dev", 10)
	// Still 2 tasks? No, A is closed. So only B (open) should be returned.
	if len(ready) != 1 || ready[0].Title != "B" {
		t.Fatalf("expected B ready after A closed, got %+v", ready)
	}
	// Ensure persistence
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("expected db file: %v", err)
	}
}

func TestStoreNextHint(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	manifest := Manifest{
		Title: "Hints",
		Tasks: []Task{
			{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:hint", NextHint: "Do B next", DoD: DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"verified manually"}}},
		},
	}
	ctx := context.Background()
	ids, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	iss, meta, err := client.GetTask(ctx, ids["A"])
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if iss.NextHint != "Do B next" || meta.NextHint != "Do B next" {
		t.Fatalf("next_hint not stored: issue=%q meta=%q", iss.NextHint, meta.NextHint)
	}
}

func TestStoreAddTaskAndList(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()
	id, err := client.AddTask(ctx, Task{
		Title:    "Quick",
		Template: "backend_endpoint",
		Role:     "dev",
		Artifact: "spec:quick",
		DoD:      DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"validated"}},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	issues, err := client.List(ctx, []string{"open"}, "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != id {
		t.Fatalf("list mismatch: %+v", issues)
	}
}

func TestStoreCommentsMethod(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()
	_, _ = client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: DefinitionOfDone{Manual: "check", Tests: []string{"echo ok"}, Criteria: []string{"validated"}}}},
	})
	if err := client.AddComment(ctx, "pt-1", "note"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	comments, err := client.Comments(ctx, "pt-1")
	if err != nil || len(comments) != 1 {
		t.Fatalf("comments: %v %v", comments, err)
	}
}

func TestStoreBlocked(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()
	ids, _ := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: minimalDoD}},
	})
	id := ids["A"]

	// Initially not blocked
	_, blocked, _ := client.GetBlocked(ctx, id)
	if blocked {
		t.Fatalf("expected not blocked initially")
	}

	// Set blocked
	if err := client.SetBlocked(ctx, id, "waiting for API", "alice"); err != nil {
		t.Fatalf("set blocked: %v", err)
	}

	// Verify blocked
	info, blocked, _ := client.GetBlocked(ctx, id)
	if !blocked {
		t.Fatalf("expected blocked")
	}
	if info.Reason != "waiting for API" {
		t.Fatalf("expected reason 'waiting for API', got %q", info.Reason)
	}
	if info.BlockedBy != "alice" {
		t.Fatalf("expected blocked by 'alice', got %q", info.BlockedBy)
	}

	// List blocked
	blockedMap, _ := client.ListBlocked(ctx)
	if len(blockedMap) != 1 {
		t.Fatalf("expected 1 blocked task, got %d", len(blockedMap))
	}

	// Clear blocked
	if err := client.ClearBlocked(ctx, id); err != nil {
		t.Fatalf("clear blocked: %v", err)
	}

	// Verify cleared
	_, blocked, _ = client.GetBlocked(ctx, id)
	if blocked {
		t.Fatalf("expected not blocked after clear")
	}

	// List blocked should be empty
	blockedMap, _ = client.ListBlocked(ctx)
	if len(blockedMap) != 0 {
		t.Fatalf("expected 0 blocked tasks, got %d", len(blockedMap))
	}
}

func TestStoreUpdateTask(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()
	ids, _ := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "Original", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: minimalDoD}},
	})
	id := ids["Original"]

	// Update title
	if err := client.UpdateTask(ctx, id, UpdateOptions{Title: "New Title"}); err != nil {
		t.Fatalf("update title: %v", err)
	}
	iss, _, _ := client.GetTask(ctx, id)
	if iss.Title != "New Title" {
		t.Fatalf("expected 'New Title', got %q", iss.Title)
	}

	// Update assignee
	if err := client.UpdateTask(ctx, id, UpdateOptions{Assignee: "alice"}); err != nil {
		t.Fatalf("update assignee: %v", err)
	}
	iss, _, _ = client.GetTask(ctx, id)
	if iss.Assignee != "alice" {
		t.Fatalf("expected assignee 'alice', got %q", iss.Assignee)
	}

	// Clear assignee with "-"
	if err := client.UpdateTask(ctx, id, UpdateOptions{Assignee: "-"}); err != nil {
		t.Fatalf("clear assignee: %v", err)
	}
	iss, _, _ = client.GetTask(ctx, id)
	if iss.Assignee != "" {
		t.Fatalf("expected assignee cleared, got %q", iss.Assignee)
	}

	// Update priority
	newPriority := 1
	if err := client.UpdateTask(ctx, id, UpdateOptions{Priority: &newPriority}); err != nil {
		t.Fatalf("update priority: %v", err)
	}
	iss, _, _ = client.GetTask(ctx, id)
	if iss.Priority != 1 {
		t.Fatalf("expected priority 1, got %d", iss.Priority)
	}

	// Update next_hint
	if err := client.UpdateTask(ctx, id, UpdateOptions{NextHint: "do next task"}); err != nil {
		t.Fatalf("update next_hint: %v", err)
	}
	iss, _, _ = client.GetTask(ctx, id)
	if iss.NextHint != "do next task" {
		t.Fatalf("expected next_hint 'do next task', got %q", iss.NextHint)
	}

	// Verify history recorded
	history, _ := client.History(ctx, id)
	updateCount := 0
	for _, ev := range history {
		if len(ev.Action) >= 7 && ev.Action[:7] == "updated" {
			updateCount++
		}
	}
	if updateCount != 5 {
		t.Fatalf("expected 5 update events in history, got %d", updateCount)
	}

	// No-op update (no changes)
	if err := client.UpdateTask(ctx, id, UpdateOptions{Title: "New Title"}); err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	// History should still have 4 updates (no new event for no-op)
	history, _ = client.History(ctx, id)
	updateCount = 0
	for _, ev := range history {
		if len(ev.Action) >= 7 && ev.Action[:7] == "updated" {
			updateCount++
		}
	}
	if updateCount != 5 {
		t.Fatalf("expected still 5 update events after no-op, got %d", updateCount)
	}
}

func TestUpdateContext(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()
	ids, _ := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "HandoffUpdate", Template: "backend_endpoint", Role: "dev", Artifact: "spec:handoff", DoD: minimalDoD}},
	})
	id := ids["HandoffUpdate"]

	// Update context
	if err := client.UpdateTask(ctx, id, UpdateOptions{Context: "Bug: users can submit invalid emails"}); err != nil {
		t.Fatalf("update context: %v", err)
	}
	_, meta, _ := client.GetTask(ctx, id)
	if meta.Context != "Bug: users can submit invalid emails" {
		t.Fatalf("expected context updated, got %q", meta.Context)
	}

	// Update inputs
	if err := client.UpdateTask(ctx, id, UpdateOptions{Inputs: []string{"pkg/user.go", "pkg/validation.go"}}); err != nil {
		t.Fatalf("update inputs: %v", err)
	}
	_, meta, _ = client.GetTask(ctx, id)
	if len(meta.Inputs) != 2 || meta.Inputs[0] != "pkg/user.go" {
		t.Fatalf("expected inputs updated, got %v", meta.Inputs)
	}

	// Update scope
	if err := client.UpdateTask(ctx, id, UpdateOptions{Scope: "IN: validation. OUT: UI"}); err != nil {
		t.Fatalf("update scope: %v", err)
	}
	_, meta, _ = client.GetTask(ctx, id)
	if meta.Scope != "IN: validation. OUT: UI" {
		t.Fatalf("expected scope updated, got %q", meta.Scope)
	}

	// Update reference
	if err := client.UpdateTask(ctx, id, UpdateOptions{Reference: "https://github.com/issue/123"}); err != nil {
		t.Fatalf("update reference: %v", err)
	}
	_, meta, _ = client.GetTask(ctx, id)
	if meta.Reference != "https://github.com/issue/123" {
		t.Fatalf("expected reference updated, got %q", meta.Reference)
	}

	// Clear context with "-"
	if err := client.UpdateTask(ctx, id, UpdateOptions{Context: "-"}); err != nil {
		t.Fatalf("clear context: %v", err)
	}
	_, meta, _ = client.GetTask(ctx, id)
	if meta.Context != "" {
		t.Fatalf("expected context cleared, got %q", meta.Context)
	}

	// Verify history recorded handoff changes
	history, _ := client.History(ctx, id)
	updateCount := 0
	for _, ev := range history {
		if len(ev.Action) >= 7 && ev.Action[:7] == "updated" {
			updateCount++
		}
	}
	if updateCount != 5 {
		t.Fatalf("expected 5 update events for handoff fields, got %d", updateCount)
	}
}

func TestAtomicWriteNoTempLeftBehind(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	// Create a task - triggers save
	_, err := client.AddTask(ctx, Task{
		Title:    "AtomicTest",
		Template: "backend_endpoint",
		Role:     "dev",
		Artifact: "spec:atomic",
		DoD:      minimalDoD,
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	// Verify main file exists
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("expected db file: %v", err)
	}

	// Verify temp file was cleaned up
	tmpPath := tmp + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temp file should not exist after save, but got: %v", err)
	}

	// Reload and verify data integrity
	client2 := NewStoreClient(tmp, "pt")
	issues, _ := client2.List(ctx, nil, "", 10)
	if len(issues) != 1 || issues[0].Title != "AtomicTest" {
		t.Fatalf("data integrity check failed: got %d issues", len(issues))
	}
}

func TestAtomicWritePreservesPermissions(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"

	// Create file with custom permissions
	if err := os.WriteFile(tmp, []byte("{}"), 0o600); err != nil {
		t.Fatalf("create file: %v", err)
	}

	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	// Add task - triggers save
	_, _ = client.AddTask(ctx, Task{
		Title:    "PermTest",
		Template: "backend_endpoint",
		Role:     "dev",
		Artifact: "spec:perm",
		DoD:      minimalDoD,
	})

	// Check permissions were preserved
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Compare only permission bits (ignore type bits)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestStoreWorktreeMethods(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	// Create a task
	id, err := client.AddTask(ctx, Task{
		Title:    "WorktreeTask",
		Template: "backend_endpoint",
		Role:     "dev",
		Artifact: "spec:wt",
		DoD:      minimalDoD,
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	// No worktree initially
	wts, _ := client.ListWorktrees(ctx)
	if len(wts) != 0 {
		t.Fatalf("expected no worktrees, got %d", len(wts))
	}

	// Set worktree
	wtInfo := WorktreeInfo{
		Path:      "/tmp/worktree-test",
		Branch:    "test-branch",
		CreatedAt: "2025-01-01T00:00:00Z",
	}
	if err := client.SetWorktree(ctx, id, wtInfo); err != nil {
		t.Fatalf("set worktree: %v", err)
	}

	// Get worktree
	got, hasWT, err := client.GetWorktree(ctx, id)
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if !hasWT {
		t.Fatalf("expected worktree to exist")
	}
	if got.Path != wtInfo.Path || got.Branch != wtInfo.Branch {
		t.Fatalf("worktree mismatch: got %+v", got)
	}
	if got.TaskID != id {
		t.Fatalf("expected TaskID=%s, got %s", id, got.TaskID)
	}

	// List worktrees
	wts, _ = client.ListWorktrees(ctx)
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}

	// Clear worktree
	if err := client.ClearWorktree(ctx, id, "worktree:done"); err != nil {
		t.Fatalf("clear worktree: %v", err)
	}

	// Verify cleared
	_, hasWT, _ = client.GetWorktree(ctx, id)
	if hasWT {
		t.Fatalf("expected worktree to be cleared")
	}

	// History should include worktree events
	history, _ := client.History(ctx, id)
	var foundStart, foundDone bool
	for _, ev := range history {
		if ev.Action == "worktree:start" {
			foundStart = true
		}
		if ev.Action == "worktree:done" {
			foundDone = true
		}
	}
	if !foundStart || !foundDone {
		t.Fatalf("expected worktree:start and worktree:done in history, got %v", history)
	}
}

func TestStoreSyncRemovesDraftOnCompleteDoD(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	// First, create a task with empty DoD (draft-like)
	ids, _ := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "DraftTask", Template: "backend_endpoint", Role: "dev", Artifact: "spec:draft", DoD: DefinitionOfDone{}}},
	})
	id := ids["DraftTask"]

	// Manually add draft label
	if err := client.AddLabels(ctx, id, "state:draft"); err != nil {
		t.Fatalf("add draft label: %v", err)
	}

	// Verify draft label exists
	iss, _, _ := client.GetTask(ctx, id)
	hasDraft := false
	for _, l := range iss.Labels {
		if l == "state:draft" {
			hasDraft = true
			break
		}
	}
	if !hasDraft {
		t.Fatalf("expected state:draft label to be present initially")
	}

	// Now sync with complete DoD
	_, err := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "DraftTask", Template: "backend_endpoint", Role: "dev", Artifact: "spec:draft", DoD: minimalDoD}},
	})
	if err != nil {
		t.Fatalf("sync with complete DoD: %v", err)
	}

	// Verify draft label removed
	iss, _, _ = client.GetTask(ctx, id)
	for _, l := range iss.Labels {
		if l == "state:draft" {
			t.Fatalf("expected state:draft label to be removed after sync with complete DoD")
		}
	}
}

func TestReleaseClearsAssignee(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	ids, _ := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "ReleaseTest", Template: "backend_endpoint", Role: "dev", Artifact: "spec:release", DoD: minimalDoD}},
	})
	id := ids["ReleaseTest"]

	// Assign someone
	if err := client.UpdateIssue(ctx, id, "in_progress", "agent"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Verify assigned
	iss, _, _ := client.GetTask(ctx, id)
	if iss.Assignee != "agent" {
		t.Fatalf("expected assignee 'agent', got %q", iss.Assignee)
	}

	// Release using "-" to clear assignee
	if err := client.UpdateIssue(ctx, id, "open", "-"); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Verify assignee cleared
	iss, _, _ = client.GetTask(ctx, id)
	if iss.Assignee != "" {
		t.Fatalf("expected empty assignee after release, got %q", iss.Assignee)
	}
	if iss.Status != "open" {
		t.Fatalf("expected status 'open', got %q", iss.Status)
	}
}

func TestHistoryShowsCorrectTransitions(t *testing.T) {
	tmp := t.TempDir() + "/pt.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	ids, _ := client.Sync(ctx, Manifest{
		Tasks: []Task{{Title: "HistoryTest", Template: "backend_endpoint", Role: "dev", Artifact: "spec:hist", DoD: minimalDoD}},
	})
	id := ids["HistoryTest"]

	// Transition: open -> in_progress
	if err := client.UpdateIssue(ctx, id, "in_progress", "agent"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Transition: in_progress -> open (release with assignee clear)
	if err := client.UpdateIssue(ctx, id, "open", "-"); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Check history
	history, _ := client.History(ctx, id)
	var foundClaimTransition, foundReleaseTransition bool
	for _, ev := range history {
		// Look for open->in_progress
		if ev.Action == "status:open->in_progress" || (len(ev.Action) > 20 && ev.Action[:21] == "status:open->in_progr") {
			foundClaimTransition = true
		}
		// Look for in_progress->open with assignee change
		if ev.Action == "status:in_progress->open;assignee:agent->" {
			foundReleaseTransition = true
		}
	}
	if !foundClaimTransition {
		t.Fatalf("expected 'status:open->in_progress' in history, got: %v", history)
	}
	if !foundReleaseTransition {
		t.Fatalf("expected 'status:in_progress->open;assignee:agent->' in history, got: %v", history)
	}
}
