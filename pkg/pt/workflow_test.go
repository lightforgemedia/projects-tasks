package pt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWorkflow(t *testing.T) {
	dir := t.TempDir()
	content := `
name = "frontend-first"
description = "Validate external deps → Build frontend → Wire backend → Validate"

[phase_assignment]
default_phase = "implementation"
label_prefix = "phase:"

[phase_assignment.by_template]
external_dep_validation = "risk"
frontend_component = "frontend"
backend_endpoint = "backend"

[[phases]]
id = "risk"
name = "Risk Assessment"
order = 1
description = "Validate external dependencies"

[phases.gate]
type = "soft"
condition = "all_closed"
reminder_before = "External deps not validated"
reminder_after = "Risk assessment complete"

[phases.proof]
required = true
description = "Capture actual API response"
hint = "Save to outputs/<task-id>/response.json"

[[phases]]
id = "frontend"
name = "Frontend Development"
order = 2
description = "Build UI against mocks"

[phases.gate]
type = "soft"
condition = "phase:risk complete"
reminder_before = "Starting frontend without risk validation"

[phases.checkpoint]
trigger = "first_task_complete"
prompt = "Request user approval on UI"

[[phases]]
id = "backend"
name = "Backend Integration"
order = 3

[phases.gate]
type = "hard"
condition = "has_comment:user-approved"
block_message = "Need user approval on frontend first"

[[phases]]
id = "validation"
name = "Final Validation"
order = 4
`
	path := filepath.Join(dir, "workflow.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := ParseWorkflow(path)
	if err != nil {
		t.Fatalf("ParseWorkflow: %v", err)
	}

	if wf.Name != "frontend-first" {
		t.Errorf("name = %q, want 'frontend-first'", wf.Name)
	}

	if len(wf.Phases) != 4 {
		t.Fatalf("got %d phases, want 4", len(wf.Phases))
	}

	// Check phases are sorted by order
	for i, p := range wf.Phases {
		if p.Order != i+1 {
			t.Errorf("phase %d order = %d, want %d", i, p.Order, i+1)
		}
	}

	// Check phase_assignment
	if wf.PhaseAssignment.DefaultPhase != "implementation" {
		t.Errorf("default_phase = %q, want 'implementation'", wf.PhaseAssignment.DefaultPhase)
	}
	if wf.PhaseAssignment.ByTemplate["frontend_component"] != "frontend" {
		t.Errorf("by_template[frontend_component] = %q, want 'frontend'", wf.PhaseAssignment.ByTemplate["frontend_component"])
	}

	// Check gate
	risk := wf.GetPhaseByID("risk")
	if risk == nil {
		t.Fatal("risk phase not found")
	}
	if risk.Gate.Type != "soft" {
		t.Errorf("risk gate type = %q, want 'soft'", risk.Gate.Type)
	}
	if risk.Gate.Condition != "all_closed" {
		t.Errorf("risk gate condition = %q, want 'all_closed'", risk.Gate.Condition)
	}

	// Check checkpoint
	frontend := wf.GetPhaseByID("frontend")
	if frontend.Checkpoint == nil {
		t.Fatal("frontend checkpoint is nil")
	}
	if frontend.Checkpoint.Trigger != "first_task_complete" {
		t.Errorf("checkpoint trigger = %q, want 'first_task_complete'", frontend.Checkpoint.Trigger)
	}

	// Check hard gate
	backend := wf.GetPhaseByID("backend")
	if backend.Gate.Type != "hard" {
		t.Errorf("backend gate type = %q, want 'hard'", backend.Gate.Type)
	}

	// Check proof
	if risk.Proof == nil {
		t.Fatal("risk proof is nil")
	}
	if !risk.Proof.Required {
		t.Error("risk proof should be required")
	}
}

func TestGetTaskPhase(t *testing.T) {
	wf := Workflow{
		PhaseAssignment: PhaseAssignment{
			DefaultPhase: "implementation",
			LabelPrefix:  "phase:",
			ByTemplate: map[string]string{
				"frontend_component": "frontend",
				"backend_endpoint":   "backend",
			},
		},
		Phases: []Phase{
			{ID: "risk", Order: 1},
			{ID: "frontend", Order: 2},
			{ID: "backend", Order: 3},
		},
	}

	tests := []struct {
		name     string
		issue    Issue
		meta     TaskMeta
		expected string
	}{
		{
			name:     "explicit label takes priority",
			issue:    Issue{Labels: []string{"phase:risk", "other"}},
			meta:     TaskMeta{Template: "frontend_component"},
			expected: "risk",
		},
		{
			name:     "template mapping",
			issue:    Issue{Labels: []string{"other"}},
			meta:     TaskMeta{Template: "frontend_component"},
			expected: "frontend",
		},
		{
			name:     "default phase",
			issue:    Issue{Labels: []string{}},
			meta:     TaskMeta{Template: "bug_fix"},
			expected: "implementation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wf.GetTaskPhase(tt.issue, tt.meta)
			if got != tt.expected {
				t.Errorf("GetTaskPhase = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEvaluateGate(t *testing.T) {
	wf := Workflow{
		Phases: []Phase{
			{ID: "risk", Order: 1, Gate: Gate{Type: "soft", Condition: "all_closed"}},
		},
	}

	t.Run("all_closed satisfied", func(t *testing.T) {
		tasks := []Issue{
			{ID: "pt-1", Status: "closed"},
			{ID: "pt-2", Status: "done"},
		}
		satisfied, reason := wf.EvaluateGate(wf.Phases[0], tasks, tasks, nil)
		if !satisfied {
			t.Errorf("expected satisfied, got reason: %s", reason)
		}
	})

	t.Run("all_closed not satisfied", func(t *testing.T) {
		tasks := []Issue{
			{ID: "pt-1", Status: "closed"},
			{ID: "pt-2", Status: "open"},
		}
		satisfied, reason := wf.EvaluateGate(wf.Phases[0], tasks, tasks, nil)
		if satisfied {
			t.Error("expected not satisfied")
		}
		if reason == "" {
			t.Error("expected reason")
		}
	})

	t.Run("has_comment satisfied", func(t *testing.T) {
		phase := Phase{Gate: Gate{Condition: "has_comment:user-approved"}}
		tasks := []Issue{{ID: "pt-1"}}
		comments := map[string][]string{
			"pt-1": {"user-approved: looks good"},
		}
		satisfied, _ := wf.EvaluateGate(phase, tasks, tasks, comments)
		if !satisfied {
			t.Error("expected satisfied with matching comment")
		}
	})

	t.Run("has_comment not satisfied", func(t *testing.T) {
		phase := Phase{Gate: Gate{Condition: "has_comment:user-approved"}}
		tasks := []Issue{{ID: "pt-1"}}
		comments := map[string][]string{
			"pt-1": {"some other comment"},
		}
		satisfied, _ := wf.EvaluateGate(phase, tasks, tasks, comments)
		if satisfied {
			t.Error("expected not satisfied without matching comment")
		}
	})

	t.Run("phase:X has_comment:Y satisfied", func(t *testing.T) {
		wf2 := Workflow{
			PhaseAssignment: PhaseAssignment{LabelPrefix: "phase:"},
			Phases:          []Phase{{ID: "frontend", Order: 1}},
		}
		phase := Phase{Gate: Gate{Condition: "phase:frontend has_comment:user-approved"}}
		allIssues := []Issue{
			{ID: "pt-1", Labels: []string{"phase:frontend"}},
			{ID: "pt-2", Labels: []string{"phase:backend"}},
		}
		comments := map[string][]string{
			"pt-1": {"user-approved: UI looks good"},
		}
		satisfied, _ := wf2.EvaluateGate(phase, nil, allIssues, comments)
		if !satisfied {
			t.Error("expected satisfied with matching comment in specified phase")
		}
	})

	t.Run("phase:X has_comment:Y not satisfied", func(t *testing.T) {
		wf2 := Workflow{
			PhaseAssignment: PhaseAssignment{LabelPrefix: "phase:"},
			Phases:          []Phase{{ID: "frontend", Order: 1}},
		}
		phase := Phase{Gate: Gate{Condition: "phase:frontend has_comment:user-approved"}}
		allIssues := []Issue{
			{ID: "pt-1", Labels: []string{"phase:frontend"}},
			{ID: "pt-2", Labels: []string{"phase:backend"}},
		}
		comments := map[string][]string{
			"pt-2": {"user-approved: but wrong phase"},
		}
		satisfied, _ := wf2.EvaluateGate(phase, nil, allIssues, comments)
		if satisfied {
			t.Error("expected not satisfied - comment in wrong phase")
		}
	})

	t.Run("unknown condition fails closed", func(t *testing.T) {
		phase := Phase{Gate: Gate{Condition: "some_unknown_condition"}}
		tasks := []Issue{{ID: "pt-1", Status: "closed"}}
		satisfied, reason := wf.EvaluateGate(phase, tasks, tasks, nil)
		if satisfied {
			t.Error("unknown gate condition should fail closed, not pass")
		}
		if !strings.Contains(reason, "unknown gate condition") {
			t.Errorf("expected clear error message, got: %s", reason)
		}
	})
}

func TestCheckGate(t *testing.T) {
	wf := Workflow{
		PhaseAssignment: PhaseAssignment{
			LabelPrefix: "phase:",
		},
		Phases: []Phase{
			{ID: "risk", Order: 1, Gate: Gate{Type: "soft", Condition: "all_closed", ReminderBefore: "Risk not validated"}},
			{ID: "frontend", Order: 2, Gate: Gate{Type: "hard", Condition: "phase:risk complete", BlockMessage: "Complete risk first"}},
		},
	}

	t.Run("first phase has no blockers", func(t *testing.T) {
		issue := Issue{ID: "pt-1", Labels: []string{"phase:risk"}}
		meta := TaskMeta{}
		canProceed, isHard, _ := wf.CheckGate("pt-1", issue, meta, nil, nil)
		if !canProceed {
			t.Error("first phase should have no blockers")
		}
		if isHard {
			t.Error("should not be hard blocked")
		}
	})

	t.Run("second phase blocked by first (soft)", func(t *testing.T) {
		allIssues := []Issue{
			{ID: "pt-1", Status: "open", Labels: []string{"phase:risk"}},
			{ID: "pt-2", Status: "open", Labels: []string{"phase:frontend"}},
		}
		issue := allIssues[1]
		meta := TaskMeta{}
		canProceed, isHard, msg := wf.CheckGate("pt-2", issue, meta, allIssues, nil)
		if canProceed {
			t.Error("should be blocked by incomplete risk phase")
		}
		if isHard {
			t.Error("risk gate is soft, should not be hard blocked")
		}
		if msg == "" {
			t.Error("expected message")
		}
	})

	t.Run("hard gate blocks", func(t *testing.T) {
		// Risk complete, but frontend hard gate not satisfied
		wf2 := Workflow{
			PhaseAssignment: PhaseAssignment{LabelPrefix: "phase:"},
			Phases: []Phase{
				{ID: "risk", Order: 1, Gate: Gate{Type: "soft", Condition: "all_closed"}},
				{ID: "frontend", Order: 2, Gate: Gate{Type: "hard", Condition: "has_comment:user-approved", BlockMessage: "Need approval"}},
			},
		}
		allIssues := []Issue{
			{ID: "pt-1", Status: "closed", Labels: []string{"phase:risk"}},
			{ID: "pt-2", Status: "open", Labels: []string{"phase:frontend"}},
		}
		issue := allIssues[1]
		meta := TaskMeta{}
		canProceed, isHard, msg := wf2.CheckGate("pt-2", issue, meta, allIssues, nil)
		if canProceed {
			t.Error("should be blocked by hard gate")
		}
		if !isHard {
			t.Error("should be hard blocked")
		}
		if msg != "Need approval" {
			t.Errorf("msg = %q, want 'Need approval'", msg)
		}
	})
}

func TestWorkflowValidation(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		dir := t.TempDir()
		content := `
[[phases]]
id = "test"
order = 1
`
		path := filepath.Join(dir, "wf.toml")
		os.WriteFile(path, []byte(content), 0644)
		_, err := ParseWorkflow(path)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("no phases", func(t *testing.T) {
		dir := t.TempDir()
		content := `name = "empty"`
		path := filepath.Join(dir, "wf.toml")
		os.WriteFile(path, []byte(content), 0644)
		_, err := ParseWorkflow(path)
		if err == nil {
			t.Error("expected error for no phases")
		}
	})

	t.Run("invalid gate type", func(t *testing.T) {
		dir := t.TempDir()
		content := `
name = "test"

[[phases]]
id = "p1"
order = 1

[phases.gate]
type = "invalid"
`
		path := filepath.Join(dir, "wf.toml")
		os.WriteFile(path, []byte(content), 0644)
		_, err := ParseWorkflow(path)
		if err == nil {
			t.Error("expected error for invalid gate type")
		}
	})
}
