package pt

import (
	"errors"
	"fmt"
	"sync"
)

// Status represents the lifecycle of a task.
type Status string

const (
	StatusPlanned     Status = "planned"
	StatusReady       Status = "ready"
	StatusInProgress  Status = "in_progress"
	StatusNeedsReview Status = "needs_review"
	StatusDone        Status = "done"
)

// TaskNode defines a task and its dependencies.
type TaskNode struct {
	ID    string
	Deps  []string
	Role  string
	Title string
}

type taskState struct {
	TaskNode
	Status   Status
	Assignee string
	Comments []string
}

// StateManager enforces allowed transitions and gating on dependencies.
type StateManager struct {
	mu    sync.Mutex
	tasks map[string]*taskState
}

// NewStateManager seeds tasks; tasks with no deps are ready, others planned.
func NewStateManager(nodes []TaskNode) *StateManager {
	tasks := make(map[string]*taskState, len(nodes))
	for _, n := range nodes {
		state := StatusPlanned
		if len(n.Deps) == 0 {
			state = StatusReady
		}
		tasks[n.ID] = &taskState{TaskNode: n, Status: state}
	}
	return &StateManager{tasks: tasks}
}

// Ready returns IDs eligible to start (ready status and deps satisfied).
func (m *StateManager) Ready() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for id, t := range m.tasks {
		if t.Status == StatusReady && m.depsDone(t) {
			out = append(out, id)
		}
	}
	return out
}

// Claim moves a ready task to in_progress; errors if already claimed.
func (m *StateManager) Claim(id, assignee string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("unknown task %s", id)
	}
	if t.Status != StatusReady {
		return fmt.Errorf("task %s not ready (status=%s)", id, t.Status)
	}
	if !m.depsDone(t) {
		return fmt.Errorf("task %s blocked by deps", id)
	}
	if t.Assignee != "" {
		return fmt.Errorf("task %s already claimed by %s", id, t.Assignee)
	}
	t.Status = StatusInProgress
	t.Assignee = assignee
	return nil
}

// Release returns an in-progress task to ready.
func (m *StateManager) Release(id, assignee string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("unknown task %s", id)
	}
	if t.Status != StatusInProgress {
		return fmt.Errorf("task %s not in progress", id)
	}
	if t.Assignee != "" && t.Assignee != assignee {
		return fmt.Errorf("task %s owned by %s", id, t.Assignee)
	}
	t.Status = StatusReady
	t.Assignee = ""
	return nil
}

// MarkNeedsReview transitions from in_progress to needs_review.
func (m *StateManager) MarkNeedsReview(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("unknown task %s", id)
	}
	if t.Status != StatusInProgress {
		return fmt.Errorf("task %s not in progress", id)
	}
	t.Status = StatusNeedsReview
	return nil
}

// Reject sends a needs_review task back to in_progress with a comment.
func (m *StateManager) Reject(id, comment string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("unknown task %s", id)
	}
	if t.Status != StatusNeedsReview {
		return fmt.Errorf("task %s not in needs_review", id)
	}
	t.Status = StatusInProgress
	if comment != "" {
		t.Comments = append(t.Comments, comment)
	}
	return nil
}

// Complete sets a needs_review task to done and unlocks dependents.
func (m *StateManager) Complete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("unknown task %s", id)
	}
	if t.Status != StatusNeedsReview {
		return fmt.Errorf("task %s not in needs_review", id)
	}
	t.Status = StatusDone
	// update dependents readiness
	for _, dependent := range m.tasks {
		if dependent.Status == StatusPlanned && m.depsDone(dependent) {
			dependent.Status = StatusReady
		}
	}
	return nil
}

func (m *StateManager) depsDone(t *taskState) bool {
	for _, dep := range t.Deps {
		d, ok := m.tasks[dep]
		if !ok || d.Status != StatusDone {
			return false
		}
	}
	return true
}

// SetStatus is used by tests to seed statuses safely.
func (m *StateManager) SetStatus(id string, status Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return errors.New("unknown task")
	}
	t.Status = status
	return nil
}

// Seed sets status and assignee atomically; used when hydrating from external state.
func (m *StateManager) Seed(id string, status Status, assignee string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return errors.New("unknown task")
	}
	t.Status = status
	t.Assignee = assignee
	return nil
}
