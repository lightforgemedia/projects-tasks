package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	projects_tasks_pkg_contract "projects-tasks/pkg/contract"
	"projects-tasks/pkg/pt"
)

func newClientWith(db, prefix string) pt.Client {
	if strings.TrimSpace(db) != "" || strings.TrimSpace(prefix) != "" {
		return pt.NewStoreClient(db, prefix)
	}
	return pt.NewClientFromEnv()
}

func newClient() pt.Client {
	return newClientWith("", "")
}

func requireUser(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		return "", errors.New("no user identity set; use --as or set $USER")
	}
	return user, nil
}

// reorderArgs moves task ID arguments (like "pt-1") to the end so Go's flag
// package can parse flags that appear after them.
// e.g., "pt-1 --as=bob" becomes "--as=bob pt-1"
// Only moves args that look like task IDs (contain "-" followed by digits).
func reorderArgs(args []string) []string {
	var reordered, taskIDs []string
	for _, arg := range args {
		// Check if this looks like a task ID (e.g., pt-1, task-42)
		if isTaskID(arg) {
			taskIDs = append(taskIDs, arg)
		} else {
			reordered = append(reordered, arg)
		}
	}
	return append(reordered, taskIDs...)
}

// isTaskID checks if arg looks like a task ID (prefix-number pattern).
func isTaskID(arg string) bool {
	if strings.HasPrefix(arg, "-") {
		return false // It's a flag
	}
	// Look for pattern like "prefix-N" where N is a number
	idx := strings.LastIndex(arg, "-")
	if idx == -1 || idx == len(arg)-1 {
		return false
	}
	suffix := arg[idx+1:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func splitManualSteps(prompt string) []string {
	lines := strings.Split(prompt, "\n")
	var steps []string
	for _, l := range lines {
		s := strings.TrimSpace(l)
		if s != "" {
			steps = append(steps, s)
		}
	}
	if len(steps) == 0 && strings.TrimSpace(prompt) != "" {
		steps = append(steps, strings.TrimSpace(prompt))
	}
	return steps
}

func combineHooks(parts ...[]HookResult) []HookResult {
	var out []HookResult
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func printJSON(data interface{}) error {
	enc, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(enc))
	return nil
}

func projectDoDPath() string {
	if v := strings.TrimSpace(os.Getenv("PT_PROJECT_DOD")); v != "" {
		return v
	}
	return "PROJECT_DOD.md"
}

func projectDoDStatus() (string, bool) {
	path := projectDoDPath()
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err == nil {
			path = filepath.Join(wd, path)
		}
	}
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return path, false
}

func feedbackBaseDir(explicit string) (string, error) {
	if v := strings.TrimSpace(explicit); v != "" {
		return v, nil
	}
	if env := strings.TrimSpace(os.Getenv("PT_FEEDBACK_DIR")); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".pt", "feedback"), nil
}

func feedbackSlug(desc string) string {
	s := strings.ToLower(strings.TrimSpace(desc))
	if s == "" {
		return "review"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "review"
	}
	return out
}

func feedbackFilename(desc string, now time.Time) string {
	return fmt.Sprintf("%s.%s.feedback.md", now.Format("2006-01-02-1504"), feedbackSlug(desc))
}

func feedbackTemplate() string {
	return `# Feedback

Summary: <one paragraph of what was reviewed and the main verdict>

Findings:
- Critical: <file:line – issue>
- Major: <file:line – issue>
- Minor: <file:line – issue>

Gaps/Risks:
- <short bullets for auth/wiring/build/tests/etc.>

Recommendations:
1. <next action>
2. <next action>

Status: Ready | Blocked | Needs follow-up
`
}

func readyBlockers(ctx context.Context, client pt.Client, iss pt.Issue) []string {
	deps, err := client.Dependencies(ctx, iss.ID)
	if err != nil {
		return nil
	}
	var blocked []string
	for _, d := range deps {
		status := strings.ToLower(strings.TrimSpace(d.Status))
		if status == "" {
			status = "unknown"
		}
		if status == "closed" || status == "done" || status == "resolved" {
			continue
		}
		blocked = append(blocked, fmt.Sprintf("%s(%s)", d.ID, status))
	}
	return blocked
}

func isDoneStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "closed", "done", "resolved":
		return true
	default:
		return false
	}
}

func depsSatisfied(ctx context.Context, client pt.Client, id string) bool {
	deps, err := client.Dependencies(ctx, id)
	if err != nil {
		return false
	}
	for _, d := range deps {
		if !isDoneStatus(d.Status) {
			return false
		}
	}
	return true
}

