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
				"artifact": "api:/login",
				"next_hint": "Hook up backend integration next",
				"params": {"path": "/login"},
				"dod": {
					"tests": ["go test ./src/auth/..."],
					"manual": "Verify JWT is returned in header",
					"criteria": ["JWT present in header"]
				}
			},
			{
				"template": "frontend_component",
				"title": "Login Form UI",
				"role": "frontend-dev",
				"artifact": "ui:LoginForm",
				"deps": ["Implement POST /login"],
				"dod": {
					"tests": ["bun test src/components/LoginForm.test.ts"],
					"validation_cmd": "bun test src/components/LoginForm.test.ts",
					"manual": "Visual check",
					"criteria": ["UI matches design", "tests green"]
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
	if got.Tasks[0].NextHint != "Hook up backend integration next" {
		t.Fatalf("expected next_hint parsed, got %+v", got.Tasks[0].NextHint)
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
artifact = "api:/login"
[tasks.params]
path = "/login"
[tasks.dod]
tests = ["go test ./src/auth/..."]
manual = "Verify JWT is returned in header"
criteria = ["JWT present in header"]

[[tasks]]
template = "frontend_component"
title = "Login Form UI"
role = "frontend-dev"
artifact = "ui:LoginForm"
deps = ["Implement POST /login"] # Dependency by title
[tasks.dod]
tests = ["bun test src/components/LoginForm.test.ts"]
validation_cmd = "bun test src/components/LoginForm.test.ts"
criteria = ["UI matches design", "tests green"]
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
			 "artifact": "spec:a",
			 "dod": {"manual": "check", "tests": ["go test ./..."], "criteria": ["validated"]}
			},
			{"template": "frontend_component", "title": "B", "role": "dev",
			 "deps": ["Missing"],
			 "artifact": "spec:b",
			 "dod": {"manual": "check", "tests": ["bun test ./..."], "criteria": ["validated"]}
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
			 "artifact": "spec:a",
			 "dod": {"manual": "check", "tests": ["go test ./..."], "criteria": ["validated"]}
			}
		]
	}`
	path := writeTemp(t, dir, "tmpl.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected template validation error")
	}
}

func TestManualOnlyDoDFails(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "ManualOnly",
		"tasks": [
			{"template": "bug_fix", "title": "Fix", "role": "dev",
			 "artifact": "spec:fix",
			 "dod": {"manual": "verify repro is gone"}
			}
		]
	}`
	path := writeTemp(t, dir, "manual.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected manual-only dod to fail")
	}
}

func TestInvalidOnFailure(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
		"title": "OnFailure",
		"tasks": [
			{"template": "refactor", "title": "Refactor", "role": "dev",
			 "dod": {"manual": "check", "tests": ["go test ./..."], "on_failure": "unknown"}
			}
		]
	}`
	path := writeTemp(t, dir, "failure.json", manifest)
	if _, err := ParseManifest(path); err == nil {
		t.Fatalf("expected on_failure validation error")
	}
}

func TestManifestNewFields(t *testing.T) {
	t.Run("JSON with handoff fields", func(t *testing.T) {
		dir := t.TempDir()
		manifest := `{
			"title": "Handoff Test",
			"tasks": [{
				"template": "backend_endpoint",
				"title": "Task With Context",
				"role": "backend-dev",
				"artifact": "code:pkg/user.go",
				"context": "Users can submit invalid emails causing downstream failures",
				"inputs": ["pkg/user/registration.go", "pkg/user/validation_test.go"],
				"scope": "IN: email format validation. OUT: no UI changes",
				"reference": "https://example.com/issue/123",
				"dod": {
					"tests": ["go test ./pkg/user/..."],
					"manual": "Register with invalid email, verify error",
					"criteria": ["Invalid emails rejected", "Valid emails pass"]
				}
			}]
		}`
		path := writeTemp(t, dir, "handoff.json", manifest)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() error = %v", err)
		}
		task := got.Tasks[0]
		if task.Context != "Users can submit invalid emails causing downstream failures" {
			t.Fatalf("context not parsed: %q", task.Context)
		}
		if len(task.Inputs) != 2 || task.Inputs[0] != "pkg/user/registration.go" {
			t.Fatalf("inputs not parsed: %v", task.Inputs)
		}
		if task.Scope != "IN: email format validation. OUT: no UI changes" {
			t.Fatalf("scope not parsed: %q", task.Scope)
		}
		if task.Reference != "https://example.com/issue/123" {
			t.Fatalf("reference not parsed: %q", task.Reference)
		}
	})

	t.Run("TOML with handoff fields", func(t *testing.T) {
		dir := t.TempDir()
		content := `
