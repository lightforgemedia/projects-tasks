package pt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Client abstracts the backend (store).
type Client interface {
	Sync(ctx context.Context, manifest Manifest) (map[string]string, error)
	Ready(ctx context.Context, role string, limit int) ([]Issue, error)
	List(ctx context.Context, statuses []string, role string, limit int) ([]Issue, error)
	Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error)
	UpdateIssue(ctx context.Context, id, status, assignee string) error
	UpdateTask(ctx context.Context, id string, opts UpdateOptions) error
	AddLabels(ctx context.Context, id string, labels ...string) error
	RemoveLabels(ctx context.Context, id string, labels ...string) error
	AddComment(ctx context.Context, id, body string) error
	Comments(ctx context.Context, id string) ([]string, error)
	AddTask(ctx context.Context, task Task) (string, error)
	GetTask(ctx context.Context, id string) (Issue, TaskMeta, error)
	Dependencies(ctx context.Context, id string) ([]Dependency, error)
	History(ctx context.Context, id string) ([]HistoryEvent, error)
	SetBlocked(ctx context.Context, id, reason, blockedBy string) error
	ClearBlocked(ctx context.Context, id string) error
	GetBlocked(ctx context.Context, id string) (BlockedInfo, bool, error)
	ListBlocked(ctx context.Context) (map[string]BlockedInfo, error)
	// Worktree management
	SetWorktree(ctx context.Context, taskID string, info WorktreeInfo) error
	ClearWorktree(ctx context.Context, taskID string, action string) error
	GetWorktree(ctx context.Context, taskID string) (WorktreeInfo, bool, error)
	ListWorktrees(ctx context.Context) (map[string]WorktreeInfo, error)
}

// NewClientFromEnv chooses backend based on PT_BACKEND env var.
// Supported: "", "store" (default, local store).
// Auto-discovers parent store when in a git worktree (unless PT_NO_DISCOVER is set).
func NewClientFromEnv() Client {
	dbPath := getEnv("PT_DB")
	prefix := getEnv("PT_PREFIX")

	// If PT_DB not explicitly set and discovery not disabled, try auto-discover
	if dbPath == "" && getEnv("PT_NO_DISCOVER") == "" {
		if discovered := discoverParentStore(); discovered != "" {
			dbPath = discovered
		}
	}

	return NewStoreClient(dbPath, prefix)
}

// DiscoveredStorePath returns the store path that will be used, for informational purposes.
// Returns empty string if using default path.
func DiscoveredStorePath() string {
	if getEnv("PT_DB") != "" {
		return getEnv("PT_DB")
	}
	if getEnv("PT_NO_DISCOVER") != "" {
		return ""
	}
	return discoverParentStore()
}

func getEnv(key string) string {
	if v, ok := lookupEnv(key); ok {
		return v
	}
	return ""
}

// allow tests to stub env
var lookupEnv = os.LookupEnv

// discoverParentStore checks if we're in a git worktree and returns the path
// to the parent repository's PT store, if it exists.
// Caches the result for performance.
func discoverParentStore() string {
	discoverOnce.Do(func() {
		discoveredPath = doDiscoverParentStore()
	})
	return discoveredPath
}

var (
	discoverOnce   sync.Once
	discoveredPath string
)

// ResetDiscoverCache resets the cached discovery result (for testing).
func ResetDiscoverCache() {
	discoverOnce = sync.Once{}
	discoveredPath = ""
}

func doDiscoverParentStore() string {
	// Get git common dir (points to main repo's .git from worktree)
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return "" // not in a git repo
	}
	commonDir := strings.TrimSpace(string(out))
	if commonDir == "" || commonDir == ".git" {
		return "" // not in a worktree, or common dir is local
	}

	// Get current git dir to compare
	cmd2 := exec.Command("git", "rev-parse", "--git-dir")
	out2, err := cmd2.Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(out2))

	// If git-dir equals git-common-dir, we're in the main repo (not a worktree)
	absCommon, _ := filepath.Abs(commonDir)
	absGit, _ := filepath.Abs(gitDir)
	if absCommon == absGit {
		return "" // main repo, not worktree
	}

	// We're in a worktree - find the parent repo root
	// common-dir is typically <parent>/.git, so parent is dirname(common-dir)
	parentRoot := filepath.Dir(absCommon)
	if filepath.Base(absCommon) != ".git" {
		// bare repo or unusual layout - try worktrees structure
		// common-dir might be <parent>/.git for linked worktrees
		parentRoot = filepath.Dir(absCommon)
	}

	// Check for PT store in parent root
	storePath := filepath.Join(parentRoot, ".pt.db.json")
	if _, err := os.Stat(storePath); err == nil {
		return storePath
	}

	return "" // no parent store found
}
