package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

func cmdWorktree(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pt worktree <start|done|status|abort> [options]")
	}
	switch args[0] {
	case "start":
		return cmdWorktreeStart(args[1:])
	case "done":
		return cmdWorktreeDone(args[1:])
	case "status":
		return cmdWorktreeStatus(args[1:])
	case "abort":
		return cmdWorktreeAbort(args[1:])
	default:
		return fmt.Errorf("unknown subcommand: %s (use start, done, status, abort)", args[0])
	}
}

// cmdWorktreeStart creates a git worktree for working on a task
func cmdWorktreeStart(args []string) error {
	fs := flag.NewFlagSet("worktree start", flag.ContinueOnError)
	branch := fs.String("branch", "", "branch name (default: derived from task ID)")
	basePath := fs.String("path", "", "worktree base directory (default: ../worktrees)")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt worktree start <task-id> [--branch=<name>] [--path=<dir>]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("task ID required")
	}
	taskID := fs.Arg(0)

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify task exists
	issue, _, err := client.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	// Check if task already has a worktree
	if existing, hasWT, _ := client.GetWorktree(ctx, taskID); hasWT {
		return fmt.Errorf("task %s already has worktree at %s", taskID, existing.Path)
	}

	// Derive branch name
	branchName := *branch
	if branchName == "" {
		branchName = sanitizeBranch(taskID + "-" + issue.Title)
	}

	// Validate branch name is safe
	if err := validateBranchName(branchName); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	// Find the git repo root (works from any subdirectory)
	repoRoot, err := gitRepoRoot("")
	if err != nil {
		return fmt.Errorf("find git repo: %w", err)
	}

	// Determine worktree path
	wtPath := *basePath
	if wtPath == "" {
		// Default: ../worktrees/<branch> (relative to repo root)
		wtPath = filepath.Join(filepath.Dir(repoRoot), "worktrees", branchName)
	} else {
		wtPath = filepath.Join(wtPath, branchName)
	}

	// Ensure worktree directory doesn't exist
	if _, err := os.Stat(wtPath); err == nil {
		return fmt.Errorf("worktree path already exists: %s", wtPath)
	}

	// Check for uncommitted changes in main repo (using explicit path)
	if dirty, err := gitIsDirtyAt(repoRoot); err != nil {
		return fmt.Errorf("check git status: %w", err)
	} else if dirty {
		return fmt.Errorf("main repo has uncommitted changes; commit or stash first")
	}

	// Check if branch already exists (using explicit path)
	branchExists, err := gitBranchExistsAt(repoRoot, branchName)
	if err != nil {
		return fmt.Errorf("check branch: %w", err)
	}

	// Create worktree (and branch if needed) using explicit repo path
	var wtErr error
	if branchExists {
		// Use existing branch
		wtErr = gitWorktreeAddAt(repoRoot, wtPath, branchName, false)
	} else {
		// Create new branch from current HEAD
		wtErr = gitWorktreeAddAt(repoRoot, wtPath, branchName, true)
	}
	if wtErr != nil {
		return fmt.Errorf("create worktree: %w", wtErr)
	}

	// Record worktree in store
	wtInfo := pt.WorktreeInfo{
		TaskID:    taskID,
		Path:      wtPath,
		Branch:    branchName,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := client.SetWorktree(ctx, taskID, wtInfo); err != nil {
		// Try to cleanup worktree on failure
		_ = gitWorktreeRemoveAt(repoRoot, wtPath)
		return fmt.Errorf("record worktree: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"task_id": taskID,
			"path":    wtPath,
			"branch":  branchName,
			"status":  "created",
		})
	}
	fmt.Printf("Created worktree for %s at %s (branch: %s)\n", taskID, wtPath, branchName)
	fmt.Printf("cd %s\n", wtPath)
	return nil
}

