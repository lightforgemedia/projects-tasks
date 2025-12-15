package pt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// StoreClient implements Client using a JSON-backed local store (no external deps).
type StoreClient struct {
	path   string
	prefix string

	mu       sync.Mutex
	data     storeData
	lockPath string
}

// CommentsFor returns stored comments for an issue (testing/introspection).
func (c *StoreClient) CommentsFor(id string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.data.Comments[id]...)
}

type storeData struct {
	NextID    int                            `json:"next_id"`
	Issues    map[string]Issue               `json:"issues"`
	Labels    map[string]map[string]struct{} `json:"labels"` // issue -> set
	Deps      map[string][]string            `json:"deps"`   // issue -> deps
	Comments  map[string][]string            `json:"comments"`
	TitleMap  map[string]string              `json:"title_map"` // title -> id
	History   map[string][]HistoryEvent      `json:"history"`   // issue -> events
	Blocked   map[string]BlockedInfo         `json:"blocked"`   // issue -> blocked info
	Worktrees map[string]WorktreeInfo        `json:"worktrees"` // task_id -> worktree info
	Mocks     map[string]MockInfo            `json:"mocks"`     // mock_id -> mock info
}

// MockInfo tracks a mock/stub implementation that needs to be replaced.
type MockInfo struct {
	ID              string `json:"id"`
	Description     string `json:"description"`
	Location        string `json:"location"`         // file:line or path
	SpikeTaskID     string `json:"spike_task_id"`    // originating spike (required)
	IntegrationTask string `json:"integration_task"` // task that will replace this mock
	CreatedAt       string `json:"created_at"`
	RetiredAt       string `json:"retired_at,omitempty"`
}

// NewStoreClient creates or opens a store at path. If path empty, defaults to ".pt.db.json".
// Paths are resolved to absolute at construction time to ensure operations are cwd-independent.
func NewStoreClient(path, prefix string) *StoreClient {
	if strings.TrimSpace(path) == "" {
		path = ".pt.db.json"
	}
	if strings.TrimSpace(prefix) == "" {
		prefix = "pt"
	}
	// Resolve to absolute path at construction time.
	// This ensures all subsequent operations are independent of working directory changes.
	absPath, err := filepath.Abs(path)
	if err != nil {
		// Fall back to original path if Abs fails (shouldn't happen in practice)
		absPath = path
	}
	lockPath := absPath + ".lock"
	c := &StoreClient{path: absPath, prefix: prefix, lockPath: lockPath}
	c.load()
	return c
}

func (c *StoreClient) Sync(ctx context.Context, manifest Manifest) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Detect dependency cycles by title before writing anything.
	// This prevents creating a corrupted plan that can never be made ready.
	if err := detectManifestCycles(manifest); err != nil {
		return nil, err
	}
	idByTitle := make(map[string]string, len(manifest.Tasks))
	for _, task := range manifest.Tasks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		id, err := c.ensureIssueLocked(task)
		if err != nil {
			return nil, err
		}
		idByTitle[task.Title] = id
		c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: "synced"})
	}
	for _, task := range manifest.Tasks {
		taskID := idByTitle[task.Title]
		for _, depTitle := range task.Deps {
			depID := idByTitle[depTitle]
			c.addDepLocked(taskID, depID)
		}
	}
	if err := c.saveLocked(); err != nil {
		return nil, err
	}
	return idByTitle, nil
}

