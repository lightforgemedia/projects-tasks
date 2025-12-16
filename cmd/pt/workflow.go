package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"projects-tasks/pkg/pt"
)

func cmdWorkflow(args []string) error {
	if len(args) == 0 {
		return workflowUsage()
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "status":
		return cmdWorkflowStatus(subArgs)
	case "next":
		return cmdWorkflowNext(subArgs)
	case "check":
		return cmdWorkflowCheck(subArgs)
	case "-h", "--help", "help":
		return workflowUsage()
	default:
		workflowUsage()
		return fmt.Errorf("unknown workflow subcommand %q", subcmd)
	}
}

func workflowUsage() error {
	fmt.Print(`pt workflow - Development process guidance

Subcommands:
  status              Show workflow phase progress and blockers
  next                Show recommended next action with context
  check --task=<id>   Check if a task can proceed (gate evaluation)

Options (all subcommands):
  --db=PATH           Path to store (default: $PT_DB or .pt/db.json; may auto-discover parent store in worktrees)
  --workflow=PATH     Path to workflow template (default: workflows/*.toml or $PT_WORKFLOW)

Examples:
  pt workflow status
  pt workflow status --workflow=workflows/frontend-first.toml
  pt workflow next
  pt workflow check --task=pt-5
  pt workflow check --task=pt-5 --exit-on-hard-gate  # For hooks

Hook Integration:
  Add to hooks.toml to check workflow gates before claiming:

  [[hook]]
  event = "pre-claim"
  cmd = "pt workflow check --task={{id}} --exit-on-hard-gate"
  on_fail = "fail"
`)
	return errors.New("")
}

func findWorkflowFile() (string, error) {
	// Check env var first
	if env := os.Getenv("PT_WORKFLOW"); env != "" {
		if resolved, ok := resolveWorkflowPath(env); ok {
			return resolved, nil
		}
		return "", fmt.Errorf("workflow file not found: %s", env)
	}

	// Look for workflows/*.toml
	matches := findWorkflowGlobMatches("workflows/*.toml")
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple workflow files found (%s); specify --workflow=PATH", strings.Join(matches, ", "))
	}

	// Look for .pt/workflow.toml
	if resolved, ok := resolveWorkflowPath(".pt/workflow.toml"); ok {
		return resolved, nil
	}

	return "", errors.New("no workflow file found; create workflows/<name>.toml or use --workflow=PATH")
}

func resolveWorkflowPath(path string) (string, bool) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", false
	}

	// 1) As provided (relative to current working dir, or absolute).
	if _, err := os.Stat(p); err == nil {
		return p, true
	}

	// 2) If relative, attempt to resolve relative to the git repo root.
	if !filepath.IsAbs(p) {
		if root, err := gitRepoRoot(""); err == nil && root != "" {
			candidate := filepath.Join(root, p)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, true
			}
		}
	}

	return "", false
}

func findWorkflowGlobMatches(glob string) []string {
	var out []string

	// 1) CWD
	if matches, err := filepath.Glob(glob); err == nil {
		for _, m := range matches {
			if resolved, ok := resolveWorkflowPath(m); ok {
				out = append(out, resolved)
			}
		}
	}

	// 2) Repo root
	if root, err := gitRepoRoot(""); err == nil && root != "" {
		candidate := filepath.Join(root, glob)
		if matches, err := filepath.Glob(candidate); err == nil {
			for _, m := range matches {
				if resolved, ok := resolveWorkflowPath(m); ok {
					out = append(out, resolved)
				}
			}
		}
	}

	// De-dup + stable ordering.
	seen := make(map[string]struct{}, len(out))
	uniq := make([]string, 0, len(out))
	for _, p := range out {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		uniq = append(uniq, p)
	}
	sort.Strings(uniq)
	return uniq
}

