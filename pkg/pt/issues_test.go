package pt

import (
	"strings"
	"testing"
)

func TestSortIssuesByPriority(t *testing.T) {
	issues := []Issue{
		{Title: "B", Priority: 1},
		{Title: "A", Priority: 2},
	}
	SortIssues(issues, "priority")
	if issues[0].Title != "A" {
		t.Fatalf("expected highest priority first")
	}
}

func TestSortIssuesByTitle(t *testing.T) {
	issues := []Issue{
		{Title: "B"},
		{Title: "A"},
	}
	SortIssues(issues, "title")
	if issues[0].Title != "A" {
		t.Fatalf("expected alphabetical by title")
	}
}

func TestIssueExtra(t *testing.T) {
	iss := Issue{Assignee: "", Labels: []string{"state:blocked"}}
	extra := IssueExtra(iss)
	if extra == "" || extra[0] != '[' || extra[len(extra)-1] != ']' {
		t.Fatalf("expected bracketed extra, got %q", extra)
	}
	if !strings.Contains(extra, "unassigned") {
		t.Fatalf("expected unassigned marker, got %q", extra)
	}
}