func listReadyOpen(ctx context.Context, client pt.Client, role string, limit int) ([]pt.Issue, error) {
	issues, err := client.List(ctx, []string{"open"}, role, 0)
	if err != nil {
		return nil, err
	}
	pt.SortIssues(issues, "priority")
	blockedMap, _ := client.ListBlocked(ctx)

	var out []pt.Issue
	for _, iss := range issues {
		if _, blocked := blockedMap[iss.ID]; blocked {
			continue
		}
		if !depsSatisfied(ctx, client, iss.ID) {
			continue
		}
		out = append(out, iss)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func confirmManual(steps []string, autoYes bool) bool {
	if len(steps) == 0 {
		return true
	}
	if autoYes {
		return true
	}
	scanner := bufio.NewScanner(os.Stdin)
	for i, step := range steps {
		fmt.Printf("Manual step %d/%d: %s\nConfirm? [y/N]: ", i+1, len(steps), step)
		if !scanner.Scan() {
			return false
		}
		resp := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if resp != "y" && resp != "yes" {
			return false
		}
	}
	return true
}

func main() {
	if err := run(os.Args); err != nil {
		if errString := err.Error(); errString != "" {
			fmt.Fprintln(os.Stderr, errString)
		}
		os.Exit(1)
	}
}

// issueArtifact extracts artifact from task meta embedded in description.
func issueArtifact(iss pt.Issue) string {
	meta, err := pt.ParseTaskMeta(iss.Description)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Artifact)
}

// issueRole extracts the role from task meta or labels.
func issueRole(iss pt.Issue) string {
	meta, err := pt.ParseTaskMeta(iss.Description)
	if err == nil && meta.Role != "" {
		return meta.Role
	}
	// Fallback: check labels for role:X
	for _, l := range iss.Labels {
		if strings.HasPrefix(l, "role:") {
			return strings.TrimPrefix(l, "role:")
		}
	}
	return "unknown"
}

// issueCriteria returns a compact criteria string (semicolon-delimited) from task meta.
func issueCriteria(iss pt.Issue) string {
	meta, err := pt.ParseTaskMeta(iss.Description)
	if err != nil {
		return ""
	}
	if len(meta.DoD.Criteria) == 0 {
		return ""
	}
	return strings.Join(meta.DoD.Criteria, "; ")
}

func run(args []string) error {
	if len(args) < 2 {
		usage()
		return errors.New("")
	}
	if loadedHooks == nil {
		cfg, err := loadHooks()
		if err != nil {
			return err
		}
		loadedHooks = cfg
	}

	cmd := args[1]
	cmdArgs := args[2:]

	switch cmd {
	case "sync":
		return cmdSync(cmdArgs)
	case "ready":
		return cmdReady(cmdArgs)
	case "claim":
		return cmdClaim(cmdArgs)
	case "release":
		return cmdRelease(cmdArgs)
	case "validate":
		return cmdValidate(cmdArgs)
	case "approve":
		return cmdApprove(cmdArgs)
	case "reject":
		return cmdReject(cmdArgs)
	case "reopen":
		return cmdReopen(cmdArgs)
	case "blocked":
		return cmdBlocked(cmdArgs)
	case "unblock":
		return cmdUnblock(cmdArgs)
	case "context":
		return cmdContext(cmdArgs)
	case "worktree":
		return cmdWorktree(cmdArgs)
	case "doctor":
		return cmdDoctor(cmdArgs)
	case "export":
		return cmdExport(cmdArgs)
	case "import":
		return cmdImport(cmdArgs)
	case "graph":
		return cmdGraph(cmdArgs)
	case "list":
		return cmdList(cmdArgs)
	case "show":
		return cmdShow(cmdArgs)
	case "add":
		return cmdAdd(cmdArgs)
	case "comment":
		return cmdComment(cmdArgs)
	case "update":
		return cmdUpdate(cmdArgs)
	case "snapshot":
		return cmdSnapshot(cmdArgs)
	case "propose":
		return cmdPropose(cmdArgs)
	case "multi-ready":
		return cmdMultiReady(cmdArgs)
	case "search":
		return cmdSearch(cmdArgs)
	case "feedback":
		return cmdFeedback(cmdArgs)
	case "hooks":
		return cmdHooksPrint()
	case "history":
		return cmdHistory(cmdArgs)
	case "workflow":
		return cmdWorkflow(cmdArgs)
	case "-h", "--help", "help":
		return cmdHelp(cmdArgs)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Print(`pt CLI (store-backed)

Commands (SDLC flow):
  sync <manifest>                    Apply manifest (create/update tasks, deps)
  ready [--role=ROLE] [--limit=N]    List unblocked work (status=open)
  list  [--status=...]               List tasks by status (open|in_progress|needs_review|closed)
  show  <id> [--json]                Show task details (DoD, deps, comments)
  claim <id> [--as=USER] [--draft]   Mark in_progress and assign (--draft skips DoD check)
  release <id>                       Return to open and clear assignee
  validate <id> [--yes]              Run DoD hooks; on success -> needs_review
  approve <id>                       Mark done
  reject <id> --reason="text"        Send back to in_progress with a comment
  reopen <id> [--as=USER]            Reopen a closed task (done -> in_progress)
  blocked [<id> <reason>]            Mark task as blocked, or list all blocked
  unblock <id>                       Clear blocked status from task
  add "Title" [flags]                Create ad-hoc task (role/template required)
  update <id> [flags]                Update task fields (title/assignee/priority/next-hint)
  comment <id> "text"                Append a comment to a task
  snapshot [--out=...]               Copy the store to a timestamped file
  propose <manifest> [--db=PATH]     Show proposed adds/updates without writing
  multi-ready --dbs=a.json,b.json    Read-only ready aggregation across stores
  search --query="text"              Search titles/labels/description (read-only)
  feedback --desc=TEXT [--dry-run]   Create a feedback template under ~/.pt/feedback
  context init <id>|validate|prime    Manage agent context (prime outputs project summary)
  worktree start|done|status|abort    Manage git worktrees for task isolation
  doctor [--fix]                     Check store integrity (--fix attempts repairs)
  export [--out=FILE]                Export store to portable JSON format
  import <file> [--mode=merge|replace] Import from exported file (merge or replace)
  graph <manifest>                   Visualize manifest dependencies (cycles shown)
  hooks                              Print merged hook configuration (global + local)
  history <id>                       Show task history (created/claimed/validated/approved)
  workflow status|next|check         Workflow guidance (phases, gates, suggested actions)

Happy-path primer:
  1) pt sync phases/<file>.toml
  2) pt ready --role=<role> --verbose
  3) pt claim <id> [--as=you]
  4) do work; pt validate <id> [--yes]
  5) pt approve <id>  |  pt reject <id> --reason="..."
  6) pt release <id> (if you're stuck)

For guidance on writing good tasks: pt help task-authoring
`)
}

func cmdHelp(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	topic := args[0]
	switch topic {
	case "task-authoring":
		printTaskAuthoringHelp()
	default:
		usage()
		fmt.Printf("\nUnknown help topic: %q\n", topic)
		fmt.Println("Available topics: task-authoring")
	}
	return nil
}

func printTaskAuthoringHelp() {
	fmt.Print(`# Task Authoring Guide

Good tasks enable agents to work autonomously without project context.
This guide shows how to write tasks that are ready for handoff.

## Required Fields

Every task MUST have these fields:

  template    Type of work (backend_endpoint, frontend_component, bug_fix, etc.)
  title       Clear, action-oriented title starting with a verb
  role        Who should do this (backend-dev, frontend-dev, planner)
  artifact    Deliverable location (code:path, spec:name, doc:path)
  dod.tests   At least one automated test command
  dod.criteria Observable, pass/fail acceptance criteria

## Handoff Fields (for agent-ready tasks)

These fields help agents understand work without needing project knowledge:

  context     WHY: Problem being solved, motivation, business value
  inputs      WHERE: Files/directories to read or modify
  scope       BOUNDS: What's IN-scope vs OUT-of-scope
  reference   RELATED: Links to docs, issues, or prior work

## Good vs Bad Examples

❌ BAD: Vague, missing context
  [[tasks]]
  title = "Fix the bug"
  role = "dev"
  template = "bug_fix"
  artifact = "code:app.go"
  [tasks.dod]
  tests = ["go test"]
  criteria = ["bug fixed"]

✓ GOOD: Clear, complete, agent-ready
  [[tasks]]
  title = "Fix null pointer in user login handler"
  role = "backend-dev"
  template = "bug_fix"
  artifact = "code:pkg/auth/login.go"
  context = "Users report 500 errors on login. Stack trace shows nil dereference when session cookie is missing."
  inputs = ["pkg/auth/login.go", "pkg/auth/session.go"]
  scope = "IN: fix null check in login handler. OUT: refactor session management"
  reference = "https://github.com/example/issues/123"
  [tasks.dod]
  tests = ["go test ./pkg/auth/... -run TestLogin"]
  manual = "Test login with and without existing session cookie"
  criteria = ["No panic on missing session cookie", "Returns 401 instead of 500"]

## Task Template

Copy and customize for new tasks:

  [[tasks]]
  title = "ACTION + specific thing"
  role = "backend-dev|frontend-dev|planner"
  template = "backend_endpoint|bug_fix|refactor|discovery"
  artifact = "code:path/to/file.go"
  context = "WHY this task exists and what problem it solves"
  inputs = ["file1.go", "file2.go"]
  scope = "IN: what to do. OUT: what NOT to do"
  [tasks.dod]
  tests = ["go test ./path/... -run TestName"]
  manual = "Manual verification steps if needed"
  criteria = ["Observable outcome 1", "Observable outcome 2"]

## Review Checklist

Before syncing a manifest, verify each task:
  □ Title is action-oriented (starts with verb like Add, Fix, Update)
  □ Artifact points to specific deliverable location
  □ Tests are automated and specific (not just "go test ./...")
  □ Criteria are observable and binary (pass/fail)
  □ Context explains WHY (not just WHAT)
  □ Inputs list actual files to modify
  □ Scope has clear boundaries

Use 'pt sync --generate-reviews <manifest>' to auto-create review tasks.
`)
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	generateReviews := fs.Bool("generate-reviews", false, "generate review tasks that block implementation tasks")
	fs.Usage = func() { fmt.Println("Usage: pt sync <manifest> [--generate-reviews]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing manifest argument")
	}
	path := fs.Arg(0)
	manifest, err := pt.ParseManifest(path)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	actor := strings.TrimSpace(os.Getenv("USER"))
	preHooks, err := runHooks("pre-sync", hookPayload{Actor: actor})
	if err != nil {
		return err
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	idMap, err := client.Sync(ctx, manifest)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Generate review tasks if requested
	var reviewIDs map[string]string
	if *generateReviews {
		reviewIDs, err = generateReviewTasks(ctx, client, manifest, idMap)
		if err != nil {
			return fmt.Errorf("generate reviews: %w", err)
		}
	}

	postHooks, err := runHooks("post-sync", hookPayload{Actor: actor})
	if err != nil {
		return err
	}
	if *jsonOut {
		result := map[string]interface{}{
			"status": "ok",
			"synced": idMap,
			"hooks":  combineHooks(preHooks, postHooks),
		}
		if reviewIDs != nil {
			result["reviews"] = reviewIDs
		}
		return printJSON(result)
	}
	for title, id := range idMap {
		fmt.Printf("%s -> %s\n", title, id)
	}
	if reviewIDs != nil {
		fmt.Println("\nReview tasks generated (must be approved before implementation):")
		for title, id := range reviewIDs {
			fmt.Printf("  %s -> %s\n", title, id)
		}
	}
	path, exists := projectDoDStatus()
	if exists {
		fmt.Printf("Project DoD: %s (ensure a review/sign-off task tracks it; set PT_PROJECT_DOD to override)\n", path)
	} else {
		fmt.Printf("Project DoD missing. Create %s (or set PT_PROJECT_DOD) and add a review/sign-off task to guide completion.\n", path)
	}
	return nil
}

// generateReviewTasks creates a review task for each implementation task.
// Review tasks have role=planner and block the corresponding implementation task.
func generateReviewTasks(ctx context.Context, client pt.Client, manifest pt.Manifest, idMap map[string]string) (map[string]string, error) {
	reviewIDs := make(map[string]string)

	// Review checklist content
	reviewChecklist := `## Task Handoff Checklist

Verify this task is ready for an agent to claim:

- [ ] **Context**: Task explains WHY (problem/motivation)
- [ ] **Inputs**: Lists specific files/directories to read/modify
- [ ] **Scope**: Clear IN-scope and OUT-of-scope boundaries
- [ ] **Artifact**: Specifies deliverable type and location
- [ ] **Tests**: At least one automated test command
- [ ] **Criteria**: Observable pass/fail acceptance criteria

Use 'pt update <id> --context/--inputs/--scope' to fix gaps.
Approve this review task to unblock the implementation.`

	for _, task := range manifest.Tasks {
		implID := idMap[task.Title]
		reviewTitle := fmt.Sprintf("[Review] %s", task.Title)

		// Check if review task already exists
		if existingID, exists := idMap[reviewTitle]; exists {
			reviewIDs[reviewTitle] = existingID
			continue
		}

		// Create review task
		reviewTask := pt.Task{
			Title:    reviewTitle,
			Template: "discovery", // Use discovery template for review tasks
			Role:     "planner",
			Artifact: fmt.Sprintf("review:%s", implID),
			Context:  fmt.Sprintf("Review task for '%s' - verify handoff readiness before agents can claim", task.Title),
			DoD: pt.DefinitionOfDone{
				Tests:    []string{"echo 'Review checklist verified'"},
				Manual:   reviewChecklist,
				Criteria: []string{"All checklist items verified", "Implementation task has sufficient context for agent handoff"},
			},
		}

		// Add review task
		reviewID, err := client.AddTask(ctx, reviewTask)
		if err != nil {
			return nil, fmt.Errorf("add review for %s: %w", task.Title, err)
		}
		reviewIDs[reviewTitle] = reviewID

		// Make implementation task depend on review task
		// We need to add this dependency - but AddTask doesn't support adding deps to existing tasks
		// So we need to use a different approach: update the implementation task's deps
		// For now, we'll add the review task with the impl as a "blocks" relationship
		// Actually, the simpler approach: add dep from impl -> review in the store

		// Get the store client to add dependency directly
		if sc, ok := client.(*pt.StoreClient); ok {
			if err := sc.AddDependency(ctx, implID, reviewID); err != nil {
				return nil, fmt.Errorf("add dependency %s -> %s: %w", implID, reviewID, err)
			}
		}
	}

	return reviewIDs, nil
}

func cmdReady(args []string) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 10, "max issues")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	verbose := fs.Bool("verbose", false, "show extra info (assignee, blockers)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt ready [--role=ROLE] [--limit=N] [--sort=priority|title] [--verbose]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issues, err := client.Ready(ctx, *role, *limit)
	if err != nil {
		return fmt.Errorf("ready failed: %w", err)
	}
	pt.SortIssues(issues, *sortKey)
	if *jsonOut {
		out := []map[string]interface{}{}
		for _, iss := range issues {
			if iss.Status != "open" {
				continue
			}
			blockers := readyBlockers(ctx, client, iss)
			out = append(out, map[string]interface{}{
				"id":        iss.ID,
				"title":     iss.Title,
				"status":    iss.Status,
				"assignee":  iss.Assignee,
				"labels":    iss.Labels,
				"blockers":  blockers,
				"next_hint": iss.NextHint,
				"artifact":  issueArtifact(iss),
				"criteria":  issueCriteria(iss),
			})
		}
		return printJSON(out)
	}
	printed := false
	for _, iss := range issues {
		if iss.Status != "open" { // hide claimed/in_progress
			continue
		}
		printed = true
		blockers := readyBlockers(ctx, client, iss)

		if *verbose {
			// Multi-line block format for verbose output
			assignee := iss.Assignee
			if strings.TrimSpace(assignee) == "" {
				assignee = "(unassigned)"
			}
			fmt.Printf("### %s: %s\n", iss.ID, iss.Title)
			fmt.Printf("  Role: %s  Assignee: %s\n", issueRole(iss), assignee)

			if art := issueArtifact(iss); art != "" {
				fmt.Printf("  Artifact: %s\n", art)
			}

			// Parse metadata for context/inputs/scope
			if meta, err := pt.ParseTaskMeta(iss.Description); err == nil {
				if meta.Context != "" {
					fmt.Printf("  Context: %s\n", meta.Context)
				}
				if len(meta.Inputs) > 0 {
					fmt.Printf("  Inputs: %s\n", strings.Join(meta.Inputs, ", "))
				}
				if meta.Scope != "" {
					fmt.Printf("  Scope: %s\n", meta.Scope)
				}
			}

			if crit := issueCriteria(iss); crit != "" {
				fmt.Printf("  Criteria: %s\n", crit)
			}
			if len(blockers) > 0 {
				fmt.Printf("  ⚠️  Blocked by: %s\n", strings.Join(blockers, ", "))
			}
			if strings.TrimSpace(iss.NextHint) != "" {
				fmt.Printf("  Next: %s\n", iss.NextHint)
			}
			fmt.Println()
		} else {
			// Compact single-line format
			line := fmt.Sprintf("%s [%s] %s", iss.ID, iss.IssueType, iss.Title)
			if strings.TrimSpace(iss.Assignee) == "" {
				line = fmt.Sprintf("%s [unassigned]", line)
			}
			if len(blockers) > 0 {
				indicator := blockers[0]
				if len(blockers) > 1 {
					indicator = fmt.Sprintf("%s(+%d)", blockers[0], len(blockers)-1)
				}
				line = fmt.Sprintf("%s [blocked %s]", line, indicator)
			}
			fmt.Println(line)
		}
	}
	if !printed {
		path, exists := projectDoDStatus()
		if exists {
			fmt.Printf("No ready tasks. Review project DoD at %s (set PT_PROJECT_DOD to override). If the DoD is not satisfied, identify the gaps and add tasks (via manifest or pt add) with explicit tests + manual checks (and docs/review)—avoid shortcuts. Only ask the user when requirements are unclear or external approval is needed.\n", path)
		} else {
			fmt.Printf("No ready tasks. Add a project DoD (e.g., %s or set PT_PROJECT_DOD), then create tasks to reach that DoD using best practices: explicit tests + manual checks, docs, and review. Minimize user prompts unless requirements are unclear.\n", path)
		}
	}
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	status := fs.String("status", "open", "comma-separated statuses (open,in_progress,needs_review,closed). Empty for all.")
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 50, "max issues")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt list [--status=open,in_progress,needs_review,closed] [--role=ROLE] [--limit=N] [--json]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	var statuses []string
	if strings.TrimSpace(*status) != "" {
		for _, s := range strings.Split(*status, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				statuses = append(statuses, s)
			}
		}
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issues, err := client.List(ctx, statuses, *role, *limit)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}
	pt.SortIssues(issues, *sortKey)
	if *jsonOut {
		return printJSON(issues)
	}
	for _, iss := range issues {
		line := fmt.Sprintf("%s [%s] %s status=%s", iss.ID, iss.IssueType, iss.Title, iss.Status)
		if strings.TrimSpace(iss.Assignee) == "" {
			line = fmt.Sprintf("%s [unassigned]", line)
		} else {
			line = fmt.Sprintf("%s @%s", line, iss.Assignee)
		}
		if strings.TrimSpace(iss.NextHint) != "" {
			line = fmt.Sprintf("%s next:%s", line, iss.NextHint)
		}
		fmt.Println(line)
	}
	return nil
}

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt show <id> [--json]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return err
	}
	deps, _ := client.Dependencies(ctx, id)
	comments, _ := client.Comments(ctx, id)
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"id":          iss.ID,
			"title":       iss.Title,
			"status":      iss.Status,
			"assignee":    iss.Assignee,
			"labels":      iss.Labels,
			"next_hint":   iss.NextHint,
			"meta":        meta,
			"deps":        deps,
			"comments":    comments,
			"description": iss.Description,
		})
	}
	fmt.Printf("%s [%s]\n", iss.ID, iss.Title)
	fmt.Printf("Status: %s\n", iss.Status)
	if iss.Assignee != "" {
		fmt.Printf("Assignee: %s\n", iss.Assignee)
	} else {
		fmt.Println("Assignee: (unassigned)")
	}
	if iss.NextHint != "" {
		fmt.Printf("Next: %s\n", iss.NextHint)
	}
	fmt.Printf("Role: %s  Template: %s\n", meta.Role, meta.Template)
	if strings.TrimSpace(meta.Artifact) != "" {
		fmt.Printf("Artifact: %s\n", meta.Artifact)
	}
	if len(iss.Labels) > 0 {
		fmt.Printf("Labels: %s\n", strings.Join(iss.Labels, ","))
	}
	if len(deps) > 0 {
		var parts []string
		for _, d := range deps {
			parts = append(parts, fmt.Sprintf("%s(%s)", d.ID, d.Status))
		}
		fmt.Printf("Deps: %s\n", strings.Join(parts, ", "))
	}
	fmt.Println("DoD:")
	if len(meta.DoD.Tests) > 0 {
		fmt.Printf("  tests: %s\n", strings.Join(meta.DoD.Tests, ", "))
	}
	if meta.DoD.ValidationCmd != "" {
		fmt.Printf("  validation_cmd: %s\n", meta.DoD.ValidationCmd)
	}
	if meta.DoD.Manual != "" {
		fmt.Printf("  manual: %s\n", meta.DoD.Manual)
	}
	if len(meta.DoD.Criteria) > 0 {
		fmt.Printf("  criteria: %s\n", strings.Join(meta.DoD.Criteria, "; "))
	}
	if len(comments) > 0 {
		fmt.Println("Comments:")
		for _, c := range comments {
			fmt.Printf("  - %s\n", c)
		}
	}
	return nil
}

