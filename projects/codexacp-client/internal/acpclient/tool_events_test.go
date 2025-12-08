package acpclient

import (
	"bytes"
	"io"
	"os"
	"strings"
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

func TestFromToolCallUpdateWithFields(t *testing.T) {
	kind := acp.ToolKindExecute
	status := acp.ToolCallStatusCompleted
	title := "run ls"
	tc := &acp.SessionToolCallUpdate{
		ToolCallId: "call-3",
		Kind:       &kind,
		Status:     &status,
		Title:      &title,
		RawInput:   map[string]any{"cmd": "ls"},
	}
	ev := fromToolCallUpdate("sess-3", tc)
	if ev.Kind != "execute" || ev.Status != "completed" || ev.Title != "run ls" {
		t.Fatalf("unexpected fields: %+v", ev)
	}
	if ev.RawInput["cmd"] != "ls" {
		t.Fatalf("raw input missing: %+v", ev.RawInput)
	}
}

func TestLogToolEventOutput(t *testing.T) {
	var buf bytes.Buffer
	swapStdout(&buf, func() {
		_ = logToolEvent(ToolEvent{SessionID: "sess-log", ToolCallID: "call-log", Status: "completed"})
	})
	out := buf.String()
	if !strings.Contains(out, "[tool-event]") || !strings.Contains(out, `"call-log"`) {
		t.Fatalf("log output missing expected content: %q", out)
	}
}

// swapStdout captures stdout during fn for testing.
func swapStdout(w io.Writer, fn func()) {
	old := os.Stdout
	r, pw, _ := os.Pipe()
	os.Stdout = pw
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(w, r)
		close(done)
	}()
	fn()
	_ = pw.Close()
	os.Stdout = old
	<-done
}
