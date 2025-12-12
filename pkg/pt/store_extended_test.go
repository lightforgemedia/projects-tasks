package pt

import (
	"context"
	"os"
	"testing"
)

func TestStoreClient_Extended(t *testing.T) {
	// Setup
	tmp := t.TempDir() + "/pt_ext.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	// Seed initial data
	manifest := Manifest{
		Title: "Ext Test",
		Tasks: []Task{
			{Title: "Root", Template: "backend_endpoint", Role: "dev", Artifact: "spec:root", DoD: minimalDoD},
			{Title: "Child", Template: "backend_endpoint", Role: "dev", Artifact: "spec:child", Deps: []string{"Root"}, DoD: minimalDoD},
		},
	}
	ids, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	rootID := ids["Root"]
	childID := ids["Child"]

	// 1. Test Dependencies
	t.Run("Dependencies", func(t *testing.T) {
		deps, err := client.Dependencies(ctx, childID)
		if err != nil {
			t.Fatalf("Dependencies failed: %v", err)
		}
		if len(deps) != 1 {
			t.Fatalf("expected 1 dep, got %d", len(deps))
		}
		if deps[0].ID != rootID {
			t.Errorf("expected dep ID %s, got %s", rootID, deps[0].ID)
		}
		if deps[0].Status != "open" {
			t.Errorf("expected dep status 'open', got %s", deps[0].Status)
		}
	})

	// 2. Test UpdateIssue & GetTask
	t.Run("UpdateIssue", func(t *testing.T) {
		err := client.UpdateIssue(ctx, rootID, "closed", "alice")
		if err != nil {
			t.Fatalf("UpdateIssue failed: %v", err)
		}

		iss, _, err := client.GetTask(ctx, rootID)
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if iss.Status != "closed" {
			t.Errorf("expected status 'closed', got %s", iss.Status)
		}
		if iss.Assignee != "alice" {
			t.Errorf("expected assignee 'alice', got %s", iss.Assignee)
		}

		// Error case: Update non-existent
		if err := client.UpdateIssue(ctx, "missing-id", "closed", ""); err == nil {
			t.Error("expected error for missing ID, got nil")
		}
	})

	// 3. Test AddComment & CommentsFor
	t.Run("Comments", func(t *testing.T) {
		if err := client.AddComment(ctx, rootID, "First comment"); err != nil {
			t.Fatalf("AddComment failed: %v", err)
		}
		if err := client.AddComment(ctx, rootID, "Second comment"); err != nil {
			t.Fatalf("AddComment failed: %v", err)
		}

		comments := client.CommentsFor(rootID)
		if len(comments) != 2 {
			t.Fatalf("expected 2 comments, got %d", len(comments))
		}
		if comments[0] != "First comment" {
			t.Errorf("unexpected comment content: %s", comments[0])
		}

		// Error case: Empty body
		if err := client.AddComment(ctx, rootID, ""); err == nil {
			t.Error("expected error for empty comment, got nil")
		}
	})

	// 4. Test Labels
	t.Run("Labels", func(t *testing.T) {
		if err := client.AddLabels(ctx, rootID, "urgent", "frontend"); err != nil {
			t.Fatalf("AddLabels failed: %v", err)
		}

		iss, _, _ := client.GetTask(ctx, rootID)
		hasUrgent := false
		for _, l := range iss.Labels {
			if l == "urgent" {
				hasUrgent = true
				break
			}
		}
		if !hasUrgent {
			t.Error("expected label 'urgent' to be present")
		}

		if err := client.RemoveLabels(ctx, rootID, "urgent"); err != nil {
			t.Fatalf("RemoveLabels failed: %v", err)
		}

		iss, _, _ = client.GetTask(ctx, rootID)
		for _, l := range iss.Labels {
			if l == "urgent" {
				t.Error("expected label 'urgent' to be removed")
			}
		}

		// Error case: Add/Remove on missing ID
		if err := client.AddLabels(ctx, "missing", "x"); err == nil {
			t.Error("expected error AddLabels on missing ID")
		}
		if err := client.RemoveLabels(ctx, "missing", "x"); err == nil {
			t.Error("expected error RemoveLabels on missing ID")
		}
	})

	// 5. Test Ready Limit & Filtering
	t.Run("ReadyLimit", func(t *testing.T) {
		// Sync more tasks
		m2 := Manifest{
			Title: "Bulk",
			Tasks: []Task{
				{Title: "T1", Template: "bug_fix", Role: "dev", Artifact: "spec:t1", DoD: minimalDoD},
				{Title: "T2", Template: "bug_fix", Role: "dev", Artifact: "spec:t2", DoD: minimalDoD},
				{Title: "T3", Template: "bug_fix", Role: "dev", Artifact: "spec:t3", DoD: minimalDoD},
			},
		}
		_, err := client.Sync(ctx, m2)
		if err != nil {
			t.Fatal(err)
		}

		ready, err := client.Ready(ctx, "dev", 2)
		if err != nil {
			t.Fatal(err)
		}
		// Note: exact count depends on previously synced tasks too.
		// We have Child (open, blocked by Root which is closed? No, Root is closed).
		// Root is closed. Child is open. T1, T2, T3 are open.
		// Child depends on Root (closed). So Child is ready.
		// Total ready: Child, T1, T2, T3 = 4.
		// Limit 2.
		if len(ready) != 2 {
			t.Errorf("expected 2 tasks (limit), got %d", len(ready))
		}
	})
}

