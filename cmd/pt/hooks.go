package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type hookDefaults struct {
	Timeout string
	OnFail  string
	Verbose bool
}

type hookEntry struct {
	Event   string
	Cmd     string
	OnFail  string
	Timeout string
}

type hookConfig struct {
	Defaults hookDefaults
	Hooks    []hookEntry
}

type hookPayload struct {
	ID         string
	Title      string
	Assignee   string
	Actor      string
	StatusFrom string
	StatusTo   string
	Role       string
	DoDJSON    string
}

var loadedHooks *hookConfig

func loadHooks() (*hookConfig, error) {
	if skipHooks() {
		return nil, nil
	}
	path := os.Getenv("PT_HOOKS")
	if strings.TrimSpace(path) != "" {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("stat hooks: %w", err)
		}
		return parseHooksTOML(path)
	}

	var merged hookConfig
	paths := []string{
		userHooksPath(),
		"hooks.toml",
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		cfg, err := parseHooksTOML(p)
		if err != nil {
			return nil, err
		}
		if merged.Defaults.Timeout == "" {
			merged.Defaults.Timeout = cfg.Defaults.Timeout
		}
		if merged.Defaults.OnFail == "" {
			merged.Defaults.OnFail = cfg.Defaults.OnFail
		}
		merged.Hooks = append(merged.Hooks, cfg.Hooks...)
	}
	if len(merged.Hooks) == 0 {
		return nil, nil
	}
	return &merged, nil
}

func parseHooksTOML(path string) (*hookConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read hooks: %w", err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	cfg := &hookConfig{}
	var current *hookEntry
	context := "root" // root, defaults, hook
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(stripHookComment(scanner.Text()))
		if line == "" {
			continue
		}
		switch line {
		case "[defaults]":
			context = "defaults"
			continue
		case "[[hook]]":
			if current != nil {
				cfg.Hooks = append(cfg.Hooks, *current)
			}
			current = &hookEntry{}
			context = "hook"
			continue
		}
		key, val, err := parseHookKV(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		switch context {
		case "defaults":
			switch key {
			case "timeout":
				cfg.Defaults.Timeout = val
			case "on_fail":
				cfg.Defaults.OnFail = strings.ToLower(val)
			case "verbose":
				cfg.Defaults.Verbose = parseBool(val)
			default:
				return nil, fmt.Errorf("line %d: unknown defaults key %q", lineNum, key)
			}
		case "hook":
			if current == nil {
				return nil, fmt.Errorf("line %d: hook fields outside hook block", lineNum)
			}
			switch key {
			case "event":
				current.Event = val
			case "cmd":
				current.Cmd = val
			case "on_fail":
				current.OnFail = strings.ToLower(val)
			case "timeout":
				current.Timeout = val
			default:
				return nil, fmt.Errorf("line %d: unknown hook key %q", lineNum, key)
			}
		default:
			return nil, fmt.Errorf("line %d: unexpected content at root", lineNum)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan hooks: %w", err)
	}
	if current != nil {
		cfg.Hooks = append(cfg.Hooks, *current)
	}
	return cfg, nil
}

func stripHookComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

func parseHookKV(line string) (string, string, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected key = value, got %q", line)
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	val = strings.Trim(val, `"`)
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}
	return key, val, nil
}

func parseBool(v string) bool {
	v = strings.TrimSpace(strings.Trim(v, `"`))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type HookResult struct {
	Event   string `json:"event"`
	Command string `json:"command"`
	Status  string `json:"status"`
	Output  string `json:"output,omitempty"`
}

func runHooks(event string, payload hookPayload) ([]HookResult, error) {
	if skipHooks() || loadedHooks == nil {
		return nil, nil
	}
	matches := matchingHooks(event, loadedHooks.Hooks)
	var results []HookResult
	for _, h := range matches {
		res, err := runHook(h, loadedHooks.Defaults, event, payload)
		results = append(results, res)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func matchingHooks(event string, hooks []hookEntry) []hookEntry {
	var out []hookEntry
	for _, h := range hooks {
		if h.Event == event {
			out = append(out, h)
		}
	}
	return out
}

func runHook(h hookEntry, defaults hookDefaults, event string, payload hookPayload) (HookResult, error) {
	cmdStr := strings.TrimSpace(h.Cmd)
	if cmdStr == "" {
		return HookResult{}, nil
	}
	cmdStr = applyPlaceholders(cmdStr, payload)
	timeout := h.Timeout
	if timeout == "" {
		timeout = defaults.Timeout
	}
	if timeout == "" {
		timeout = "15s"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		dur = 15 * time.Second
	}

	onFail := h.OnFail
	if onFail == "" {
		onFail = defaults.OnFail
	}
	if onFail == "" {
		if strings.HasPrefix(event, "pre-") {
			onFail = "fail"
		} else {
			onFail = "warn"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	jsonPayload, _ := json.Marshal(payload)
	env := append(os.Environ(),
		"PT_EVENT="+event,
		"PT_ID="+payload.ID,
		"PT_TITLE="+payload.Title,
		"PT_ASSIGNEE="+payload.Assignee,
		"PT_ACTOR="+payload.Actor,
		"PT_STATUS_FROM="+payload.StatusFrom,
		"PT_STATUS_TO="+payload.StatusTo,
		"PT_ROLE="+payload.Role,
		"PT_DOD="+payload.DoDJSON,
	)

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Env = env
	wd, _ := os.Getwd()
	cmd.Dir = wd
	stdinBuf := bytes.NewBuffer(jsonPayload)
	cmd.Stdin = stdinBuf
	start := time.Now()
	if hookVerbose() {
		fmt.Fprintf(os.Stderr, "[hook] %s -> %s\n", event, cmdStr)
	}
	out, err := cmd.CombinedOutput()
	if hookVerbose() {
		status := "ok"
		if err != nil {
			status = "fail"
		}
		fmt.Fprintf(os.Stderr, "[hook] %s completed (%s) in %s\n", event, status, time.Since(start))
	}
	status := "ok"
	if err != nil {
		snippet := strings.TrimSpace(string(out))
		if snippet == "" {
			snippet = err.Error()
		}
		if strings.ToLower(onFail) == "fail" {
			return HookResult{Event: event, Command: cmdStr, Status: "fail", Output: strings.TrimSpace(string(out))}, fmt.Errorf("hook %s failed running %q: %s", event, cmdStr, snippet)
		}
		status = "warn"
		fmt.Fprintf(os.Stderr, "[hook warn] %s running %q: %s\n", event, cmdStr, snippet)
	}
	if len(out) > 0 {
		fmt.Fprintf(os.Stderr, "[hook ok] %s: %s\n", event, strings.TrimSpace(string(out)))
	}
	return HookResult{Event: event, Command: cmdStr, Status: status, Output: strings.TrimSpace(string(out))}, nil
}

func applyPlaceholders(s string, p hookPayload) string {
	repl := strings.NewReplacer(
		"{{id}}", p.ID,
		"{{title}}", p.Title,
		"{{assignee}}", p.Assignee,
		"{{actor}}", p.Actor,
		"{{status_from}}", p.StatusFrom,
		"{{status_to}}", p.StatusTo,
		"{{role}}", p.Role,
	)
	return repl.Replace(s)
}

func skipHooks() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PT_SKIP_HOOKS")))
	return v == "1" || v == "true" || v == "yes"
}

func hookVerbose() bool {
	if loadedHooks != nil && loadedHooks.Defaults.Verbose {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PT_HOOK_VERBOSE")))
	return v == "1" || v == "true" || v == "yes"
}

func userHooksPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "pt", "hooks.toml")
}
