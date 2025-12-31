package engine

import (
	"fmt"
	"sort"
	"strings"

	"projects-tasks/pkg/pt"
)

// V2 is a workflow engine backed by compiled IR.
// It is feature-flagged behind PT_ENGINE=v2 in the CLI.
type V2 struct {
	ir *IR
}

func NewV2(workflow pt.Workflow) (*V2, error) {
	ir, err := Compile(workflow)
	if err != nil {
		return nil, err
	}
	return &V2{ir: ir}, nil
}

func (e *V2) Workflow() pt.Workflow {
	return e.ir.Workflow
}

// EvaluateGate checks if a phase's gate condition is satisfied.
// Returns (satisfied, reason).
func (e *V2) EvaluateGate(phase pt.Phase, phaseTasks []pt.Issue, allIssues []pt.Issue, comments map[string][]string) (bool, string) {
	if strings.TrimSpace(phase.Gate.Condition) == "" {
		return true, ""
	}

	// Soft-gate override: if any task comment contains "gate-override:<phase-id>",
	// treat the gate as satisfied. This provides an explicit, auditable bypass.
	if strings.TrimSpace(phase.Gate.Type) == "soft" && hasGateOverride(phase.ID, comments) {
		return true, fmt.Sprintf("soft gate overridden for phase %q", phase.ID)
	}

	expr := e.lookupExpr(phase.ID)
	if expr == nil {
		// Should not happen; fall back to v1-compatible unknown failure.
		return false, fmt.Sprintf("unknown gate condition %q (supported: all_closed, phase:X complete, has_comment:X, phase:X has_comment:Y, discovery:approved:X)", strings.TrimSpace(phase.Gate.Condition))
	}

	ok, reason := expr.Eval(GateContext{
		Workflow:   e.ir.Workflow,
		Phase:      phase,
		PhaseTasks: phaseTasks,
		AllIssues:  allIssues,
		Comments:   comments,
	})
	return ok, reason
}

func (e *V2) lookupExpr(phaseID string) GateExpr {
	for _, p := range e.ir.Phases {
		if p.Phase.ID == phaseID {
			return p.Gate.Expr
		}
	}
	return nil
}

// CheckGate evaluates phase gates for a specific task.
// Returns: (canProceed, isHardBlock, blockingPhaseID, message)
func (e *V2) CheckGate(taskID string, issue pt.Issue, meta pt.TaskMeta, allIssues []pt.Issue, comments map[string][]string) (bool, bool, string, string) {
	workflow := e.ir.Workflow
	phaseID := workflow.GetTaskPhase(issue, meta)
	phase := workflow.GetPhaseByID(phaseID)
	if phase == nil {
		return true, false, "", "" // no phase, no gate
	}

	// Stable, order-sorted phase list.
	phases := append([]pt.Phase{}, workflow.Phases...)
	sort.Slice(phases, func(i, j int) bool { return phases[i].Order < phases[j].Order })

	phaseIdx := -1
	for i := range phases {
		if phases[i].ID == phaseID {
			phaseIdx = i
			break
		}
	}

	// Check all prior phases' gates.
	for i := 0; i < phaseIdx; i++ {
		priorPhase := phases[i]

		var priorTasks []pt.Issue
		for _, iss := range allIssues {
			m, _ := pt.ParseTaskMeta(iss.Description)
			if workflow.GetTaskPhase(iss, m) == priorPhase.ID {
				priorTasks = append(priorTasks, iss)
			}
		}

		satisfied, reason := e.EvaluateGate(priorPhase, priorTasks, allIssues, comments)
		if !satisfied {
			isHard := priorPhase.Gate.Type == "hard"
			msg := priorPhase.Gate.ReminderBefore
			if isHard && priorPhase.Gate.BlockMessage != "" {
				msg = priorPhase.Gate.BlockMessage
			}
			if msg == "" {
				msg = reason
			}
			return false, isHard, priorPhase.ID, msg
		}
	}

	// Also check the current phase's entry gate for phases after the first.
	// For the first phase (index 0), the phase gate is treated as an exit gate.
	if phaseIdx > 0 && strings.TrimSpace(phase.Gate.Condition) != "" {
		var currentPhaseTasks []pt.Issue
		for _, iss := range allIssues {
			m, _ := pt.ParseTaskMeta(iss.Description)
			if workflow.GetTaskPhase(iss, m) == phaseID {
				currentPhaseTasks = append(currentPhaseTasks, iss)
			}
		}
		satisfied, reason := e.EvaluateGate(*phase, currentPhaseTasks, allIssues, comments)
		if !satisfied {
			isHard := phase.Gate.Type == "hard"
			msg := phase.Gate.ReminderBefore
			if isHard && phase.Gate.BlockMessage != "" {
				msg = phase.Gate.BlockMessage
			}
			if msg == "" {
				msg = reason
			}
			return false, isHard, phase.ID, msg
		}
	}

	return true, false, "", ""
}

