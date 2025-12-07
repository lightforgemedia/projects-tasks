package pt

import (
	"context"
	"testing"
)

func TestSearchFindsByTitleAndLabel(t *testing.T) {
	client := NewStoreClient(t.TempDir()+"/pt.db.json", "pt")
	ctx := context.Background()
	_, _ = client.Sync(ctx, Manifest{
		Tasks: []Task{
			{Title: "Fix login bug", Template: "bug_fix", Role: "dev", DoD: DefinitionOfDone{Manual: "check"}},
		},
	})
	results, err := client.Search(ctx, SearchOptions{Query: "login", Limit: 5})
	if err != nil {
		t.Fatalf("search err: %v", err)
	}
	if len(results) != 1 || results[0].Issue.Title != "Fix login bug" {
		t.Fatalf("unexpected search results: %+v", results)
	}
}