func cmdClaim(args []string) error {
	args = reorderArgs(args) // Allow flags after positional args
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee (defaults to $USER)")
	draft := fs.Bool("draft", false, "claim in draft mode (skip DoD validation)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt claim <id> [--as=USER] [--draft]") }
	// Parse flags first, then get positional arg
	if err := fs.Parse(args); err != nil {
		return err
	}
	id := ""
	if fs.NArg() > 0 {
		id = fs.Arg(0)
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if id == "" {
		fs.Usage()
		return errors.New("missing id argument")
	}
	user, err := requireUser(*as)
	if err != nil {
		return err
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   user,
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-claim", payload)
	if err != nil {
		return err
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Claim(ctx, id, user); err != nil {
		return fmt.Errorf("claim failed: %w", err)
	}
	// Add draft label if --draft flag is set
	if *draft {
		if err := client.AddLabels(ctx, id, "state:draft"); err != nil {
			return fmt.Errorf("add draft label: %w", err)
		}
	}
	postHooks, err := runHooks("post-claim", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":   "ok",
			"id":       id,
			"assignee": user,
			"draft":    *draft,
			"hooks":    combineHooks(preHooks, postHooks),
		})
	}
	if *draft {
		fmt.Printf("Claimed %s as %s (draft mode - DoD not enforced until validate)\n", id, user)
	} else {
		fmt.Printf("Claimed %s as %s\n", id, user)
	}

	// Suggest worktree if in a git repo and task doesn't have one
	if isGitRepo() {
		if _, hasWT, _ := client.GetWorktree(ctx, id); !hasWT {
			fmt.Printf("\n💡 Start a worktree for isolated work:\n")
			fmt.Printf("   pt worktree start %s\n", id)
		}
	} else {
		fmt.Printf("\n⚠️  Not a git repo. Consider: git init && git add -A && git commit -m \"initial\"\n")
		fmt.Printf("   Worktrees require git for branch isolation.\n")
	}

	return nil
}

