package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"projects-tasks/pkg/pt"
)

func TestAskWritesCanonicalFileAndArchives(t *testing.T) {
	_, store := setupStoreEnv(t)

	ctx := t.Context()
	id, err := store.AddTask(ctx, pt.Task{
		Title:    "Test Task",
		Template: "backend_endpoint",
		Role:     "dev",
		Artifact: "spec:test",
		DoD: pt.DefinitionOfDone{
			Manual:   "check it",
			Tests:    []string{"go test ./..."},
			Criteria: []string{"observed pass"},
		},
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	if err := cmdAsk([]string{id}); err != nil {
		t.Fatalf("cmdAsk: %v", err)
	}

	reviewsDir := filepath.Join(filepath.Dir(os.Getenv("PT_DB")), ".pt", "reviews")
	askPath := filepath.Join(reviewsDir, "ASK-"+id+".md")
	b, err := os.ReadFile(askPath)
	if err != nil {
		t.Fatalf("read ask file: %v", err)
	}
	if !strings.Contains(string(b), "# ASK Packet: Test Task") {
		t.Fatalf("expected ask file to contain title, got:\n%s", string(b))
	}

	// Second run archives previous version and overwrites canonical.
	if err := cmdAsk([]string{id}); err != nil {
		t.Fatalf("cmdAsk (2): %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(reviewsDir, "archive"))
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ASK-"+id+".") && strings.HasSuffix(e.Name(), ".md") {
			found = true
			break
		}
	}
	if !found {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected archived ask file, got: %v", names)
	}

	store2 := pt.NewStoreClient(os.Getenv("PT_DB"), "pt")
	comms, err := store2.Comments(ctx, id)
	if err != nil {
		t.Fatalf("comments: %v", err)
	}
	hasAskFileComment := false
	for _, c := range comms {
		if strings.Contains(c, "ask-file:") {
			hasAskFileComment = true
			break
		}
	}
	if !hasAskFileComment {
		t.Fatalf("expected ask-file comment, got: %v", comms)
	}
}
