package echo

import (
	"context"

	"agenttools/internal/agenttools"
)

// EchoRequest is the input payload for the echo tool.
type EchoRequest struct {
	Text string `json:"text"`
}

// EchoResponse is the output payload for the echo tool.
type EchoResponse struct {
	Echoed string `json:"echoed"`
}

// Tool is the exported dynamic tool.
var Tool = agenttools.NewTool(
	agenttools.ToolConfig[EchoRequest, EchoResponse]{
		Name:        "echo",
		Description: "Echo back the provided text.",
	},
	func(ctx context.Context, req EchoRequest) (EchoResponse, error) {
		return EchoResponse{Echoed: req.Text}, nil
	},
)

// Register on import.
func init() {
	agenttools.Default.MustRegister(Tool)
}
