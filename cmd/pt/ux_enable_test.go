package main

import (
	"strings"
	"testing"

	pt "projects-tasks/pkg/pt"
)

func TestEnsureUXEnabled_ExplicitTypeEnables(t *testing.T) {
	meta := pt.TaskMeta{
		Template: "refactor",
		Artifact: "code:cmd/pt/ux.go",
	}

	updated, changed, err := ensureUXEnabled("pt-1", meta, "web")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}
	if updated.UX == nil || updated.UX.Type != "web" {
		t.Fatalf("expected UX type web, got %#v", updated.UX)
	}
}

func TestEnsureUXEnabled_AutoEnablesDiscoveryDocTasks(t *testing.T) {
	meta := pt.TaskMeta{
		Template: "discovery",
		Artifact: "doc:toolkit-core-docs/FOO.md",
	}

	updated, changed, err := ensureUXEnabled("pt-2", meta, "")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}
	if updated.UX == nil || updated.UX.Type != "doc" {
		t.Fatalf("expected UX type doc, got %#v", updated.UX)
	}
}

func TestEnsureUXEnabled_RejectsWithoutTypeForNonDiscoveryTasks(t *testing.T) {
	meta := pt.TaskMeta{
		Template: "refactor",
		Artifact: "code:cmd/pt/ux.go",
	}

	_, _, err := ensureUXEnabled("pt-3", meta, "")
	if err == nil {
		t.Fatalf("expected err")
	}
	if !strings.Contains(err.Error(), "pt ux-cases --type web pt-3") {
		t.Fatalf("expected remediation command in error, got: %s", err.Error())
	}
}
