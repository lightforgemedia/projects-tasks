package pt

import (
	"context"
	"fmt"
	"strings"
)

// Transitioner enforces StateManager transitions while persisting via the client backend.
type Transitioner struct {
	Client Client
}

func (t Transitioner) ensureClient() (Client, error) {
	if t.Client == nil {
		return nil, fmt.Errorf("nil client")
	}
	return t.Client, nil
}

// Claim validates the transition and writes to the backend.
func (t Transitioner) Claim(ctx context.Context, id, assignee string) error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	issue, sm, err := t.loadIssueState(ctx, id)
	if err != nil {
		return err
	}
	if err := sm.Claim(issue.ID, assignee); err != nil {
		return err
	}
	if err := client.UpdateIssue(ctx, issue.ID, string(StatusInProgress), assignee); err != nil {
		return err
	}
	return client.AddLabels(ctx, issue.ID, "state:claimed")
}

// Release moves an in-progress task back to ready/open.
func (t Transitioner) Release(ctx context.Context, id, assignee string) error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	issue, sm, err := t.loadIssueState(ctx, id)
	if err != nil {
		return err
	}
	if err := sm.Release(issue.ID, assignee); err != nil {
		return err
	}
	// Use "-" to explicitly clear assignee
	if err := client.UpdateIssue(ctx, issue.ID, "open", "-"); err != nil {
		return err
	}
	_ = client.RemoveLabels(ctx, issue.ID, "state:claimed", "state:needs_review")
	return nil
}

// SubmitForReview marks a task needs_review after validation.
func (t Transitioner) SubmitForReview(ctx context.Context, id string, comment string) error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	issue, sm, err := t.loadIssueState(ctx, id)
	if err != nil {
		return err
	}
	if err := sm.MarkNeedsReview(issue.ID); err != nil {
		return err
	}
	if err := client.UpdateIssue(ctx, issue.ID, string(StatusNeedsReview), ""); err != nil {
		return err
	}
	if err := client.AddLabels(ctx, issue.ID, "state:needs_review"); err != nil {
		return err
	}
	if strings.TrimSpace(comment) != "" {
		if err := client.AddComment(ctx, issue.ID, comment); err != nil {
			return err
		}
	}
	return nil
}

// Approve completes a needs_review task.
func (t Transitioner) Approve(ctx context.Context, id string) error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	issue, sm, err := t.loadIssueState(ctx, id)
	if err != nil {
		return err
	}
	if err := sm.Complete(issue.ID); err != nil {
		return err
	}
	if err := client.UpdateIssue(ctx, issue.ID, "closed", ""); err != nil {
		return err
	}
	_ = client.RemoveLabels(ctx, issue.ID, "state:needs_review", "state:claimed")
	_ = client.AddComment(ctx, issue.ID, "Approved and closed")
	return nil
}

// Reject sends a needs_review task back to in_progress with a comment.
func (t Transitioner) Reject(ctx context.Context, id, reason string) error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	issue, sm, err := t.loadIssueState(ctx, id)
	if err != nil {
		return err
	}
	if err := sm.Reject(issue.ID, reason); err != nil {
		return err
	}
	if err := client.UpdateIssue(ctx, issue.ID, string(StatusInProgress), ""); err != nil {
		return err
	}
	_ = client.RemoveLabels(ctx, issue.ID, "state:needs_review")
	if reason != "" {
		if err := client.AddComment(ctx, issue.ID, fmt.Sprintf("Rejected: %s", reason)); err != nil {
			return err
		}
	}
	return nil
}

// Reopen moves a done/closed task back to in_progress.
func (t Transitioner) Reopen(ctx context.Context, id, assignee string) error {
	client, err := t.ensureClient()
	if err != nil {
		return err
	}
	issue, sm, err := t.loadIssueState(ctx, id)
	if err != nil {
		return err
	}
	if err := sm.Reopen(issue.ID, assignee); err != nil {
		return err
	}
	if err := client.UpdateIssue(ctx, issue.ID, string(StatusInProgress), assignee); err != nil {
		return err
	}
	_ = client.AddLabels(ctx, issue.ID, "state:claimed")
	_ = client.AddComment(ctx, issue.ID, fmt.Sprintf("Reopened by %s", assignee))
	return nil
}

func (t Transitioner) loadIssueState(ctx context.Context, id string) (Issue, *StateManager, error) {
	client, err := t.ensureClient()
	if err != nil {
		return Issue{}, nil, err
	}

	issue, _, err := client.GetTask(ctx, id)
	if err != nil {
		return Issue{}, nil, err
	}
	deps, err := client.Dependencies(ctx, id)
	if err != nil {
		deps = nil // best effort; don’t block transitions on dep fetch issues
	}

	nodes := make([]TaskNode, 0, len(deps)+1)
	depIDs := make([]string, 0, len(deps))
	for _, d := range deps {
		depIDs = append(depIDs, d.ID)
		nodes = append(nodes, TaskNode{ID: d.ID})
	}
	nodes = append(nodes, TaskNode{ID: issue.ID, Deps: depIDs})

	sm := NewStateManager(nodes)

	// Seed dependency statuses
	for _, d := range deps {
		depStatus, err := mapIssueStatus(d.Status)
		if err != nil {
			return Issue{}, nil, fmt.Errorf("dependency %s: %w", d.ID, err)
		}
		if err := sm.Seed(d.ID, depStatus, ""); err != nil {
			return Issue{}, nil, err
		}
	}

	issueStatus, err := mapIssueStatus(issue.Status)
	if err != nil {
		return Issue{}, nil, err
	}
	if err := sm.Seed(issue.ID, issueStatus, issue.Assignee); err != nil {
		return Issue{}, nil, err
	}
	return issue, sm, nil
}

func mapIssueStatus(status string) (Status, error) {
	switch status {
	case "", "open":
		return StatusReady, nil
	case string(StatusInProgress):
		return StatusInProgress, nil
	case string(StatusNeedsReview):
		return StatusNeedsReview, nil
	case "closed", string(StatusDone):
		return StatusDone, nil
	default:
		return "", fmt.Errorf("unsupported status %q", status)
	}
}