func hasGateOverride(phaseID string, comments map[string][]string) bool {
	if strings.TrimSpace(phaseID) == "" || len(comments) == 0 {
		return false
	}

	needle1 := strings.ToLower("gate-override:" + phaseID)
	needle2 := strings.ToLower("gate-override: " + phaseID)

	for _, cs := range comments {
		for _, c := range cs {
			lc := strings.ToLower(c)
			if strings.Contains(lc, needle1) || strings.Contains(lc, needle2) {
				return true
			}
		}
	}
	return false
}

// BuildWorkflowStatus matches V1's status model but uses the V2 gate evaluator.
func (e *V2) BuildWorkflowStatus(allIssues []pt.Issue, comments map[string][]string) pt.WorkflowStatus {
	workflow := e.ir.Workflow
	status := pt.WorkflowStatus{Workflow: workflow}

	phaseIssues := make(map[string][]pt.Issue)
	for _, iss := range allIssues {
		meta, _ := pt.ParseTaskMeta(iss.Description)
		phaseID := workflow.GetTaskPhase(iss, meta)
		phaseIssues[phaseID] = append(phaseIssues[phaseID], iss)
	}

	phases := append([]pt.Phase{}, workflow.Phases...)
	sort.Slice(phases, func(i, j int) bool { return phases[i].Order < phases[j].Order })

	for i, phase := range phases {
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

		if ps.TotalTasks > 0 && ps.ClosedTasks < ps.TotalTasks {
			for j := 0; j < i; j++ {
				prior := phases[j]
				priorTasks := phaseIssues[prior.ID]
				satisfied, reason := e.EvaluateGate(prior, priorTasks, allIssues, comments)
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

			if !ps.IsBlocked && i > 0 && strings.TrimSpace(phase.Gate.Condition) != "" {
				satisfied, reason := e.EvaluateGate(phase, tasks, allIssues, comments)
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

// CurrentPhaseID returns the current workflow phase following the V1 "first unfinished phase with tasks" heuristic.
func (e *V2) CurrentPhaseID(allIssues []pt.Issue, roleFilter string) (string, *pt.Phase) {
	workflow := e.ir.Workflow
	phases := append([]pt.Phase{}, workflow.Phases...)
	sort.Slice(phases, func(i, j int) bool { return phases[i].Order < phases[j].Order })

	for _, p := range phases {
		assigned := 0
		unfinished := 0
		for _, iss := range allIssues {
			if strings.TrimSpace(roleFilter) != "" && !hasLabel(iss, fmt.Sprintf("role:%s", strings.TrimSpace(roleFilter))) {
				continue
			}
			meta, _ := pt.ParseTaskMeta(iss.Description)
			if workflow.GetTaskPhase(iss, meta) != p.ID {
				continue
			}
			assigned++
			if iss.Status != "closed" && iss.Status != "done" {
				unfinished++
			}
		}
		if assigned > 0 && unfinished > 0 {
			cp := p
			return p.ID, &cp
		}
	}
	return "", nil
}

func hasLabel(iss pt.Issue, label string) bool {
	for _, l := range iss.Labels {
		if l == label {
			return true
		}
	}
	return false
}
