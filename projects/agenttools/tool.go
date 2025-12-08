package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
)

// DynamicTool is the provider-agnostic interface exposed to registries/adapters.
type DynamicTool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	OutputSchema() json.RawMessage
	Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// ToolConfig holds metadata for typed tool construction.
type ToolConfig[Req any, Resp any] struct {
	Name        string
	Description string
}

type typedTool[Req any, Resp any] struct {
	cfg ToolConfig[Req, Resp]
	fn  func(ctx context.Context, req Req) (Resp, error)
}

// NewTool wraps a typed function and exposes DynamicTool.
func NewTool[Req any, Resp any](cfg ToolConfig[Req, Resp], fn func(ctx context.Context, req Req) (Resp, error)) DynamicTool {
	return &typedTool[Req, Resp]{cfg: cfg, fn: fn}
}

func (t *typedTool[Req, Resp]) Name() string        { return t.cfg.Name }
func (t *typedTool[Req, Resp]) Description() string { return t.cfg.Description }

func (t *typedTool[Req, Resp]) InputSchema() json.RawMessage {
	return generateJSONSchema[Req]()
}

func (t *typedTool[Req, Resp]) OutputSchema() json.RawMessage {
	return generateJSONSchema[Resp]()
}

func (t *typedTool[Req, Resp]) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var req Req
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}
	resp, err := t.fn(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}
