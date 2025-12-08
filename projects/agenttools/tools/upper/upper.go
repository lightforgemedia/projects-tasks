package upper

import (
	"context"
	"strings"

	"agenttools/internal/agenttools"
)

// UpperRequest is the input payload for the upper tool.
type UpperRequest struct {
	Text string `json:"text"`
}

// UpperResponse is the output payload for the upper tool.
type UpperResponse struct {
	Upper string `json:"upper"`
}

// Tool uppercases the provided text.
var Tool = agenttools.NewTool(
	agenttools.ToolConfig[UpperRequest, UpperResponse]{
		Name:        "upper",
		Description: "Uppercase the provided text.",
	},
	func(ctx context.Context, req UpperRequest) (UpperResponse, error) {
		return UpperResponse{Upper: strings.ToUpper(req.Text)}, nil
	},
)

func init() {
	agenttools.Default.MustRegister(Tool)
}