// cmdWorktreeDone completes a task's worktree (cleanup + optional task completion)
func cmdWorktreeDone(args []string) error {
	fs := flag.NewFlagSet("worktree done", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	keepBranch := fs.Bool("keep-branch", false, "don't delete the branch after removing worktree")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt worktree done <task-id> [--keep-branch]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("task ID required")
	}
	taskID := fs.Arg(0)

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get worktree info
	wtInfo, hasWT, err := client.GetWorktree(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	if !hasWT {
		return fmt.Errorf("no worktree found for task %s", taskID)
	}

	// Check worktree exists on disk
	if _, err := os.Stat(wtInfo.Path); os.IsNotExist(err) {
		// Worktree doesn't exist anymore, just clear the record
		_ = client.ClearWorktree(ctx, taskID, "worktree:done")
		if *jsonOut {
			return printJSON(map[string]interface{}{
				"task_id": taskID,
				"path":    wtInfo.Path,
				"status":  "cleared",
				"note":    "worktree directory was already removed",
			})
		}
		fmt.Printf("Worktree directory already removed, cleared record for %s\n", taskID)
		return nil
	}

	// Check for uncommitted changes in worktree (using explicit path, no chdir)
	dirty, err := gitIsDirtyAt(wtInfo.Path)
	if err != nil {
		return fmt.Errorf("check worktree status: %w", err)
	}
	if dirty {
		return fmt.Errorf("worktree has uncommitted changes at %s; commit or stash first", wtInfo.Path)
	}

	// Find the main repo root for git operations (derive from worktree path, not cwd)
	repoRoot, err := gitRepoRoot(wtInfo.Path)
	if err != nil {
		return fmt.Errorf("find git repo from worktree %s: %w", wtInfo.Path, err)
	}

	// Remove worktree (using explicit repo path)
	if err := gitWorktreeRemoveAt(repoRoot, wtInfo.Path); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}

	// Optionally delete branch (using explicit repo path)
	branchDeleted := false
	if !*keepBranch {
		// Try to delete branch (may fail if not merged, that's ok)
		if err := gitDeleteBranchAt(repoRoot, wtInfo.Branch); err == nil {
			branchDeleted = true
		}
	}

	// Clear worktree record
	if err := client.ClearWorktree(ctx, taskID, "worktree:done"); err != nil {
		return fmt.Errorf("clear worktree record: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"task_id":        taskID,
			"path":           wtInfo.Path,
			"branch":         wtInfo.Branch,
			"branch_deleted": branchDeleted,
			"status":         "removed",
		})
	}
	msg := fmt.Sprintf("Removed worktree for %s at %s", taskID, wtInfo.Path)
	if branchDeleted {
		msg += fmt.Sprintf(" (branch %s deleted)", wtInfo.Branch)
	}
	fmt.Println(msg)
	return nil
}

// cmdWorktreeStatus shows active worktrees
func cmdWorktreeStatus(args []string) error {
	fs := flag.NewFlagSet("worktree status", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt worktree status [--json]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worktrees, err := client.ListWorktrees(ctx)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}

	// Enrich with task info and check existence
	type wtStatus struct {
		TaskID    string `json:"task_id"`
		TaskTitle string `json:"task_title"`
		Path      string `json:"path"`
		Branch    string `json:"branch"`
		CreatedAt string `json:"created_at"`
		Exists    bool   `json:"exists"`
	}
	var statuses []wtStatus
	for taskID, info := range worktrees {
		title := "(unknown)"
		if issue, _, err := client.GetTask(ctx, taskID); err == nil {
			title = issue.Title
		}
		_, pathErr := os.Stat(info.Path)
		statuses = append(statuses, wtStatus{
			TaskID:    taskID,
			TaskTitle: title,
			Path:      info.Path,
			Branch:    info.Branch,
			CreatedAt: info.CreatedAt,
			Exists:    pathErr == nil,
		})
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"worktrees": statuses,
			"count":     len(statuses),
		})
	}

	if len(statuses) == 0 {
		fmt.Println("No active worktrees")
		return nil
	}

	fmt.Printf("Active worktrees (%d):\n", len(statuses))
	for _, s := range statuses {
		existsStr := ""
		if !s.Exists {
			existsStr = " [MISSING]"
		}
		fmt.Printf("  %s: %s%s\n", s.TaskID, s.TaskTitle, existsStr)
		fmt.Printf("    Path: %s\n", s.Path)
		fmt.Printf("    Branch: %s\n", s.Branch)
	}
	return nil
}

