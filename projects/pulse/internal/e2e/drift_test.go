//go:build integration

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pulse/internal/demo"
)

func TestDrift_CosmeticTolerated_EmitsPatch(t *testing.T) {
	pulseRoot := mustPulseRootForDrift(t)
	s := mustStartDemo(t)

	flowPath := writeTempFlow(t, pulseRoot, "drift_cosmetic_test", "/products?query=socks&drift=cosmetic")
	out, err := runPulseCLI(t, pulseRoot, s.BaseURL, flowPath)
	if err != nil {
		t.Fatalf("expected pass, got err=%v\n%s", err, out)
	}
	if !strings.Contains(out, "drift: COSMETIC") {
		t.Fatalf("expected cosmetic drift in output:\n%s", out)
	}
	patch := mustExtractAfter(t, out, "suggested_patch:")
	if _, err := os.Stat(patch); err != nil {
		t.Fatalf("expected patch file to exist: %s (%v)", patch, err)
	}
}

func TestDrift_StructuralFails_EmitsPatch(t *testing.T) {
	pulseRoot := mustPulseRootForDrift(t)
	s := mustStartDemo(t)

	flowPath := writeTempFlow(t, pulseRoot, "drift_structural_test", "/products?query=socks&drift=structural")
	out, err := runPulseCLI(t, pulseRoot, s.BaseURL, flowPath)
	if err == nil {
		t.Fatalf("expected failure, got success:\n%s", out)
	}
	if !strings.Contains(out, "drift: STRUCTURAL") {
		t.Fatalf("expected structural drift in output:\n%s", out)
	}
	patch := mustExtractAfter(t, out, "suggested_patch:")
	if _, err := os.Stat(patch); err != nil {
		t.Fatalf("expected patch file to exist: %s (%v)", patch, err)
	}
}

func mustStartDemo(t *testing.T) *demo.Server {
	t.Helper()
	s, err := demo.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start demo: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func writeTempFlow(t *testing.T, pulseRoot string, id string, route string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, id+".toml")
	raw := strings.Join([]string{
		`id = "` + id + `"`,
		`version = 1`,
		`tags = ["p0"]`,
		``,
		`[preconditions]`,
		`route = "` + route + `"`,
		``,
		`[[steps]]`,
		`action = "click"`,
		`target = "role=button[name='Quick Add']"`,
		``,
		`[[assertions]]`,
		`type = "visible"`,
		`target = "data-testid=mini-cart"`,
		``,
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}
	_ = pulseRoot
	return path
}

func runPulseCLI(t *testing.T, pulseRoot string, baseURL string, flowPath string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/pulse",
		"--run",
		"--flow", flowPath,
		"--base-url", baseURL,
		"--headless=true",
	)
	cmd.Dir = pulseRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustExtractAfter(t *testing.T, out string, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("missing %q line in output:\n%s", prefix, out)
	return ""
}

func mustPulseRootForDrift(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

