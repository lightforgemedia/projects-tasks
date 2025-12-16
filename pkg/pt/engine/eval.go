package engine

import (
	"fmt"
	"strings"

	"projects-tasks/pkg/pt"
)

type AllClosedExpr struct{}

func (AllClosedExpr) String() string { return "all_closed" }

func (AllClosedExpr) Eval(ctx GateContext) (bool, string) {
	openCount := 0
	for _, t := range ctx.PhaseTasks {
		if t.Status != "closed" && t.Status != "done" {
			openCount++
		}
	}
	if openCount > 0 {
		return false, fmt.Sprintf("%d tasks still open in phase %q", openCount, ctx.Phase.ID)
	}
	return true, ""
}

type PhaseCompleteExpr struct {
	PhaseID string
}

func (e PhaseCompleteExpr) String() string { return fmt.Sprintf("phase:%s complete", e.PhaseID) }

func (e PhaseCompleteExpr) Eval(ctx GateContext) (bool, string) {
	for _, issue := range ctx.AllIssues {
		meta, _ := pt.ParseTaskMeta(issue.Description)
		if ctx.Workflow.GetTaskPhase(issue, meta) == e.PhaseID {
			if issue.Status != "closed" && issue.Status != "done" {
				return false, fmt.Sprintf("phase %q not complete", e.PhaseID)
			}
		}
	}
	return true, ""
}

type HasCommentExpr struct {
	Tag string
}

func (e HasCommentExpr) String() string { return fmt.Sprintf("has_comment:%s", e.Tag) }

func (e HasCommentExpr) Eval(ctx GateContext) (bool, string) {
	tag := e.Tag
	if len(ctx.PhaseTasks) == 0 {
		return true, fmt.Sprintf("phase %q has no tasks; skipping comment gate %q", ctx.Phase.ID, tag)
	}
	for _, t := range ctx.PhaseTasks {
		if taskComments, ok := ctx.Comments[t.ID]; ok {
			for _, c := range taskComments {
				if strings.Contains(strings.ToLower(c), strings.ToLower(tag)) {
					return true, ""
				}
			}
		}
	}
	return false, fmt.Sprintf("no comment containing %q found", tag)
}

type PhaseHasCommentExpr struct {
	PhaseID string
	Tag     string
}

func (e PhaseHasCommentExpr) String() string {
	return fmt.Sprintf("phase:%s has_comment:%s", e.PhaseID, e.Tag)
}

func (e PhaseHasCommentExpr) Eval(ctx GateContext) (bool, string) {
	reqPhase := e.PhaseID
	tag := e.Tag
	phaseTaskCount := 0
	for _, issue := range ctx.AllIssues {
		meta, _ := pt.ParseTaskMeta(issue.Description)
		if ctx.Workflow.GetTaskPhase(issue, meta) != reqPhase {
			continue
		}
		phaseTaskCount++
		if taskComments, ok := ctx.Comments[issue.ID]; ok {
			for _, c := range taskComments {
				if strings.Contains(strings.ToLower(c), strings.ToLower(tag)) {
					return true, ""
				}
			}
		}
	}
	if phaseTaskCount == 0 {
		return true, fmt.Sprintf("phase %q has no tasks; skipping comment gate %q", reqPhase, tag)
	}
	return false, fmt.Sprintf("no comment containing %q found in phase %q", tag, reqPhase)
}

type DiscoveryApprovedExpr struct {
	ComponentID string
}

func (e DiscoveryApprovedExpr) String() string {
	return fmt.Sprintf("discovery:approved:%s", e.ComponentID)
}

func (e DiscoveryApprovedExpr) Eval(ctx GateContext) (bool, string) {
	componentID := e.ComponentID
	expectedComment := fmt.Sprintf("discovery-approved: %s", componentID)
	for _, t := range ctx.PhaseTasks {
		if taskComments, ok := ctx.Comments[t.ID]; ok {
			for _, c := range taskComments {
				if strings.Contains(strings.ToLower(c), strings.ToLower(expectedComment)) {
					return true, ""
				}
			}
		}
	}
	return false, fmt.Sprintf("discovery not approved for component %q", componentID)
}

type OrExpr struct {
	Parts []GateExpr
}

func (e OrExpr) String() string {
	var parts []string
	for _, p := range e.Parts {
		if p == nil {
			continue
		}
		parts = append(parts, p.String())
	}
	return strings.Join(parts, " OR ")
}

func (e OrExpr) Eval(ctx GateContext) (bool, string) {
	for _, part := range e.Parts {
		if part == nil {
			continue
		}
		satisfied, _ := part.Eval(ctx)
		if satisfied {
			return true, ""
		}
	}
	return false, "no OR condition satisfied"
}

type UnknownExpr struct {
	Condition string
}

func (e UnknownExpr) String() string { return e.Condition }

func (e UnknownExpr) Eval(_ GateContext) (bool, string) {
	return false, fmt.Sprintf("unknown gate condition %q (supported: all_closed, phase:X complete, has_comment:X, phase:X has_comment:Y, discovery:approved:X)", e.Condition)
}
