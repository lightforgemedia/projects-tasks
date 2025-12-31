package engine

import (
	"testing"

	"projects-tasks/pkg/pt"
)

func TestV2EvaluateGate_AllClosed(t *testing.T) {
	w := pt.Workflow{
		Name: "wf",
		PhaseAssignment: pt.PhaseAssignment{
			LabelPrefix: "phase:",
		},
		Phases: []pt.Phase{
			{ID: "prove", Name: "Prove", Order: 1, Gate: pt.Gate{Type: "soft", Condition: "all_closed"}},
		},
	}
	e, err := NewV2(w)
	if err != nil {
		t.Fatalf("NewV2: %v", err)
	}

	phase := w.Phases[0]
	phaseTasks := []pt.Issue{
		{ID: "pt-1", Status: "open", Labels: []string{"phase:prove"}},
		{ID: "pt-2", Status: "closed", Labels: []string{"phase:prove"}},
	}

	ok, reason := e.EvaluateGate(phase, phaseTasks, phaseTasks, map[string][]string{})
	if ok {
		t.Fatalf("expected gate to fail")
	}
	if reason == "" {
		t.Fatalf("expected failure reason")
	}
}

func TestV2EvaluateGate_SoftOverride(t *testing.T) {
	w := pt.Workflow{
		Name: "wf",
		PhaseAssignment: pt.PhaseAssignment{
			LabelPrefix: "phase:",
		},
		Phases: []pt.Phase{
			{ID: "prove", Name: "Prove", Order: 1, Gate: pt.Gate{Type: "soft", Condition: "all_closed"}},
		},
	}
	e, err := NewV2(w)
	if err != nil {
		t.Fatalf("NewV2: %v", err)
	}

	phase := w.Phases[0]
	phaseTasks := []pt.Issue{{ID: "pt-1", Status: "open", Labels: []string{"phase:prove"}}}
	comments := map[string][]string{"pt-1": {"gate-override:prove"}}

	ok, reason := e.EvaluateGate(phase, phaseTasks, phaseTasks, comments)
	if !ok {
		t.Fatalf("expected soft override to satisfy gate, got reason=%q", reason)
	}
	if reason == "" {
		t.Fatalf("expected override reason")
	}
}

func TestV2EvaluateGate_PhaseComplete(t *testing.T) {
	w := pt.Workflow{
		Name: "wf",
		PhaseAssignment: pt.PhaseAssignment{
			LabelPrefix: "phase:",
		},
		Phases: []pt.Phase{
			{ID: "prove", Name: "Prove", Order: 1},
			{ID: "build", Name: "Build", Order: 2, Gate: pt.Gate{Type: "hard", Condition: "phase:prove complete"}},
		},
	}
	e, err := NewV2(w)
	if err != nil {
		t.Fatalf("NewV2: %v", err)
	}

	all := []pt.Issue{
		{ID: "pt-1", Status: "open", Labels: []string{"phase:prove"}},
		{ID: "pt-2", Status: "open", Labels: []string{"phase:build"}},
	}
	phase := w.Phases[1]

	ok, reason := e.EvaluateGate(phase, []pt.Issue{{ID: "pt-2", Status: "open", Labels: []string{"phase:build"}}}, all, map[string][]string{})
	if ok {
		t.Fatalf("expected gate to fail while prove is incomplete")
	}
	if reason == "" {
		t.Fatalf("expected failure reason")
	}
}

func TestV2CheckGate_PriorPhaseBlocksLater(t *testing.T) {
	w := pt.Workflow{
		Name: "wf",
		PhaseAssignment: pt.PhaseAssignment{
			LabelPrefix: "phase:",
		},
		Phases: []pt.Phase{
			{ID: "prove", Name: "Prove", Order: 1, Gate: pt.Gate{Type: "hard", Condition: "all_closed"}},
			{ID: "build", Name: "Build", Order: 2},
		},
	}
	e, err := NewV2(w)
	if err != nil {
		t.Fatalf("NewV2: %v", err)
	}

	all := []pt.Issue{
		{ID: "pt-1", Status: "open", Labels: []string{"phase:prove"}},
		{ID: "pt-2", Status: "open", Labels: []string{"phase:build"}},
	}
	task := all[1]
	meta, _ := pt.ParseTaskMeta(task.Description)

	ok, isHard, phaseID, msg := e.CheckGate(task.ID, task, meta, all, map[string][]string{})
	if ok || !isHard || phaseID != "prove" || msg == "" {
		t.Fatalf("expected hard block on prove; got ok=%v hard=%v phase=%q msg=%q", ok, isHard, phaseID, msg)
	}
}
