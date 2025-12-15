package main

import (
	"path/filepath"
	"testing"

	"projects-tasks/pkg/pt"
)

func setupStoreEnv(t *testing.T) (string, *pt.StoreClient) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pt.db.json")
	t.Setenv("PT_BACKEND", "store")
	t.Setenv("PT_DB", path)
	// Ensure tests are not affected by the developer's environment (e.g. hooks that run
	// `go test ./...` inherit env vars like PT_WORKFLOW).
	t.Setenv("PT_WORKFLOW", "")
	t.Setenv("PT_PROJECT_DOD", "")
	return path, pt.NewStoreClient(path, "pt")
}
