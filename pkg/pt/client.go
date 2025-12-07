package pt

import (
	"context"
	"os"
)

// Client abstracts the backend (bd CLI or local store).
type Client interface {
	Sync(ctx context.Context, manifest Manifest) (map[string]string, error)
	Ready(ctx context.Context, role string, limit int) ([]Issue, error)
	UpdateIssue(ctx context.Context, id, status, assignee string) error
	AddLabels(ctx context.Context, id string, labels ...string) error
	RemoveLabels(ctx context.Context, id string, labels ...string) error
	AddComment(ctx context.Context, id, body string) error
	GetTask(ctx context.Context, id string) (Issue, TaskMeta, error)
	Dependencies(ctx context.Context, id string) ([]Dependency, error)
}

// NewClientFromEnv chooses backend based on PT_BACKEND env var.
// Supported: "" or "bd" (default BDClient), "store" (local store).
func NewClientFromEnv(runner CommandRunner) Client {
	switch backend := getEnv("PT_BACKEND"); backend {
	case "store":
		return NewStoreClient(getEnv("PT_DB"), getEnv("PT_PREFIX"))
	default:
		return NewBDClient(runner)
	}
}

func getEnv(key string) string {
	if v, ok := lookupEnv(key); ok {
		return v
	}
	return ""
}

// allow tests to stub env
var lookupEnv = os.LookupEnv
