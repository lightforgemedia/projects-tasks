package acpclient

import (
	acp "github.com/coder/acp-go-sdk"
)

// BuildMcpServers constructs stdio MCP server entries for ACP sessions.
// If cmd is empty, no servers are returned.
func BuildMcpServers(cmd string, args []string, env []acp.EnvVariable) []acp.McpServer {
	if cmd == "" {
		return nil
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
