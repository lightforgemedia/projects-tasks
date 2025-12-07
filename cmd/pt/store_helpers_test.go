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
	return path, pt.NewStoreClient(path, "pt")
}
