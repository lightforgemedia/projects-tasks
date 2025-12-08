package acpclient

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestMcpServersFieldPresent(t *testing.T) {
	req := acp.NewSessionRequest{
		Cwd: "/tmp",
		McpServers: []acp.McpServer{
			{Stdio: &acp.McpServerStdio{Name: "demo", Command: "echo", Args: []string{"hi"}}},
		},
	}
	if len(req.McpServers) != 1 || req.McpServers[0].Stdio == nil || req.McpServers[0].Stdio.Name != "demo" {
		t.Fatalf("unexpected mcpServers: %+v", req.McpServers)
	}
}
