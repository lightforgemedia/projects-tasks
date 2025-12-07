package acpclient

import (
	"encoding/json"

	acp "github.com/coder/acp-go-sdk"
)

// ToolEvent is a structured log of a tool call/update.
type ToolEvent struct {
	SessionID  string         `json:"session_id"`
	ToolCallID string         `json:"tool_call_id"`
	Title      string         `json:"title,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Status     string         `json:"status,omitempty"`
	RawInput   map[string]any `json:"raw_input,omitempty"`
	RawOutput  map[string]any `json:"raw_output,omitempty"`
}

func fromToolCall(sess string, tc *acp.SessionUpdateToolCall) ToolEvent {
	return ToolEvent{
		SessionID:  sess,
		ToolCallID: string(tc.ToolCallId),
		Title:      tc.Title,
		Kind:       string(tc.Kind),
		Status:     string(tc.Status),
		RawInput:   toMap(tc.RawInput),
		RawOutput:  toMap(tc.RawOutput),
	}
}

func fromToolCallUpdate(sess string, tc *acp.SessionToolCallUpdate) ToolEvent {
	ev := ToolEvent{
		SessionID:  sess,
		ToolCallID: string(tc.ToolCallId),
		RawInput:   toMap(tc.RawInput),
		RawOutput:  toMap(tc.RawOutput),
	}
	if tc.Title != nil {
		ev.Title = *tc.Title
	}
	if tc.Kind != nil {
		ev.Kind = string(*tc.Kind)
	}
	if tc.Status != nil {
		ev.Status = string(*tc.Status)
	}
	return ev
}

// toMap attempts to convert an arbitrary RawInput/RawOutput to map for logging.
func toMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	// Attempt JSON round-trip if possible
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}
