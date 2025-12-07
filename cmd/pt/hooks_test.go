package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHooksExecutesWithEnv(t *testing.T) {
	td := t.TempDir()
	outPath := filepath.Join(td, "out.txt")
	hooksPath := filepath.Join(td, "hooks.toml")
	cfg := `
[[hook]]
event = "post-validate"
cmd = "echo $PT_ID $PT_EVENT > ` + outPath + `"
`
	if err := os.WriteFile(hooksPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	t.Setenv("PT_HOOKS", hooksPath)
	t.Setenv("PT_SKIP_HOOKS", "")
	var err error
	loadedHooks, err = loadHooks()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	t.Cleanup(func() { loadedHooks = nil })

	payload := hookPayload{ID: "pt-99"}
	res, err := runHooks("post-validate", payload)
	if err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	if len(res) != 1 || res[0].Status != "ok" {
		t.Fatalf("expected ok hook result, got %+v", res)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "pt-99 post-validate" {
		t.Fatalf("unexpected hook output: %q", got)
	}
}

func TestRunHooksPayloadAvailable(t *testing.T) {
	td := t.TempDir()
	outPath := filepath.Join(td, "out.txt")
	hooksPath := filepath.Join(td, "hooks.toml")
	cfg := `
[[hook]]
event = "post-validate"
cmd = "cat > ` + outPath + `"
`
	if err := os.WriteFile(hooksPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	t.Setenv("PT_HOOKS", hooksPath)
	t.Setenv("PT_SKIP_HOOKS", "")
	var err error
	loadedHooks, err = loadHooks()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	t.Cleanup(func() { loadedHooks = nil })
	payload := hookPayload{ID: "pt-100", Title: "T", Assignee: "a", DoDJSON: `{"manual":"m"}`}
	if _, err := runHooks("post-validate", payload); err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if !strings.Contains(string(data), "pt-100") || !strings.Contains(string(data), "manual") {
		t.Fatalf("payload missing fields: %s", string(data))
	}
}

func TestHookVerboseFromDefaults(t *testing.T) {
	loadedHooks = &hookConfig{Defaults: hookDefaults{Verbose: true}}
	t.Cleanup(func() { loadedHooks = nil })
	if !hookVerbose() {
		t.Fatalf("expected hookVerbose from defaults")
	}
}

func TestPreHookFailBlocks(t *testing.T) {
	td := t.TempDir()
	hooksPath := filepath.Join(td, "hooks.toml")
	cfg := `
[[hook]]
event = "pre-claim"
cmd = "exit 3"
on_fail = "fail"
`
	if err := os.WriteFile(hooksPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	t.Setenv("PT_HOOKS", hooksPath)
	t.Setenv("PT_SKIP_HOOKS", "")
	var err error
	loadedHooks, err = loadHooks()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	t.Cleanup(func() { loadedHooks = nil })
	if _, err := runHooks("pre-claim", hookPayload{ID: "pt-1"}); err == nil {
		t.Fatalf("expected pre-hook failure")
	}
}

func TestPostHookWarnDoesNotBlock(t *testing.T) {
	td := t.TempDir()
	hooksPath := filepath.Join(td, "hooks.toml")
	cfg := `
[[hook]]
event = "post-validate"
cmd = "exit 2"
on_fail = "warn"
`
	if err := os.WriteFile(hooksPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	t.Setenv("PT_HOOKS", hooksPath)
	t.Setenv("PT_SKIP_HOOKS", "")
	var err error
	loadedHooks, err = loadHooks()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	t.Cleanup(func() { loadedHooks = nil })
	if _, err := runHooks("post-validate", hookPayload{ID: "pt-1"}); err != nil {
		t.Fatalf("warn hook should not block: %v", err)
	}
}

func TestGlobalAndLocalHooksMerge(t *testing.T) {
	td := t.TempDir()
	globalDir := filepath.Join(td, ".config", "pt")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	globalHooks := filepath.Join(globalDir, "hooks.toml")
	localHooks := filepath.Join(td, "hooks.toml")

	if err := os.WriteFile(globalHooks, []byte("[[hook]]\nevent=\"post-validate\"\ncmd=\"echo global > "+filepath.Join(td, "g.txt")+"\"\n"), 0644); err != nil {
		t.Fatalf("write global: %v", err)
	}
	if err := os.WriteFile(localHooks, []byte("[[hook]]\nevent=\"post-validate\"\ncmd=\"echo local > "+filepath.Join(td, "l.txt")+"\"\n"), 0644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	t.Setenv("PT_HOOKS", "")
	t.Setenv("HOME", td)
	t.Setenv("PWD", td)
	t.Setenv("PT_SKIP_HOOKS", "")
	if err := os.Chdir(td); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	var err error
	loadedHooks, err = loadHooks()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	t.Cleanup(func() { loadedHooks = nil })

	if _, err := runHooks("post-validate", hookPayload{ID: "pt-1"}); err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(td, "g.txt")); err != nil {
		t.Fatalf("expected global hook to run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(td, "l.txt")); err != nil {
		t.Fatalf("expected local hook to run: %v", err)
	}
}
