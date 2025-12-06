package pt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TaskMeta holds structured metadata for a task stored in the description.
type TaskMeta struct {
	Template string           `json:"template"`
	Role     string           `json:"role"`
	DoD      DefinitionOfDone `json:"dod"`
}

// CommandRunner abstracts external command execution for bd; allows mocking in tests.
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

// BDClient wraps bd CLI interactions.
type BDClient struct {
	Runner CommandRunner
}

// NewBDClient constructs a client with the provided runner (defaults to ExecRunner).
func NewBDClient(runner CommandRunner) *BDClient {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &BDClient{Runner: runner}
}

// Sync applies a manifest to Beads: ensures issues exist, labels roles, and wires dependencies.
// Returns a map of task title -> issue ID.
func (c *BDClient) Sync(ctx context.Context, manifest Manifest) (map[string]string, error) {
	if c == nil || c.Runner == nil {
		return nil, errors.New("BDClient runner is nil")
	}
	idByTitle := make(map[string]string, len(manifest.Tasks))

	// First ensure issues exist.
	for _, task := range manifest.Tasks {
		id, err := c.ensureIssue(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", task.Title, err)
		}
		idByTitle[task.Title] = id
	}

	// Then wire dependencies.
	for _, task := range manifest.Tasks {
		taskID := idByTitle[task.Title]
		for _, depTitle := range task.Deps {
			depID := idByTitle[depTitle]
			if err := c.addDependency(ctx, taskID, depID); err != nil {
				return nil, fmt.Errorf("add dep %s -> %s: %w", taskID, depID, err)
			}
		}
	}
	return idByTitle, nil
}

// Ready lists unblocked tasks; role filters by label role:<role> if provided.
func (c *BDClient) Ready(ctx context.Context, role string, limit int) ([]Issue, error) {
	if c == nil || c.Runner == nil {
		return nil, errors.New("BDClient runner is nil")
	}
	args := []string{"bd", "ready", "--json"}
	if role != "" {
		args = append(args, "--label", fmt.Sprintf("role:%s", role))
	}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w (%s)", err, bytes.TrimSpace(out))
	}
	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse bd ready json: %w", err)
	}
	return issues, nil
}

// UpdateIssue updates status/assignee; zero values are ignored.
func (c *BDClient) UpdateIssue(ctx context.Context, id, status, assignee string) error {
	if c == nil || c.Runner == nil {
		return errors.New("BDClient runner is nil")
	}
	args := []string{"bd", "update", id}
	if status != "" {
		args = append(args, "--status", status)
	}
	if assignee != "" {
		args = append(args, "--assignee", assignee)
	}
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return fmt.Errorf("bd update: %w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

// AddComment posts a comment to an issue.
func (c *BDClient) AddComment(ctx context.Context, id, body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("comment body required")
	}
	out, err := c.Runner.Run(ctx, "bd", "comment", id, body)
	if err != nil {
		return fmt.Errorf("bd comment: %w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

// GetTask fetches issue details and parses pt metadata (DoD/template/role) from description.
func (c *BDClient) GetTask(ctx context.Context, id string) (Issue, TaskMeta, error) {
	if c == nil || c.Runner == nil {
		return Issue{}, TaskMeta{}, errors.New("BDClient runner is nil")
	}
	out, err := c.Runner.Run(ctx, "bd", "show", id, "--json")
	if err != nil {
		return Issue{}, TaskMeta{}, fmt.Errorf("bd show: %w (%s)", err, bytes.TrimSpace(out))
	}
	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return Issue{}, TaskMeta{}, fmt.Errorf("parse bd show json: %w", err)
	}
	if len(issues) == 0 {
		return Issue{}, TaskMeta{}, fmt.Errorf("issue %s not found", id)
	}
	meta, err := parseTaskMeta(issues[0].Description)
	if err != nil {
		return issues[0], TaskMeta{}, err
	}
	return issues[0], meta, nil
}

// Issue is a minimal view of a bd issue.
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	IssueType   string   `json:"issue_type"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

func (c *BDClient) ensureIssue(ctx context.Context, task Task) (string, error) {
	// Check if issue already exists by title.
	if id, ok, err := c.findIssueByTitle(ctx, task.Title); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}

	labels := fmt.Sprintf("role:%s,template:%s", task.Role, task.Template)
	desc, err := buildDescription(task)
	if err != nil {
		return "", fmt.Errorf("build description: %w", err)
	}

	args := []string{
		"bd", "create",
		"--type", "task",
		"--title", task.Title,
		"--labels", labels,
		"--description", desc,
	}

	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("bd create failed: %w (%s)", err, bytes.TrimSpace(out))
	}
	id, err := extractIssueID(string(out))
	if err != nil {
		return "", fmt.Errorf("parse issue id: %w (output: %s)", err, bytes.TrimSpace(out))
	}
	return id, nil
}

func (c *BDClient) findIssueByTitle(ctx context.Context, title string) (string, bool, error) {
	args := []string{"bd", "list", "--json", "--title", title, "--limit", "1"}
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return "", false, fmt.Errorf("bd list: %w (%s)", err, bytes.TrimSpace(out))
	}
	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return "", false, fmt.Errorf("parse bd list json: %w", err)
	}
	if len(issues) == 0 {
		return "", false, nil
	}
	return issues[0].ID, true, nil
}

func (c *BDClient) addDependency(ctx context.Context, issueID, depID string) error {
	args := []string{"bd", "dep", "add", issueID, depID, "--type", "blocks"}
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		// If already exists, treat as success.
		if bytes.Contains(out, []byte("already depends")) {
			return nil
		}
		return fmt.Errorf("bd dep add: %w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

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
		DoD:      task.DoD,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	// Hidden marker for robust retrieval.
	fmt.Fprintf(&b, "\n<!-- pt-meta: %s -->", metaJSON)
	return b.String(), nil
}

var issueIDRegex = regexp.MustCompile(`Created issue: ([A-Za-z0-9_.-]+)`)

func extractIssueID(output string) (string, error) {
	matches := issueIDRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return "", errors.New("no issue id found")
	}
	return strings.TrimSpace(matches[1]), nil
}

// ContextWithTimeout is a helper to create a context with a sensible default timeout.
func ContextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 10 * time.Second
	}
	return context.WithTimeout(parent, d)
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

// AddLabels adds labels to an issue.
func (c *BDClient) AddLabels(ctx context.Context, id string, labels ...string) error {
	if len(labels) == 0 {
		return nil
	}
	args := []string{"bd", "update", id}
	for _, l := range labels {
		args = append(args, "--add-label", l)
	}
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return fmt.Errorf("bd add-label: %w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

// RemoveLabels removes labels from an issue.
func (c *BDClient) RemoveLabels(ctx context.Context, id string, labels ...string) error {
	if len(labels) == 0 {
		return nil
	}
	args := []string{"bd", "update", id}
	for _, l := range labels {
		args = append(args, "--remove-label", l)
	}
	out, err := c.Runner.Run(ctx, args...)
	if err != nil {
		return fmt.Errorf("bd remove-label: %w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}
