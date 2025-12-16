package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestWorkflowTemplates_ParseAndCompile(t *testing.T) {
	// This test ensures that shipped workflow templates remain compatible with:
	// - V1 loader (`pt.ParseWorkflow`)
	// - V2 engine compiler (`engine.NewV2`)
	//
	// Keep the path relative to the package dir (pkg/pt/engine).
	pattern := filepath.Join("..", "..", "..", "workflows", "*.toml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob workflows: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no workflow templates found at %s", pattern)
	}

	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if len(raw) == 0 {
				t.Fatalf("empty workflow file")
			}

			wf, err := pt.ParseWorkflow(f)
			if err != nil {
				t.Fatalf("parse workflow: %v", err)
			}
			if _, err := NewV2(wf); err != nil {
				t.Fatalf("compile workflow v2: %v", err)
			}
		})
	}
}

func TestCompile_InvalidGateExpressionIncludesPhaseContext(t *testing.T) {
	wf := pt.Workflow{
		Name: "wf",
		PhaseAssignment: pt.PhaseAssignment{
			DefaultPhase: "build",
			LabelPrefix:  "phase:",
		},
		Phases: []pt.Phase{
			{
				ID:    "build",
				Name:  "Build",
				Order: 1,
				Gate: pt.Gate{
					Type:      "hard",
					Condition: "phase: complete", // invalid: missing phase id
				},
			},
		},
	}

	_, err := Compile(wf)
	if err == nil {
		t.Fatalf("expected compile error for invalid gate condition")
	}
	// Must mention the phase id and the condition string for actionable debugging.
	if want := "phase \"build\""; !strings.Contains(err.Error(), want) {
		t.Fatalf("error missing %q: %v", want, err)
	}
	if want := "phase: complete"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error missing condition %q: %v", want, err)
	}
}

func TestCompile_ORConditionParses(t *testing.T) {
	wf := pt.Workflow{
		Name: "wf",
		PhaseAssignment: pt.PhaseAssignment{
			DefaultPhase: "build",
			LabelPrefix:  "phase:",
		},
		Phases: []pt.Phase{
			{
				ID:    "build",
				Name:  "Build",
				Order: 1,
				Gate: pt.Gate{
					Type:      "soft",
					Condition: "has_comment:ok OR has_comment:accepted",
				},
			},
		},
	}
	if _, err := Compile(wf); err != nil {
		t.Fatalf("compile OR condition: %v", err)
	}
}
