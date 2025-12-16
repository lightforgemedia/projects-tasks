package pulse

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPulseRuntimeFileExistsAndHasAPI(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	path := filepath.Join(moduleRoot, "runtime", "pulse_runtime.js")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime file: %v", err)
	}
	s := string(raw)

	// Contract: must define window.__pulse with these method names.
	for _, needle := range []string{"window.__pulse", "GetState", "ListInteractive", "Act", "Snapshot"} {
		if !strings.Contains(s, needle) {
			t.Fatalf("runtime missing %q (%s)", needle, path)
		}
	}

	// Contract: runtime must not log by default.
	for _, needle := range []string{"console.log", "console.debug", "console.info", "process.stdout"} {
		if strings.Contains(s, needle) {
			t.Fatalf("runtime contains disallowed logging %q (%s)", needle, path)
		}
	}
}

