package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadContract(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "contracts", "builder.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Meta.Role != "builder" {
		t.Fatalf("role mismatch: %s", c.Meta.Role)
	}
	goal, ok := c.Fields["goal"]
	if !ok || goal.Rules["prompt"].Type != "string" {
		t.Fatalf("goal.prompt rule missing: %+v", goal)
	}
}

func TestValidatePayloadPasses(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "client.go")
	f2 := filepath.Join(tmp, "client_test.go")
	_ = os.WriteFile(f1, []byte("// ok"), 0o644)
	_ = os.WriteFile(f2, []byte("// ok"), 0o644)

	payload := map[string]any{
		"goal": map[string]any{
			"prompt": "Add retry logic to the API client with backoff.",
		},
		"scope": map[string]any{
			"files": []any{f1, f2},
		},
		"success": map[string]any{
			"criteria": []any{"go test ./..."},
		},
		"provenance": map[string]any{
			"inputs":    []any{map[string]any{"field": "goal.prompt"}},
			"issued_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
	bytes, _ := json.Marshal(payload)

	c, err := Load(filepath.Join("..", "..", "contracts", "builder.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := ValidatePayload(bytes, c); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidatePayloadFailsMissing(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "contracts", "builder.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	bad := `{"goal":{"prompt":""}}`
	if err := ValidatePayload([]byte(bad), c); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestFreshnessCheck(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "contracts", "reviewer.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	payload := map[string]any{
		"goal": map[string]any{
			"original_prompt": "Do X",
		},
		"artifacts": map[string]any{
			"diff":        "diff data",
			"test_output": "ok",
		},
		"provenance": map[string]any{
			"builder":   map[string]any{"name": "bot"},
			"issued_at": time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	bytes, _ := json.Marshal(payload)
	if err := ValidatePayload(bytes, c); err == nil {
		t.Fatalf("expected freshness error")
	}
}
