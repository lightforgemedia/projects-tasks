package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHooks_WritesLogAndPrintsSummaryUnlessVerbose(t *testing.T) {
	td := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(td); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	hooksPath := filepath.Join(td, "hooks.toml")
	cfg := `
[[hook]]
event = "pre-approve"
cmd = "printf 'line1\nline2\n'"
`
	if err := os.WriteFile(hooksPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}

	t.Setenv("PT_HOOKS", hooksPath)
	t.Setenv("PT_SKIP_HOOKS", "")
	t.Setenv("PT_HOOK_VERBOSE", "")
	t.Setenv("PT_DB", filepath.Join(td, ".pt", "db.json"))
	t.Setenv("PT_NO_DISCOVER", "1")

	var err error
	loadedHooks, err = loadHooks()
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	t.Cleanup(func() { loadedHooks = nil })

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	_, runErr := runHooks("pre-approve", hookPayload{ID: "pt-1"})
	_ = w.Close()
	os.Stderr = old
	if runErr != nil {
		t.Fatalf("runHooks: %v", runErr)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	got := buf.String()

	if strings.Contains(got, "line2") {
		t.Fatalf("expected summary output only; got stderr: %q", got)
	}
	if !strings.Contains(got, "[hook ok] pre-approve: line1") {
		t.Fatalf("expected summary line; got stderr: %q", got)
	}
	if !strings.Contains(got, ".pt/reviews/artifacts/pt-1/pre-approve.log") {
		t.Fatalf("expected log path hint; got stderr: %q", got)
	}

	logPath := filepath.Join(td, ".pt", "reviews", "artifacts", "pt-1", "pre-approve.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(data) != "line1\nline2\n" {
		t.Fatalf("unexpected log contents: %q", string(data))
	}
}
