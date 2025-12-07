package acpclient

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestFromToolCall(t *testing.T) {
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "call-1",
		Title:      "run",
		Kind:       acp.ToolKindExecute,
		Status:     acp.ToolCallStatusPending,
		RawInput:   map[string]any{"cmd": "ls"},
		RawOutput:  map[string]any{"ok": true},
	}

	ev := fromToolCall("sess-1", tc)

	if ev.SessionID != "sess-1" || ev.ToolCallID != "call-1" {
		t.Fatalf("unexpected ids: %+v", ev)
	}
	if ev.Title != "run" || ev.Kind != "execute" || ev.Status != "pending" {
		t.Fatalf("unexpected fields: %+v", ev)
	}
	if ev.RawInput["cmd"] != "ls" || ev.RawOutput["ok"] != true {
		t.Fatalf("raw fields not captured: %+v", ev)
	}
}

func TestFromToolCallUpdateOptionalFields(t *testing.T) {
	tc := &acp.SessionToolCallUpdate{
		ToolCallId: "call-2",
		RawOutput:  map[string]any{"ok": true},
	}

	ev := fromToolCallUpdate("sess-2", tc)

	if ev.Title != "" || ev.Kind != "" || ev.Status != "" {
		t.Fatalf("optional fields should be empty: %+v", ev)
	}
	if ev.RawOutput["ok"] != true {
		t.Fatalf("raw output not captured: %+v", ev)
	}
}

func TestToMapStructRoundTrip(t *testing.T) {
	type payload struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	out := toMap(payload{A: "x", B: 2})
	if out["a"] != "x" || out["b"].(float64) != float64(2) {
		t.Fatalf("unexpected map contents: %+v", out)
	}
}