func detectManifestCycles(manifest Manifest) error {
	// Build adjacency by title.
	adj := make(map[string][]string, len(manifest.Tasks))
	for _, t := range manifest.Tasks {
		adj[t.Title] = append([]string{}, t.Deps...)
	}

	const (
		unseen  = 0
		visiting = 1
		done    = 2
	)
	state := make(map[string]int, len(adj))

	var stack []string
	indexInStack := make(map[string]int, len(adj))

	var visit func(string) error
	visit = func(n string) error {
		switch state[n] {
		case visiting:
			// Cycle found: report path from first occurrence.
			start := indexInStack[n]
			cycle := append([]string{}, stack[start:]...)
			cycle = append(cycle, n)
			return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
		case done:
			return nil
		}
		state[n] = visiting
		indexInStack[n] = len(stack)
		stack = append(stack, n)

		for _, dep := range adj[n] {
			// Unknown deps are already validated elsewhere; ignore here.
			if _, ok := adj[dep]; !ok {
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}

		// Pop
		stack = stack[:len(stack)-1]
		delete(indexInStack, n)
		state[n] = done
		return nil
	}

	for title := range adj {
		if state[title] == unseen {
			if err := visit(title); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *StoreClient) Ready(ctx context.Context, role string, limit int) ([]Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []Issue
	for _, iss := range c.data.Issues {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if iss.Status != "open" {
			continue
		}
		if role != "" && !c.hasLabelLocked(iss.ID, fmt.Sprintf("role:%s", role)) {
			continue
		}
		// CLI handles blocker visualization, so we return all open tasks.
		// Transitioner.Claim enforces the actual dependency gating.
		out = append(out, iss)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// List returns issues matching statuses (empty means all). Role filters by role label.
func (c *StoreClient) List(ctx context.Context, statuses []string, role string, limit int) ([]Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := make(map[string]struct{})
	for _, s := range statuses {
		want[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	var out []Issue
	for _, iss := range c.data.Issues {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if role != "" && !c.hasLabelLocked(iss.ID, fmt.Sprintf("role:%s", role)) {
			continue
		}
		if len(want) > 0 {
			if _, ok := want[strings.ToLower(iss.Status)]; !ok {
				continue
			}
		}
		out = append(out, iss)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (c *StoreClient) UpdateIssue(ctx context.Context, id, status, assignee string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	iss, ok := c.data.Issues[id]
	if !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	// Capture old values before updating
	oldStatus := iss.Status
	oldAssignee := iss.Assignee

	if status != "" {
		iss.Status = status
	}
	// Special value "-" means clear assignee; empty string means no change
	if assignee == "-" {
		iss.Assignee = ""
	} else if assignee != "" {
		iss.Assignee = assignee
	}
	c.data.Issues[id] = iss

	// Build history action with correct old->new format
	actor := iss.Assignee
	if strings.TrimSpace(actor) == "" {
		actor = oldAssignee
	}
	action := fmt.Sprintf("status:%s->%s", oldStatus, iss.Status)
	if oldAssignee != iss.Assignee {
		action = fmt.Sprintf("%s;assignee:%s->%s", action, oldAssignee, iss.Assignee)
	}
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: actor, Action: action})
	return c.saveLocked()
}

// UpdateTask updates task fields specified in opts. Only non-empty/non-nil fields are applied.
func (c *StoreClient) UpdateTask(ctx context.Context, id string, opts UpdateOptions) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	iss, ok := c.data.Issues[id]
	if !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	var changes []string

	// Update top-level Issue fields
	if strings.TrimSpace(opts.Title) != "" && opts.Title != iss.Title {
		oldTitle := iss.Title
		iss.Title = opts.Title
		// Update title map
		delete(c.data.TitleMap, oldTitle)
		c.data.TitleMap[opts.Title] = id
		changes = append(changes, fmt.Sprintf("title:%s->%s", oldTitle, opts.Title))
	}
	if strings.TrimSpace(opts.Assignee) != "" && opts.Assignee != iss.Assignee {
		oldAssignee := iss.Assignee
		iss.Assignee = opts.Assignee
		changes = append(changes, fmt.Sprintf("assignee:%s->%s", oldAssignee, opts.Assignee))
	}
	if opts.Priority != nil && *opts.Priority != iss.Priority {
		oldPriority := iss.Priority
		iss.Priority = *opts.Priority
		changes = append(changes, fmt.Sprintf("priority:%d->%d", oldPriority, *opts.Priority))
	}
	if strings.TrimSpace(opts.NextHint) != "" && opts.NextHint != iss.NextHint {
		oldHint := iss.NextHint
		iss.NextHint = opts.NextHint
		changes = append(changes, fmt.Sprintf("next_hint:%s->%s", oldHint, opts.NextHint))
	}

	// Update handoff fields in TaskMeta (embedded in description)
	metaChanged := false
	meta, _ := parseTaskMeta(iss.Description) // ignore error, may not have meta yet
	if opts.Context != "" {
		if opts.Context == "-" {
			if meta.Context != "" {
				meta.Context = ""
				changes = append(changes, "context:cleared")
				metaChanged = true
			}
		} else if opts.Context != meta.Context {
			changes = append(changes, fmt.Sprintf("context:%s->%s", truncate(meta.Context, 20), truncate(opts.Context, 20)))
			meta.Context = opts.Context
			metaChanged = true
		}
	}
	if len(opts.Inputs) > 0 {
		oldInputs := strings.Join(meta.Inputs, ",")
		newInputs := strings.Join(opts.Inputs, ",")
		if newInputs == "-" {
			if len(meta.Inputs) > 0 {
				meta.Inputs = nil
				changes = append(changes, "inputs:cleared")
				metaChanged = true
			}
		} else if oldInputs != newInputs {
			changes = append(changes, fmt.Sprintf("inputs:%s->%s", truncate(oldInputs, 20), truncate(newInputs, 20)))
			meta.Inputs = opts.Inputs
			metaChanged = true
		}
	}
	if opts.Scope != "" {
		if opts.Scope == "-" {
			if meta.Scope != "" {
				meta.Scope = ""
				changes = append(changes, "scope:cleared")
				metaChanged = true
			}
		} else if opts.Scope != meta.Scope {
			changes = append(changes, fmt.Sprintf("scope:%s->%s", truncate(meta.Scope, 20), truncate(opts.Scope, 20)))
			meta.Scope = opts.Scope
			metaChanged = true
		}
	}
	if opts.Reference != "" {
		if opts.Reference == "-" {
			if meta.Reference != "" {
				meta.Reference = ""
				changes = append(changes, "reference:cleared")
				metaChanged = true
			}
		} else if opts.Reference != meta.Reference {
			changes = append(changes, fmt.Sprintf("reference:%s->%s", truncate(meta.Reference, 20), truncate(opts.Reference, 20)))
			meta.Reference = opts.Reference
			metaChanged = true
		}
	}

	// Rebuild description if meta changed
	if metaChanged {
		task := Task{
			Template:  meta.Template,
			Title:     iss.Title,
			Role:      meta.Role,
			Artifact:  meta.Artifact,
			NextHint:  meta.NextHint,
			DoD:       meta.DoD,
			Context:   meta.Context,
			Inputs:    meta.Inputs,
			Scope:     meta.Scope,
			Reference: meta.Reference,
		}
		desc, err := buildDescription(task)
		if err != nil {
			return fmt.Errorf("rebuild description: %w", err)
		}
		iss.Description = desc
	}

	if len(changes) == 0 {
		return nil // No changes
	}
	c.data.Issues[id] = iss
	action := "updated:" + strings.Join(changes, ";")
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: iss.Assignee, Action: action})
	return c.saveLocked()
}

// truncate shortens a string for logging purposes
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (c *StoreClient) AddComment(ctx context.Context, id, body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("comment body required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data.Comments == nil {
		c.data.Comments = make(map[string][]string)
	}
	c.data.Comments[id] = append(c.data.Comments[id], body)
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: "commented", Note: body})
	return c.saveLocked()
}

// Comments returns comments for an issue.
func (c *StoreClient) Comments(ctx context.Context, id string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	comments := c.data.Comments[id]
	return append([]string{}, comments...), nil
}

// History returns lifecycle events for an issue.
func (c *StoreClient) History(ctx context.Context, id string) ([]HistoryEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]HistoryEvent{}, c.data.History[id]...), nil
}