// isGitRepoAt checks if the given path is in a git repository.
// If path is empty, uses current working directory.
func isGitRepoAt(path string) bool {
	var cmd *exec.Cmd
	if path != "" {
		cmd = exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	} else {
		cmd = exec.Command("git", "rev-parse", "--git-dir")
	}
	return cmd.Run() == nil
}

// isGitRepo checks if current directory is in a git repository.
// Deprecated: prefer isGitRepoAt with explicit path.
func isGitRepo() bool {
	return isGitRepoAt("")
}

func cmdRelease(args []string) error {
	args = reorderArgs(args) // Allow flags after positional args
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee check (defaults to $USER)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt release <id> [--as=USER]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	user, err := requireUser(*as)
	if err != nil {
		return err
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   "",
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   "open",
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Release(ctx, id, user); err != nil {
		return fmt.Errorf("release failed: %w", err)
	}
	postHooks, err := runHooks("post-release", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":   "ok",
			"id":       id,
			"assignee": "",
			"hooks":    postHooks,
		})
	}
	fmt.Printf("Released %s\n", id)
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "auto-confirm manual checks")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt validate <id> [--yes]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}

	// Check if this is a draft task
	isDraft := false
	for _, label := range issue.Labels {
		if label == "state:draft" {
			isDraft = true
			break
		}
	}

	// If draft, check if DoD is incomplete and warn
	if isDraft {
		dodIncomplete := len(meta.DoD.Tests) == 0 || strings.TrimSpace(meta.DoD.Manual) == "" || len(meta.DoD.Criteria) == 0
		if dodIncomplete {
			fmt.Println("WARNING: This is a draft task with incomplete DoD:")
			if len(meta.DoD.Tests) == 0 {
				fmt.Println("  - No tests defined")
			}
			if strings.TrimSpace(meta.DoD.Manual) == "" {
				fmt.Println("  - No manual steps defined")
			}
			if len(meta.DoD.Criteria) == 0 {
				fmt.Println("  - No acceptance criteria defined")
			}
			if !*yes {
				fmt.Print("Continue with validation anyway? [y/N]: ")
				scanner := bufio.NewScanner(os.Stdin)
				if !scanner.Scan() {
					return errors.New("validation cancelled")
				}
				resp := strings.TrimSpace(strings.ToLower(scanner.Text()))
				if resp != "y" && resp != "yes" {
					return errors.New("validation cancelled - please complete DoD first")
				}
			}
		}
		// Remove draft label since we're proceeding with validation
		_ = client.RemoveLabels(ctx, id, "state:draft")
	}

	dodJSON, _ := json.Marshal(meta.DoD)
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusNeedsReview),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-validate", payload)
	if err != nil {
		return err
	}

	// Determine working directory for test execution
	// If task has an active worktree, run tests there; otherwise use current directory
	workDir := ""
	if wtInfo, hasWT, _ := client.GetWorktree(ctx, id); hasWT {
		workDir = wtInfo.Path
		fmt.Printf("Running tests in worktree: %s\n", workDir)
	}

	manualSteps := splitManualSteps(meta.DoD.Manual)
	confirm := confirmManual(manualSteps, *yes)
	// Pass working directory to runner instead of using os.Chdir
	vr := pt.ValidationRunner{Runner: pt.ExecRunner{Dir: workDir}}
	res, err := vr.ValidateDoD(ctx, meta.DoD, confirm)
	fmt.Print(res.Output)
	if err != nil || !res.Passed {
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		return errors.New("validation failed")
	}
	trans := pt.Transitioner{Client: client}
	comment := "Validation passed; ready for review"
	if len(manualSteps) > 0 {
		comment = fmt.Sprintf("Validation passed; manual steps confirmed: %s", strings.Join(manualSteps, "; "))
	}
	if isDraft {
		comment = fmt.Sprintf("%s (was draft)", comment)
	}
	if err := trans.SubmitForReview(ctx, issue.ID, comment); err != nil {
		return fmt.Errorf("mark needs_review failed: %w", err)
	}
	postHooks, err := runHooks("post-validate", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":           "ok",
			"id":               issue.ID,
			"new_status":       string(pt.StatusNeedsReview),
			"manual_confirmed": confirm,
			"was_draft":        isDraft,
			"hooks":            combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Task %s marked needs_review\n", issue.ID)
	return nil
}

