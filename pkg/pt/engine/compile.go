package engine

import (
	"fmt"
	"sort"
	"strings"

	"projects-tasks/pkg/pt"
)

// Compile converts a pt.Workflow into a compiled IR suitable for evaluation.
func Compile(workflow pt.Workflow) (*IR, error) {
	phases := append([]pt.Phase{}, workflow.Phases...)
	sort.Slice(phases, func(i, j int) bool { return phases[i].Order < phases[j].Order })

	ir := &IR{Workflow: workflow}
	for _, p := range phases {
		g := p.Gate
		gate := GateIR{
			Type:           strings.TrimSpace(g.Type),
			Condition:      strings.TrimSpace(g.Condition),
			ReminderBefore: g.ReminderBefore,
			ReminderAfter:  g.ReminderAfter,
			BlockMessage:   g.BlockMessage,
		}
		if gate.Condition != "" {
			expr, err := compileGateExpr(gate.Condition)
			if err != nil {
				return nil, fmt.Errorf("phase %q gate condition %q: %w", p.ID, gate.Condition, err)
			}
			gate.Expr = expr
		}
		ir.Phases = append(ir.Phases, PhaseIR{Phase: p, Gate: gate})
	}

	return ir, nil
}

func compileGateExpr(cond string) (GateExpr, error) {
	c := strings.TrimSpace(cond)
	if c == "" {
		return nil, nil
	}

	// Compound OR conditions.
	if strings.Contains(c, " OR ") {
		parts := strings.Split(c, " OR ")
		var exprs []GateExpr
		for _, part := range parts {
			sub := strings.TrimSpace(part)
			if sub == "" {
				continue
			}
			ex, err := compileGateExpr(sub)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, ex)
		}
		return OrExpr{Parts: exprs}, nil
	}

	switch {
	case c == "all_closed":
		return AllClosedExpr{}, nil
	case strings.HasPrefix(c, "phase:") && strings.HasSuffix(c, " complete"):
		reqPhase := strings.TrimSuffix(strings.TrimPrefix(c, "phase:"), " complete")
		reqPhase = strings.TrimSpace(reqPhase)
		if reqPhase == "" {
			return nil, fmt.Errorf("invalid gate condition %q", cond)
		}
		return PhaseCompleteExpr{PhaseID: reqPhase}, nil
	case strings.HasPrefix(c, "has_comment:"):
		tag := strings.TrimSpace(strings.TrimPrefix(c, "has_comment:"))
		if tag == "" {
			return nil, fmt.Errorf("invalid gate condition %q", cond)
		}
		return HasCommentExpr{Tag: tag}, nil
	case strings.HasPrefix(c, "phase:") && strings.Contains(c, " has_comment:"):
		parts := strings.SplitN(c, " has_comment:", 2)
		reqPhase := strings.TrimSpace(strings.TrimPrefix(parts[0], "phase:"))
		tag := strings.TrimSpace(parts[1])
		if reqPhase == "" || tag == "" {
			return nil, fmt.Errorf("invalid gate condition %q", cond)
		}
		return PhaseHasCommentExpr{PhaseID: reqPhase, Tag: tag}, nil
	case strings.HasPrefix(c, "discovery:approved:"):
		componentID := strings.TrimSpace(strings.TrimPrefix(c, "discovery:approved:"))
		if componentID == "" {
			return nil, fmt.Errorf("invalid gate condition %q", cond)
		}
		return DiscoveryApprovedExpr{ComponentID: componentID}, nil
	default:
		return UnknownExpr{Condition: c}, nil
	}
}