// cmdWorktreeAbort removes a worktree without completing the task
func cmdWorktreeAbort(args []string) error {
	fs := flag.NewFlagSet("worktree abort", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	force := fs.Bool("force", false, "force removal even with uncommitted changes")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt worktree abort <task-id> [--force]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("task ID required")
	}
	taskID := fs.Arg(0)

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get worktree info
	wtInfo, hasWT, err := client.GetWorktree(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	if !hasWT {
		return fmt.Errorf("no worktree found for task %s", taskID)
	}

	// Check worktree exists on disk
	if _, err := os.Stat(wtInfo.Path); os.IsNotExist(err) {
		// Already gone, just clear record
		_ = client.ClearWorktree(ctx, taskID, "worktree:abort")
		if *jsonOut {
			return printJSON(map[string]interface{}{
				"task_id": taskID,
				"path":    wtInfo.Path,
				"status":  "cleared",
			})
		}
		fmt.Printf("Worktree already removed, cleared record for %s\n", taskID)
		return nil
	}

	// Check for uncommitted changes unless forced (using explicit path, no chdir)
	if !*force {
		dirty, err := gitIsDirtyAt(wtInfo.Path)
		if err != nil {
			return fmt.Errorf("check worktree status: %w", err)
		}
		if dirty {
			return fmt.Errorf("worktree has uncommitted changes; use --force to discard")
		}
	}

	// Find the main repo root for git operations (derive from worktree path, not cwd)
	repoRoot, err := gitRepoRoot(wtInfo.Path)
	if err != nil {
		return fmt.Errorf("find git repo from worktree %s: %w", wtInfo.Path, err)
	}

	// Remove worktree (force mode, using explicit repo path)
	if err := gitWorktreeRemoveForceAt(repoRoot, wtInfo.Path); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}

	// Clear record
	if err := client.ClearWorktree(ctx, taskID, "worktree:abort"); err != nil {
		return fmt.Errorf("clear worktree record: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"task_id": taskID,
			"path":    wtInfo.Path,
			"branch":  wtInfo.Branch,
			"status":  "aborted",
		})
	}
	fmt.Printf("Aborted worktree for %s at %s\n", taskID, wtInfo.Path)
	return nil
}

// --- git helpers ---
// All git helpers that depend on repo context now take an explicit repoPath.
// They use 'git -C <path>' to ensure operations are cwd-independent.

func sanitizeBranch(s string) string {
	// Replace spaces and special chars with dashes, lowercase
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	s = re.ReplaceAllString(s, "-")
	// Trim leading/trailing dashes
	s = strings.Trim(s, "-")
	// Limit length
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}

func validateBranchName(name string) error {
	// Basic validation: no spaces, no special chars that git doesn't like
	if strings.ContainsAny(name, " \t\n~^:?*[\\") {
		return fmt.Errorf("branch name contains invalid characters")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("branch name has invalid format")
	}
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}
	return nil
}

