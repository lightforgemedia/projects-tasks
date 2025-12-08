package agenttools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MockTool struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}

func mockEchoTool() MockTool {
	tool := mcp.NewTool("echo",
		mcp.WithDescription("Echo text back"),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to echo")),
	)
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(text), nil
	}
	return MockTool{Tool: tool, Handler: handler}
}
