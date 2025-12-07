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
	DoD      DefinitionOfDone `json:"dod"`
}

// CommandRunner abstracts external command execution; allows mocking in tests.
type CommandRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner executes commands using os/exec.
type ExecRunner struct{}

// Run executes a command with context.
func (ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no command provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec
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

// buildDescription embeds metadata for later retrieval.
func buildDescription(task Task) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Template: %s\nRole: %s\n", task.Template, task.Role)
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
	if task.DoD.OnFailure != "" {
		fmt.Fprintf(&b, " on_failure=%s", task.DoD.OnFailure)
	}
	meta := TaskMeta{
		Template: task.Template,
		Role:     task.Role,
		NextHint: task.NextHint,
		DoD:      task.DoD,
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