title = "TOML Handoff Test"

[[tasks]]
template = "backend_endpoint"
title = "Task With Context"
role = "backend-dev"
artifact = "code:pkg/user.go"
context = "Bug found in demo: release not clearing assignee"
inputs = ["pkg/pt/store.go", "cmd/pt/main.go"]
scope = "IN: fix UpdateIssue. OUT: no other status changes"
reference = "https://github.com/example/issue/456"
[tasks.dod]
tests = ["go test ./pkg/pt/... -run TestRelease"]
manual = "Release a task, verify assignee empty"
criteria = ["release sets assignee to empty", "history shows assignee cleared"]
`
		path := writeTemp(t, dir, "handoff.toml", content)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() error = %v", err)
		}
		task := got.Tasks[0]
		if task.Context != "Bug found in demo: release not clearing assignee" {
			t.Fatalf("TOML context not parsed: %q", task.Context)
		}
		if len(task.Inputs) != 2 || task.Inputs[1] != "cmd/pt/main.go" {
			t.Fatalf("TOML inputs not parsed: %v", task.Inputs)
		}
		if task.Scope != "IN: fix UpdateIssue. OUT: no other status changes" {
			t.Fatalf("TOML scope not parsed: %q", task.Scope)
		}
		if task.Reference != "https://github.com/example/issue/456" {
			t.Fatalf("TOML reference not parsed: %q", task.Reference)
		}
	})

	t.Run("Missing handoff fields do not break sync", func(t *testing.T) {
		dir := t.TempDir()
		manifest := `{
			"title": "No Handoff Fields",
			"tasks": [{
				"template": "backend_endpoint",
				"title": "Simple Task",
				"role": "backend-dev",
				"artifact": "code:simple.go",
				"dod": {
					"tests": ["go test ./..."],
					"manual": "verify",
					"criteria": ["works"]
				}
			}]
		}`
		path := writeTemp(t, dir, "simple.json", manifest)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() should not fail with missing handoff fields: %v", err)
		}
		task := got.Tasks[0]
		if task.Context != "" || len(task.Inputs) != 0 || task.Scope != "" || task.Reference != "" {
			t.Fatalf("expected empty handoff fields, got context=%q inputs=%v scope=%q ref=%q",
				task.Context, task.Inputs, task.Scope, task.Reference)
		}
	})
}

func TestManifestProject(t *testing.T) {
	dir := t.TempDir()

	t.Run("TOML with project section", func(t *testing.T) {
		manifest := `
title = "PT CLI Tool"
owner = "platform"

[project]
summary = "CLI tool for task management in agent-driven development"
structure = ["cmd/pt", "pkg/pt", "docs"]