// SetBlocked marks a task as blocked with a reason.
func (c *StoreClient) SetBlocked(ctx context.Context, id, reason, blockedBy string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[id]; !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	if c.data.Blocked == nil {
		c.data.Blocked = make(map[string]BlockedInfo)
	}
	c.data.Blocked[id] = BlockedInfo{
		Reason:    reason,
		BlockedBy: blockedBy,
		BlockedAt: time.Now().UTC().Format(time.RFC3339),
	}
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: blockedBy, Action: "blocked", Note: reason})
	return c.saveLocked()
}

// ClearBlocked removes the blocked status from a task.
func (c *StoreClient) ClearBlocked(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[id]; !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	delete(c.data.Blocked, id)
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: "unblocked"})
	return c.saveLocked()
}

// GetBlocked returns blocked info for a task, and whether it is blocked.
func (c *StoreClient) GetBlocked(ctx context.Context, id string) (BlockedInfo, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[id]; !ok {
		return BlockedInfo{}, false, fmt.Errorf("issue %s not found", id)
	}
	info, ok := c.data.Blocked[id]
	return info, ok, nil
}

// ListBlocked returns all blocked tasks with their blocked info.
func (c *StoreClient) ListBlocked(ctx context.Context) (map[string]BlockedInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]BlockedInfo, len(c.data.Blocked))
	for id, info := range c.data.Blocked {
		result[id] = info
	}
	return result, nil
}