func cmdWorkflowStatus(args []string) error {
	fs := flag.NewFlagSet("workflow status", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to store")
	prefix := fs.String("prefix", "", "override issue prefix")
	workflowPath := fs.String("workflow", "", "path to workflow template")
	jsonOut := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check git repo status for workflow onboarding
	checkGitRepoWarning()

	// Find workflow file
	wfPath := *workflowPath
	if wfPath == "" {
		var err error
		wfPath, err = findWorkflowFile()
		if err != nil {
			return err
		}
	}

	workflow, err := pt.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	// Get all issues
	allIssues, err := client.List(ctx, nil, "", 1000)
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}

	// Get comments for all issues
	comments := make(map[string][]string)
	for _, iss := range allIssues {
		comms, _ := client.Comments(ctx, iss.ID)
		comments[iss.ID] = comms
	}

	// Build phase status
	status := buildWorkflowStatus(workflow, allIssues, comments)

	// Surface task-level blockers (deps + manual blocks) in output and in suggested next.
	manualBlocked, _ := client.ListBlocked(ctx)
	taskBlockers := make(map[string][]string, len(allIssues))
	for _, iss := range allIssues {
		if iss.Status != "open" {
			continue
		}
		taskBlockers[iss.ID] = readyBlockers(ctx, client, iss)
	}
	applyWorkflowTaskBlockers(&status, manualBlocked, taskBlockers)

	if *jsonOut {
		return printJSON(struct {
			pt.WorkflowStatus
			TaskBlockers  map[string][]string       `json:"task_blockers,omitempty"`
			ManualBlocked map[string]pt.BlockedInfo `json:"manual_blocked,omitempty"`
		}{
			WorkflowStatus: status,
			TaskBlockers:   taskBlockers,
			ManualBlocked:  manualBlocked,
		})
	}

	printWorkflowStatus(status, manualBlocked, taskBlockers)
	return nil
}

func buildWorkflowStatus(workflow pt.Workflow, allIssues []pt.Issue, comments map[string][]string) pt.WorkflowStatus {
	status := pt.WorkflowStatus{
		Workflow: workflow,
	}

	// Group issues by phase
	phaseIssues := make(map[string][]pt.Issue)
	for _, iss := range allIssues {
		meta, _ := pt.ParseTaskMeta(iss.Description)
		phaseID := workflow.GetTaskPhase(iss, meta)
		phaseIssues[phaseID] = append(phaseIssues[phaseID], iss)
	}

	// Build status for each phase and compute whether earlier phases block later ones.
	for i, phase := range workflow.Phases {
		tasks := phaseIssues[phase.ID]
		closedCount := 0
		for _, t := range tasks {
			if t.Status == "closed" || t.Status == "done" {
				closedCount++
			}
		}

		ps := pt.PhaseStatus{
			Phase:       phase,
			Tasks:       tasks,
			TotalTasks:  len(tasks),
			ClosedTasks: closedCount,
		}

		// Gates only matter if there is work remaining in this phase.
		// Completed/empty phases are not "blocked" even if their gates would evaluate false.
		// NOTE: A phase's gate blocks LATER phases, not itself. Phase 1's "all_closed" gate
		// means Phase 2 can't start until Phase 1 is done - it doesn't block Phase 1 tasks.
		if ps.TotalTasks > 0 && ps.ClosedTasks < ps.TotalTasks {
			// Check if blocked by any prior phase gate.
			for j := 0; j < i; j++ {
				prior := workflow.Phases[j]
				priorTasks := phaseIssues[prior.ID]
				satisfied, reason := workflow.EvaluateGate(prior, priorTasks, allIssues, comments)
				if !satisfied {
					ps.IsBlocked = true
					if prior.Gate.Type == "hard" {
						ps.BlockReason = fmt.Sprintf("🚫 Hard gate (phase:%s): %s", prior.ID, reason)
					} else {
						ps.BlockReason = fmt.Sprintf("⚠️  Soft gate (phase:%s): %s", prior.ID, reason)
					}
					break
				}
			}

			// Also check this phase's own entry gate for phases after the first.
			// This keeps `pt workflow status` consistent with `pt claim` enforcement.
			if !ps.IsBlocked && i > 0 && strings.TrimSpace(phase.Gate.Condition) != "" {
				satisfied, reason := workflow.EvaluateGate(phase, tasks, allIssues, comments)
				if !satisfied {
					ps.IsBlocked = true
					if phase.Gate.Type == "hard" {
						ps.BlockReason = fmt.Sprintf("🚫 Hard gate (phase:%s): %s", phase.ID, reason)
					} else {
						ps.BlockReason = fmt.Sprintf("⚠️  Soft gate (phase:%s): %s", phase.ID, reason)
					}
				}
			}
		}

		status.Phases = append(status.Phases, ps)
	}

	// Determine current phase as the first phase with unfinished work (open/in_progress/needs_review),
	// preferring phases that actually have tasks assigned.
	for i := range status.Phases {
		ps := &status.Phases[i]
		if ps.TotalTasks == 0 {
			continue
		}
		if ps.ClosedTasks < ps.TotalTasks {
			ps.IsCurrent = true
			break
		}
	}

	// Find suggested next task: earliest phase-order open task that isn't blocked by gates.
	for _, ps := range status.Phases {
		if ps.IsBlocked {
			continue
		}
		for _, t := range ps.Tasks {
			if t.Status == "open" {
				status.SuggestedNext = &t
				status.NextReason = fmt.Sprintf("Phase: %s - %s", ps.Phase.Name, ps.Phase.Description)
				break
			}
		}
		if status.SuggestedNext != nil {
			break
		}
	}

	return status
}

