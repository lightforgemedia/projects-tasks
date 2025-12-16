//go:build integration

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"pulse/internal/demo"
)

func TestE2E_ThreeFlowsPassOnDemoServer(t *testing.T) {
	pulseRoot := mustPulseRoot(t)

	s, err := demo.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start demo: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})

	flows := []string{
		filepath.Join(pulseRoot, "testdata", "flows", "valid", "product_card_quickadd.toml"),
		filepath.Join(pulseRoot, "testdata", "flows", "valid", "settings_profile_save.toml"),
		filepath.Join(pulseRoot, "testdata", "flows", "valid", "auth_login_basic.toml"),
	}

	for _, p := range flows {
		t.Run(filepath.Base(p), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "go", "run", "./cmd/pulse",
				"--run",
				"--flow", p,
				"--base-url", s.BaseURL,
				"--headless=true",
			)
			cmd.Dir = pulseRoot
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("pulse run failed: %v\n%s", err, string(out))
			}
		})
	}
}

func mustPulseRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// .../projects/pulse/internal/e2e/e2e_test.go -> .../projects/pulse
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