// SetWorktree associates a worktree with a task.
func (c *StoreClient) SetWorktree(ctx context.Context, taskID string, info WorktreeInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[taskID]; !ok {
		return fmt.Errorf("issue %s not found", taskID)
	}
	if c.data.Worktrees == nil {
		c.data.Worktrees = make(map[string]WorktreeInfo)
	}
	info.TaskID = taskID
	c.data.Worktrees[taskID] = info
	c.appendHistoryLocked(taskID, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: "worktree:start", Note: info.Path})
	return c.saveLocked()
}

// ClearWorktree removes the worktree association for a task.
func (c *StoreClient) ClearWorktree(ctx context.Context, taskID string, action string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[taskID]; !ok {
		return fmt.Errorf("issue %s not found", taskID)
	}
	info, hasWT := c.data.Worktrees[taskID]
	if !hasWT {
		return nil // no worktree to clear
	}
	delete(c.data.Worktrees, taskID)
	if action == "" {
		action = "worktree:done"
	}
	c.appendHistoryLocked(taskID, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: action, Note: info.Path})
	return c.saveLocked()
}

// GetWorktree returns worktree info for a task, and whether one exists.
func (c *StoreClient) GetWorktree(ctx context.Context, taskID string) (WorktreeInfo, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[taskID]; !ok {
		return WorktreeInfo{}, false, fmt.Errorf("issue %s not found", taskID)
	}
	info, ok := c.data.Worktrees[taskID]
	return info, ok, nil
}

// ListWorktrees returns all active worktree associations.
func (c *StoreClient) ListWorktrees(ctx context.Context) (map[string]WorktreeInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]WorktreeInfo, len(c.data.Worktrees))
	for id, info := range c.data.Worktrees {
		result[id] = info
	}
	return result, nil
}

// UpdateMeta updates the task description with new metadata.
func (c *StoreClient) UpdateMeta(ctx context.Context, id string, meta TaskMeta) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	iss, ok := c.data.Issues[id]
	if !ok {
		return fmt.Errorf("issue %s not found", id)
	}

	// Rebuild description with updated meta
	desc, err := rebuildDescriptionWithMeta(iss.Description, meta)
	if err != nil {
		return fmt.Errorf("rebuild description: %w", err)
	}
	iss.Description = desc
	c.data.Issues[id] = iss

	return c.saveLocked()
}

