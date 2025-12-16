package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestWorktreeStartAndDone_EndToEnd(t *testing.T) {
	t.Setenv("PT_SKIP_HOOKS", "1")
	t.Setenv("PT_WORKFLOW", "")

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	gitRun(t, repoDir, "init")
	gitRun(t, repoDir, "config", "user.email", "test@test.com")
	gitRun(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".pt/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	gitRun(t, repoDir, "add", ".")
	gitRun(t, repoDir, "commit", "-m", "init")

	storePath := filepath.Join(repoDir, ".pt", "db.json")
	t.Setenv("PT_DB", storePath)
	t.Setenv("PT_PREFIX", "pt")

	store := pt.NewStoreClient(storePath, "pt")
	if _, err := store.Sync(t.Context(), pt.Manifest{
		Tasks: []pt.Task{
			{Title: "WT Task", Template: "refactor", Role: "dev", Artifact: "code:worktree", DoD: pt.DefinitionOfDone{Tests: []string{"echo ok"}, Manual: "ok", Criteria: []string{"ok"}}},
		},
	}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}

	worktreesBase := filepath.Join(baseDir, "worktrees")
	if err := cmdWorktreeStart([]string{"--path", worktreesBase, "pt-1"}); err != nil {
		t.Fatalf("worktree start: %v", err)
	}

	store2 := pt.NewStoreClient(storePath, "pt")
	wtInfo, hasWT, err := store2.GetWorktree(t.Context(), "pt-1")
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if !hasWT {
		t.Fatalf("expected worktree record for pt-1")
	}
	if wtInfo.Path == "" || wtInfo.Branch == "" {
		t.Fatalf("expected worktree path+branch, got: %+v", wtInfo)
	}
	if _, err := os.Stat(wtInfo.Path); err != nil {
		t.Fatalf("worktree path missing: %v", err)
	}

	out := gitOutput(t, wtInfo.Path, "rev-parse", "--show-toplevel")
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected worktree to be a git repo")
	}

	if err := cmdWorktreeDone([]string{"--keep-branch", "pt-1"}); err != nil {
		t.Fatalf("worktree done: %v", err)
	}

	store3 := pt.NewStoreClient(storePath, "pt")
	if _, hasWT, _ := store3.GetWorktree(t.Context(), "pt-1"); hasWT {
		t.Fatalf("expected worktree record cleared for pt-1")
	}
	if _, err := os.Stat(wtInfo.Path); !os.IsNotExist(err) {
		t.Fatalf("expected worktree directory removed, stat err=%v", err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
