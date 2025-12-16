package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

// TestPathScenarios_StoreResolution verifies that store paths are resolved
// to absolute at construction time and remain stable across directory changes.
func TestPathScenarios_StoreResolution(t *testing.T) {
	// Use t.TempDir() for safer cleanup handling
	tmpDir := t.TempDir()

	// Create store with path in temp dir
	storePath := filepath.Join(tmpDir, ".pt", "db.json")
	client := pt.NewStoreClient(storePath, "test")

	// The store should work from anywhere now
	// Create a subdirectory and change to it
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("chdir to subdir: %v", err)
	}

	// Add a task - this should still work because path was resolved to absolute
	task := pt.Task{
		Template: "refactor",
		Title:    "Test task from subdir",
		Role:     "dev",
		Artifact: "code:test",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "verify", Criteria: []string{"done"}},
	}

	ctx := t.Context()
	id, err := client.AddTask(ctx, task)
	if err != nil {
		t.Fatalf("add task from subdir: %v", err)
	}

	// Verify task was added
	issue, _, err := client.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if issue.Title != "Test task from subdir" {
		t.Errorf("wrong title: got %q", issue.Title)
	}

	// Verify store file is in original location
	if _, err := os.Stat(storePath); err != nil {
		t.Errorf("store not at expected path %s: %v", storePath, err)
	}
}

// TestPathScenarios_GitHelpers verifies git helpers work with explicit paths.
func TestPathScenarios_GitHelpers(t *testing.T) {
	// Skip if git not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Use t.TempDir() for safer cleanup handling
	tmpDir := t.TempDir()

	// Init git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create a file and commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Create subdir
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	// Test gitRepoRoot with explicit subdir path - should find parent repo
	root, err := gitRepoRoot(subDir)
	if err != nil {
		t.Fatalf("gitRepoRoot from subdir: %v", err)
	}
	// Normalize paths for comparison
	absRoot, _ := filepath.EvalSymlinks(root)
	absTmp, _ := filepath.EvalSymlinks(tmpDir)
	if absRoot != absTmp {
		t.Errorf("gitRepoRoot: got %q, want %q", absRoot, absTmp)
	}

	// Test gitIsDirtyAt with explicit path - no chdir needed
	dirty, err := gitIsDirtyAt(tmpDir)
	if err != nil {
		t.Fatalf("gitIsDirtyAt: %v", err)
	}
	if dirty {
		t.Error("repo should be clean after commit")
	}

	// Make a change
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	dirty, err = gitIsDirtyAt(tmpDir)
	if err != nil {
		t.Fatalf("gitIsDirtyAt after modify: %v", err)
	}
	if !dirty {
		t.Error("repo should be dirty after modification")
	}
}