// rebuildDescriptionWithMeta replaces the pt-meta marker in a description with new metadata.
func rebuildDescriptionWithMeta(oldDesc string, meta TaskMeta) (string, error) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	newMarker := fmt.Sprintf("<!-- pt-meta: %s -->", metaJSON)

	start := strings.Index(oldDesc, "<!-- pt-meta:")
	end := strings.Index(oldDesc, "-->")
	if start != -1 && end != -1 && end > start {
		// Replace existing marker
		return oldDesc[:start] + newMarker, nil
	}
	// No existing marker, append
	return oldDesc + "\n" + newMarker, nil
}

// AddTask creates a single task ad-hoc.
func (c *StoreClient) AddTask(ctx context.Context, task Task) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := validateTask(task); err != nil {
		return "", err
	}
	// Validate deps refer to existing issue IDs or titles
	for _, dep := range task.Deps {
		if _, ok := c.data.Issues[dep]; ok {
			continue
		}
		if id, ok := c.data.TitleMap[dep]; ok {
			task.Deps = replaceValue(task.Deps, dep, id)
			continue
		}
		return "", fmt.Errorf("unknown dependency %q", dep)
	}
	id, err := c.ensureIssueLocked(task)
	if err != nil {
		return "", err
	}
	for _, dep := range task.Deps {
		c.addDepLocked(id, dep)
	}
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: "added"})
	if err := c.saveLocked(); err != nil {
		return "", err
	}
	return id, nil
}

func (c *StoreClient) GetTask(ctx context.Context, id string) (Issue, TaskMeta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	iss, ok := c.data.Issues[id]
	if !ok {
		return Issue{}, TaskMeta{}, fmt.Errorf("issue %s not found", id)
	}
	meta, err := parseTaskMeta(iss.Description)
	if err != nil {
		return iss, TaskMeta{}, err
	}
	return iss, meta, nil
}

func (c *StoreClient) Dependencies(ctx context.Context, id string) ([]Dependency, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var deps []Dependency
	for _, dep := range c.data.Deps[id] {
		depIssue, ok := c.data.Issues[dep]
		status := "unknown"
		if ok {
			status = depIssue.Status
		}
		deps = append(deps, Dependency{ID: dep, Status: status})
	}
	return deps, nil
}

func (c *StoreClient) AddLabels(ctx context.Context, id string, labels ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[id]; !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	if c.data.Labels[id] == nil {
		c.data.Labels[id] = make(map[string]struct{})
	}
	for _, l := range labels {
		c.data.Labels[id][l] = struct{}{}
	}
	c.refreshIssueLabels(id)
	return c.saveLocked()
}

func (c *StoreClient) RemoveLabels(ctx context.Context, id string, labels ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[id]; !ok {
		return fmt.Errorf("issue %s not found", id)
	}
	if c.data.Labels[id] == nil {
		return nil
	}
	for _, l := range labels {
		delete(c.data.Labels[id], l)
	}
	c.refreshIssueLabels(id)
	return c.saveLocked()
}

// --- helpers ---