func applyWorkflowTaskBlockers(status *pt.WorkflowStatus, manualBlocked map[string]pt.BlockedInfo, taskBlockers map[string][]string) {
	// Recompute suggested next to avoid recommending a deps-blocked task as READY.
	status.SuggestedNext = nil
	status.NextReason = ""
	for _, ps := range status.Phases {
		if ps.IsBlocked {
			continue
		}
		for _, t := range ps.Tasks {
			if t.Status != "open" {
				continue
			}
			if _, ok := manualBlocked[t.ID]; ok {
				continue
			}
			if len(taskBlockers[t.ID]) > 0 {
				continue
			}
			cp := t
			status.SuggestedNext = &cp
			status.NextReason = fmt.Sprintf("Phase: %s - %s", ps.Phase.Name, ps.Phase.Description)
			return
		}
	}
}

func printWorkflowStatus(status pt.WorkflowStatus, manualBlocked map[string]pt.BlockedInfo, taskBlockers map[string][]string) {
	// Header
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Printf("│  Workflow: %-48s │\n", status.Workflow.Name)
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Collect pending checkpoints
	var pendingCheckpoints []pt.Issue
	for _, ps := range status.Phases {
		for _, t := range ps.Tasks {
			if hasCheckpointLabel(t) && t.Status != "closed" && t.Status != "done" {
				pendingCheckpoints = append(pendingCheckpoints, t)
			}
		}
	}

	// Show pending checkpoints banner if any
	if len(pendingCheckpoints) > 0 {
		fmt.Println("⚠️  PENDING CHECKPOINTS (require user approval):")
		for _, t := range pendingCheckpoints {
			statusIcon := "⏸️"
			if t.Status == "needs_review" {
				statusIcon = "🔍"
			}
			fmt.Printf("   %s %s: %s [%s]\n", statusIcon, t.ID, t.Title, t.Status)
		}
		fmt.Println()
	}

	// Phases
	for _, ps := range status.Phases {
		// Progress bar
		pct := 0
		if ps.TotalTasks > 0 {
			pct = (ps.ClosedTasks * 100) / ps.TotalTasks
		}
		bar := progressBar(pct, 15)

		statusStr := ""
		if ps.TotalTasks == 0 {
			statusStr = "NO TASKS"
		} else if ps.IsBlocked {
			statusStr = "BLOCKED"
		} else if ps.ClosedTasks == ps.TotalTasks && ps.TotalTasks > 0 {
			statusStr = "COMPLETE"
		}

		fmt.Printf("Phase %d: %-30s [%d/%d] %s %s\n",
			ps.Phase.Order, ps.Phase.Name, ps.ClosedTasks, ps.TotalTasks, bar, statusStr)

		// Show block reason
		if ps.IsBlocked && ps.BlockReason != "" {
			fmt.Printf("  %s\n", ps.BlockReason)
		}

		// Show tasks
		for _, t := range ps.Tasks {
			marker := "☐"
			if t.Status == "closed" || t.Status == "done" {
				marker = "✓"
			} else if t.Status == "in_progress" {
				marker = "▶"
			}

			extra := ""
			if t.Status == "open" && !ps.IsBlocked {
				ready := true
				if _, ok := manualBlocked[t.ID]; ok {
					ready = false
					extra = extra + " [blocked-manual]"
				}
				if b := taskBlockers[t.ID]; len(b) > 0 {
					ready = false
					indicator := b[0]
					if len(b) > 1 {
						indicator = fmt.Sprintf("%s(+%d)", b[0], len(b)-1)
					}
					extra = extra + fmt.Sprintf(" [blocked %s]", indicator)
				}
				if ready {
					extra = extra + " ← READY"
				}
			}
			// Mark checkpoint tasks
			if hasCheckpointLabel(t) {
				extra = extra + " [checkpoint]"
			}
			fmt.Printf("  %s %-6s %s%s\n", marker, t.ID, t.Title, extra)
		}
		if ps.TotalTasks == 0 {
			fmt.Println("  (no tasks assigned to this phase)")
		}
		fmt.Println()
	}

	// Suggested next
	fmt.Println("─────────────────────────────────────────────────────────────")
	if status.SuggestedNext != nil {
		fmt.Printf("Suggested next:  pt claim %s    (%s)\n", status.SuggestedNext.ID, status.SuggestedNext.Title)
		if status.NextReason != "" {
			fmt.Printf("                 %s\n", status.NextReason)
		}
	} else {
		fmt.Println("Suggested next:  (no ready tasks)")
	}
}

