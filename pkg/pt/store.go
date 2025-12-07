package pt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// StoreClient implements Client using a JSON-backed local store (no external deps).
type StoreClient struct {
	path   string
	prefix string

	mu   sync.Mutex
	data storeData
}

// CommentsFor returns stored comments for an issue (testing/introspection).
func (c *StoreClient) CommentsFor(id string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.data.Comments[id]...)
}

type storeData struct {
	NextID   int                            `json:"next_id"`
	Issues   map[string]Issue               `json:"issues"`
	Labels   map[string]map[string]struct{} `json:"labels"` // issue -> set
	Deps     map[string][]string            `json:"deps"`   // issue -> deps
	Comments map[string][]string            `json:"comments"`
}

// NewStoreClient creates or opens a store at path. If path empty, defaults to ".pt.db.json".
func NewStoreClient(path, prefix string) *StoreClient {
	if strings.TrimSpace(path) == "" {
		path = ".pt.db.json"
	}
	if strings.TrimSpace(prefix) == "" {
		prefix = "pt"
	}
	c := &StoreClient{path: path, prefix: prefix}
	c.load()
	return c
}

func (c *StoreClient) Sync(ctx context.Context, manifest Manifest) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		if !c.depsDoneLocked(iss.ID) {
			continue
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
	if status != "" {
		iss.Status = status
	}
	if assignee != "" {
		iss.Assignee = assignee
	}
	c.data.Issues[id] = iss
	return c.saveLocked()
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
	return c.saveLocked()
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
	for _, iss := range c.data.Issues {
		if iss.Title == task.Title {
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
		Labels: []string{
			fmt.Sprintf("role:%s", task.Role),
			fmt.Sprintf("template:%s", task.Template),
		},
	}
	c.data.Issues[id] = issue
	c.data.Labels[id] = make(map[string]struct{})
	for _, l := range issue.Labels {
		c.data.Labels[id][l] = struct{}{}
	}
	return id, nil
}

func (c *StoreClient) addDepLocked(issueID, depID string) {
	if issueID == depID || depID == "" {
		return
	}
	c.data.Deps[issueID] = appendUnique(c.data.Deps[issueID], depID)
}

func (c *StoreClient) depsDoneLocked(issueID string) bool {
	for _, dep := range c.data.Deps[issueID] {
		if depIssue, ok := c.data.Issues[dep]; !ok || depIssue.Status != "closed" && depIssue.Status != "done" {
			return false
		}
	}
	return true
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
}

func (c *StoreClient) saveLocked() error {
	dir := filepath.Dir(c.path)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	raw, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, raw, 0o644)
}

func appendUnique(list []string, val string) []string {
	for _, v := range list {
		if v == val {
			return list
		}
	}
	return append(list, val)
}
