package acpclient

import (
	acp "github.com/coder/acp-go-sdk"
)

// BuildMcpServers constructs stdio MCP server entries for ACP sessions.
// If cmd is empty, return an empty slice (never nil). Some agents validate that
// mcpServers must be an array, and reject null.
func BuildMcpServers(cmd string, args []string, env []acp.EnvVariable) []acp.McpServer {
	if cmd == "" {
		return []acp.McpServer{}
	}
	// Some agents validate that env is an array, not null. Ensure we send an empty slice when unset.
	if env == nil {
		env = []acp.EnvVariable{}
	}
	return []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{
				Name:    "agenttools-mcp",
				Command: cmd,
				Args:    args,
				Env:     env,
			},
		},
	}
}
