package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"codexacp-client/internal/acpclient"
)

// acp-mcp-smoke launches an ACP-capable agent (e.g., gemini --experimental-acp or claude)
// and registers an MCP server via session/new.mcpServers. It sends a prompt that asks the
// agent to call the echo tool so we can observe whether the agent actually honors the MCP
// registration. This is a manual harness; it will hang or fail if the agent binary requires
// credentials and they are not provided in the environment.
func main() {
	var (
		agentPath = flag.String("agent", "gemini", "path to ACP-capable agent binary (gemini, claude, etc.)")
		agentArgs = flag.String("agent-args", "--experimental-acp", "args passed to the agent (space separated)")
		mcpCmd    = flag.String("mcp-cmd", "", "MCP server binary to launch via stdio (required)")
		mcpArgs   = flag.String("mcp-args", "-transport=stdio", "args for the MCP server (space separated)")
		cwd       = flag.String("cwd", ".", "working directory for the ACP session")
		prompt    = flag.String("prompt", "Call the MCP echo tool with text=hello", "prompt to send to the agent")
		timeout   = flag.Duration("timeout", 20*time.Second, "overall timeout for the session")
		debug     = flag.Bool("debug", false, "enable slog debug output")
	)
	flag.Parse()

	if strings.TrimSpace(*mcpCmd) == "" {
		fmt.Fprintf(os.Stderr, "mcp-cmd is required (point to your MCP server binary, e.g. projects/agenttools/cmd/mcp-server)\n")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, *timeout)
	defer cancel()

	agentArgsList := fields(*agentArgs)
	stdin, stdout, wait, kill, err := acpclient.StartAgentProcess(ctx, acpclient.AgentProcessConfig{
		Path: *agentPath,
		Args: agentArgsList,
	})
	if err != nil {
		fatal("start agent", err)
	}
	defer func() { _ = kill() }()

	client := &acpclient.CodexClient{}
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	if *debug {
		conn.SetLogger(slog.Default())
	}

	if _, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
		ClientInfo: &acp.Implementation{Name: "acp-mcp-smoke", Title: ptr("ACP MCP Smoke"), Version: "0.0.1"},
	}); err != nil {
		printRequestError("initialize", err)
		return
	}

	mcpArgsList := fields(*mcpArgs)
	session, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        *cwd,
		McpServers: acpclient.BuildMcpServers(*mcpCmd, mcpArgsList, nil),
	})
	if err != nil {
		printRequestError("newSession", err)
		return
	}

	if _, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(*prompt)},
	}); err != nil {
		printRequestError("prompt", err)
	}

	done := make(chan error, 1)
	go func() { done <- wait() }()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

func fields(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Fields(s)
}

func printRequestError(label string, err error) {
	if re, ok := err.(*acp.RequestError); ok {
		if b, mErr := json.MarshalIndent(re, "", "  "); mErr == nil {
			fmt.Fprintf(os.Stderr, "%s error: %s\n", label, string(b))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "%s error: %v\n", label, err)
}

func fatal(label string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
	os.Exit(1)
}

func ptr[T any](v T) *T { return &v }