func cmdApprove(args []string) error {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt approve <id>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   "closed",
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-approve", payload)
	if err != nil {
		return err
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Approve(ctx, id); err != nil {
		return fmt.Errorf("approve failed: %w", err)
	}
	postHooks, err := runHooks("post-approve", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":     "ok",
			"id":         id,
			"new_status": "closed",
			"hooks":      combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Approved %s\n", id)

	// Show next unblocked open tasks (deps satisfied, not manually blocked).
	issues, err := listReadyOpen(ctx, client, "", 5)
	if err == nil && len(issues) > 0 {
		fmt.Println("\nReady Work:")
		for _, iss := range issues {
			fmt.Printf("%s [%s] %s\n", iss.ID, iss.IssueType, iss.Title)
		}
	}
	return nil
}

func cmdReject(args []string) error {
	fs := flag.NewFlagSet("reject", flag.ContinueOnError)
	reason := fs.String("reason", "", "reason for rejection")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt reject <id> --reason=\"text\"") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("reason is required")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Reject(ctx, id, *reason); err != nil {
		return fmt.Errorf("reject failed: %w", err)
	}
	postHooks, err := runHooks("post-reject", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":     "ok",
			"id":         id,
			"new_status": string(pt.StatusInProgress),
			"reason":     *reason,
			"hooks":      postHooks,
		})
	}
	fmt.Printf("Rejected %s\n", id)
	return nil
}

