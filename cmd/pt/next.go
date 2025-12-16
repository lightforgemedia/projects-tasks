package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

type nextMode string

const (
	nextModeBlocked nextMode = "BLOCKED"
	nextModeReview  nextMode = "REVIEW"
	nextModeWork    nextMode = "WORK"
	nextModePlan    nextMode = "PLAN"
	nextModeDone    nextMode = "DONE"
)

type nextRecommendation struct {
	Cmd  string `json:"cmd"`
	Kind string `json:"kind"` // work|review|unblock|plan
}

type nextPhase struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Order int    `json:"order,omitempty"`
}

type nextBlocking struct {
	GateType      string   `json:"gate_type,omitempty"` // hard|soft
	BlockingPhase string   `json:"blocking_phase,omitempty"`
	Message       string   `json:"message,omitempty"`
	UnblockSteps  []string `json:"unblock_steps,omitempty"`
}

type nextProjectDoD struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type nextResult struct {
	Mode            nextMode             `json:"mode"`
	Recommended     []nextRecommendation `json:"recommended"`
	Why             []string             `json:"why,omitempty"`
	CurrentPhase    nextPhase            `json:"current_phase,omitempty"`
	Blocking        *nextBlocking        `json:"blocking,omitempty"`
	ApprovalsNeeded []string             `json:"approvals_needed,omitempty"` // internal|end_user
	AskUser         bool                 `json:"ask_user"`
	AskUserPrompts  []string             `json:"ask_user_prompts,omitempty"`
	ProjectDoD      nextProjectDoD       `json:"project_dod"`
}