// TestPathScenarios_DiscoveryFromWorktree tests that db discovery finds
// the main repo's store when running from a worktree.
func TestPathScenarios_DiscoveryFromWorktree(t *testing.T) {
	// Clear PT_DB env var so discovery is based on git, not env
	t.Setenv("PT_DB", "")

	// Skip if git not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Use t.TempDir() for safer cleanup handling
	baseDir := t.TempDir()

	mainRepo := filepath.Join(baseDir, "main")
	if err := os.MkdirAll(mainRepo, 0755); err != nil {
		t.Fatalf("create main repo dir: %v", err)
	}

	// Init main repo
	cmd := exec.Command("git", "init")
	cmd.Dir = mainRepo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init main: %v\n%s", err, out)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = mainRepo
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = mainRepo
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(mainRepo, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = mainRepo
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = mainRepo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Create PT store in main repo
	storePath := filepath.Join(mainRepo, ".pt", "db.json")
	client := pt.NewStoreClient(storePath, "test")
	ctx := t.Context()
	_, err := client.AddTask(ctx, pt.Task{
		Template: "refactor",
		Title:    "Main repo task",
		Role:     "dev",
		Artifact: "code:test",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "verify", Criteria: []string{"done"}},
	})
	if err != nil {
		t.Fatalf("add task to main: %v", err)
	}

	// Create worktree
	worktreeDir := filepath.Join(baseDir, "worktree")
	cmd = exec.Command("git", "worktree", "add", "-b", "feature", worktreeDir)
	cmd.Dir = mainRepo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	// Save and change to worktree
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatalf("chdir to worktree: %v", err)
	}

	// Reset discovery cache
	pt.ResetDiscoverCache()

	// Discover should find main repo's store
	discovered := pt.DiscoveredStorePath()
	if discovered == "" {
		t.Fatal("expected to discover store from worktree")
	}

	// Normalize for comparison
	absDiscovered, _ := filepath.EvalSymlinks(discovered)
	absStore, _ := filepath.EvalSymlinks(storePath)
	if absDiscovered != absStore {
		t.Errorf("discovered wrong store: got %q, want %q", absDiscovered, absStore)
	}

	// Verify we can read tasks from discovered store
	discoveredClient := pt.NewStoreClient(discovered, "test")
	issues, err := discoveredClient.List(ctx, nil, "", 10)
	if err != nil {
		t.Fatalf("list from discovered: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
	if len(issues) > 0 && !strings.Contains(issues[0].Title, "Main repo task") {
		t.Errorf("wrong task title: %q", issues[0].Title)
	}
}

// TestPathScenarios_DiscoveryPrefersNearestStore verifies that when multiple stores exist
// in a single git repo (e.g. monorepo subprojects), discovery prefers the nearest store
// between the current directory and the repo root.
func TestPathScenarios_DiscoveryPrefersNearestStore(t *testing.T) {
	t.Setenv("PT_DB", "")

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	baseDir := t.TempDir()
	repoRoot := filepath.Join(baseDir, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = repoRoot
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = repoRoot
	_ = cmd.Run()

	testFile := filepath.Join(repoRoot, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoRoot
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Create a store at repo root (should be ignored when a nearer store exists).
	rootStore := filepath.Join(repoRoot, ".pt", "db.json")
	rootClient := pt.NewStoreClient(rootStore, "test")
	ctx := t.Context()
	if _, err := rootClient.AddTask(ctx, pt.Task{
		Template: "refactor",
		Title:    "Root task",
		Role:     "dev",
		Artifact: "code:root",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "verify", Criteria: []string{"done"}},
	}); err != nil {
		t.Fatalf("add root task: %v", err)
	}

	// Create a subproject store.
	subproj := filepath.Join(repoRoot, "projects", "subproj")
	if err := os.MkdirAll(subproj, 0o755); err != nil {
		t.Fatalf("mkdir subproj: %v", err)
	}
	subStore := filepath.Join(subproj, ".pt", "db.json")
	subClient := pt.NewStoreClient(subStore, "test")
	if _, err := subClient.AddTask(ctx, pt.Task{
		Template: "refactor",
		Title:    "Subproject task",
		Role:     "dev",
		Artifact: "code:sub",
		DoD:      pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "verify", Criteria: []string{"done"}},
	}); err != nil {
		t.Fatalf("add sub task: %v", err)
	}

	// Change to a deeper directory under the subproject.
	deep := filepath.Join(subproj, "nested")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(deep); err != nil {
		t.Fatalf("chdir deep: %v", err)
	}

	pt.ResetDiscoverCache()
	discovered := pt.DiscoveredStorePath()
	if discovered == "" {
		t.Fatal("expected to discover store")
	}
	absDiscovered, _ := filepath.EvalSymlinks(discovered)
	absSubStore, _ := filepath.EvalSymlinks(subStore)
	if absDiscovered != absSubStore {
		t.Fatalf("discovered=%q want %q", absDiscovered, absSubStore)
	}
}

func TestDBDiscovery(t *testing.T) {
	TestPathScenarios_DiscoveryFromWorktree(t)
}
