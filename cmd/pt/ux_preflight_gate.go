package main

import (
	"strings"

	"projects-tasks/pkg/pt"
)

func uxPreflightRequired(meta pt.TaskMeta) bool {
	if meta.UX == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(meta.UX.Type)) {
	case "web", "tui", "cli":
		return true
	default:
		return false
	}
}

func uxPreflightMissing(meta pt.TaskMeta) bool {
	if meta.UXState == nil {
		return true
	}
	return !meta.UXState.PreflightDone
}

func uxTypeForPreflight(meta pt.TaskMeta) string {
	if meta.UX == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(meta.UX.Type))
}

func uxPreflightHard(meta pt.TaskMeta, uxType string) bool {
	if strings.TrimSpace(meta.Template) != "discovery" {
		return false
	}
	if !strings.HasPrefix(strings.TrimSpace(meta.Artifact), "doc:") {
		return false
	}
	switch uxType {
	case "web", "tui", "cli":
		return true
	default:
		return false
	}
}
