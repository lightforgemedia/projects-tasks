package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"codexacp-client/internal/acpclient"
)

// gemini-acp is a thin CLI wrapper that launches the Gemini CLI in ACP mode,
// sends a single prompt, and streams the response/tool events to stdout.
//
// Notes / parity with the upstream acp-go-sdk example:
//   - The official example blocks on a REPL; here we send a single prompt and exit.
//   - Timeout is optional (default 0 = no timeout) to avoid premature "context deadline exceeded"
//     errors during initialize/prompt, matching the forgiving behavior of the upstream sample.
//   - If you need MCP tools, wire mcpServers into NewSessionRequest (flags provided below).
func main() {
	var (
		agent     = flag.String("agent", "gemini", "path to the Gemini CLI binary")
		agentArgs = flag.String("agent-args", "--experimental-acp", "args to pass to Gemini (space separated)")
		cwdFlag   = flag.String("cwd", ".", "working directory for the ACP session")
		prompt    = flag.String("prompt", "Summarize this workspace.", "prompt text to send")
		timeout   = flag.Duration("timeout", 0, "overall timeout (0 = no timeout)")
		debug     = flag.Bool("debug", false, "enable debug logging")
		mcpCmd    = flag.String("mcp-cmd", "", "MCP server binary to launch via stdio (optional)")
		mcpArgs   = flag.String("mcp-args", "-transport=stdio", "args for MCP server (space separated; optional)")
		mcpEnv    = flag.String("mcp-env", "", "comma-separated KEY=VALUE env vars for MCP server (optional)")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}

	cwd, err := filepath.Abs(*cwdFlag)
	if err != nil {
		fatal("resolve cwd", err)
	}

	args := fields(*agentArgs)
	stdin, stdout, wait, kill, err := acpclient.StartAgentProcess(ctx, acpclient.AgentProcessConfig{
		Path: *agent,
		Args: args,
	})
	if err != nil {
		fatal("start gemini", err)
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
		ClientInfo: &acp.Implementation{
			Name:    "gemini-acp",
			Title:   ptr("Gemini ACP Prompt"),
			Version: "0.0.1",
		},
	}); err != nil {
		printRequestError("initialize", err)
		return
	}

	var mcpServers []acp.McpServer
	if strings.TrimSpace(*mcpCmd) != "" {
		mcpServers = acpclient.BuildMcpServers(*mcpCmd, fields(*mcpArgs), parseEnv(*mcpEnv))
	}
	session, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: mcpServers,
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
		return
	}

	done := make(chan error, 1)
	go func() { done <- wait() }()

	select {
	case <-ctx.Done():
	case <-done:
	default:
		// Give trailing updates a brief window; exit if the agent finishes early.
		select {
		case <-time.After(5 * time.Second):
		case <-done:
		case <-ctx.Done():
		}
	}
}

func fields(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Fields(s)
}

func parseEnv(s string) []acp.EnvVariable {
	if strings.TrimSpace(s) == "" {
		return []acp.EnvVariable{}
	}
	parts := strings.Split(s, ",")
	out := make([]acp.EnvVariable, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		out = append(out, acp.EnvVariable{Name: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
	}
	return out
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