func cmdReopen(args []string) error {
	args = reorderArgs(args) // Allow flags after positional args
	fs := flag.NewFlagSet("reopen", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee (defaults to $USER)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt reopen <id> [--as=USER]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	user, err := requireUser(*as)
	if err != nil {
		return err
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   user,
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-reopen", payload)
	if err != nil {
		return err
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Reopen(ctx, id, user); err != nil {
		return fmt.Errorf("reopen failed: %w", err)
	}
	postHooks, err := runHooks("post-reopen", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":     "ok",
			"id":         id,
			"assignee":   user,
			"new_status": string(pt.StatusInProgress),
			"hooks":      combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Reopened %s as %s\n", id, user)
	return nil
}

func cmdBlocked(args []string) error {
	fs := flag.NewFlagSet("blocked", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt blocked [<id> <reason>]")
		fmt.Println("  Without args: list all blocked tasks")
		fmt.Println("  With id and reason: mark task as blocked")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get positional args
	var id, reason string
	if fs.NArg() > 0 {
		id = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		reason = strings.Join(fs.Args()[1:], " ")
	}

	// If no ID, list all blocked
	if id == "" {
		blocked, err := client.ListBlocked(ctx)
		if err != nil {
			return fmt.Errorf("list blocked: %w", err)
		}
		if *jsonOut {
			return printJSON(blocked)
		}
		if len(blocked) == 0 {
			fmt.Println("No blocked tasks")
			return nil
		}
		for taskID, info := range blocked {
			iss, _, _ := client.GetTask(ctx, taskID)
			line := fmt.Sprintf("%s [%s] blocked: %s", taskID, iss.Title, info.Reason)
			if info.BlockedBy != "" {
				line += fmt.Sprintf(" (by %s)", info.BlockedBy)
			}
			fmt.Println(line)
		}
		return nil
	}

	// Mark as blocked
	if reason == "" {
		return errors.New("reason required when marking task as blocked")
	}
	actor := strings.TrimSpace(os.Getenv("USER"))
	if err := client.SetBlocked(ctx, id, reason, actor); err != nil {
		return fmt.Errorf("set blocked: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]string{"status": "ok", "id": id, "blocked": "true", "reason": reason})
	}
	fmt.Printf("Marked %s as blocked: %s\n", id, reason)
	return nil
}

func cmdUnblock(args []string) error {
	fs := flag.NewFlagSet("unblock", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt unblock <id>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.ClearBlocked(ctx, id); err != nil {
		return fmt.Errorf("clear blocked: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]string{"status": "ok", "id": id, "blocked": "false"})
	}
	fmt.Printf("Unblocked %s\n", id)
	return nil
}

func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	role := fs.String("role", "", "role label (required)")
	template := fs.String("template", "", "task template (required)")
	artifact := fs.String("artifact", "", "artifact implemented (API/UI spec/ADR) (required)")
	manual := fs.String("manual", "", "manual DoD text (required)")
	tests := fs.String("tests", "", "comma-separated test commands (required)")
	criteria := fs.String("criteria", "", "comma-separated acceptance criteria (required)")
	validation := fs.String("validation-cmd", "", "validation command")
	deps := fs.String("deps", "", "comma-separated dependency IDs")
	nextHint := fs.String("next-hint", "", "suggested next task")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt add \"Title\" --role=... --template=... --artifact=... --manual=... --tests=... --criteria=... [--validation-cmd=...] [--deps=...] [--next-hint=...]")
	}
	var title string
	var flagArgs []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && title == "" {
			title = a
			continue
		}
		flagArgs = append(flagArgs, a)
	}
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if title == "" {
		if fs.NArg() > 0 {
			title = fs.Arg(0)
		} else {
			fs.Usage()
			return errors.New("missing title argument")
		}
	}
	if strings.TrimSpace(*role) == "" || strings.TrimSpace(*template) == "" {
		return errors.New("role and template are required")
	}
	if strings.TrimSpace(*artifact) == "" {
		return errors.New("artifact is required (link to API/UI/ADR)")
	}
	var depList []string
	if strings.TrimSpace(*deps) != "" {
		for _, d := range strings.Split(*deps, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				depList = append(depList, d)
			}
		}
	}
	var testList []string
	if strings.TrimSpace(*tests) != "" {
		for _, t := range strings.Split(*tests, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				testList = append(testList, t)
			}
		}
	}
	var criteriaList []string
	if strings.TrimSpace(*criteria) != "" {
		for _, c := range strings.Split(*criteria, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				criteriaList = append(criteriaList, c)
			}
		}
	}
	dod := pt.DefinitionOfDone{
		Tests:         testList,
		ValidationCmd: *validation,
		Manual:        *manual,
		Criteria:      criteriaList,
	}
	if len(dod.Tests) == 0 {
		return errors.New("definition of done requires tests (--tests)")
	}
	if strings.TrimSpace(dod.Manual) == "" {
		return errors.New("definition of done requires manual instructions (--manual)")
	}
	if len(dod.Criteria) == 0 {
		return errors.New("definition of done requires acceptance criteria (--criteria)")
	}
	task := pt.Task{
		Title:    title,
		Template: *template,
		Role:     *role,
		Artifact: *artifact,
		Deps:     depList,
		NextHint: *nextHint,
		DoD:      dod,
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := client.AddTask(ctx, task)
	if err != nil {
		return fmt.Errorf("add task failed: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]string{"status": "ok", "id": id})
	}
	fmt.Printf("Created %s\n", id)
	printTaskAuthoringChecklist()
	return nil
}

// printTaskAuthoringChecklist reminds authors to make tasks self-contained for low-context agents.
func printTaskAuthoringChecklist() {
	fmt.Println("Task authoring checklist (self-contained; assume no extra context):")
	fmt.Println("- Include artifact/spec link and affected files/modules.")
	fmt.Println("- State exact commands to run (tests, validation) and manual steps to perform.")
	fmt.Println("- Add acceptance criteria bullets (what proof/outcome is needed).")
	fmt.Println("- Note dependencies and next likely task if applicable.")
	fmt.Println("- Record repro/symptoms for bugs; for features, link UI/API contracts.")
	fmt.Println("- Leave a short status comment if you stop early so others can continue.")
}

func cmdFeedback(args []string) error {
	fs := flag.NewFlagSet("feedback", flag.ContinueOnError)
	desc := fs.String("desc", "", "short description for filename (e.g., ce-ui)")
	dir := fs.String("dir", "", "override feedback directory (default PT_FEEDBACK_DIR or ~/.pt/feedback)")
	dryRun := fs.Bool("dry-run", false, "print path/template without writing a file")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Println("Usage: pt feedback --desc=TEXT [--dir=PATH] [--dry-run]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*desc) == "" {
		fs.Usage()
		return errors.New("description is required (example: --desc=\"ce-ui\")")
	}
	baseDir, err := feedbackBaseDir(*dir)
	if err != nil {
		return err
	}
	path := filepath.Join(baseDir, feedbackFilename(*desc, time.Now()))
	template := feedbackTemplate()
	if *dryRun {
		if *jsonOut {
			return printJSON(map[string]interface{}{
				"status":   "ok",
				"path":     path,
				"written":  false,
				"template": template,
			})
		}
		fmt.Printf("Feedback path: %s\n", path)
		fmt.Println(template)
		return nil
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create feedback dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("feedback file already exists: %s", path)
	}
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		return fmt.Errorf("write feedback file: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":   "ok",
			"path":     path,
			"written":  true,
			"template": template,
		})
	}
	fmt.Printf("Feedback template created at %s\n", path)
	return nil
}

func cmdComment(args []string) error {
	fs := flag.NewFlagSet("comment", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt comment <id> \"text\"") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return errors.New("requires id and comment text")
	}
	id := fs.Arg(0)
	body := strings.TrimSpace(strings.Join(fs.Args()[1:], " "))
	if body == "" {
		return errors.New("comment text required")
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.AddComment(ctx, id, body); err != nil {
		return fmt.Errorf("comment failed: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]string{"status": "ok", "id": id})
	}
	fmt.Printf("Comment added to %s\n", id)
	return nil
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	title := fs.String("title", "", "new title")
	assignee := fs.String("assignee", "", "new assignee")
	priority := fs.Int("priority", -1, "new priority (0-5)")
	nextHint := fs.String("next-hint", "", "new next hint")
	// Handoff fields
	contextFlag := fs.String("context", "", "task context (WHY)")
	inputsFlag := fs.String("inputs", "", "comma-separated input files/dirs (WHERE)")
	scopeFlag := fs.String("scope", "", "IN/OUT scope (BOUNDS)")
	referenceFlag := fs.String("reference", "", "related links (RELATED)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt update <id> [--title=...] [--assignee=...] [--priority=N] [--next-hint=...]")
		fmt.Println("       Handoff fields: [--context=...] [--inputs=file1,file2] [--scope=...] [--reference=...]")
		fmt.Println("       Use '-' to clear a field (e.g., --context=-)")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	opts := pt.UpdateOptions{
		Title:     *title,
		Assignee:  *assignee,
		NextHint:  *nextHint,
		Context:   *contextFlag,
		Scope:     *scopeFlag,
		Reference: *referenceFlag,
	}
	// Parse comma-separated inputs
	if *inputsFlag != "" {
		if *inputsFlag == "-" {
			opts.Inputs = []string{"-"}
		} else {
			opts.Inputs = strings.Split(*inputsFlag, ",")
		}
	}
	if *priority >= 0 {
		opts.Priority = priority
	}
	hasUpdate := opts.Title != "" || opts.Assignee != "" || opts.Priority != nil || opts.NextHint != "" ||
		opts.Context != "" || len(opts.Inputs) > 0 || opts.Scope != "" || opts.Reference != ""
	if !hasUpdate {
		return errors.New("at least one update flag required (--title, --assignee, --priority, --next-hint, --context, --inputs, --scope, --reference)")
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Get task before update for hooks
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:       issue.ID,
		Title:    issue.Title,
		Assignee: issue.Assignee,
		Actor:    actor,
		Role:     meta.Role,
	}
	preHooks, err := runHooks("pre-update", payload)
	if err != nil {
		return err
	}
	if err := client.UpdateTask(ctx, id, opts); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	postHooks, err := runHooks("post-update", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status": "ok",
			"id":     id,
			"hooks":  combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Updated %s\n", id)
	return nil
}

func cmdSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	out := fs.String("out", "", "output file path (optional)")
	dbPathFlag := fs.String("db", "", "override store path")
	fs.Usage = func() { fmt.Println("Usage: pt snapshot [--out=path]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	dbPath := *dbPathFlag
	if strings.TrimSpace(dbPath) == "" {
		dbPath = os.Getenv("PT_DB")
	}
	if strings.TrimSpace(dbPath) == "" {
		dbPath = ".pt.db.json"
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("store not found at %s", dbPath)
	}
	target := *out
	if strings.TrimSpace(target) == "" {
		target = fmt.Sprintf("%s.snap.%s.json", dbPath, time.Now().Format("20060102-150405"))
	}
	if err := copyFile(dbPath, target); err != nil {
		return fmt.Errorf("snapshot failed: %w", err)
	}
	fmt.Printf("Snapshot written to %s\n", target)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func cmdPropose(args []string) error {
	fs := flag.NewFlagSet("propose", flag.ContinueOnError)
	dbPath := fs.String("db", "", "target store path")
	prefix := fs.String("prefix", "", "override issue prefix when interpreting manifest")
	jsonOut := fs.Bool("json", true, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt propose <manifest> [--db=path] [--prefix=pfx]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing manifest argument")
	}
	manifestPath := fs.Arg(0)
	manifest, err := pt.ParseManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	existing, err := client.List(ctx, nil, "", 0) // all
	if err != nil {
		return fmt.Errorf("list existing: %w", err)
	}
	titleToIssue := make(map[string]pt.Issue)
	for _, iss := range existing {
		titleToIssue[iss.Title] = iss
	}
	var adds []pt.Task
	var updates []map[string]interface{}
	for _, task := range manifest.Tasks {
		if iss, ok := titleToIssue[task.Title]; ok {
			_, meta, _ := client.GetTask(ctx, iss.ID)
			changes := diffTask(task, iss, meta)
			if len(changes) > 0 {
				updates = append(updates, map[string]interface{}{
					"id":      iss.ID,
					"title":   iss.Title,
					"changes": changes,
				})
			}
		} else {
			adds = append(adds, task)
		}
	}
	out := map[string]interface{}{
		"target_db": *dbPath,
		"adds":      adds,
		"updates":   updates,
		"notes":     []string{"proposal only; no writes performed", "prefix will be " + defaultPrefix(*prefix)},
	}
	if *jsonOut {
		return printJSON(out)
	}
	fmt.Printf("Adds: %d Updates: %d\n", len(adds), len(updates))
	return nil
}

func defaultPrefix(pfx string) string {
	if strings.TrimSpace(pfx) == "" {
		return "pt"
	}
	return pfx
}

func diffTask(task pt.Task, iss pt.Issue, meta pt.TaskMeta) map[string]interface{} {
	changes := make(map[string]interface{})
	if task.Role != meta.Role {
		changes["role"] = map[string]string{"from": meta.Role, "to": task.Role}
	}
	if task.Template != meta.Template {
		changes["template"] = map[string]string{"from": meta.Template, "to": task.Template}
	}
	if task.NextHint != meta.NextHint {
		changes["next_hint"] = map[string]string{"from": meta.NextHint, "to": task.NextHint}
	}
	if !equalStringSlices(task.Deps, nil) { // compare by titles? we only have IDs; best-effort on meta not storing deps
		changes["deps"] = task.Deps
	}
	// DoD diffs (shallow)
	if strings.Join(task.DoD.Tests, ",") != strings.Join(meta.DoD.Tests, ",") {
		changes["dod.tests"] = map[string]string{"from": strings.Join(meta.DoD.Tests, ","), "to": strings.Join(task.DoD.Tests, ",")}
	}
	if task.DoD.ValidationCmd != meta.DoD.ValidationCmd {
		changes["dod.validation_cmd"] = map[string]string{"from": meta.DoD.ValidationCmd, "to": task.DoD.ValidationCmd}
	}
	if task.DoD.Manual != meta.DoD.Manual {
		changes["dod.manual"] = map[string]string{"from": meta.DoD.Manual, "to": task.DoD.Manual}
	}
	return changes
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cmdMultiReady(args []string) error {
	fs := flag.NewFlagSet("multi-ready", flag.ContinueOnError)
	dbs := fs.String("dbs", "", "comma-separated store paths")
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 10, "max issues per store")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt multi-ready --dbs=a.json,b.json [--role=ROLE] [--limit=N] [--json]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dbs) == "" {
		fs.Usage()
		return errors.New("missing --dbs")
	}
	var results []map[string]interface{}
	for _, db := range strings.Split(*dbs, ",") {
		db = strings.TrimSpace(db)
		if db == "" {
			continue
		}
		client := newClientWith(db, "")
		ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
		issues, err := client.Ready(ctx, *role, *limit)
		cancel()
		if err != nil {
			return fmt.Errorf("ready for %s: %w", db, err)
		}
		pt.SortIssues(issues, *sortKey)
		for _, iss := range issues {
			if iss.Status != "open" {
				continue
			}
			meta, _ := pt.ParseTaskMeta(iss.Description)
			results = append(results, map[string]interface{}{
				"db":        db,
				"id":        iss.ID,
				"title":     iss.Title,
				"assignee":  iss.Assignee,
				"status":    iss.Status,
				"next_hint": iss.NextHint,
				"role":      meta.Role,
				"artifact":  meta.Artifact,
			})
		}
	}
	if *jsonOut {
		return printJSON(results)
	}
	for _, r := range results {
		line := fmt.Sprintf("%s %s %s [%s]", r["db"], r["id"], r["title"], r["status"])
		if r["assignee"] == "" {
			line += " [unassigned]"
		} else {
			line += fmt.Sprintf(" @%s", r["assignee"])
		}
		if art, ok := r["artifact"].(string); ok && strings.TrimSpace(art) != "" {
			line += fmt.Sprintf(" artifact:%s", art)
		}
		if roleVal, ok := r["role"].(string); ok && strings.TrimSpace(roleVal) != "" {
			line += fmt.Sprintf(" role:%s", roleVal)
		}
		if nh, ok := r["next_hint"].(string); ok && strings.TrimSpace(nh) != "" {
			line += fmt.Sprintf(" next:%s", nh)
		}
		fmt.Println(line)
	}
	return nil
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	query := fs.String("query", "", "query text")
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 20, "max results")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Println("Usage: pt search --query=\"text\" [--role=ROLE] [--limit=N] [--db=path] [--json]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*query) == "" {
		fs.Usage()
		return errors.New("missing --query")
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, err := client.Search(ctx, pt.SearchOptions{
		Query: *query,
		Role:  *role,
		Limit: *limit,
	})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if *jsonOut {
		return printJSON(results)
	}
	for _, r := range results {
		line := fmt.Sprintf("%s [%s] %s (match:%s)", r.Issue.ID, r.Issue.IssueType, r.Issue.Title, r.Match)
		fmt.Println(line)
	}
	return nil
}

func cmdContext(args []string) error {
	if len(args) < 1 {
		return errors.New("Usage: pt context <init|validate|prime> [args]")
	}
	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "init":
		return cmdContextInit(subArgs)
	case "validate":
		return cmdContextValidate(subArgs)
	case "prime":
		return cmdContextPrime(subArgs)
	default:
		return fmt.Errorf("unknown context command: %s", sub)
	}
}

func parseContextInitArgs(args []string) (string, string, error) {
	var id string
	var role string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--role="):
			role = strings.TrimPrefix(arg, "--role=")
		case arg == "--role":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--role requires a value")
			}
			role = args[i+1]
			i++
		case strings.HasPrefix(arg, "-"):
			return "", "", fmt.Errorf("unknown flag %s", arg)
		default:
			if id == "" {
				id = arg
			} else {
				return "", "", fmt.Errorf("Usage: pt context init <id> [--role=ROLE]")
			}
		}
	}
	if id == "" {
		return "", "", fmt.Errorf("Usage: pt context init <id> [--role=ROLE]")
	}
	return id, role, nil
}

