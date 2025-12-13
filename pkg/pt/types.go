package pt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TaskMeta holds structured metadata for a task stored in the description.
type TaskMeta struct {
	Template string           `json:"template"`
	Role     string           `json:"role"`
	NextHint string           `json:"next_hint,omitempty"`
	Artifact string           `json:"artifact,omitempty"`
	DoD      DefinitionOfDone `json:"dod"`

	// Handoff fields - help agents understand the task without project context
	Context   string   `json:"context,omitempty"`   // WHY: problem being solved, motivation
	Inputs    []string `json:"inputs,omitempty"`    // WHERE: files/directories to read/modify
	Scope     string   `json:"scope,omitempty"`     // BOUNDS: IN-scope and OUT-of-scope
	Reference string   `json:"reference,omitempty"` // RELATED: links to docs, issues, prior work

	// UX discovery - optional exploration loop
	UX      *UXConfig `json:"ux,omitempty"`       // UX requirements from manifest
	UXState *UXState  `json:"ux_state,omitempty"` // Runtime UX exploration state
}

// UXState tracks the current state of UX exploration for a task.
type UXState struct {
	Status     string    `json:"status"`               // pending|cases|explore|selected|approved
	UseCases   []UseCase `json:"use_cases,omitempty"`  // Confirmed use cases
	Options    []string  `json:"options,omitempty"`    // Generated options (labels: A, B, C...)
	Selection  string    `json:"selection,omitempty"`  // User's choice (e.g., "A+C")
	Note       string    `json:"note,omitempty"`       // User's notes on selection
	Iterations int       `json:"iterations"`           // Refinement count
	ApprovedAt string    `json:"approved_at,omitempty"`
}

// UseCase describes a user scenario that the UX must satisfy.
type UseCase struct {
	ID      string `json:"id"`                // Short identifier (e.g., "UC1")
	Actor   string `json:"actor"`             // Who is performing the action
	Goal    string `json:"goal"`              // What they want to achieve
	Context string `json:"context,omitempty"` // When/where this happens
}

// HistoryEvent captures lifecycle events for a task.
type HistoryEvent struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"` // e.g., created, claimed, released, validated, approved, rejected, commented, synced
	Note   string    `json:"note,omitempty"`
}

// CommandRunner abstracts external command execution; allows mocking in tests.
type CommandRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner executes commands using os/exec.
type ExecRunner struct {
	// Dir is the working directory for commands. If empty, uses current working directory.
	Dir string
}

// Run executes a command with context.
func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no command provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	return cmd.CombinedOutput()
}

// Issue represents a task in the store.
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Assignee    string   `json:"assignee"`
	Priority    int      `json:"priority"`
	IssueType   string   `json:"issue_type"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	NextHint    string   `json:"next_hint,omitempty"`
}

// Dependency represents a blocking relationship.
type Dependency struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// UpdateOptions specifies which fields to update on a task.
// Only non-empty fields are applied.
type UpdateOptions struct {
	Title    string `json:"title,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Priority *int   `json:"priority,omitempty"`
	NextHint string `json:"next_hint,omitempty"`

	// Handoff fields - use special value "-" to clear
	Context   string   `json:"context,omitempty"`
	Inputs    []string `json:"inputs,omitempty"`
	Scope     string   `json:"scope,omitempty"`
	Reference string   `json:"reference,omitempty"`
}

// BlockedInfo tracks why a task is blocked.
type BlockedInfo struct {
	Reason    string `json:"reason"`
	BlockedBy string `json:"blocked_by,omitempty"` // optional: who/what blocked it
	BlockedAt string `json:"blocked_at,omitempty"` // timestamp
}

// WorktreeInfo tracks a git worktree associated with a task.
type WorktreeInfo struct {
	TaskID    string `json:"task_id"`
	Path      string `json:"path"`       // worktree directory path
	Branch    string `json:"branch"`     // branch name in the worktree
	CreatedAt string `json:"created_at"` // timestamp
}

// buildDescription embeds metadata for later retrieval.
func buildDescription(task Task) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Template: %s\nRole: %s\n", task.Template, task.Role)
	if strings.TrimSpace(task.Artifact) != "" {
		fmt.Fprintf(&b, "Artifact: %s\n", task.Artifact)
	}
	if len(task.Params) > 0 {
		fmt.Fprintf(&b, "Params: %+v\n", task.Params)
	}
	fmt.Fprintf(&b, "DoD:")
	if len(task.DoD.Tests) > 0 {
		fmt.Fprintf(&b, " tests=%v", task.DoD.Tests)
	}
	if task.DoD.ValidationCmd != "" {
		fmt.Fprintf(&b, " validation_cmd=%s", task.DoD.ValidationCmd)
	}
	if task.DoD.Manual != "" {
		fmt.Fprintf(&b, " manual=%s", task.DoD.Manual)
	}
	if len(task.DoD.Criteria) > 0 {
		fmt.Fprintf(&b, " criteria=%v", task.DoD.Criteria)
	}
	if task.DoD.OnFailure != "" {
		fmt.Fprintf(&b, " on_failure=%s", task.DoD.OnFailure)
	}
	meta := TaskMeta{
		Template:  task.Template,
		Role:      task.Role,
		NextHint:  task.NextHint,
		Artifact:  task.Artifact,
		DoD:       task.DoD,
		Context:   task.Context,
		Inputs:    task.Inputs,
		Scope:     task.Scope,
		Reference: task.Reference,
		UX:        task.UX,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "\n<!-- pt-meta: %s -->", metaJSON)
	return b.String(), nil
}

// parseTaskMeta extracts TaskMeta from description marker.
func parseTaskMeta(desc string) (TaskMeta, error) {
	start := strings.Index(desc, "<!-- pt-meta:")
	end := strings.Index(desc, "-->")
	if start == -1 || end == -1 || end <= start {
		return TaskMeta{}, errors.New("pt-meta not found in description")
	}
	payload := strings.TrimSpace(desc[start+len("<!-- pt-meta:") : end])
	var meta TaskMeta
	if err := json.Unmarshal([]byte(payload), &meta); err != nil {
		return TaskMeta{}, fmt.Errorf("parse pt-meta: %w", err)
	}
	return meta, nil
}

// ParseTaskMeta is an exported wrapper around parseTaskMeta for consumers that
// need to extract metadata from an issue description.
func ParseTaskMeta(desc string) (TaskMeta, error) {
	return parseTaskMeta(desc)
}

// ContextWithTimeout is a helper to create a context with a sensible default timeout.
func ContextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 10 * time.Second
	}
	return context.WithTimeout(parent, d)
}

// RunCommand is a convenience wrapper for ExecRunner; used by CommandRunner interface.
// It mirrors ExecRunner.Run but kept for clarity.
func RunCommand(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no command provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
	return cmd.CombinedOutput()
}