func TestNewStoreClient_Defaults(t *testing.T) {
	// Create client with empty paths
	c := NewStoreClient("", "")
	// Check private fields via reflection or just assume if it doesn't crash it's ok?
	// Since we can't access private fields, we verify behavior that depends on defaults.
	// Ideally we'd check the path, but it's private.
	// We can check if it creates the default file.

	// Clean up potential default file
	defer os.Remove(".pt.db.json")

	// Force a save to create the file
	_ = c.saveLocked()

	if _, err := os.Stat(".pt.db.json"); err != nil {
		t.Errorf("expected default file .pt.db.json to be created")
	}
}

func TestAddDependency(t *testing.T) {
	tmp := t.TempDir() + "/pt_adddep.db.json"
	client := NewStoreClient(tmp, "pt")
	ctx := context.Background()

	// Create two independent tasks
	manifest := Manifest{
		Title: "AddDep Test",
		Tasks: []Task{
			{Title: "Task A", Template: "backend_endpoint", Role: "dev", Artifact: "spec:a", DoD: minimalDoD},
			{Title: "Task B", Template: "backend_endpoint", Role: "dev", Artifact: "spec:b", DoD: minimalDoD},
		},
	}
	ids, err := client.Sync(ctx, manifest)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	taskA := ids["Task A"]
	taskB := ids["Task B"]

	// Initially, Task B has no dependencies
	deps, _ := client.Dependencies(ctx, taskB)
	if len(deps) != 0 {
		t.Fatalf("expected 0 deps, got %d", len(deps))
	}

	// Add dependency: Task B depends on Task A
	if err := client.AddDependency(ctx, taskB, taskA); err != nil {
		t.Fatalf("AddDependency failed: %v", err)
	}

	// Verify dependency was added
	deps, _ = client.Dependencies(ctx, taskB)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].ID != taskA {
		t.Errorf("expected dep ID %s, got %s", taskA, deps[0].ID)
	}

	// Error case: non-existent issueID
	if err := client.AddDependency(ctx, "missing-id", taskA); err == nil {
		t.Error("expected error for missing issueID")
	}

	// Error case: non-existent depID
	if err := client.AddDependency(ctx, taskB, "missing-dep"); err == nil {
		t.Error("expected error for missing depID")
	}
}
