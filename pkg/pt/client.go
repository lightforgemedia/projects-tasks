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
	// First, try to find store in the current git repo root (main repo or worktree)
	repoRoot := findGitRepoRoot()
	if repoRoot != "" {
		// Check for stores in current repo root
		if found := findStoreInDir(repoRoot); found != "" {
			return found
		}
	}

	// If we're in a worktree, check the main repo
	mainRepoRoot := findMainRepoRoot()
	if mainRepoRoot != "" && mainRepoRoot != repoRoot {
		if found := findStoreInDir(mainRepoRoot); found != "" {
			return found
		}
	}

	return "" // no store found
}

// findGitRepoRoot returns the root of the current git repository.
func findGitRepoRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// findMainRepoRoot returns the root of the main repository when in a worktree.
// Returns empty string if not in a worktree or if we're in the main repo.
func findMainRepoRoot() string {
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
	return parentRoot
}

// findStoreInDir looks for a PT store in the given directory.
// Checks multiple locations in order of preference:
//  1. .pt/db.json (recommended, gitignored)
//  2. .pt.db.json (legacy, often git-tracked)
func findStoreInDir(dir string) string {
	// Check .pt/db.json first (recommended location)
	newPath := filepath.Join(dir, ".pt", "db.json")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}

	// Check legacy .pt.db.json
	legacyPath := filepath.Join(dir, ".pt.db.json")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}

	return ""
}