[[tasks]]
template = "backend_endpoint"
title = "Add feature"
role = "dev"
artifact = "code:feature.go"
[tasks.dod]
tests = ["go test ./..."]
manual = "verify"
criteria = ["works"]
`
		path := writeTemp(t, dir, "project.toml", manifest)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() error: %v", err)
		}

		if got.Project.Summary != "CLI tool for task management in agent-driven development" {
			t.Errorf("expected project summary, got %q", got.Project.Summary)
		}
		if len(got.Project.Structure) != 3 {
			t.Errorf("expected 3 structure entries, got %d", len(got.Project.Structure))
		}
		if got.Project.Structure[0] != "cmd/pt" {
			t.Errorf("expected first structure entry 'cmd/pt', got %q", got.Project.Structure[0])
		}
	})

	t.Run("JSON with project section", func(t *testing.T) {
		manifest := `{
			"title": "PT CLI",
			"owner": "platform",
			"project": {
				"summary": "Task management CLI",
				"structure": ["cmd", "pkg"]
			},
			"tasks": [{
				"template": "backend_endpoint",
				"title": "Add feature",
				"role": "dev",
				"artifact": "code:feature.go",
				"dod": {"tests": ["go test"], "manual": "verify", "criteria": ["works"]}
			}]
		}`
		path := writeTemp(t, dir, "project.json", manifest)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() error: %v", err)
		}

		if got.Project.Summary != "Task management CLI" {
			t.Errorf("expected project summary, got %q", got.Project.Summary)
		}
		if len(got.Project.Structure) != 2 {
			t.Errorf("expected 2 structure entries, got %d", len(got.Project.Structure))
		}
	})

	t.Run("Missing project section is OK", func(t *testing.T) {
		manifest := `
title = "No Project"

[[tasks]]
template = "backend_endpoint"
title = "Task"
role = "dev"
artifact = "code:task.go"
[tasks.dod]
tests = ["echo ok"]
manual = "verify"
criteria = ["works"]
`
		path := writeTemp(t, dir, "noproj.toml", manifest)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() should not fail without project section: %v", err)
		}
		if got.Project.Summary != "" || len(got.Project.Structure) != 0 {
			t.Fatalf("expected empty project, got %+v", got.Project)
		}
	})
}

func TestSpikeTaskValidation(t *testing.T) {
	dir := t.TempDir()

	t.Run("Valid spike task", func(t *testing.T) {
		manifest := `
title = "Investigation"

[[tasks]]
template = "spike"
title = "Investigate caching options"
role = "backend-dev"
artifact = "doc:outputs/spike-caching/findings.md"
max_hours = 4
[tasks.dod]
tests = ["test -f outputs/spike-caching/findings.md"]
manual = "Document findings with recommendation"
criteria = ["Options documented", "Recommendation provided"]
`
		path := writeTemp(t, dir, "spike.toml", manifest)
		got, err := ParseManifest(path)
		if err != nil {
			t.Fatalf("ParseManifest() error: %v", err)
		}
		if len(got.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(got.Tasks))
		}
		if got.Tasks[0].MaxHours != 4 {
			t.Errorf("expected max_hours=4, got %d", got.Tasks[0].MaxHours)
		}
	})

	t.Run("Spike without max_hours fails", func(t *testing.T) {
		manifest := `
title = "Investigation"

[[tasks]]
template = "spike"
title = "Missing time-box"
role = "backend-dev"
artifact = "doc:findings.md"
[tasks.dod]
tests = ["echo ok"]
manual = "verify"
criteria = ["done"]
`
		path := writeTemp(t, dir, "spike_no_hours.toml", manifest)
		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("expected error for spike without max_hours")
		}
		if !contains(err.Error(), "max_hours") {
			t.Errorf("expected error about max_hours, got: %v", err)
		}
	})

	t.Run("Spike with non-doc artifact fails", func(t *testing.T) {
		manifest := `
title = "Investigation"

[[tasks]]
template = "spike"
title = "Wrong artifact"
role = "backend-dev"
artifact = "code:impl.go"
max_hours = 2
[tasks.dod]
tests = ["echo ok"]
manual = "verify"
criteria = ["done"]
`
		path := writeTemp(t, dir, "spike_bad_artifact.toml", manifest)
		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("expected error for spike with non-doc artifact")
		}
		if !contains(err.Error(), "doc:") {
			t.Errorf("expected error about doc artifact, got: %v", err)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
