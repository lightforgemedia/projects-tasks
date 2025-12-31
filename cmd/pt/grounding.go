package main

import (
	"fmt"
	"strings"

	"projects-tasks/pkg/pt"
)

func groundingPackMissing(meta pt.TaskMeta) bool {
	if meta.Grounding == nil {
		return true
	}
	return len(meta.Grounding.Files) == 0 && len(meta.Grounding.Symbols) == 0 && len(meta.Grounding.Commands) == 0
}

func groundingStrictBlock(meta pt.TaskMeta) bool {
	if !templateRequiresGrounding(meta.Template) {
		return false
	}
	if len(realInputPaths(meta.Inputs)) > 0 {
		return true
	}
	if p, ok := artifactFilePath(meta.Artifact); ok && looksLikeFilePath(p) {
		return true
	}
	return false
}

func templateRequiresGrounding(template string) bool {
	switch strings.TrimSpace(strings.ToLower(template)) {
	case "backend_endpoint", "frontend_component", "migration", "bug_fix", "observability_hook", "refactor":
		return true
	default:
		return false
	}
}

func groundingRemediation(taskID string, meta pt.TaskMeta) string {
	files := suggestedGroundingFiles(meta)
	val := "<paths>"
	if len(files) > 0 {
		val = strings.Join(files, ",")
	}
	return fmt.Sprintf("pt update %s --grounding-files=%s", taskID, val)
}

func suggestedGroundingFiles(meta pt.TaskMeta) []string {
	var out []string
	out = append(out, realInputPaths(meta.Inputs)...)

	if len(out) == 0 {
		if p, ok := artifactFilePath(meta.Artifact); ok {
			out = append(out, p)
		}
	}

	seen := map[string]bool{}
	var uniq []string
	for _, p := range out {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		uniq = append(uniq, p)
	}
	return uniq
}

func artifactFilePath(artifact string) (string, bool) {
	a := strings.TrimSpace(artifact)
	if a == "" {
		return "", false
	}
	parts := strings.SplitN(a, ":", 2)
	if len(parts) != 2 {
		return "", false
	}
	kind := strings.ToLower(strings.TrimSpace(parts[0]))
	path := strings.TrimSpace(parts[1])
	switch kind {
	case "code", "doc", "ui":
		if path != "" {
			return path, true
		}
	}
	return "", false
}

func looksLikeFilePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	return strings.Contains(p, "/") || strings.Contains(p, ".")
}

func realInputPaths(inputs []string) []string {
	var out []string
	for _, p := range inputs {
		p = strings.TrimSpace(p)
		if p == "" || p == "-" {
			continue
		}
		upper := strings.ToUpper(p)
		if strings.HasPrefix(upper, "TODO") || strings.HasPrefix(p, "{") {
			continue
		}
		out = append(out, p)
	}
	return out
}
