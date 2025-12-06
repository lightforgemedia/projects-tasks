package pt

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestParseJSONManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "User Login Flow",
		"owner": "team-auth",
		"tasks": [
			{
				"template": "backend_endpoint",
				"title": "Implement POST /login",
				"role": "backend-dev",
				"params": {"path": "/login"},
				"dod": {
					"tests": ["go test ./src/auth/..."],
					"manual": "Verify JWT is returned in header"
				}
			},
			{
				"template": "frontend_component",
				"title": "Login Form UI",
				"role": "frontend-dev",
				"deps": ["Implement POST /login"],
				"dod": {
					"validation_cmd": "bun test src/components/LoginForm.test.ts",
					"manual": "Visual check"
				}
			}
		]
	}`
	path := writeTemp(t, dir, "manifest.json", manifest)

	got, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}

	if got.Title != "User Login Flow" || got.Owner != "team-auth" {
		t.Fatalf("unexpected manifest metadata: %+v", got)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got.Tasks))
	}
	if got.Tasks[0].Template != "backend_endpoint" || got.Tasks[0].DoD.Manual == "" {
		t.Fatalf("unexpected first task: %+v", got.Tasks[0])
	}
	if got.Tasks[1].Deps[0] != "Implement POST /login" {
		t.Fatalf("dependency not parsed: %+v", got.Tasks[1].Deps)
	}
}

func TestParseTOMLManifest(t *testing.T) {
	dir := t.TempDir()
	content := `
title = "User Login Flow"
owner = "team-auth"

[[tasks]]
template = "backend_endpoint"
title = "Implement POST /login"
role = "backend-dev"
[tasks.params]
path = "/login"
[tasks.dod]
tests = ["go test ./src/auth/..."]
manual = "Verify JWT is returned in header"

[[tasks]]
template = "frontend_component"
title = "Login Form UI"
role = "frontend-dev"
deps = ["Implement POST /login"] # Dependency by title
[tasks.dod]
validation_cmd = "bun test src/components/LoginForm.test.ts"
manual = "Visual check"
`
	path := writeTemp(t, dir, "manifest.toml", content)

	got, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got.Tasks))
	}
	if got.Tasks[0].Params["path"] != "/login" {
		t.Fatalf("param not parsed: %+v", got.Tasks[0].Params)
	}
	if got.Tasks[1].Deps[0] != "Implement POST /login" {
		t.Fatalf("dependency not parsed: %+v", got.Tasks[1].Deps)
	}
}

func TestValidationErrors(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "Broken",
		"tasks": [
			{"template": "backend_endpoint", "title": "A", "role": "dev", "dod": {}}
		]
	}`
	path := writeTemp(t, dir, "broken.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected validation error for missing DoD")
	}
}

func TestUnknownDependency(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "Deps",
		"tasks": [
			{"template": "backend_endpoint", "title": "A", "role": "dev",
			 "dod": {"manual": "check"}
			},
			{"template": "frontend_component", "title": "B", "role": "dev",
			 "deps": ["Missing"],
			 "dod": {"manual": "check"}
			}
		]
	}`
	path := writeTemp(t, dir, "deps.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected error for missing dependency")
	}
}

func TestInvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "BadTemplate",
		"tasks": [
			{"template": "unknown", "title": "A", "role": "dev",
			 "dod": {"manual": "check"}
			}
		]
	}`
	path := writeTemp(t, dir, "tmpl.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected template validation error")
	}
}

func TestManualOnlyDoDAllowed(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "ManualOnly",
		"tasks": [
			{"template": "bug_fix", "title": "Fix", "role": "dev",
			 "dod": {"manual": "verify repro is gone"}
			}
		]
	}`
	path := writeTemp(t, dir, "manual.json", manifest)
	if _, err := ParseManifest(path); err != nil {
		t.Fatalf("expected manual-only dod to pass, got %v", err)
	}
}

func TestInvalidOnFailure(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "OnFailure",
		"tasks": [
			{"template": "refactor", "title": "Refactor", "role": "dev",
			 "dod": {"manual": "check", "on_failure": "unknown"}
			}
		]
	}`
	path := writeTemp(t, dir, "failure.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected on_failure validation error")
	}
}