func cmdContextInit(args []string) error {
	id, roleFlag, err := parseContextInitArgs(args)
	if err != nil {
		fmt.Println("Usage: pt context init <id> [--role=ROLE]")
		return err
	}

	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	targetRole := meta.Role
	if roleFlag != "" {
		targetRole = roleFlag
	}
	if targetRole == "" {
		return errors.New("no role found in task or flag")
	}

	contractPath := fmt.Sprintf("contracts/%s.toml", targetRole)
	contract, err := projects_tasks_pkg_contract.Load(contractPath)
	if err != nil {
		return fmt.Errorf("load contract %s: %w", contractPath, err)
	}
	_ = contract

	// Scaffold payload
	payload := map[string]any{
		"goal": map[string]any{
			"prompt":      issue.Description, // simplistic mapping
			"description": issue.Description,
		},
		"scope": map[string]any{
			"files": []string{},
		},
		"success": map[string]any{
			"criteria": meta.DoD.Tests,
			"tests":    meta.DoD.Tests,
		},
		"provenance": map[string]any{
			"inputs": []map[string]string{
				{"field": "goal.prompt", "source": fmt.Sprintf("pt:%s", id)},
			},
			"issued_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	out, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(out))
	return nil
}

func cmdContextValidate(args []string) error {
	fs := flag.NewFlagSet("context validate", flag.ContinueOnError)
	contractPath := fs.String("contract", "", "path to contract TOML (optional, defaults to contracts/<role>.toml if role key exists in payload)")
	fs.Usage = func() { fmt.Println("Usage: pt context validate <context.json> [--contract=PATH]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing context file")
	}
	payloadPath := fs.Arg(0)

	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return fmt.Errorf("read payload: %w", err)
	}

	// Determine contract path
	cPath := *contractPath
	if cPath == "" {
		role, roleErr := extractRoleFromPayload(data)
		if roleErr != nil {
			return fmt.Errorf("--contract flag is required (and role not found in payload): %w", roleErr)
		}
		cPath = fmt.Sprintf("contracts/%s.toml", role)
	}

	cPath = resolveContractPath(cPath)

	contract, err := projects_tasks_pkg_contract.Load(cPath)
	if err != nil {
		return fmt.Errorf("load contract: %w", err)
	}

	if err := projects_tasks_pkg_contract.ValidatePayload(data, contract); err != nil {
		fmt.Fprintln(os.Stderr, "VALIDATION FAILED")
		return err
	}
	fmt.Println("Context is valid.")
	return nil
}

func extractRoleFromPayload(data []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	if role, ok := payload["role"].(string); ok && strings.TrimSpace(role) != "" {
		return role, nil
	}
	if metaRaw, ok := payload["meta"]; ok {
		if metaMap, ok := metaRaw.(map[string]any); ok {
			if role, ok := metaMap["role"].(string); ok && strings.TrimSpace(role) != "" {
				return role, nil
			}
		}
	}
	return "", errors.New("role not found in payload")
}

func resolveContractPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	candidates := []string{
		path,
		filepath.Join("..", path),
		filepath.Join("..", "..", path),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return path
}

func cmdGraph(args []string) error {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: pt graph <manifest>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing manifest argument")
	}
	path := fs.Arg(0)
	manifest, err := pt.ParseManifest(path)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	fmt.Print(pt.RenderManifestTree(manifest))
	return nil
}

