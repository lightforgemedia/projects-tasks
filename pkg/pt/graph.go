package pt

import (
	"fmt"
	"sort"
	"strings"
)

// RenderManifestTree returns an ASCII tree representation of the manifest tasks.
func RenderManifestTree(m Manifest) string {
	// 1. Index tasks by title and build adjacency list
	taskMap := make(map[string]Task)
	children := make(map[string][]string) // parent -> [children]
	roots := make(map[string]struct{})    // set of potential roots

	for _, t := range m.Tasks {
		taskMap[t.Title] = t
		roots[t.Title] = struct{}{}
	}

	for _, t := range m.Tasks {
		for _, dep := range t.Deps {
			if _, exists := taskMap[dep]; exists {
				children[dep] = append(children[dep], t.Title)
				delete(roots, t.Title) // It has a parent, so not a root
			}
		}
	}

	// 2. Sort roots and children for deterministic output
	sortedRoots := make([]string, 0, len(roots))
	for r := range roots {
		sortedRoots = append(sortedRoots, r)
	}
	sort.Strings(sortedRoots)

	for p := range children {
		sort.Strings(children[p])
	}

	// 3. Render
	var b strings.Builder
	fmt.Fprintf(&b, "Phase: %s\n", m.Title)

	visited := make(map[string]bool)
	if len(sortedRoots) == 0 {
		// Cycle or all nodes have parents; render all to still show structure.
		sortedRoots = make([]string, 0, len(taskMap))
		for title := range taskMap {
			sortedRoots = append(sortedRoots, title)
		}
		sort.Strings(sortedRoots)
	}
	for i, root := range sortedRoots {
		isLast := i == len(sortedRoots)-1
		renderNode(&b, root, "", isLast, taskMap, children, visited)
	}

	return b.String()
}

func renderNode(b *strings.Builder, title string, prefix string, isLast bool, taskMap map[string]Task, children map[string][]string, path map[string]bool) {
	task := taskMap[title]

	marker := "├──"
	if isLast {
		marker = "└──"
	}

	if path[title] {
		fmt.Fprintf(b, "%s%s [%s] %s (cycle)\n", prefix, marker, task.Role, title)
		return
	}
	path[title] = true

	// Status/Role coloring could go here in a real TUI
	fmt.Fprintf(b, "%s%s [%s] %s\n", prefix, marker, task.Role, title)

	newPrefix := prefix
	if isLast {
		newPrefix += "    "
	} else {
		newPrefix += "│   "
	}

	kids := children[title]
	for i, kid := range kids {
		isLastKid := i == len(kids)-1
		renderNode(b, kid, newPrefix, isLastKid, taskMap, children, path)
	}

	delete(path, title)
}