func (c *StoreClient) ensureIssueLocked(task Task) (string, error) {
	// Check by title
	for id, iss := range c.data.Issues {
		if iss.Title == task.Title {
			c.data.TitleMap[task.Title] = id
			desc, err := buildDescription(task)
			if err != nil {
				return "", err
			}
			iss.Description = desc
			iss.NextHint = task.NextHint
			// Ensure role/template labels stay current
			if c.data.Labels[id] == nil {
				c.data.Labels[id] = make(map[string]struct{})
			}
			c.data.Labels[id][fmt.Sprintf("role:%s", task.Role)] = struct{}{}
			c.data.Labels[id][fmt.Sprintf("template:%s", task.Template)] = struct{}{}
			// If task was draft and manifest now provides complete DoD, remove draft label
			if _, isDraft := c.data.Labels[id]["state:draft"]; isDraft {
				dodComplete := len(task.DoD.Tests) > 0 || strings.TrimSpace(task.DoD.Manual) != "" || len(task.DoD.Criteria) > 0
				if dodComplete {
					delete(c.data.Labels[id], "state:draft")
				}
			}
			c.data.Issues[id] = iss
			c.refreshIssueLabels(id) // must be after setting issue to update its Labels
			return iss.ID, nil
		}
	}
	c.data.NextID++
	id := fmt.Sprintf("%s-%d", c.prefix, c.data.NextID)
	desc, err := buildDescription(task)
	if err != nil {
		return "", err
	}
	issue := Issue{
		ID:          id,
		Title:       task.Title,
		Description: desc,
		Status:      "open",
		Priority:    2,
		IssueType:   "task",
		NextHint:    task.NextHint,
		Labels: []string{
			fmt.Sprintf("role:%s", task.Role),
			fmt.Sprintf("template:%s", task.Template),
		},
	}
	c.data.Issues[id] = issue
	if c.data.TitleMap == nil {
		c.data.TitleMap = make(map[string]string)
	}
	c.data.TitleMap[task.Title] = id
	c.data.Labels[id] = make(map[string]struct{})
	for _, l := range issue.Labels {
		c.data.Labels[id][l] = struct{}{}
	}
	c.appendHistoryLocked(id, HistoryEvent{At: time.Now().UTC(), Actor: "", Action: "created"})
	return id, nil
}

func (c *StoreClient) addDepLocked(issueID, depID string) {
	if issueID == depID || depID == "" {
		return
	}
	c.data.Deps[issueID] = appendUnique(c.data.Deps[issueID], depID)
}

// AddDependency adds a dependency from issueID to depID.
// The issue with issueID will be blocked until depID is closed.
func (c *StoreClient) AddDependency(ctx context.Context, issueID, depID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data.Issues[issueID]; !ok {
		return fmt.Errorf("issue %s not found", issueID)
	}
	if _, ok := c.data.Issues[depID]; !ok {
		return fmt.Errorf("dependency issue %s not found", depID)
	}
	c.addDepLocked(issueID, depID)
	return c.saveLocked()
}

func (c *StoreClient) depsDoneLocked(issueID string) bool {
	for _, dep := range c.data.Deps[issueID] {
		if depIssue, ok := c.data.Issues[dep]; !ok || depIssue.Status != "closed" && depIssue.Status != "done" {
			return false
		}
	}
	return true
}

func (c *StoreClient) appendHistoryLocked(id string, ev HistoryEvent) {
	if c.data.History == nil {
		c.data.History = make(map[string][]HistoryEvent)
	}
	c.data.History[id] = append(c.data.History[id], ev)
}

func (c *StoreClient) hasLabelLocked(id, label string) bool {
	set := c.data.Labels[id]
	if set == nil {
		return false
	}
	_, ok := set[label]
	return ok
}

func (c *StoreClient) refreshIssueLabels(id string) {
	issue := c.data.Issues[id]
	issue.Labels = issue.Labels[:0]
	for l := range c.data.Labels[id] {
		issue.Labels = append(issue.Labels, l)
	}
	c.data.Issues[id] = issue
}

func (c *StoreClient) load() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data.Issues != nil {
		return
	}
	lockFile, err := c.lockFile()
	if err != nil {
		return
	}
	defer c.unlockFile(lockFile)
	c.data = storeData{
		NextID:   0,
		Issues:   make(map[string]Issue),
		Labels:   make(map[string]map[string]struct{}),
		Deps:     make(map[string][]string),
		Comments: make(map[string][]string),
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(raw, &c.data)
	// ensure maps not nil
	if c.data.Issues == nil {
		c.data.Issues = make(map[string]Issue)
	}
	if c.data.Labels == nil {
		c.data.Labels = make(map[string]map[string]struct{})
	}
	if c.data.Deps == nil {
		c.data.Deps = make(map[string][]string)
	}
	if c.data.TitleMap == nil {
		c.data.TitleMap = make(map[string]string)
	}
	if c.data.Comments == nil {
		c.data.Comments = make(map[string][]string)
	}
	if c.data.History == nil {
		c.data.History = make(map[string][]HistoryEvent)
	}
	if c.data.Blocked == nil {
		c.data.Blocked = make(map[string]BlockedInfo)
	}
	if c.data.Worktrees == nil {
		c.data.Worktrees = make(map[string]WorktreeInfo)
	}
}

