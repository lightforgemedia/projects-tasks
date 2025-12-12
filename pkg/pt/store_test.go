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
	if updateCount != 4 {
		t.Fatalf("expected 4 update events in history, got %d", updateCount)
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
	if updateCount != 4 {
		t.Fatalf("expected still 4 update events after no-op, got %d", updateCount)
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