// gitRepoRoot returns the root of the git repository containing the given path.
// If path is empty, uses current working directory.
func gitRepoRoot(path string) (string, error) {
	var cmd *exec.Cmd
	if path != "" {
		cmd = exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	} else {
		cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// gitIsDirtyAt checks if the repo at repoPath has uncommitted changes.
// If repoPath is empty, uses current working directory.
func gitIsDirtyAt(repoPath string) (bool, error) {
	var cmd *exec.Cmd
	if repoPath != "" {
		cmd = exec.Command("git", "-C", repoPath, "status", "--porcelain")
	} else {
		cmd = exec.Command("git", "status", "--porcelain")
	}
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// gitIsDirty checks if the current repo has uncommitted changes.
// Deprecated: prefer gitIsDirtyAt with explicit path.
func gitIsDirty() (bool, error) {
	return gitIsDirtyAt("")
}

// gitBranchExistsAt checks if a branch exists in the repo at repoPath.
func gitBranchExistsAt(repoPath, name string) (bool, error) {
	var cmd *exec.Cmd
	if repoPath != "" {
		cmd = exec.Command("git", "-C", repoPath, "rev-parse", "--verify", "refs/heads/"+name)
	} else {
		cmd = exec.Command("git", "rev-parse", "--verify", "refs/heads/"+name)
	}
	err := cmd.Run()
	if err != nil {
		// Branch doesn't exist (exit code != 0)
		return false, nil
	}
	return true, nil
}

// gitBranchExists checks if a branch exists in the current repo.
// Deprecated: prefer gitBranchExistsAt with explicit path.
func gitBranchExists(name string) (bool, error) {
	return gitBranchExistsAt("", name)
}

// gitWorktreeAddAt creates a worktree from the repo at repoPath.
func gitWorktreeAddAt(repoPath, wtPath, branch string, createBranch bool) error {
	var cmd *exec.Cmd
	if createBranch {
		if repoPath != "" {
			cmd = exec.Command("git", "-C", repoPath, "worktree", "add", "-b", branch, wtPath)
		} else {
			cmd = exec.Command("git", "worktree", "add", "-b", branch, wtPath)
		}
	} else {
		if repoPath != "" {
			cmd = exec.Command("git", "-C", repoPath, "worktree", "add", wtPath, branch)
		} else {
			cmd = exec.Command("git", "worktree", "add", wtPath, branch)
		}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitWorktreeAdd creates a worktree (uses cwd as repo).
// Deprecated: prefer gitWorktreeAddAt with explicit path.
func gitWorktreeAdd(path, branch string, createBranch bool) error {
	return gitWorktreeAddAt("", path, branch, createBranch)
}

// gitWorktreeRemoveAt removes a worktree from the repo at repoPath.
func gitWorktreeRemoveAt(repoPath, wtPath string) error {
	var cmd *exec.Cmd
	if repoPath != "" {
		cmd = exec.Command("git", "-C", repoPath, "worktree", "remove", wtPath)
	} else {
		cmd = exec.Command("git", "worktree", "remove", wtPath)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitWorktreeRemove removes a worktree (uses cwd as repo).
// Deprecated: prefer gitWorktreeRemoveAt with explicit path.
func gitWorktreeRemove(path string) error {
	return gitWorktreeRemoveAt("", path)
}

// gitWorktreeRemoveForceAt force-removes a worktree from the repo at repoPath.
func gitWorktreeRemoveForceAt(repoPath, wtPath string) error {
	var cmd *exec.Cmd
	if repoPath != "" {
		cmd = exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", wtPath)
	} else {
		cmd = exec.Command("git", "worktree", "remove", "--force", wtPath)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitWorktreeRemoveForce force-removes a worktree (uses cwd as repo).
// Deprecated: prefer gitWorktreeRemoveForceAt with explicit path.
func gitWorktreeRemoveForce(path string) error {
	return gitWorktreeRemoveForceAt("", path)
}

// gitDeleteBranchAt deletes a branch in the repo at repoPath.
func gitDeleteBranchAt(repoPath, name string) error {
	var cmd *exec.Cmd
	if repoPath != "" {
		cmd = exec.Command("git", "-C", repoPath, "branch", "-d", name)
	} else {
		cmd = exec.Command("git", "branch", "-d", name)
	}
	return cmd.Run()
}

// gitDeleteBranch deletes a branch in the current repo.
// Deprecated: prefer gitDeleteBranchAt with explicit path.
func gitDeleteBranch(name string) error {
	return gitDeleteBranchAt("", name)
}
