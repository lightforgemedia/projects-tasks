package acpclient

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestBuildMcpServers(t *testing.T) {
	env := []acp.EnvVariable{{Name: "KEY", Value: "VAL"}}
	servers := BuildMcpServers("mcp-server", []string{"--stdio"}, env)
	if len(servers) != 1 || servers[0].Stdio == nil {
		t.Fatalf("expected one stdio server: %+v", servers)
	}
	if servers[0].Stdio.Command != "mcp-server" || len(servers[0].Stdio.Args) != 1 {
		t.Fatalf("unexpected stdio entry: %+v", servers[0].Stdio)
	}
	if servers[0].Stdio.Env[0].Name != "KEY" || servers[0].Stdio.Env[0].Value != "VAL" {
		t.Fatalf("env not propagated: %+v", servers[0].Stdio.Env)
	}

	// Ensure env is not null when omitted.
	servers = BuildMcpServers("mcp-server", nil, nil)
	if servers[0].Stdio == nil || servers[0].Stdio.Env == nil {
		t.Fatalf("expected empty env slice, got nil: %+v", servers[0].Stdio)
	}
	if len(servers[0].Stdio.Env) != 0 {
		t.Fatalf("expected empty env slice: %+v", servers[0].Stdio.Env)
	}

	servers = BuildMcpServers("", nil, nil)
	if len(servers) != 0 {
		t.Fatalf("expected no servers when cmd empty")
	}
}