func cmdNext(args []string) error {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	role := fs.String("role", "", "filter recommended work by role label")
	strict := fs.Bool("strict", false, "treat soft gates as blocking")
	allPhases := fs.Bool("all-phases", false, "debug: consider open tasks across all phases")
	skipCheckpoints := fs.Bool("skip-checkpoints", false, "do not recommend phase checkpoint rails (kickoff/closeout/demo)")
	workflowPath := fs.String("workflow", "", "path to workflow template (overrides auto-discovery)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt next [--role=ROLE] [--strict] [--all-phases] [--skip-checkpoints] [--workflow=PATH] [--json]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Base store view.
	allIssues, err := client.List(ctx, nil, "", 1000)
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}
	comments := make(map[string][]string, len(allIssues))
	for _, iss := range allIssues {
		comms, _ := client.Comments(ctx, iss.ID)
		comments[iss.ID] = comms
	}
	blocked, _ := client.ListBlocked(ctx)

	dodPath, dodExists := projectDoDStatus()
	out := nextResult{
		AskUser:    false,
		ProjectDoD: nextProjectDoD{Path: dodPath, Exists: dodExists},
	}

	// 1) Prefer clearing needs_review first (internal review).
	var needsReview []pt.Issue
	for _, iss := range allIssues {
		if iss.Status == "needs_review" {
			needsReview = append(needsReview, iss)
		}
	}
	if len(needsReview) > 0 {
		pt.SortIssues(needsReview, "priority")
		task := needsReview[0]
		out.Mode = nextModeReview
		out.ApprovalsNeeded = []string{"internal"}
		out.Why = []string{"tasks awaiting review exist", "reviewing reduces drift before starting new work"}
		out.Recommended = []nextRecommendation{
			{Cmd: fmt.Sprintf("pt show %s", task.ID), Kind: "review"},
			{Cmd: fmt.Sprintf("pt approve %s  # or: pt reject %s --reason=\"...\"", task.ID, task.ID), Kind: "review"},
			{Cmd: "pt next", Kind: "review"},
		}
		return printNext(out, *jsonOut)
	}

	// 2) If workflow exists, use it to determine current phase and enforce gates.
	var wf *pt.Workflow
	var workflowErr error
	wfPath := strings.TrimSpace(*workflowPath)
	if wfPath == "" {
		path, err := findWorkflowFile()
		if err == nil {
			wfPath = path
		} else {
			workflowErr = err
		}
	}
	if wfPath != "" {
		parsed, err := pt.ParseWorkflow(wfPath)
		if err != nil {
			return fmt.Errorf("parse workflow: %w", err)
		}
		wf = &parsed
	}
	if wf == nil && workflowErr != nil {
		out.Why = append(out.Why, fmt.Sprintf("workflow not selected: %s", workflowErr.Error()))
		out.Why = append(out.Why, "set PT_WORKFLOW or pass --workflow=PATH to enable phase/gate guidance")
	}

	currentPhaseID := ""
	var currentPhase *pt.Phase
	if wf != nil {
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
				// Copy loop variable.
				cp := p
				currentPhase = &cp
				break
			}
		}
		if currentPhase != nil {
			out.CurrentPhase = nextPhase{ID: currentPhase.ID, Name: currentPhase.Name, Order: currentPhase.Order}
		}
	}

	// 2.5) Checkpoint rails (non-blocking): recommend phase kickoff/closeout/demo tasks at the right time.
	if wf != nil && !*skipCheckpoints && strings.TrimSpace(currentPhaseID) != "" {
		var phaseTasks []pt.Issue
		for _, iss := range allIssues {
			if strings.TrimSpace(*role) != "" && !hasLabel(iss, fmt.Sprintf("role:%s", strings.TrimSpace(*role))) {
				continue
			}
			meta, _ := pt.ParseTaskMeta(iss.Description)
			if wf.GetTaskPhase(iss, meta) == currentPhaseID {
				phaseTasks = append(phaseTasks, iss)
			}
		}
		// Split into checkpoint tasks vs "work" tasks.
		var pre, post, demo *pt.Issue
		workTotal := 0
		workClosed := 0
		for i := range phaseTasks {
			t := phaseTasks[i]
			if hasLabel(t, "checkpoint:pre") {
				if t.Status == "open" {
					cp := t
					pre = &cp
				}
				continue
			}
			if hasLabel(t, "checkpoint:post") {
				if t.Status == "open" {
					cp := t
					post = &cp
				}
				continue
			}
			if hasLabel(t, "checkpoint:demo") {
				if t.Status == "open" {
					cp := t
					demo = &cp
				}
				continue
			}
			// Treat non-checkpoint tasks as "work".
			if t.Status == "closed" || t.Status == "done" {
				workClosed++
			}
			if t.Status != "" {
				workTotal++
			}
		}
		// Kickoff: before any work is done, recommend the pre-phase review.
		if workTotal > 0 && workClosed == 0 && pre != nil {
			out.Mode = nextModeReview
			out.Why = []string{"phase kickoff checkpoint recommended before starting work (not a hard gate)", "records plan/options/risks and locks in the final demo expectations early"}
			out.Recommended = []nextRecommendation{
				{Cmd: fmt.Sprintf("pt show %s", pre.ID), Kind: "review"},
				{Cmd: fmt.Sprintf("pt claim %s --as=%s", pre.ID, defaultIdentityForPrint()), Kind: "review"},
				{Cmd: fmt.Sprintf("pt review write %s --kind=pre --phase=%s --desc=\"kickoff\"  # creates .pt/reviews/... and links it", pre.ID, currentPhaseID), Kind: "review"},
				{Cmd: fmt.Sprintf("pt validate %s --yes", pre.ID), Kind: "review"},
				{Cmd: fmt.Sprintf("pt approve %s", pre.ID), Kind: "review"},
				{Cmd: "pt next", Kind: "review"},
			}
			// Make the escape hatch explicit (avoids endless approval loops while keeping rails).
			out.Recommended = append(out.Recommended, nextRecommendation{Cmd: "pt next --skip-checkpoints  # proceed without checkpoint", Kind: "work"})
			return printNext(out, *jsonOut)
		}
		// Closeout: once all work tasks are closed/done, recommend the post-phase review (and/or demo).
		if workTotal > 0 && workClosed == workTotal {
			if post != nil {
				out.Mode = nextModeReview
				out.Why = []string{"phase closeout checkpoint recommended (not a hard gate)", "captures evidence and deltas to reduce drift for later phases and reviewers"}
				out.Recommended = []nextRecommendation{
					{Cmd: fmt.Sprintf("pt show %s", post.ID), Kind: "review"},
					{Cmd: fmt.Sprintf("pt claim %s --as=%s", post.ID, defaultIdentityForPrint()), Kind: "review"},
					{Cmd: fmt.Sprintf("pt review write %s --kind=post --phase=%s --desc=\"closeout\"  # creates .pt/reviews/... and links it", post.ID, currentPhaseID), Kind: "review"},
					{Cmd: fmt.Sprintf("pt validate %s --yes", post.ID), Kind: "review"},
					{Cmd: fmt.Sprintf("pt approve %s", post.ID), Kind: "review"},
					{Cmd: "pt next", Kind: "review"},
				}
				out.Recommended = append(out.Recommended, nextRecommendation{Cmd: "pt next --skip-checkpoints  # proceed without checkpoint", Kind: "work"})
				return printNext(out, *jsonOut)
			}
			// If the phase has an explicit demo checkpoint, prefer it at the end.
			if demo != nil {
				out.Mode = nextModeReview
				out.Why = []string{"project demo checkpoint recommended (not a hard gate)", "ensure there is a runnable, user-visible walkthrough with captured artifacts"}
				out.Recommended = []nextRecommendation{
					{Cmd: fmt.Sprintf("pt show %s", demo.ID), Kind: "review"},
					{Cmd: fmt.Sprintf("pt claim %s --as=%s", demo.ID, defaultIdentityForPrint()), Kind: "review"},
					{Cmd: fmt.Sprintf("pt review write %s --kind=demo --phase=%s --desc=\"demo\"  # creates .pt/reviews/... and links it", demo.ID, currentPhaseID), Kind: "review"},
					{Cmd: fmt.Sprintf("pt validate %s --yes", demo.ID), Kind: "review"},
					{Cmd: fmt.Sprintf("pt approve %s", demo.ID), Kind: "review"},
					{Cmd: "pt next", Kind: "review"},
				}
				out.Recommended = append(out.Recommended, nextRecommendation{Cmd: "pt next --skip-checkpoints  # proceed without checkpoint", Kind: "work"})
				return printNext(out, *jsonOut)
			}
		}
	}

	// 3) Pick candidate open tasks.
	var candidates []pt.Issue
	for _, iss := range allIssues {
		if iss.Status != "open" {
			continue
		}
		if strings.TrimSpace(*role) != "" {
			if !hasLabel(iss, fmt.Sprintf("role:%s", *role)) {
				continue
			}
		}
		if wf != nil && !*allPhases {
			meta, _ := pt.ParseTaskMeta(iss.Description)
			if wf.GetTaskPhase(iss, meta) != currentPhaseID {
				continue
			}
		}
		candidates = append(candidates, iss)
	}
	pt.SortIssues(candidates, "priority")

	// 4) Find the first task that is actually claimable (no blockers), and check workflow gates.
	var selected *pt.Issue
	var selectedMeta pt.TaskMeta
	var fallback *pt.Issue
	var fallbackMeta pt.TaskMeta
	var fallbackWhy []string

	skippedManual := 0
	skippedDeps := 0
	skippedSoftGate := 0
	skippedPreflight := 0
	for i := range candidates {
		task := candidates[i]
		if _, isBlocked := blocked[task.ID]; isBlocked {
			skippedManual++
			continue
		}
		blockers := readyBlockers(ctx, client, task)
		if len(blockers) > 0 {
			skippedDeps++
			continue
		}
		meta, _ := pt.ParseTaskMeta(task.Description)

		if wf != nil {
			canProceed, isHard, blockingPhase, msg := wf.CheckGate(task.ID, task, meta, allIssues, comments)
			if !canProceed {
				// Hard gates always block.
				if isHard || *strict {
					out.Mode = nextModeBlocked
					gateType := "soft"
					if isHard {
						gateType = "hard"
					}
					out.Blocking = &nextBlocking{
						GateType:      gateType,
						BlockingPhase: blockingPhase,
						Message:       msg,
						UnblockSteps: []string{
							"pt workflow status",
							fmt.Sprintf("pt ready --phase=%s --all-phases --verbose", blockingPhase),
							"pt next",
						},
					}
					out.Why = append(out.Why, "workflow gate prevents starting the next task")
					out.Recommended = []nextRecommendation{
						{Cmd: "pt workflow status", Kind: "unblock"},
						{Cmd: fmt.Sprintf("pt ready --phase=%s --all-phases --verbose", blockingPhase), Kind: "unblock"},
						{Cmd: "pt next", Kind: "unblock"},
					}
					// Soft-gate override path is explicit, but not the default.
					if !isHard {
						out.Recommended = append(out.Recommended, nextRecommendation{
							Cmd:  fmt.Sprintf("pt claim %s --as=%s --override-soft=\"<reason>\"", task.ID, defaultIdentityForPrint()),
							Kind: "unblock",
						})
					}

					// If this looks like an end-user checkpoint, call it out.
					if strings.Contains(strings.ToLower(msg), "user") || strings.Contains(strings.ToLower(blockingPhase), "review") {
						out.ApprovalsNeeded = append(out.ApprovalsNeeded, "end_user")
					} else {
						out.ApprovalsNeeded = append(out.ApprovalsNeeded, "internal")
					}
					return printNext(out, *jsonOut)
				}
				// Non-strict soft gate: still avoid recommending it as a normal WORK item,
				// because pt claim requires an explicit override.
				skippedSoftGate++
				continue
			}
		}

		// Preflight: missing file artifacts should not stall the whole workflow. Prefer tasks
		// whose referenced file exists, but if none do, still recommend the best candidate
		// with an explicit warning.
		if reason := unclaimableReason(task, meta); reason != "" {
			skippedPreflight++
			if fallback == nil {
				cp := task
				fallback = &cp
				fallbackMeta = meta
				fallbackWhy = []string{
					fmt.Sprintf("preflight warning: %s", reason),
					"if this file is intended to be created by the task, ignore this warning; otherwise, fix the artifact path via pt update or manifest",
				}
			}
			continue
		}

		selected = &task
		selectedMeta = meta
		break
	}
	if selected == nil && fallback != nil {
		selected = fallback
		selectedMeta = fallbackMeta
		out.Why = append(out.Why, fallbackWhy...)
	}

	// 5) WORK recommendation if we have any candidates.
	if selected != nil {
		task := *selected
		meta := selectedMeta
		out.Mode = nextModeWork
		out.Why = append(out.Why, "task is open and selected from the current focus set")
		if strings.TrimSpace(meta.Context) != "" {
			out.Why = append(out.Why, "context is available in task metadata")
		}
		out.Recommended = []nextRecommendation{
			{Cmd: fmt.Sprintf("pt show %s", task.ID), Kind: "work"},
			{Cmd: fmt.Sprintf("pt claim %s --as=%s", task.ID, defaultIdentityForPrint()), Kind: "work"},
			{Cmd: fmt.Sprintf("pt validate %s --yes", task.ID), Kind: "work"},
			{Cmd: "pt next", Kind: "work"},
		}
		return printNext(out, *jsonOut)
	}

	// 6) No claimable candidates: if there are open tasks at all, we’re blocked by deps, manual blocks, role filter, phase focus, or gates.
	openCount := 0
	openBlockedCount := 0
	for _, iss := range allIssues {
		if iss.Status == "open" {
			openCount++
			if _, isBlocked := blocked[iss.ID]; isBlocked {
				openBlockedCount++
			}
		}
	}
	if openCount > 0 {
		out.Mode = nextModeBlocked
		if openBlockedCount == openCount && openCount > 0 {
			out.Why = append(out.Why, "all open tasks are marked blocked")
			out.Recommended = []nextRecommendation{
				{Cmd: "pt blocked", Kind: "unblock"},
				{Cmd: "pt next", Kind: "unblock"},
			}
			return printNext(out, *jsonOut)
		}
		out.Why = append(out.Why, "open tasks exist but none are currently claimable (deps/blocked/gates/filters)")
		if len(candidates) > 0 {
			out.Why = append(out.Why, fmt.Sprintf("focus set: %d open tasks; skipped manual=%d deps=%d soft_gates=%d preflight=%d", len(candidates), skippedManual, skippedDeps, skippedSoftGate, skippedPreflight))
		} else if wf != nil && !*allPhases && strings.TrimSpace(currentPhaseID) != "" {
			out.Why = append(out.Why, fmt.Sprintf("no open tasks matched current phase focus (%s); try --all-phases", currentPhaseID))
		}
		// Point to one concrete task to inspect, plus the broad views.
		var openIssues []pt.Issue
		for _, iss := range allIssues {
			if iss.Status == "open" {
				openIssues = append(openIssues, iss)
			}
		}
		pt.SortIssues(openIssues, "priority")
		out.Recommended = []nextRecommendation{
			{Cmd: "pt workflow status", Kind: "unblock"},
			{Cmd: "pt ready --all-phases --verbose", Kind: "unblock"},
		}
		if len(openIssues) > 0 {
			out.Recommended = append(out.Recommended, nextRecommendation{Cmd: fmt.Sprintf("pt show %s", openIssues[0].ID), Kind: "unblock"})
		}
		out.Recommended = append(out.Recommended,
			nextRecommendation{Cmd: "pt blocked", Kind: "unblock"},
			nextRecommendation{Cmd: "pt next --all-phases", Kind: "unblock"},
		)
		return printNext(out, *jsonOut)
	}

	// 7) No open/in_progress/needs_review tasks: either PLAN (DoD missing) or DONE (DoD exists).
	if !dodExists {
		out.Mode = nextModePlan
		out.Why = []string{"no tasks remain but project DoD is missing"}
		out.Recommended = []nextRecommendation{
			{Cmd: fmt.Sprintf("cat %s  # (create this file)", dodPath), Kind: "plan"},
			{Cmd: "pt sync phases/<manifest>.toml  # or: pt add ...", Kind: "plan"},
			{Cmd: "pt next", Kind: "plan"},
		}
		return printNext(out, *jsonOut)
	}

	out.Mode = nextModeDone
	out.Why = []string{"no tasks remain and project DoD exists"}
	out.Recommended = []nextRecommendation{{Cmd: "pt ready", Kind: "done"}}
	return printNext(out, *jsonOut)
}

