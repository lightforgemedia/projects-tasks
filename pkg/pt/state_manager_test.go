package pt

import (
	"sync"
	"testing"
)

func TestReadyGatingAndCompletion(t *testing.T) {
	m := NewStateManager([]TaskNode{
		{ID: "A"},
		{ID: "B", Deps: []string{"A"}},
	})
	ready := m.Ready()
	if len(ready) != 1 || ready[0] != "A" {
		t.Fatalf("expected only A ready, got %v", ready)
	}
	if err := m.Claim("A", "alice"); err != nil {
		t.Fatalf("claim A: %v", err)
	}
	if err := m.MarkNeedsReview("A"); err != nil {
		t.Fatalf("needs review: %v", err)
	}
	if err := m.Complete("A"); err != nil {
		t.Fatalf("complete A: %v", err)
	}
	ready = m.Ready()
	if len(ready) != 1 || ready[0] != "B" {
		t.Fatalf("expected B ready after A done, got %v", ready)
	}
}

func TestRejectLoop(t *testing.T) {
	m := NewStateManager([]TaskNode{
		{ID: "A"},
	})
	_ = m.Claim("A", "alice")
	_ = m.MarkNeedsReview("A")
	if err := m.Reject("A", "fix import"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	ready := m.Ready()
	if len(ready) != 0 {
		t.Fatalf("in_progress tasks should not be ready: %v", ready)
	}
	if err := m.MarkNeedsReview("A"); err != nil {
		t.Fatalf("needs review again: %v", err)
	}
	if err := m.Complete("A"); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestClaimConflict(t *testing.T) {
	m := NewStateManager([]TaskNode{{ID: "A"}})
	var wg sync.WaitGroup
	wg.Add(2)
	var err1, err2 error
	go func() {
		defer wg.Done()
		err1 = m.Claim("A", "alice")
	}()
	go func() {
		defer wg.Done()
		err2 = m.Claim("A", "bob")
	}()
	wg.Wait()
	claims := []error{err1, err2}
	successes := 0
	for _, err := range claims {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one claim success, got %d (errs: %v %v)", successes, err1, err2)
	}
}

func TestCrossPhaseDeps(t *testing.T) {
	m := NewStateManager([]TaskNode{
		{ID: "phase1-A"},
		{ID: "phase2-B", Deps: []string{"phase1-A"}},
	})
	if err := m.Claim("phase1-A", "alice"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = m.MarkNeedsReview("phase1-A")
	_ = m.Complete("phase1-A")
	ready := m.Ready()
	if len(ready) != 1 || ready[0] != "phase2-B" {
		t.Fatalf("phase2-B should be ready after phase1-A done, got %v", ready)
	}
}
