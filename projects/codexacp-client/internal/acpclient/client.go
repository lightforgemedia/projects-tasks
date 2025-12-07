package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	acp "github.com/coder/acp-go-sdk"
)

// CodexClient implements acp.Client and stubs terminal support.
type CodexClient struct{}

var _ acp.Client = (*CodexClient)(nil)

// RequestPermission auto-approves the first option (placeholder).
func (c *CodexClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	resp := acp.RequestPermissionResponse{}
	if len(p.Options) > 0 {
		resp.Outcome.Selected = &acp.RequestPermissionOutcomeSelected{
			OptionId: p.Options[0].OptionId,
			Outcome:  "selected",
		}
	}
	return resp, nil
}

// SessionUpdate streams agent messages and logs tool events.
func (c *CodexClient) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	if n.Update.AgentMessageChunk != nil {
		blk := n.Update.AgentMessageChunk.Content
		if blk.Text != nil {
			fmt.Print(blk.Text.Text)
		}
	}
	if n.Update.ToolCall != nil {
		ev := fromToolCall(string(n.SessionId), n.Update.ToolCall)
		_ = logToolEvent(ev)
	}
	if n.Update.ToolCallUpdate != nil {
		ev := fromToolCallUpdate(string(n.SessionId), n.Update.ToolCallUpdate)
		_ = logToolEvent(ev)
	}
	return nil
}

func logToolEvent(ev ToolEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	fmt.Printf("[tool-event] %s\n", string(b))
	return nil
}

// ReadTextFile reads a file; enforces absolute paths.
func (c *CodexClient) ReadTextFile(ctx context.Context, req acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if !filepath.IsAbs(req.Path) {
		return acp.ReadTextFileResponse{}, errors.New("path must be absolute")
	}
	data, err := os.ReadFile(req.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}
	content := string(data)
	return acp.ReadTextFileResponse{Content: content}, nil
}

// WriteTextFile writes content to a file; enforces absolute paths.
func (c *CodexClient) WriteTextFile(ctx context.Context, req acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	if !filepath.IsAbs(req.Path) {
		return acp.WriteTextFileResponse{}, errors.New("path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(req.Path), 0o755); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	if err := os.WriteFile(req.Path, []byte(req.Content), fs.FileMode(0o644)); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, nil
}

// Terminal stubs: these are placeholders; adapt for real terminal usage.
func (c *CodexClient) CreateTerminal(ctx context.Context, req acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{TerminalId: "term-1"}, nil
}

func (c *CodexClient) KillTerminalCommand(ctx context.Context, req acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	return acp.KillTerminalCommandResponse{}, nil
}

func (c *CodexClient) TerminalOutput(ctx context.Context, req acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{Output: "terminal output not implemented", Truncated: false}, nil
}

func (c *CodexClient) ReleaseTerminal(ctx context.Context, req acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *CodexClient) WaitForTerminalExit(ctx context.Context, req acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}