func printNext(out nextResult, jsonOut bool) error {
	if jsonOut {
		return printJSON(out)
	}

	fmt.Printf("Mode: %s\n", out.Mode)
	if out.CurrentPhase.ID != "" {
		fmt.Printf("Current phase: %s\n", out.CurrentPhase.ID)
	}
	if len(out.Why) > 0 {
		for _, w := range out.Why {
			fmt.Printf("Why: %s\n", w)
		}
	}
	if out.Blocking != nil && out.Blocking.Message != "" {
		fmt.Printf("Blocked: (%s gate, phase:%s) %s\n", out.Blocking.GateType, out.Blocking.BlockingPhase, out.Blocking.Message)
	}
	if len(out.Recommended) > 0 {
		fmt.Println("Next:")
		fmt.Printf("  %s\n", out.Recommended[0].Cmd)
	}
	fmt.Println("Then: pt next")
	return nil
}

func hasLabel(iss pt.Issue, label string) bool {
	for _, l := range iss.Labels {
		if l == label {
			return true
		}
	}
	return false
}

func defaultIdentityForPrint() string {
	// Avoid embedding a local username in suggested commands; prefer a shell placeholder.
	if u := strings.TrimSpace(os.Getenv("USER")); u != "" {
		_ = u
		return "$USER"
	}
	if u := strings.TrimSpace(os.Getenv("USERNAME")); u != "" {
		_ = u
		return "$USERNAME"
	}
	return "<you>"
}

func unclaimableReason(task pt.Issue, meta pt.TaskMeta) string {
	artifact := strings.TrimSpace(meta.Artifact)
	if artifact == "" {
		artifact = strings.TrimSpace(issueArtifact(task))
	}
	if strings.HasPrefix(artifact, "code:") {
		p := strings.TrimSpace(strings.TrimPrefix(artifact, "code:"))
		// Only treat explicit file artifacts as preflight blockers. Directories or symbolic
		// artifacts are allowed to be created by the task itself.
		if filepath.Ext(p) != "" {
			if _, err := os.Stat(p); err != nil {
				return fmt.Sprintf("missing artifact file: %s", p)
			}
		}
	}
	return ""
}
