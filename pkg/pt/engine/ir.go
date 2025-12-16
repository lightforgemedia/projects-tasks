package engine

import "projects-tasks/pkg/pt"

// IR is a compiled, internal representation of a workflow template.
// It is derived from pt.Workflow and is intended to be stable for evaluation.
type IR struct {
	Workflow pt.Workflow
	Phases   []PhaseIR
}

type PhaseIR struct {
	Phase pt.Phase
	Gate  GateIR
}

type GateIR struct {
	Type           string
	Condition      string
	ReminderBefore string
	ReminderAfter  string
	BlockMessage   string

	Expr GateExpr
}

type GateContext struct {
	Workflow   pt.Workflow
	Phase      pt.Phase
	PhaseTasks []pt.Issue
	AllIssues  []pt.Issue
	Comments   map[string][]string
}

// GateExpr evaluates gate conditions. It returns (satisfied, reason).
type GateExpr interface {
	Eval(ctx GateContext) (bool, string)
	String() string
}