func (c *StoreClient) saveLocked() error {
	lockFile, err := c.lockFile()
	if err != nil {
		return err
	}
	defer c.unlockFile(lockFile)
	dir := filepath.Dir(c.path)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	raw, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: temp file + rename
	// Get existing file permissions (if file exists), otherwise use default
	perm := os.FileMode(0o644)
	if info, err := os.Stat(c.path); err == nil {
		perm = info.Mode().Perm()
	}

	// Write to temp file in same directory (same filesystem ensures atomic rename)
	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, perm); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename (on POSIX systems, rename is atomic if target exists or not)
	if err := os.Rename(tmpPath, c.path); err != nil {
		// Cleanup temp file on failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (c *StoreClient) lockFile() (*os.File, error) {
	if c.lockPath == "" {
		return nil, fmt.Errorf("lock path not set")
	}
	f, err := os.OpenFile(c.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (c *StoreClient) unlockFile(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

func appendUnique(list []string, val string) []string {
	for _, v := range list {
		if v == val {
			return list
		}
	}
	return append(list, val)
}

func replaceValue(list []string, from, to string) []string {
	out := make([]string, len(list))
	for i, v := range list {
		if v == from {
			out[i] = to
		} else {
			out[i] = v
		}
	}
	return out
}

// RegisterMock registers a new mock/stub that needs to be replaced with real implementation.
func (c *StoreClient) RegisterMock(ctx context.Context, description, location, spikeTaskID, integrationTaskID string) (MockInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.data.Mocks == nil {
		c.data.Mocks = make(map[string]MockInfo)
	}

	// Generate mock ID
	mockID := fmt.Sprintf("mock-%d", len(c.data.Mocks)+1)

	mock := MockInfo{
		ID:              mockID,
		Description:     description,
		Location:        location,
		SpikeTaskID:     spikeTaskID,
		IntegrationTask: integrationTaskID,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	c.data.Mocks[mockID] = mock
	if err := c.saveLocked(); err != nil {
		return MockInfo{}, err
	}
	return mock, nil
}

// ListMocks returns all registered mocks.
func (c *StoreClient) ListMocks(ctx context.Context, includeRetired bool) []MockInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	var mocks []MockInfo
	for _, m := range c.data.Mocks {
		if !includeRetired && m.RetiredAt != "" {
			continue
		}
		mocks = append(mocks, m)
	}
	return mocks
}

// RetireMock marks a mock as retired (replaced with real implementation).
func (c *StoreClient) RetireMock(ctx context.Context, mockID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	mock, ok := c.data.Mocks[mockID]
	if !ok {
		return fmt.Errorf("mock %s not found", mockID)
	}
	if mock.RetiredAt != "" {
		return fmt.Errorf("mock %s already retired", mockID)
	}
	mock.RetiredAt = time.Now().UTC().Format(time.RFC3339)
	c.data.Mocks[mockID] = mock
	return c.saveLocked()
}

// CheckMocks returns orphaned mocks (mocks without a closed integration task).
func (c *StoreClient) CheckMocks(ctx context.Context) []MockInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	var orphans []MockInfo
	for _, m := range c.data.Mocks {
		if m.RetiredAt != "" {
			continue // already retired
		}
		// Check if integration task is closed
		if m.IntegrationTask != "" {
			if iss, ok := c.data.Issues[m.IntegrationTask]; ok {
				if iss.Status == "closed" || iss.Status == "done" {
					continue // integration task is done
				}
			}
		}
		orphans = append(orphans, m)
	}
	return orphans
}
