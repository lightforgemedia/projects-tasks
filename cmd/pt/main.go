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
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	projects_tasks_pkg_contract "projects-tasks/pkg/contract"
	"projects-tasks/pkg/pt"
	"projects-tasks/pkg/pt/engine"
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
	// Avoid treating paths/filenames as task IDs (e.g. "/tmp/foo-1.json").
	// This keeps reorderArgs safe for commands that accept flag values.
	if strings.ContainsAny(arg, `/\`) || strings.Contains(arg, ".") || strings.HasPrefix(arg, "~") {
		return false
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
	// Default: look for PROJECT_DOD.md at the discovered store's project root.
	// This supports multi-project repos where each project has its own .pt/db.json.
	storePath := pt.DiscoveredStorePath()
	root := projectRootFromStorePath(storePath)
	if strings.TrimSpace(root) != "" {
		candidate := filepath.Join(root, "PROJECT_DOD.md")
		return candidate
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

func projectRootFromStorePath(storePath string) string {
	p := strings.TrimSpace(storePath)
	if p == "" {
		return ""
	}
	dir := filepath.Dir(p)
	// When the store lives at <root>/.pt/db.json, tests should run from <root>,
	// not from inside the hidden ".pt" directory.
	if filepath.Base(dir) == ".pt" {
		return filepath.Dir(dir)
	}
	return dir
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
	if strings.TrimSpace(os.Getenv("PT_SELF")) == "" {
		if self, err := os.Executable(); err == nil && strings.TrimSpace(self) != "" {
			_ = os.Setenv("PT_SELF", self)
		}
	}
	if len(args) < 2 {
		usage()
		return errors.New("")
	}

	cmd := args[1]
	cmdArgs := args[2:]

	// Global helpers that shouldn't require hooks/store access.
	switch cmd {
	case "-h", "--help", "help":
		return cmdHelp(cmdArgs)
	case "-v", "--version", "version":
		return cmdVersion(cmdArgs)
	}

	if loadedHooks == nil {
		cfg, err := loadHooks()
		if err != nil {
			return err
		}
		loadedHooks = cfg
	}

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
	case "mock":
		return cmdMock(cmdArgs)
	case "cd":
		return cmdCd(cmdArgs)
	case "env":
		return cmdEnv(cmdArgs)
	case "demo":
		return cmdDemo(cmdArgs)
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
	case "submit-bug", "submit_bug":
		return cmdSubmitBug(cmdArgs)
	case "review":
		return cmdReview(cmdArgs)
	case "hooks":
		return cmdHooksPrint()
	case "history":
		return cmdHistory(cmdArgs)
	case "handoff":
		return cmdHandoff(cmdArgs)
	case "ux-cases":
		return cmdUXCases(cmdArgs)
	case "ux-explore":
		return cmdUXExplore(cmdArgs)
	case "ux-select":
		return cmdUXSelect(cmdArgs)
	case "ux-status":
		return cmdUXStatus(cmdArgs)
	case "ux-mockup":
		return cmdUXMockup(cmdArgs)
	case "ux-compare":
		return cmdUXCompare(cmdArgs)
	case "ux-upgrade":
		return cmdUXUpgrade(cmdArgs)
	case "ux-drill":
		return cmdUXDrill(cmdArgs)
	case "ux-breakout":
		return cmdUXBreakout(cmdArgs)
	case "ux-cover":
		return cmdUXCover(cmdArgs)
	case "sysmap":
		return cmdSysmap(cmdArgs)
	case "journey":
		return cmdJourney(cmdArgs)
	case "scope":
		return cmdScope(cmdArgs)
	case "discovery":
		return cmdDiscovery(cmdArgs)
	case "workflow":
		return cmdWorkflow(cmdArgs)
	// Shortcuts for common workflow commands
	case "status":
		return cmdWorkflow(append([]string{"status"}, cmdArgs...))
	case "next":
		return cmdNext(cmdArgs)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

type versionInfo struct {
	Version    string `json:"version"`
	Revision   string `json:"revision,omitempty"`
	Time       string `json:"time,omitempty"`
	Modified   bool   `json:"modified,omitempty"`
	GoVersion  string `json:"go_version,omitempty"`
	MainModule string `json:"main_module,omitempty"`
}

func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt version [--json]") }
	if err := fs.Parse(args); err != nil {
		return err
	}

	info := versionInfo{
		Version:   "dev",
		GoVersion: runtime.Version(),
	}

	if bi, ok := debug.ReadBuildInfo(); ok && bi != nil {
		info.MainModule = bi.Main.Path
		if bi.Main.Version != "" {
			info.Version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Revision = s.Value
			case "vcs.time":
				info.Time = s.Value
			case "vcs.modified":
				info.Modified = (s.Value == "true")
			}
		}
	}

	if *jsonOut {
		return printJSON(info)
	}

	out := info.Version
	if info.Revision != "" {
		rev := info.Revision
		if len(rev) > 12 {
			rev = rev[:12]
		}
		out += " " + rev
	}
	if info.Modified {
		out += " (modified)"
	}
	fmt.Println(strings.TrimSpace(out))
	return nil
}

func usage() {
	fmt.Print(`pt - Task management for humans and agents

QUICK START
  pt sync phases/my-project.toml     Import tasks from manifest
  pt ready                           See what's ready to work on
  pt claim <id>                      Start working on a task
  pt validate <id>                   Submit for review (runs tests)
  pt approve <id>                    Complete the task

COMMON WORKFLOW
  ready   [--role=ROLE] [--phase=PHASE|--all-phases] [--workflow=PATH] [--verbose] [--include-blocked]  List open tasks (workflow-aware by default)
  claim   <id> [--as=USER] [--override-soft=REASON] [--workflow=PATH]               Assign and start task (enforces workflow gates)
  release <id>                       Unassign (if stuck)
  validate <id> [--yes]              Run tests, move to review
  approve <id> [--comment="..."]     Mark complete (optionally adds an evidence comment)
  reject  <id> --reason="..."        Send back for fixes

VIEW & SEARCH
  list    [--status=...] [--phase=PHASE]  Filter by status/phase
  show    <id> [--json]              Task details, DoD, deps
  search  --query="text"             Search across all tasks
  history <id>                       View task timeline
  review  <subcmd>                   Save a kickoff/closeout/demo markdown review to .pt/reviews/
  version                           Show build/version info

CREATE & UPDATE
  sync    <manifest>                 Import/update from TOML
  add     "Title" [flags]            Create single task (seeds handoff fields by default)
  update  <id> [flags]               Modify task fields
  comment <id> "text"                Add note to task
  blocked <id> "reason"              Mark task blocked
  unblock <id>                       Clear blocked status

WORKFLOW GUIDANCE
  status                             Current phase and progress
  next                               Conductor: recommended next action (review → work → unblock → plan → done)
  workflow check --task=<id>         Validate phase gates (for hooks/CI)
  sync --generate-phase-reviews      Add non-blocking kickoff/closeout/demo checkpoint tasks per phase (recommended rails)

WORKTREES (task isolation)
  worktree start <id>                Create branch + worktree
  worktree done <id>                 Merge and cleanup
  worktree status                    List active worktrees
  cd <id>                            Output worktree path
  env                                Output PT_DB/PT_PROJECT_DOD/PT_HOOKS for shell
  demo run <id>                      Run demo commands and capture logs

UX DISCOVERY (for user-facing tasks)
  ux-cases   <id>                    Define use cases
  ux-explore <id>                    Generate design options
  ux-select  <id> <choice>           Choose approach

ADVANCED
  context init <id> [--role=ROLE]   Generate context JSON scaffold from task
  context validate <file>           Validate context JSON against contract
  context prime [--json]             Project summary for agents
  submit-bug --label=... --description=... --found_in=... --repro=...  Write a bug report to ~/.pt/bugs/ (stop work if PT is wrong)
  handoff <id>                       Generate handoff document
  doctor [--fix]                     Check/repair store
  export/import                      Backup and restore
  graph <manifest>                   Visualize dependencies

More: pt help task-authoring | pt help concurrency
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
	case "concurrency":
		printConcurrencyHelp()
	default:
		usage()
		fmt.Printf("\nUnknown help topic: %q\n", topic)
		fmt.Println("Available topics: task-authoring, concurrency")
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
  dod.manual  Manual validation steps (use "N/A" only when truly not applicable)
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
  □ Tests are automated and specific (targeted paths/tests; justify broad "go test ./..." if needed)
  □ Manual is explicit (or "N/A" with justification)
  □ Criteria are observable and binary (pass/fail)
  □ Context explains WHY (not just WHAT)
  □ Inputs list actual files to modify
  □ Scope has clear boundaries

Use 'pt sync --generate-reviews <manifest>' to auto-create review tasks.
`)
}

func printConcurrencyHelp() {
	fmt.Print(`# Concurrency Model (recommended)

PT is optimized for **one orchestrator** (single writer) and **many workers**:

- Orchestrator: runs PT commands that change state:
  - ` + "`pt sync`" + `, ` + "`pt claim`" + `, ` + "`pt validate`" + `, ` + "`pt approve`" + `, ` + "`pt update`" + `, ` + "`pt demo run`" + `, ` + "`pt review write`" + `
- Workers: do the work (code/research/wireframes) and return artifacts to the orchestrator.
  - Workers should NOT run state-changing PT commands.

Why:
- Prevents double-claims and out-of-order approvals.
- Keeps evidence/comments consistent and auditable.
- Keeps ` + "`pt next`" + ` recommendations stable.

If PT itself appears wrong or misleading:
- STOP workflow progress and file a bug: ` + "`pt submit-bug --label=... --description=... --found_in=... --repro=...`" + `
`)
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	generateReviews := fs.Bool("generate-reviews", false, "generate review tasks that block implementation tasks")
	generatePhaseReviews := fs.Bool("generate-phase-reviews", false, "generate pre/post phase review tasks (rails; do not block work by default)")
	fs.Usage = func() { fmt.Println("Usage: pt sync <manifest> [--generate-reviews] [--generate-phase-reviews]") }
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

	// Workflow-aware sync: if a workflow exists and uses a label_prefix, assign phase labels
	// based on template mapping/defaults to make phase ordering stable for agents.
	if wfPath, err := findWorkflowFileFor(*dbPath); err == nil {
		if wf, err := pt.ParseWorkflow(wfPath); err == nil {
			if strings.TrimSpace(wf.PhaseAssignment.LabelPrefix) != "" {
				for _, id := range idMap {
					iss, meta, err := client.GetTask(ctx, id)
					if err != nil {
						continue
					}
					phaseID := wf.GetTaskPhase(iss, meta)
					if phaseID == "" || phaseID == "unassigned" {
						continue
					}
					_ = client.AddLabels(ctx, id, wf.PhaseAssignment.LabelPrefix+phaseID)
				}
			}
		}
	}

	// Generate review tasks if requested
	var reviewIDs map[string]string
	if *generateReviews {
		reviewIDs, err = generateReviewTasks(ctx, client, manifest, idMap)
		if err != nil {
			return fmt.Errorf("generate reviews: %w", err)
		}
	}
	var phaseReviewIDs map[string]string
	if *generatePhaseReviews {
		phaseReviewIDs, err = generatePhaseReviewTasks(ctx, client, strings.TrimSpace(*dbPath), strings.TrimSpace(*prefix))
		if err != nil {
			return fmt.Errorf("generate phase reviews: %w", err)
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
		if phaseReviewIDs != nil {
			result["phase_reviews"] = phaseReviewIDs
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
	if phaseReviewIDs != nil {
		fmt.Println("\nPhase review rails generated (recommended checkpoints; not hard gates by default):")
		for title, id := range phaseReviewIDs {
			fmt.Printf("  %s -> %s\n", title, id)
		}
	}
	path, exists := projectDoDStatus()
	if exists {
		fmt.Printf("Project DoD: %s (ensure a review/sign-off task tracks it; set env var PT_PROJECT_DOD to override)\n", path)
	} else {
		fmt.Printf("Project DoD missing. Create %s (or set env var PT_PROJECT_DOD) and add a review/sign-off task to guide completion.\n", path)
	}
	return nil
}

func generatePhaseReviewTasks(ctx context.Context, client pt.Client, dbPath, prefix string) (map[string]string, error) {
	out := make(map[string]string)

	wfPath, err := findWorkflowFileFor(dbPath)
	if err != nil {
		return nil, fmt.Errorf("workflow required for phase review generation: %w", err)
	}
	wf, err := pt.ParseWorkflow(wfPath)
	if err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}

	issues, err := client.List(ctx, nil, "", 2000)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	titleToID := map[string]string{}
	for _, iss := range issues {
		titleToID[iss.Title] = iss.ID
	}

	addOrGet := func(title string, task pt.Task, labels ...string) (string, error) {
		if id, ok := titleToID[title]; ok {
			_ = client.AddLabels(ctx, id, labels...)
			return id, nil
		}
		id, err := client.AddTask(ctx, task)
		if err != nil {
			return "", err
		}
		titleToID[title] = id
		_ = client.AddLabels(ctx, id, labels...)
		return id, nil
	}

	setMetaDoDTests := func(id string, tests []string) {
		iss, meta, err := client.GetTask(ctx, id)
		if err != nil {
			_ = iss
			return
		}
		meta.DoD.Tests = tests
		_ = client.UpdateMeta(ctx, id, meta)
	}

	phaseIDs := append([]pt.Phase{}, wf.Phases...)
	sort.Slice(phaseIDs, func(i, j int) bool { return phaseIDs[i].Order < phaseIDs[j].Order })
	lastPhaseID := ""
	if len(phaseIDs) > 0 {
		lastPhaseID = phaseIDs[len(phaseIDs)-1].ID
	}

	// These tasks are "rails": they do not block work via deps, but pt next can recommend
	// them at phase boundaries.
	labelPrefix := strings.TrimSpace(wf.PhaseAssignment.LabelPrefix)
	if labelPrefix == "" {
		labelPrefix = "phase:"
	}
	for _, ph := range phaseIDs {
		phaseLabel := labelPrefix + ph.ID

		preTitle := fmt.Sprintf("[Phase Pre] %s", ph.Name)
		pre := pt.Task{
			Title:    preTitle,
			Template: "discovery",
			Role:     "planner",
			Artifact: fmt.Sprintf("review:%s:pre", ph.ID),
			Context:  "Kickoff review: confirm plan, options considered, risks/spikes, evidence, and final demo expectations before starting phase work.",
			DoD: pt.DefinitionOfDone{
				Tests:    []string{"echo 'review prepared'"},
				Manual:   "Run: pt review write <this-task-id> --kind=pre --phase=" + ph.ID + " --desc=\"...\"",
				Criteria: []string{"Kickoff review markdown exists and is linked via review-file comment", "Demo plan is concrete (how to run + what to look for)"},
			},
		}
		postTitle := fmt.Sprintf("[Phase Post] %s", ph.Name)
		post := pt.Task{
			Title:    postTitle,
			Template: "discovery",
			Role:     "planner",
			Artifact: fmt.Sprintf("review:%s:post", ph.ID),
			Context:  "Closeout review: what was done, what changed vs plan, evidence, and what’s next.",
			DoD: pt.DefinitionOfDone{
				Tests:    []string{"echo 'review prepared'"},
				Manual:   "Run: pt review write <this-task-id> --kind=post --phase=" + ph.ID + " --desc=\"...\"",
				Criteria: []string{"Closeout review markdown exists and is linked via review-file comment"},
			},
		}

		preID, err := addOrGet(preTitle, pre, "checkpoint:required", "checkpoint:pre", phaseLabel)
		if err != nil {
			return nil, fmt.Errorf("add pre review for phase %s: %w", ph.ID, err)
		}
		setMetaDoDTests(preID, []string{fmt.Sprintf("pt review check %s --kind=pre", preID)})
		out[preTitle] = preID

		postID, err := addOrGet(postTitle, post, "checkpoint:required", "checkpoint:post", phaseLabel)
		if err != nil {
			return nil, fmt.Errorf("add post review for phase %s: %w", ph.ID, err)
		}
		setMetaDoDTests(postID, []string{fmt.Sprintf("pt review check %s --kind=post", postID)})
		out[postTitle] = postID
	}

	// Final demo task (explicit, user-facing proof). Keep it as a rail by default.
	if lastPhaseID != "" {
		title := "[Project Demo] Prepare and capture end-to-end walkthrough"
		demo := pt.Task{
			Title:    title,
			Template: "discovery",
			Role:     "planner",
			Artifact: "demo:outputs/demo/",
			Context:  "Prepare a runnable demo with clear commands and captured artifacts (logs/screenshots). This is what the user will actually see.",
			DoD: pt.DefinitionOfDone{
				Tests:    []string{"echo 'review prepared'"},
				Manual:   "Run: pt review write <this-task-id> --kind=demo --phase=" + lastPhaseID + " --desc=\"demo\" and execute the commands it specifies.",
				Criteria: []string{"Demo markdown exists and is linked via review-file comment", "Demo commands are runnable and artifacts are captured under outputs/"},
			},
		}
		phaseLabel := labelPrefix + lastPhaseID
		id, err := addOrGet(title, demo, "checkpoint:required", "checkpoint:demo", phaseLabel)
		if err != nil {
			return nil, fmt.Errorf("add demo task: %w", err)
		}
		setMetaDoDTests(id, []string{fmt.Sprintf("pt review check %s --kind=demo", id)})
		out[title] = id
	}

	return out, nil
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
	phase := fs.String("phase", "", "filter by workflow phase (requires workflow file)")
	allPhases := fs.Bool("all-phases", false, "show open tasks across all phases (ignore workflow phase focus)")
	workflowPath := fs.String("workflow", "", "path to workflow template (overrides auto-discovery)")
	limit := fs.Int("limit", 10, "max issues")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	verbose := fs.Bool("verbose", false, "show extra info (assignee, blockers)")
	includeBlocked := fs.Bool("include-blocked", false, "include manually blocked tasks (default: hide)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt ready [--role=ROLE] [--phase=PHASE|--all-phases] [--workflow=PATH] [--limit=N] [--sort=priority|title] [--verbose] [--include-blocked]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Fetch all open tasks; apply --limit after local filtering (blocked/deps/gates).
	// This prevents a misleading "no ready tasks" when the backend returns only blocked
	// items within the first N results.
	issues, err := client.Ready(ctx, *role, 0)
	if err != nil {
		return fmt.Errorf("ready failed: %w", err)
	}
	pt.SortIssues(issues, *sortKey)
	blockedMap, _ := client.ListBlocked(ctx)

	// Optional workflow-aware filtering: default to the current (earliest unfinished) phase.
	var wf *pt.Workflow
	var allIssues []pt.Issue
	var comments map[string][]string
	currentPhaseID := ""
	wfPath := strings.TrimSpace(*workflowPath)
	if wfPath == "" {
		path, err := findWorkflowFileFor(*dbPath)
		if err == nil {
			wfPath = path
		} else if strings.Contains(err.Error(), "multiple workflow files found") {
			// Allow a non-workflow view when the caller explicitly asked for all phases.
			// This is primarily for ad-hoc triage; prefer selecting a workflow via --workflow or PT_WORKFLOW.
			if strings.TrimSpace(*phase) != "" || !*allPhases {
				return fmt.Errorf("workflow not selected: %w", err)
			}
		} else if strings.Contains(err.Error(), "no workflow file found") {
			// No workflow configured; proceed without phase filtering.
		} else {
			return fmt.Errorf("workflow not selected: %w", err)
		}
	}
	if wfPath != "" {
		parsed, err := pt.ParseWorkflow(wfPath)
		if err != nil {
			return fmt.Errorf("parse workflow: %w", err)
		}
		wf = &parsed

		allIssues, err = client.List(ctx, nil, "", 1000)
		if err != nil {
			return fmt.Errorf("workflow ready: list issues: %w", err)
		}

		comments = make(map[string][]string, len(allIssues))
		for _, iss := range allIssues {
			comms, _ := client.Comments(ctx, iss.ID)
			comments[iss.ID] = comms
		}

		// Determine current phase as the earliest phase with any unfinished tasks.
		if useEngineV2() {
			eng, err := engine.NewV2(*wf)
			if err != nil {
				return fmt.Errorf("compile workflow: %w", err)
			}
			currentPhaseID, _ = eng.CurrentPhaseID(allIssues, strings.TrimSpace(*role))
		} else {
			phases := append([]pt.Phase{}, wf.Phases...)
			sort.Slice(phases, func(i, j int) bool { return phases[i].Order < phases[j].Order })
			for _, p := range phases {
				assigned := 0
				unfinished := 0
				for _, iss := range allIssues {
					if strings.TrimSpace(*role) != "" && !hasLabel(iss, fmt.Sprintf("role:%s", strings.TrimSpace(*role))) {
						continue
					}
					meta, _ := pt.ParseTaskMeta(iss.Description)
					if wf.GetTaskPhase(iss, meta) != p.ID {
						continue
					}
					assigned++
					if iss.Status != "closed" && iss.Status != "done" {
						unfinished++
					}
				}
				if assigned > 0 && unfinished > 0 {
					currentPhaseID = p.ID
					break
				}
			}
		}
	}

	// Apply phase filter if requested; otherwise focus on current phase when workflow exists.
	targetPhase := strings.TrimSpace(*phase)
	if targetPhase == "" && wf != nil && !*allPhases {
		targetPhase = currentPhaseID
	}
	if wf != nil && targetPhase != "" {
		var filtered []pt.Issue
		for _, iss := range issues {
			meta, _ := pt.ParseTaskMeta(iss.Description)
			if wf.GetTaskPhase(iss, meta) == targetPhase {
				filtered = append(filtered, iss)
			}
		}
		issues = filtered
	}

	if *jsonOut {
		out := []map[string]interface{}{}
		for _, iss := range issues {
			if iss.Status != "open" {
				continue
			}
			blockedInfo, isManuallyBlocked := blockedMap[iss.ID]
			if isManuallyBlocked && !*includeBlocked {
				continue
			}
			blockers := readyBlockers(ctx, client, iss)
			phaseID := ""
			var gate interface{} = nil
			if wf != nil {
				meta, _ := pt.ParseTaskMeta(iss.Description)
				phaseID = wf.GetTaskPhase(iss, meta)
				if allIssues != nil && comments != nil {
					var canProceed bool
					var isHard bool
					var blockingPhase string
					var msg string
					if useEngineV2() {
						eng, err := engine.NewV2(*wf)
						if err != nil {
							return fmt.Errorf("compile workflow: %w", err)
						}
						canProceed, isHard, blockingPhase, msg = eng.CheckGate(iss.ID, iss, meta, allIssues, comments)
					} else {
						canProceed, isHard, blockingPhase, msg = wf.CheckGate(iss.ID, iss, meta, allIssues, comments)
					}
					gate = map[string]interface{}{
						"can_proceed":    canProceed,
						"is_hard_block":  isHard,
						"blocking_phase": blockingPhase,
						"message":        msg,
					}
				}
			}
			out = append(out, map[string]interface{}{
				"id":       iss.ID,
				"title":    iss.Title,
				"status":   iss.Status,
				"assignee": iss.Assignee,
				"labels":   iss.Labels,
				"blockers": blockers,
				"blocked":  isManuallyBlocked,
				"block_reason": func() string {
					if !isManuallyBlocked {
						return ""
					}
					return blockedInfo.Reason
				}(),
				"next_hint": iss.NextHint,
				"artifact":  issueArtifact(iss),
				"criteria":  issueCriteria(iss),
				"phase":     phaseID,
				"gate":      gate,
			})
			if *limit > 0 && len(out) >= *limit {
				break
			}
		}
		return printJSON(out)
	}
	printed := false
	skippedManual := 0
	shown := 0
	for _, iss := range issues {
		if iss.Status != "open" { // hide claimed/in_progress
			continue
		}
		blockedInfo, isManuallyBlocked := blockedMap[iss.ID]
		if isManuallyBlocked && !*includeBlocked {
			skippedManual++
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
			// Show spike tasks with time-box indicator
			title := iss.Title
			if meta, err := pt.ParseTaskMeta(iss.Description); err == nil && meta.Template == "spike" {
				title = fmt.Sprintf("⏱️  %s", title) // spike indicator
			}
			fmt.Printf("### %s: %s\n", iss.ID, title)
			fmt.Printf("  Role: %s  Assignee: %s\n", issueRole(iss), assignee)
			// Show time-box for spikes
			if meta, err := pt.ParseTaskMeta(iss.Description); err == nil && meta.MaxHours > 0 {
				fmt.Printf("  Time-box: %dh\n", meta.MaxHours)
			}

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
			if isManuallyBlocked {
				fmt.Printf("  ⛔ Blocked: %s\n", blockedInfo.Reason)
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
			if isManuallyBlocked {
				line = fmt.Sprintf("%s [blocked-manual]", line)
			}
			fmt.Println(line)
		}
		shown++
		if *limit > 0 && shown >= *limit {
			break
		}
	}
	if !printed {
		if skippedManual > 0 && !*includeBlocked {
			fmt.Printf("No ready tasks (all open tasks are marked blocked). Run: pt blocked (or: pt ready --include-blocked)\n")
			return nil
		}
		if wf != nil && targetPhase != "" {
			fmt.Printf("No open tasks in phase %q. Run: pt workflow status (or: pt ready --all-phases)\n", targetPhase)
			return nil
		}
		path, exists := projectDoDStatus()
		if exists {
			fmt.Printf("No ready tasks. Review project DoD at %s (set env var PT_PROJECT_DOD to override). If the DoD is not satisfied, identify the gaps and add tasks (via manifest or pt add) with explicit tests + manual checks (and docs/review)—avoid shortcuts. Only ask the user when requirements are unclear or external approval is needed.\n", path)
		} else {
			fmt.Printf("No ready tasks. Add a project DoD (e.g., %s or set env var PT_PROJECT_DOD), then create tasks to reach that DoD using best practices: explicit tests + manual checks, docs, and review. Minimize user prompts unless requirements are unclear.\n", path)
		}
	}
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	status := fs.String("status", "open", "comma-separated statuses (open,in_progress,needs_review,closed). Empty for all.")
	role := fs.String("role", "", "filter by role label")
	phase := fs.String("phase", "", "filter by workflow phase (requires workflow file)")
	workflowPath := fs.String("workflow", "", "path to workflow template (overrides auto-discovery)")
	limit := fs.Int("limit", 50, "max issues")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	jsonOut := fs.Bool("json", false, "output JSON")
	porcelain := fs.Bool("porcelain", false, "output stable TSV format (id, status, assignee, title)")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt list [--status=...] [--role=ROLE] [--phase=PHASE] [--workflow=PATH] [--limit=N] [--json] [--porcelain]")
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
	// Filter by phase if specified
	if *phase != "" {
		wfPath := strings.TrimSpace(*workflowPath)
		if wfPath == "" {
			p, err := findWorkflowFileFor(*dbPath)
			if err != nil {
				return fmt.Errorf("--phase requires workflow file: %w", err)
			}
			wfPath = p
		}
		wf, err := pt.ParseWorkflow(wfPath)
		if err != nil {
			return fmt.Errorf("parse workflow: %w", err)
		}
		var filtered []pt.Issue
		for _, iss := range issues {
			meta, _ := pt.ParseTaskMeta(iss.Description)
			taskPhase := wf.GetTaskPhase(iss, meta)
			if taskPhase == *phase {
				filtered = append(filtered, iss)
			}
		}
		issues = filtered
	}
	pt.SortIssues(issues, *sortKey)
	if *jsonOut {
		return printJSON(issues)
	}
	if *porcelain {
		// TSV format: id<TAB>status<TAB>assignee<TAB>title
		for _, iss := range issues {
			assignee := iss.Assignee
			if assignee == "" {
				assignee = "-"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", iss.ID, iss.Status, assignee, iss.Title)
		}
		return nil
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
	porcelain := fs.Bool("porcelain", false, "output JSON (alias for --json)")
	workflowPath := fs.String("workflow", "", "path to workflow template (overrides auto-discovery)")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt show <id> [--json] [--porcelain] [--workflow=PATH]") }
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
	if *jsonOut || *porcelain {
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
	// Show phase if workflow exists
	wfPath := strings.TrimSpace(*workflowPath)
	if wfPath == "" {
		if p, err := findWorkflowFileFor(*dbPath); err == nil {
			wfPath = p
		}
	}
	if wfPath != "" {
		if wf, err := pt.ParseWorkflow(wfPath); err == nil {
			phaseID := wf.GetTaskPhase(iss, meta)
			if phase := wf.GetPhaseByID(phaseID); phase != nil {
				fmt.Printf("Phase: %s\n", phase.Name)
			} else if phaseID != "" && phaseID != "unassigned" {
				fmt.Printf("Phase: %s\n", phaseID)
			}
		}
	}
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
	overrideSoft := fs.String("override-soft", "", "override a soft workflow gate with a reason (writes a gate-override comment)")
	workflowPath := fs.String("workflow", "", "path to workflow template (overrides auto-discovery)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt claim <id> [--as=USER] [--draft] [--override-soft=REASON] [--workflow=PATH]")
	}
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

	// Enforce workflow gates (if a workflow file exists).
	// This prevents skipping ahead into later phases without satisfying prerequisites.
	type workflowGateResult struct {
		CanProceed    bool   `json:"can_proceed"`
		IsHardBlock   bool   `json:"is_hard_block"`
		BlockingPhase string `json:"blocking_phase,omitempty"`
		Message       string `json:"message,omitempty"`
		Overridden    bool   `json:"overridden,omitempty"`
	}
	var gateRes *workflowGateResult
	var overrideComment string
	wfPath := strings.TrimSpace(*workflowPath)
	if wfPath != "" {
		if resolved, ok := resolveWorkflowPath(wfPath, workflowProjectRoot(*dbPath)); ok {
			wfPath = resolved
		} else {
			return fmt.Errorf("workflow file not found: %s", wfPath)
		}
	} else {
		path, err := findWorkflowFileFor(*dbPath)
		if err == nil {
			wfPath = path
		} else if strings.Contains(err.Error(), "no workflow file found") {
			wfPath = ""
		} else {
			return fmt.Errorf("workflow not selected: %w", err)
		}
	}
	if wfPath != "" {
		wf, err := pt.ParseWorkflow(wfPath)
		if err != nil {
			return fmt.Errorf("parse workflow: %w", err)
		}
		allIssues, err := client.List(ctx, nil, "", 1000)
		if err != nil {
			return fmt.Errorf("workflow gate: list issues: %w", err)
		}
		comments := make(map[string][]string, len(allIssues))
		for _, iss := range allIssues {
			comms, _ := client.Comments(ctx, iss.ID)
			comments[iss.ID] = comms
		}

		var canProceed bool
		var isHard bool
		var blockingPhase string
		var msg string
		if useEngineV2() {
			e, err := engine.NewV2(wf)
			if err != nil {
				return fmt.Errorf("compile workflow: %w", err)
			}
			canProceed, isHard, blockingPhase, msg = e.CheckGate(id, issue, meta, allIssues, comments)
		} else {
			canProceed, isHard, blockingPhase, msg = wf.CheckGate(id, issue, meta, allIssues, comments)
		}
		gateRes = &workflowGateResult{
			CanProceed:    canProceed,
			IsHardBlock:   isHard,
			BlockingPhase: blockingPhase,
			Message:       msg,
		}

		if !canProceed {
			if isHard {
				return fmt.Errorf("claim blocked by hard gate (phase:%s): %s", blockingPhase, msg)
			}
			// Soft gate: require explicit override to proceed, and record it as an auditable comment.
			if strings.TrimSpace(*overrideSoft) == "" {
				return fmt.Errorf(
					"claim blocked by soft gate (phase:%s): %s\nTo override: pt claim %s --override-soft=\"<reason>\" (writes: gate-override: %s <reason>)",
					blockingPhase, msg, id, blockingPhase,
				)
			}
			overrideComment = fmt.Sprintf("gate-override: %s %s", blockingPhase, strings.TrimSpace(*overrideSoft))
			gateRes.Overridden = true
		}
	}

	dodJSON, _ := json.Marshal(meta.DoD)
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   user,
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		Template:   meta.Template,
		Labels:     issue.Labels,
		Phase:      phase,
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
	if overrideComment != "" {
		if err := client.AddComment(ctx, id, overrideComment); err != nil {
			return fmt.Errorf("record workflow override: %w", err)
		}
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
			"gate":     gateRes,
			"hooks":    combineHooks(preHooks, postHooks),
		})
	}
	if *draft {
		fmt.Printf("Claimed %s as %s (draft mode - DoD not enforced until validate)\n", id, user)
	} else if overrideComment != "" {
		fmt.Printf("Claimed %s as %s (soft gate override recorded)\n", id, user)
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

	// Check if task requires UX exploration (only if not using discovery template)
	if meta.UX != nil && meta.Template != "discovery" {
		fmt.Printf("\n🎨 This task requires UX exploration before building.\n")
		fmt.Printf("   Run: pt ux-cases %s\n", id)
		fmt.Printf("\n   UX Discovery Flow:\n")
		fmt.Printf("   1. pt ux-cases %s    - Define use cases\n", id)
		fmt.Printf("   2. pt ux-explore %s  - Generate options\n", id)
		fmt.Printf("   3. pt ux-select %s   - Select approach\n", id)
	}

	// Check if task uses discovery template
	if meta.Template == "discovery" {
		// Derive component ID from artifact or task ID
		componentID := id
		if meta.Artifact != "" {
			// Extract component name from artifact (e.g., "code:cmd/sot/order.go" -> "sot-order")
			parts := strings.Split(meta.Artifact, "/")
			if len(parts) >= 2 {
				// Use last two path components (e.g., "sot/order.go" -> "sot-order")
				dir := parts[len(parts)-2]
				file := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
				componentID = dir + "-" + file
			}
		}
		// Derive UX type from artifact path
		uxType := "cli" // default
		if strings.Contains(meta.Artifact, "web") || strings.Contains(meta.Artifact, "frontend") || strings.Contains(meta.Artifact, "ui") {
			uxType = "web"
		}

		fmt.Printf("\n🔍 This task uses the discovery workflow.\n")
		fmt.Printf("   Design UX before implementation using iterative exploration.\n")
		fmt.Printf("\n   Suggested component: %s (type: %s)\n", componentID, uxType)
		fmt.Printf("\n   Discovery Flow:\n")
		fmt.Printf("   1. pt discovery init %s --type %s\n", componentID, uxType)
		fmt.Printf("   2. pt discovery capabilities %s <cap1> <cap2> ...\n", componentID)
		fmt.Printf("   3. pt discovery explore %s <approach-name> <description>\n", componentID)
		fmt.Printf("   4. pt discovery synthesize %s\n", componentID)
		fmt.Printf("   5. pt discovery feedback %s <feedback>\n", componentID)
		fmt.Printf("   6. pt discovery approve %s\n", componentID)
		fmt.Printf("\n   The workflow ensures 2+ options explored before synthesis.\n")
		fmt.Println("")
		fmt.Println("   ⚠️  ORCHESTRATION NOTE:")
		fmt.Println("      Run 'pt discovery' commands yourself to see guidance prompts.")
		fmt.Println("      Delegate the WORK (research, wireframes, analysis) to agents,")
		fmt.Println("      not the PT commands themselves.")
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
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   "",
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   "open",
		Role:       meta.Role,
		Template:   meta.Template,
		Labels:     issue.Labels,
		Phase:      phase,
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
	args = reorderArgs(args) // Allow flags after positional args
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
		if fs.NArg() == 0 {
			return errors.New("missing id argument")
		}
		return fmt.Errorf("expected 1 id argument, got %d: %s", fs.NArg(), strings.Join(fs.Args(), " "))
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
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusNeedsReview),
		Role:       meta.Role,
		Template:   meta.Template,
		Labels:     issue.Labels,
		Phase:      phase,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-validate", payload)
	if err != nil {
		return err
	}

	// Determine working directory for test execution
	// If task has an active worktree, run tests there; otherwise use project directory
	workDir := ""
	if wtInfo, hasWT, _ := client.GetWorktree(ctx, id); hasWT {
		workDir = wtInfo.Path
		fmt.Printf("Running tests in worktree: %s\n", workDir)
	} else {
		// Default to project directory (where store is located)
		storePath := pt.DiscoveredStorePath()
		if storePath != "" {
			workDir = projectRootFromStorePath(storePath)
		}
	}

	manualSteps := splitManualSteps(meta.DoD.Manual)

	// Add UX use cases as manual verification prompts if present
	if meta.UXState != nil && len(meta.UXState.UseCases) > 0 {
		fmt.Println("\n📋 UX Use Cases to Verify:")
		for _, uc := range meta.UXState.UseCases {
			ucStep := fmt.Sprintf("[%s] As a %s, verify: %s", uc.ID, uc.Actor, uc.Goal)
			manualSteps = append(manualSteps, ucStep)
			fmt.Printf("   • %s\n", ucStep)
		}
		fmt.Println()
	}

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
	args = reorderArgs(args) // Allow flags after positional args
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	comment := fs.String("comment", "", "add a comment while approving (avoids a separate pt comment)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt approve <id> [--comment=\"...\"]") }
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
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   "closed",
		Role:       meta.Role,
		Template:   meta.Template,
		Labels:     issue.Labels,
		Phase:      phase,
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
	if strings.TrimSpace(*comment) != "" {
		if err := client.AddComment(ctx, id, strings.TrimSpace(*comment)); err != nil {
			return fmt.Errorf("add comment failed: %w", err)
		}
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
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		Template:   meta.Template,
		Labels:     issue.Labels,
		Phase:      phase,
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
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   user,
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		Template:   meta.Template,
		Labels:     issue.Labels,
		Phase:      phase,
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
	contextText := fs.String("context", "", "handoff context (why this task exists)")
	inputsCSV := fs.String("inputs", "", "comma-separated files/dirs to read/modify")
	scopeText := fs.String("scope", "", "handoff scope (IN/OUT)")
	referenceText := fs.String("reference", "", "handoff reference (links/docs/issues)")
	noHandoffSeed := fs.Bool("no-handoff-seed", false, "do not seed TODO handoff placeholders when fields are missing")
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
		fmt.Println("Usage: pt add \"Title\" --role=... --template=... --artifact=... --manual=... --tests=... --criteria=... [--context=...] [--inputs=...] [--scope=...] [--reference=...] [--validation-cmd=...] [--deps=...] [--next-hint=...]")
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

	// Workflow-aware task creation:
	// - Seed handoff fields with explicit TODO placeholders (unless disabled).
	// - Attach phase label if a workflow exists and uses label_prefix.
	if !*noHandoffSeed {
		if strings.TrimSpace(*contextText) == "" {
			task.Context = "TODO: why does this task exist? What problem/outcome does it solve?"
		} else {
			task.Context = *contextText
		}
		if strings.TrimSpace(*scopeText) == "" {
			task.Scope = "TODO: IN: ... OUT: ..."
		} else {
			task.Scope = *scopeText
		}
		if strings.TrimSpace(*referenceText) != "" {
			task.Reference = *referenceText
		} else {
			task.Reference = "TODO: link relevant docs/specs/issues (or '-')"
		}
	} else {
		task.Context = *contextText
		task.Scope = *scopeText
		task.Reference = *referenceText
	}

	if strings.TrimSpace(*inputsCSV) != "" {
		for _, p := range strings.Split(*inputsCSV, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				task.Inputs = append(task.Inputs, p)
			}
		}
	} else if !*noHandoffSeed {
		task.Inputs = []string{"TODO: add file/dir paths (or '-')"}
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := client.AddTask(ctx, task)
	if err != nil {
		return fmt.Errorf("add task failed: %w", err)
	}

	// Attach phase label if workflow exists and label_prefix is configured.
	if wfPath, err := findWorkflowFileFor(*dbPath); err == nil {
		if wf, err := pt.ParseWorkflow(wfPath); err == nil {
			if strings.TrimSpace(wf.PhaseAssignment.LabelPrefix) != "" {
				iss, meta, err := client.GetTask(ctx, id)
				if err == nil {
					phaseID := wf.GetTaskPhase(iss, meta)
					if phaseID != "" && phaseID != "unassigned" {
						_ = client.AddLabels(ctx, id, wf.PhaseAssignment.LabelPrefix+phaseID)
					}
				}
			}
		}
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
	fmt.Println("- State exact commands to run (prefer targeted tests; justify broad go test) and manual steps (or 'N/A').")
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
	args = reorderArgs(args) // Allow flags after positional args
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
	phase := ""
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, "phase:") {
			phase = strings.TrimPrefix(l, "phase:")
			break
		}
	}
	payload := hookPayload{
		ID:       issue.ID,
		Title:    issue.Title,
		Assignee: issue.Assignee,
		Actor:    actor,
		Role:     meta.Role,
		Template: meta.Template,
		Labels:   issue.Labels,
		Phase:    phase,
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
		dbPath = pt.DiscoveredStorePath()
		if strings.TrimSpace(dbPath) == "" {
			dbPath = filepath.Join(".pt", "db.json")
		}
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
	case "help", "--help", "-h":
		fmt.Println("Usage: pt context <init|validate|prime> [args]")
		fmt.Println("  init     Generate a context JSON scaffold from a task")
		fmt.Println("  validate Validate a context JSON file against a contract")
		fmt.Println("  prime    Summarize project state for agents")
		return nil
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
	contractPath = resolveContractPath(contractPath)
	contract, err := projects_tasks_pkg_contract.Load(contractPath)
	if err != nil {
		return fmt.Errorf("load contract %s: %w", contractPath, err)
	}
	_ = contract

	// Scaffold payload
	criteria := meta.DoD.Criteria
	if len(criteria) == 0 {
		criteria = meta.DoD.Tests
	}
	payload := map[string]any{
		"role": targetRole,
		"meta": map[string]any{
			"role":             targetRole,
			"contract_path":    contractPath,
			"contract_version": contract.Meta.Version,
		},
		"goal": map[string]any{
			"prompt":      issue.Description, // simplistic mapping
			"description": issue.Description,
		},
		"scope": map[string]any{
			"files": []string{},
		},
		"success": map[string]any{
			"criteria": criteria,
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

func cmdHandoff(args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	outPath := fs.String("out", "", "output path (default: HANDOFF-{id}.md)")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt handoff <id> [--out=PATH]")
		fmt.Println("\nGenerate a handoff document from template + task data.")
		fmt.Println("\nFlags:")
		fs.PrintDefaults()
	}
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
		return fmt.Errorf("get task: %w", err)
	}

	// Build handoff document
	doc := buildHandoffDoc(iss, meta)

	// Write to file
	path := *outPath
	if path == "" {
		path = fmt.Sprintf("HANDOFF-%s.md", id)
	}
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		return fmt.Errorf("write handoff: %w", err)
	}
	fmt.Printf("Handoff document created: %s\n", path)
	return nil
}

func buildHandoffDoc(iss pt.Issue, meta pt.TaskMeta) string {
	now := time.Now().Format("2006-01-02")
	author := os.Getenv("USER")
	if author == "" {
		author = "Agent"
	}

	// Get scope from context if available
	scope := meta.Scope
	if scope == "" {
		scope = "{ONE_LINE_SCOPE - describe the boundary of this work}"
	}

	// Build DoD section
	var dodTests, dodManual, dodCriteria string
	if len(meta.DoD.Tests) > 0 {
		dodTests = strings.Join(meta.DoD.Tests, "\n")
	} else {
		dodTests = "{TEST_COMMANDS}"
	}
	if meta.DoD.Manual != "" {
		dodManual = meta.DoD.Manual
	} else {
		dodManual = "{MANUAL_VERIFICATION_STEPS}"
	}
	if len(meta.DoD.Criteria) > 0 {
		dodCriteria = strings.Join(meta.DoD.Criteria, "; ")
	} else {
		dodCriteria = "{ACCEPTANCE_CRITERIA}"
	}

	// Build inputs section
	inputs := "{FILE_PATHS}"
	if len(meta.Inputs) > 0 {
		var inputLines []string
		for _, inp := range meta.Inputs {
			inputLines = append(inputLines, fmt.Sprintf("- %s", inp))
		}
		inputs = strings.Join(inputLines, "\n")
	}

	// Build context section
	context := meta.Context
	if context == "" {
		context = "{DESCRIPTION_OF_PROBLEM}"
	}

	doc := fmt.Sprintf(`# Handoff: %s

**Date:** %s
**Author:** %s
**Task ID:** %s
**Scope:** %s

---

## 1. Dependency & Integration Status (REVIEW THIS FIRST)

**Mocking is permitted ONLY when ALL conditions are met:**
1. Real behavior has been proven (spike ran against actual dependency)
2. Mock faithfully reproduces proven behavior (not assumed behavior)
3. Task exists in task system to return to full integration
4. User-facing indicators show when mocked data is in use
5. Integration tests against real dependency exist and pass

### External Dependencies

| Dependency | Real Behavior Proven? | Evidence | Mock Status |
|------------|----------------------|----------|-------------|
| {DEP_NAME} | {YES/NO} | {LINK_OR_DESCRIPTION} | {No mocks / Mock in use} |

### Mock Registry

**No mocks introduced.** (Update if mocks are added)

**Reviewer MUST verify:**
- [ ] Every mock has corresponding proof of real behavior
- [ ] Every mock has a tracked task for removal/integration
- [ ] No silent mocks—user always knows when data is fake
- [ ] Integration tests exist and are not skipped in CI

---

## 2. Risk Spike Status

| Risk Area | Spike Status | What Was Proven | What's Still Assumed |
|-----------|--------------|-----------------|----------------------|
| {AREA} | {Validated/Pending/Skipped} | {DESCRIPTION} | {ASSUMPTIONS} |

**Unproven risks the reviewer should scrutinize:**
- {TODO: List unproven assumptions}

---

## 3. UX Exploration Summary

### What was shown to users (or user-proxy agents):
- [ ] CLI input-output examples
- [ ] Breadth-first options presented
- [ ] Key decision points with user sign-off

### User decisions captured:
- **Decision:** {TODO: Document key decisions}
- **Why:** {REASONING}
- **Alternatives rejected:** {LIST}

---

## 4. What Changed (Summary)

| File/Component | Before | After | Confidence |
|----------------|--------|-------|------------|
| {PATH} | {PREVIOUS_STATE} | {NEW_STATE} | {High/Medium/Low} |

---

## 5. Intent & Approach

### Problem being solved:
%s

### Approach taken:
{TODO: Describe approach}

### Alternatives rejected:
- {TODO: List alternatives and why rejected}

### What's intentionally deferred:
- {TODO: List deferred work}

---

## 6. Stub & Dummy Data Inventory

**No stubs introduced.** (Update if stubs are added)

---

## 7. User Checkpoint Map

### Requires user approval before proceeding:
- [ ] {TODO: List checkpoints}

### Can be delegated to specialized agent:
- [ ] Code review
- [ ] Test review

### No checkpoint needed (routine/mechanical):
- Running test suite
- Building binary

---

## 8. Review Focus Areas

### 1. **{TODO: Highest risk area}**
   - Location: {FILE:LINE}
   - What could go wrong: {RISK}
   - How to verify: {COMMANDS}

### Questions the reviewer must answer:
- [ ] {TODO: Key review questions}

---

## 9. How to Validate

### Run integration tests:
`+"`"+`bash
%s
`+"`"+`

### Manual verification:
%s

### Acceptance criteria:
%s

---

## 10. Context & References

### Files to review:
%s

### Related documentation:
- {TODO: Add relevant docs}

### Task metadata:
- Role: %s
- Template: %s
- Artifact: %s
`, iss.Title, now, author, iss.ID, scope, context, dodTests, dodManual, dodCriteria, inputs, meta.Role, meta.Template, meta.Artifact)

	return doc
}

// cmdMock manages mock/stub implementations that need to be replaced.
func cmdMock(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: pt mock <subcommand>")
		fmt.Println("\nSubcommands:")
		fmt.Println("  register   Register a new mock/stub")
		fmt.Println("  list       List active mocks")
		fmt.Println("  check      Check for orphaned mocks")
		fmt.Println("  retire     Mark a mock as replaced")
		return nil
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "register":
		return cmdMockRegister(subArgs)
	case "list":
		return cmdMockList(subArgs)
	case "check":
		return cmdMockCheck(subArgs)
	case "retire":
		return cmdMockRetire(subArgs)
	default:
		return fmt.Errorf("unknown mock subcommand: %s", subcmd)
	}
}

func cmdMockRegister(args []string) error {
	fs := flag.NewFlagSet("mock register", flag.ContinueOnError)
	desc := fs.String("desc", "", "description of the mock")
	location := fs.String("loc", "", "file:line or path where mock is implemented")
	spikeID := fs.String("spike", "", "originating spike task ID (required)")
	integrationID := fs.String("integration", "", "integration task that will replace this mock (required)")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")

	fs.Usage = func() {
		fmt.Println("Usage: pt mock register --desc=\"...\" --loc=\"...\" --spike=<id> --integration=<id>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *spikeID == "" {
		return errors.New("--spike is required (mocks must originate from a spike)")
	}
	if *integrationID == "" {
		return errors.New("--integration is required (mocks must have a replacement task)")
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	// Verify spike task exists and is a spike
	_, spikeMeta, err := client.GetTask(ctx, *spikeID)
	if err != nil {
		return fmt.Errorf("spike task %s not found", *spikeID)
	}
	if spikeMeta.Template != "spike" {
		return fmt.Errorf("task %s is not a spike (template=%s)", *spikeID, spikeMeta.Template)
	}

	// Verify integration task exists
	_, _, err = client.GetTask(ctx, *integrationID)
	if err != nil {
		return fmt.Errorf("integration task %s not found", *integrationID)
	}

	storeClient, ok := client.(*pt.StoreClient)
	if !ok {
		return errors.New("mock commands require store backend")
	}

	mock, err := storeClient.RegisterMock(ctx, *desc, *location, *spikeID, *integrationID)
	if err != nil {
		return err
	}
	fmt.Printf("Registered %s: %s\n", mock.ID, mock.Description)
	fmt.Printf("  Location: %s\n", mock.Location)
	fmt.Printf("  Spike: %s → Integration: %s\n", *spikeID, *integrationID)
	return nil
}

func cmdMockList(args []string) error {
	fs := flag.NewFlagSet("mock list", flag.ContinueOnError)
	includeRetired := fs.Bool("all", false, "include retired mocks")
	jsonOut := fs.Bool("json", false, "output as JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")

	if err := fs.Parse(args); err != nil {
		return err
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	storeClient, ok := client.(*pt.StoreClient)
	if !ok {
		return errors.New("mock commands require store backend")
	}

	mocks := storeClient.ListMocks(ctx, *includeRetired)

	if *jsonOut {
		return printJSON(mocks)
	}

	if len(mocks) == 0 {
		fmt.Println("No active mocks registered.")
		return nil
	}

	for _, m := range mocks {
		status := "active"
		if m.RetiredAt != "" {
			status = "retired"
		}
		fmt.Printf("%s [%s] %s\n", m.ID, status, m.Description)
		fmt.Printf("  Location: %s\n", m.Location)
		fmt.Printf("  Spike: %s → Integration: %s\n", m.SpikeTaskID, m.IntegrationTask)
	}
	return nil
}

func cmdMockCheck(args []string) error {
	fs := flag.NewFlagSet("mock check", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")

	if err := fs.Parse(args); err != nil {
		return err
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	storeClient, ok := client.(*pt.StoreClient)
	if !ok {
		return errors.New("mock commands require store backend")
	}

	orphans := storeClient.CheckMocks(ctx)

	if len(orphans) == 0 {
		fmt.Println("No orphaned mocks. All mocks are either retired or have closed integration tasks.")
		return nil
	}

	fmt.Printf("⚠️  %d orphaned mock(s) found:\n", len(orphans))
	for _, m := range orphans {
		fmt.Printf("  %s: %s\n", m.ID, m.Description)
		fmt.Printf("    Location: %s\n", m.Location)
		fmt.Printf("    Integration task: %s (not closed)\n", m.IntegrationTask)
	}
	return fmt.Errorf("%d orphaned mocks need attention", len(orphans))
}

func cmdMockRetire(args []string) error {
	fs := flag.NewFlagSet("mock retire", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")

	fs.Usage = func() {
		fmt.Println("Usage: pt mock retire <mock-id>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing mock-id argument")
	}
	mockID := fs.Arg(0)

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	storeClient, ok := client.(*pt.StoreClient)
	if !ok {
		return errors.New("mock commands require store backend")
	}

	if err := storeClient.RetireMock(ctx, mockID); err != nil {
		return err
	}
	fmt.Printf("Retired %s\n", mockID)
	return nil
}

// cmdCd outputs the worktree path for a task (for shell integration).
// Usage: eval "$(pt cd pt-5)" or cd $(pt cd pt-5)
func cmdCd(args []string) error {
	fs := flag.NewFlagSet("cd", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt cd <task-id>")
		fmt.Println("\nOutputs the worktree path for a task.")
		fmt.Println("Use with: cd $(pt cd pt-5)")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing task-id argument")
	}
	taskID := fs.Arg(0)
	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wtInfo, hasWT, err := client.GetWorktree(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	if !hasWT {
		return fmt.Errorf("task %s has no worktree; run 'pt worktree start %s' first", taskID, taskID)
	}
	fmt.Println(wtInfo.Path)
	return nil
}

// cmdEnv outputs PT environment info for shell integration.
func cmdEnv(args []string) error {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Println("Usage: pt env")
		fmt.Println("\nOutputs PT environment variables for shell integration.")
		fmt.Println("Use with: eval \"$(pt env)\"")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get the store path
	dbPath := pt.DiscoveredStorePath()
	root := projectRootFromStorePath(dbPath)
	projectDoD := filepath.Join(root, "PROJECT_DOD.md")
	projectHooks := filepath.Join(root, "hooks.toml")
	workflowPath := ""
	if strings.TrimSpace(root) != "" {
		if p, err := findWorkflowFileFor(dbPath); err == nil {
			workflowPath = p
		}
	}

	fmt.Printf("PT_DB=%s\n", dbPath)
	fmt.Printf("export PT_DB\n")
	if strings.TrimSpace(root) != "" {
		fmt.Printf("PT_PROJECT_DOD=%s\n", projectDoD)
		fmt.Printf("export PT_PROJECT_DOD\n")
		fmt.Printf("PT_HOOKS=%s\n", projectHooks)
		fmt.Printf("export PT_HOOKS\n")
		if strings.TrimSpace(workflowPath) != "" {
			fmt.Printf("PT_WORKFLOW=%s\n", workflowPath)
			fmt.Printf("export PT_WORKFLOW\n")
		}
	}
	return nil
}