func cmdContextPrime(args []string) error {
	fs := flag.NewFlagSet("context prime", flag.ContinueOnError)
	roleFilter := fs.String("role", "", "filter tasks by role")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	manifestPath := fs.String("manifest", "", "path to manifest for project metadata")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt context prime [--role=ROLE] [--manifest=PATH] [--db=PATH] [--prefix=pt] [--json]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Load manifest for project metadata if provided
	var project pt.ProjectInfo
	if *manifestPath != "" {
		if m, err := pt.ParseManifest(*manifestPath); err == nil {
			project = m.Project
		}
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all tasks
	allTasks, err := client.List(ctx, nil, "", 100)
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}

	// Categorize tasks by status
	var openTasks, inProgress, needsReview, doneTasks []pt.Issue
	for _, t := range allTasks {
		if *roleFilter != "" {
			hasRole := false
			for _, l := range t.Labels {
				if l == fmt.Sprintf("role:%s", *roleFilter) {
					hasRole = true
					break
				}
			}
			if !hasRole {
				continue
			}
		}
		switch t.Status {
		case "open":
			openTasks = append(openTasks, t)
		case "in_progress":
			inProgress = append(inProgress, t)
		case "needs_review":
			needsReview = append(needsReview, t)
		case "closed":
			doneTasks = append(doneTasks, t)
		}
	}

	// Get blocked tasks
	blockedMap, _ := client.ListBlocked(ctx)

	// Get ready tasks (unblocked open tasks)
	var readyTasks []pt.Issue
	trans := pt.Transitioner{Client: client}
	for _, t := range openTasks {
		if _, blocked := blockedMap[t.ID]; blocked {
			continue
		}
		// Check if dependencies are satisfied
		deps, _ := client.Dependencies(ctx, t.ID)
		allDone := true
		for _, d := range deps {
			if d.Status != "closed" {
				allDone = false
				break
			}
		}
		if allDone {
			readyTasks = append(readyTasks, t)
		}
	}
	_ = trans // silence unused

	// Get store path info
	storePath := strings.TrimSpace(*dbPath)
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
		if storePath == "" {
			storePath = ".pt.db.json"
		}
	}

	// Get worktree info
	worktrees, _ := client.ListWorktrees(ctx)

	if *jsonOut {
		// Build worktree status list
		type wtStatus struct {
			TaskID    string `json:"task_id"`
			TaskTitle string `json:"task_title,omitempty"`
			Path      string `json:"path"`
			Branch    string `json:"branch"`
			Exists    bool   `json:"exists"`
		}
		var wtList []wtStatus
		for taskID, wt := range worktrees {
			title := ""
			if iss, _, err := client.GetTask(ctx, taskID); err == nil {
				title = iss.Title
			}
			_, pathErr := os.Stat(wt.Path)
			wtList = append(wtList, wtStatus{
				TaskID:    taskID,
				TaskTitle: title,
				Path:      wt.Path,
				Branch:    wt.Branch,
				Exists:    pathErr == nil,
			})
		}

		// Build enriched ready tasks with full context
		type enrichedTask struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			Role      string   `json:"role,omitempty"`
			Template  string   `json:"template,omitempty"`
			Artifact  string   `json:"artifact,omitempty"`
			Context   string   `json:"context,omitempty"`
			Inputs    []string `json:"inputs,omitempty"`
			Scope     string   `json:"scope,omitempty"`
			Reference string   `json:"reference,omitempty"`
		}
		var enrichedReady []enrichedTask
		for _, t := range readyTasks {
			et := enrichedTask{ID: t.ID, Title: t.Title}
			if meta, err := pt.ParseTaskMeta(t.Description); err == nil {
				et.Role = meta.Role
				et.Template = meta.Template
				et.Artifact = meta.Artifact
				et.Context = meta.Context
				et.Inputs = meta.Inputs
				et.Scope = meta.Scope
				et.Reference = meta.Reference
			}
			enrichedReady = append(enrichedReady, et)
		}

		summary := map[string]interface{}{
			"store_path":   storePath,
			"project":      project,
			"total":        len(allTasks),
			"open":         len(openTasks),
			"in_progress":  len(inProgress),
			"needs_review": len(needsReview),
			"done":         len(doneTasks),
			"blocked":      len(blockedMap),
			"ready":        len(readyTasks),
			"ready_tasks":  enrichedReady,
			"worktrees":    wtList,
		}
		return printJSON(summary)
	}

	// Human-readable output
	fmt.Println("# Project Status Summary")
	if project.Summary != "" {
		fmt.Printf("**Project:** %s\n", project.Summary)
		if len(project.Structure) > 0 {
			fmt.Printf("**Structure:** %s\n", strings.Join(project.Structure, ", "))
		}
		fmt.Println()
	}
	fmt.Printf("Store: %s\n", storePath)
	fmt.Println()
	fmt.Printf("**Tasks:** %d total (%d open, %d in-progress, %d needs-review, %d done)\n",
		len(allTasks), len(openTasks), len(inProgress), len(needsReview), len(doneTasks))
	fmt.Printf("**Blocked:** %d tasks\n", len(blockedMap))
	fmt.Printf("**Ready:** %d tasks\n", len(readyTasks))
	fmt.Println()

	if len(inProgress) > 0 {
		fmt.Println("## In Progress")
		for _, t := range inProgress {
			assignee := t.Assignee
			if assignee == "" {
				assignee = "(unassigned)"
			}
			// Check if task has an active worktree
			wtNote := ""
			if wt, hasWT := worktrees[t.ID]; hasWT {
				if _, err := os.Stat(wt.Path); err == nil {
					wtNote = fmt.Sprintf(" (worktree: %s)", wt.Branch)
				} else {
					wtNote = " (worktree: MISSING)"
				}
			}
			fmt.Printf("- %s: %s [%s]%s\n", t.ID, t.Title, assignee, wtNote)
		}
		fmt.Println()
	}

	if len(needsReview) > 0 {
		fmt.Println("## Needs Review")
		for _, t := range needsReview {
			fmt.Printf("- %s: %s\n", t.ID, t.Title)
		}
		fmt.Println()
	}

	if len(readyTasks) > 0 {
		fmt.Println("## Ready to Start")
		for _, t := range readyTasks {
			meta, err := pt.ParseTaskMeta(t.Description)
			roleLabel := ""
			if err == nil && meta.Role != "" {
				roleLabel = fmt.Sprintf(" [%s]", meta.Role)
			}
			fmt.Printf("### %s: %s%s\n", t.ID, t.Title, roleLabel)
			if err == nil {
				if meta.Artifact != "" {
					fmt.Printf("  Artifact: %s\n", meta.Artifact)
				}
				if meta.Context != "" {
					fmt.Printf("  Context: %s\n", meta.Context)
				}
				if len(meta.Inputs) > 0 {
					fmt.Printf("  Inputs: %s\n", strings.Join(meta.Inputs, ", "))
				}
				if meta.Scope != "" {
					fmt.Printf("  Scope: %s\n", meta.Scope)
				}
			}
			fmt.Println()
		}
	}

	if len(blockedMap) > 0 {
		fmt.Println("## Blocked Tasks")
		for id, info := range blockedMap {
			iss, _, err := client.GetTask(ctx, id)
			title := id
			if err == nil {
				title = iss.Title
			}
			fmt.Printf("- %s: %s (reason: %s)\n", id, title, info.Reason)
		}
		fmt.Println()
	}

	// Show active worktrees (if any not already shown via in-progress tasks)
	orphanedWorktrees := make(map[string]pt.WorktreeInfo)
	for taskID, wt := range worktrees {
		// Check if this task is not in inProgress
		found := false
		for _, t := range inProgress {
			if t.ID == taskID {
				found = true
				break
			}
		}
		if !found {
			orphanedWorktrees[taskID] = wt
		}
	}
	if len(orphanedWorktrees) > 0 {
		fmt.Println("## Orphaned Worktrees")
		fmt.Println("(worktrees for tasks not in-progress - consider cleanup)")
		for taskID, wt := range orphanedWorktrees {
			status := ""
			if iss, _, err := client.GetTask(ctx, taskID); err == nil {
				status = iss.Status
			}
			exists := "exists"
			if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
				exists = "MISSING"
			}
			fmt.Printf("- %s [%s]: %s (%s)\n", taskID, status, wt.Path, exists)
		}
		fmt.Println()
	}

	// Show recent completions (last 3-5 done tasks)
	if len(doneTasks) > 0 {
		fmt.Println("## Recent Completions")
		limit := 5
		if len(doneTasks) < limit {
			limit = len(doneTasks)
		}
		for i := 0; i < limit; i++ {
			t := doneTasks[i]
			meta, err := pt.ParseTaskMeta(t.Description)
			artifact := ""
			if err == nil && meta.Artifact != "" {
				artifact = fmt.Sprintf(" → %s", meta.Artifact)
			}
			fmt.Printf("- %s: %s%s\n", t.ID, t.Title, artifact)
		}
		fmt.Println()
	}

	fmt.Println("---")
	fmt.Println("Use `pt ready` to see workable tasks, `pt show <id>` for details.")
	if len(worktrees) > 0 {
		fmt.Println("Use `pt worktree status` to see all active worktrees.")
	}

	return nil
}