// hasCheckpointLabel returns true if the issue has a checkpoint:required label.
func hasCheckpointLabel(iss pt.Issue) bool {
	for _, label := range iss.Labels {
		if label == "checkpoint:required" || label == "checkpoint" {
			return true
		}
	}
	return false
}

func progressBar(pct, width int) string {
	filled := (pct * width) / 100
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteRune('█')
		} else {
			b.WriteRune('░')
		}
	}
	return fmt.Sprintf("%s %d%%", b.String(), pct)
}

func cmdWorkflowNext(args []string) error {
	fs := flag.NewFlagSet("workflow next", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to store")
	prefix := fs.String("prefix", "", "override issue prefix")
	workflowPath := fs.String("workflow", "", "path to workflow template")

	if err := fs.Parse(args); err != nil {
		return err
	}

	wfPath := *workflowPath
	if wfPath == "" {
		var err error
		wfPath, err = findWorkflowFile()
		if err != nil {
			return err
		}
	}

	workflow, err := pt.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	allIssues, err := client.List(ctx, nil, "", 1000)
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}

	comments := make(map[string][]string)
	for _, iss := range allIssues {
		comms, _ := client.Comments(ctx, iss.ID)
		comments[iss.ID] = comms
	}

	status := buildWorkflowStatus(workflow, allIssues, comments)

	if status.SuggestedNext == nil {
		fmt.Println("┌─────────────────────────────────────────────────────────────┐")
		fmt.Println("│  NO READY TASKS                                             │")
		fmt.Println("└─────────────────────────────────────────────────────────────┘")
		fmt.Println()
		fmt.Println("All tasks are either:")
		fmt.Println("  - Completed (closed/done)")
		fmt.Println("  - In progress (claimed by someone)")
		fmt.Println("  - Blocked by gates")
		return nil
	}

	task := status.SuggestedNext
	meta, _ := pt.ParseTaskMeta(task.Description)
	phaseID := workflow.GetTaskPhase(*task, meta)
	phase := workflow.GetPhaseByID(phaseID)

	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  NEXT ACTION                                                │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Printf("Task:     %s - %s\n", task.ID, task.Title)
	if phase != nil {
		fmt.Printf("Phase:    %s (%d of %d)\n", phase.Name, phase.Order, len(workflow.Phases))
	}
	if status.NextReason != "" {
		fmt.Printf("Why now:  %s\n", status.NextReason)
	}
	fmt.Println()

	// What to do
	fmt.Println("What to do:")
	if meta.Context != "" {
		fmt.Printf("  Context: %s\n", meta.Context)
	}
	if len(meta.Inputs) > 0 {
		fmt.Printf("  Files: %s\n", strings.Join(meta.Inputs, ", "))
	}
	if meta.Scope != "" {
		fmt.Printf("  Scope: %s\n", meta.Scope)
	}
	if phase != nil && phase.Proof != nil && phase.Proof.Required {
		fmt.Printf("  Proof required: %s\n", phase.Proof.Description)
		if phase.Proof.Hint != "" {
			fmt.Printf("    Hint: %s\n", phase.Proof.Hint)
		}
	}
	fmt.Println()

	// Commands
	fmt.Println("Commands:")
	fmt.Printf("  pt claim %s --as=<you>\n", task.ID)
	fmt.Println("  # ... do the work ...")
	if len(meta.DoD.Tests) > 0 {
		fmt.Printf("  pt validate %s --yes\n", task.ID)
	}
	fmt.Println()

	// After this task
	fmt.Println("After this task:")
	// Find what unblocks
	nextPhase := false
	for i, ps := range status.Phases {
		if ps.Phase.ID == phaseID {
			remaining := ps.TotalTasks - ps.ClosedTasks - 1 // -1 for current task
			if remaining <= 0 && i+1 < len(status.Phases) {
				fmt.Printf("  Phase %q complete → Phase %q unblocks\n",
					ps.Phase.Name, status.Phases[i+1].Phase.Name)
				nextPhase = true
			} else if remaining > 0 {
				fmt.Printf("  %d tasks remaining in %q\n", remaining, ps.Phase.Name)
			}
			break
		}
	}
	if !nextPhase {
		fmt.Println("  Continue with remaining phase tasks")
	}

	return nil
}

