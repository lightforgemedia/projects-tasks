package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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
	Mode            nextMode            `json:"mode"`
	Recommended     []nextRecommendation`json:"recommended"`
	Why             []string            `json:"why,omitempty"`
	CurrentPhase    nextPhase           `json:"current_phase,omitempty"`
	Blocking        *nextBlocking       `json:"blocking,omitempty"`
	ApprovalsNeeded []string            `json:"approvals_needed,omitempty"` // internal|end_user
	AskUser         bool                `json:"ask_user"`
	AskUserPrompts  []string            `json:"ask_user_prompts,omitempty"`
	ProjectDoD      nextProjectDoD      `json:"project_dod"`
}

func cmdNext(args []string) error {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	role := fs.String("role", "", "filter recommended work by role label")
	strict := fs.Bool("strict", false, "treat soft gates as blocking")
	allPhases := fs.Bool("all-phases", false, "debug: consider open tasks across all phases")
	workflowPath := fs.String("workflow", "", "path to workflow template (overrides auto-discovery)")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt next [--role=ROLE] [--strict] [--all-phases] [--workflow=PATH] [--json]")
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
	for i := range candidates {
		task := candidates[i]
		blockers := readyBlockers(ctx, client, task)
		if len(blockers) > 0 {
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
				continue
			}
		}

		selected = &task
		selectedMeta = meta
		break
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
	for _, iss := range allIssues {
		if iss.Status == "open" {
			openCount++
		}
	}
	if openCount > 0 {
		out.Mode = nextModeBlocked
		out.Why = append(out.Why, "open tasks exist but none are currently claimable (deps/manual blocks/gates/filters)")
		out.Recommended = []nextRecommendation{
			{Cmd: "pt workflow status", Kind: "unblock"},
			{Cmd: "pt ready --all-phases --verbose", Kind: "unblock"},
			{Cmd: "pt next --all-phases", Kind: "unblock"},
		}
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
