package agenttools

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// This test ensures the MCP server can be constructed with tool capabilities enabled.
func TestMCPServerBuilds(t *testing.T) {
	s := server.NewMCPServer("test-mcp", "0.0.1", server.WithToolCapabilities(true))
	echoTool := mockEchoTool()
	s.AddTool(echoTool.Tool, echoTool.Handler)
	// No serve here; just ensure no panic on construction.
}