func cmdWorkflowCheck(args []string) error {
	fs := flag.NewFlagSet("workflow check", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to store")
	prefix := fs.String("prefix", "", "override issue prefix")
	workflowPath := fs.String("workflow", "", "path to workflow template")
	taskID := fs.String("task", "", "task ID to check")
	exitOnHard := fs.Bool("exit-on-hard-gate", false, "exit with code 1 if hard gate blocks")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *taskID == "" {
		return errors.New("--task=<id> is required")
	}

	wfPath := *workflowPath
	if wfPath == "" {
		var err error
		wfPath, err = findWorkflowFile()
		if err != nil {
			return err
		}
	}

	workflow, err := pt.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()

	// Get the specific task
	task, _, err := client.GetTask(ctx, *taskID)
	if err != nil {
		return fmt.Errorf("get task %s: %w", *taskID, err)
	}

	meta, _ := pt.ParseTaskMeta(task.Description)

	// Get all issues
	allIssues, err := client.List(ctx, nil, "", 1000)
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}

	// Get comments
	comments := make(map[string][]string)
	for _, iss := range allIssues {
		comms, _ := client.Comments(ctx, iss.ID)
		comments[iss.ID] = comms
	}

	// Check gate
	canProceed, isHard, blockingPhase, msg := workflow.CheckGate(*taskID, task, meta, allIssues, comments)
	phaseID := workflow.GetTaskPhase(task, meta)

	if canProceed {
		fmt.Printf("✓ Gate check passed for %s\n", *taskID)
		fmt.Printf("  Phase: %s\n", phaseID)
		return nil
	}

	if isHard {
		fmt.Printf("🚫 Hard gate BLOCKED for %s\n", *taskID)
		fmt.Printf("  Phase: %s\n", phaseID)
		if strings.TrimSpace(blockingPhase) != "" {
			fmt.Printf("  Blocking phase: %s\n", blockingPhase)
		}
		fmt.Printf("  Reason: %s\n", msg)
		fmt.Println()
		fmt.Println("Cannot proceed until gate satisfied.")
		if *exitOnHard {
			os.Exit(1)
		}
		return nil
	}

	// Soft gate
	fmt.Printf("⚠️  Soft gate warning for %s\n", *taskID)
	fmt.Printf("  Phase: %s\n", phaseID)
	if strings.TrimSpace(blockingPhase) != "" {
		fmt.Printf("  Blocking phase: %s\n", blockingPhase)
	}
	fmt.Printf("  Warning: %s\n", msg)
	fmt.Println()
	fmt.Println("To proceed anyway:")
	if strings.TrimSpace(blockingPhase) == "" {
		fmt.Printf("  pt comment %s \"gate-override: <phase-id> <reason>\"\n", *taskID)
	} else {
		fmt.Printf("  pt comment %s \"gate-override: %s <reason>\"\n", *taskID, blockingPhase)
	}

	return nil
}

// checkGitRepoWarning prints a warning if not in a git repo
// This is important for workflow onboarding since worktrees require git
func checkGitRepoWarning() {
	if !isGitRepo() {
		fmt.Println("┌─────────────────────────────────────────────────────────────┐")
		fmt.Println("│  ⚠️  WARNING: Not a git repository                          │")
		fmt.Println("└─────────────────────────────────────────────────────────────┘")
		fmt.Println()
		fmt.Println("Workflows work best with git for:")
		fmt.Println("  • Worktrees (parallel task isolation)")
		fmt.Println("  • Branch-per-task workflow")
		fmt.Println("  • Change tracking and rollback")
		fmt.Println()
		fmt.Println("Initialize git:")
		fmt.Println("  git init && git add -A && git commit -m \"initial\"")
		fmt.Println()
	}
}
