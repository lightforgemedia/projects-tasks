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
	"syscall"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"codexacp-client/internal/acpclient"
)

func main() {
	var (
		binPath = flag.String("codex-acp-path", "codex-acp", "path to codex-acp adapter binary")
		cwdFlag = flag.String("cwd", ".", "working directory for the ACP session")
		prompt  = flag.String("prompt", "Summarize this workspace.", "prompt to send to the agent")
		debug   = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cwd, err := filepath.Abs(*cwdFlag)
	if err != nil {
		fatal("resolve cwd", err)
	}

	stdin, stdout, wait, kill, err := acpclient.StartAgentProcess(ctx, acpclient.AgentProcessConfig{
		Path: *binPath,
	})
	if err != nil {
		fatal("start codex-acp", err)
	}
	defer func() { _ = kill() }()

	client := &acpclient.CodexClient{}
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	if *debug {
		conn.SetLogger(slog.Default())
	}

	initReq := acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
		ClientInfo: &acp.Implementation{
			Name:    "codexacp-client",
			Title:   ptr("Codex ACP Client"),
			Version: "0.1.0",
		},
	}

	if _, err := conn.Initialize(ctx, initReq); err != nil {
		printRequestError("initialize", err)
		return
	}

	session, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
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

	// Wait briefly for trailing updates then exit cleanly.
	done := make(chan error, 1)
	go func() { done <- wait() }()

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	case <-done:
	}
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

func ptr[T any](v T) *T {
	return &v
}
