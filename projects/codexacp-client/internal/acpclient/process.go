package acpclient

import (
    "context"
    "io"
    "os"
    "os/exec"
)

// AgentProcessConfig holds settings to launch codex-acp (or any ACP-speaking binary).
type AgentProcessConfig struct {
    Path string   // binary path; default "codex-acp"
    Args []string // extra args
    Env  []string // additional env; appended to os.Environ
}

// StartAgentProcess starts the adapter process and returns stdio + wait/kill.
func StartAgentProcess(ctx context.Context, cfg AgentProcessConfig) (stdin io.WriteCloser, stdout io.ReadCloser, wait func() error, kill func() error, err error) {
    bin := cfg.Path
    if bin == "" {
        bin = "codex-acp"
    }

    cmd := exec.CommandContext(ctx, bin, cfg.Args...) //nolint:gosec
    cmd.Env = append(os.Environ(), cfg.Env...)

    stdinPipe, err := cmd.StdinPipe()
    if err != nil {
        return nil, nil, nil, nil, err
    }
    stdoutPipe, err := cmd.StdoutPipe()
    if err != nil {
        return nil, nil, nil, nil, err
    }
    cmd.Stderr = os.Stderr

    if err := cmd.Start(); err != nil {
        return nil, nil, nil, nil, err
    }

    wait = func() error {
        return cmd.Wait()
    }
    kill = func() error {
        return cmd.Process.Kill()
    }
    return stdinPipe, stdoutPipe, wait, kill, nil
}
