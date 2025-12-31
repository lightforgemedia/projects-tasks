package pt

import "testing"

func TestWorkflowGateUXPreflightDone(t *testing.T) {
	wf := Workflow{}
	phase := Phase{ID: "p1", Gate: Gate{Condition: "ux_preflight_done"}}

	withPreflight := Issue{
		ID:          "pt-1",
		Title:       "A",
		Description: `x<!-- pt-meta: {"template":"frontend_component","role":"dev","dod":{"tests":["echo ok"],"manual":"check","criteria":["ok"]},"ux":{"type":"web"},"ux_state":{"preflight_done":true}} -->`,
	}
	ok, reason := wf.EvaluateGate(phase, []Issue{withPreflight}, []Issue{withPreflight}, map[string][]string{})
	if !ok || reason != "" {
		t.Fatalf("expected satisfied, got ok=%v reason=%q", ok, reason)
	}

	missingPreflight := Issue{
		ID:          "pt-2",
		Title:       "B",
		Description: `x<!-- pt-meta: {"template":"frontend_component","role":"dev","dod":{"tests":["echo ok"],"manual":"check","criteria":["ok"]},"ux":{"type":"web"},"ux_state":{"preflight_done":false}} -->`,
	}
	ok, reason = wf.EvaluateGate(phase, []Issue{missingPreflight}, []Issue{missingPreflight}, map[string][]string{})
	if ok || reason == "" {
		t.Fatalf("expected unsatisfied with reason, got ok=%v reason=%q", ok, reason)
	}
}
