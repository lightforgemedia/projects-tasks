package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

func cmdUXPreflight(args []string) error {
	fs := flag.NewFlagSet("ux preflight", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	uxType := fs.String("type", "", "enable UX by setting type: cli|tui|web|api|doc|error")
	dryRun := fs.Bool("dry-run", false, "print output path without writing")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Println("Usage: pt ux preflight <id>")
		fmt.Println("       Optional: --type web|tui|cli (to enable UX on tasks created without [tasks.ux])")
	}
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)

	storePath := strings.TrimSpace(*dbPath)
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
	}
	reviewsDir := reviewsDirFromStorePath(storePath)
	preflightPath := filepath.Join(reviewsDir, fmt.Sprintf("UX-PREFLIGHT-%s.md", id))

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	// Ensure UX is enabled (manifest-driven or explicitly enabled via --type).
	meta, changed, err := ensureUXEnabled(id, meta, *uxType)
	if err != nil {
		return err
	}
	if changed {
		if err := client.UpdateMeta(ctx, id, meta); err != nil {
			return fmt.Errorf("enable UX: %w", err)
		}
	}

	if meta.UXState == nil {
		meta.UXState = &pt.UXState{Status: "pending"}
	}

	now := time.Now()
	content := renderUXPreflightTemplate(iss, meta, now, preflightPath)
	if *dryRun {
		if *jsonOut {
			return printJSON(map[string]any{
				"status":  "ok",
				"written": false,
				"path":    preflightPath,
				"content": content,
			})
		}
		fmt.Printf("UX preflight path: %s\n", preflightPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(preflightPath), 0o755); err != nil {
		return fmt.Errorf("create reviews dir: %w", err)
	}
	archivedPath, archived, err := archiveExistingCanonical(preflightPath, now)
	if err != nil {
		return fmt.Errorf("archive existing preflight file: %w", err)
	}
	if err := os.WriteFile(preflightPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write preflight file: %w", err)
	}

	meta.UXState.PreflightDone = true
	meta.UXState.PreflightFile = preflightPath
	meta.UXState.PreflightAt = now.Format(time.RFC3339)
	if err := client.UpdateMeta(ctx, id, meta); err != nil {
		return fmt.Errorf("update meta: %w", err)
	}

	comment := fmt.Sprintf("ux-preflight-file: %s", preflightPath)
	if archived {
		comment += fmt.Sprintf("\nux-preflight-archived: %s", archivedPath)
	}
	if err := client.AddComment(ctx, id, comment); err != nil {
		return fmt.Errorf("link preflight via comment: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]any{
			"status":        "ok",
			"written":       true,
			"path":          preflightPath,
			"archived":      archived,
			"archived_path": archivedPath,
		})
	}
	fmt.Printf("UX preflight written: %s\n", preflightPath)
	if archived {
		fmt.Printf("UX preflight archived: %s\n", archivedPath)
	}
	return nil
}

func renderUXPreflightTemplate(iss pt.Issue, meta pt.TaskMeta, now time.Time, outPath string) string {
	metaJSON, _ := json.Marshal(meta)
	uxType := ""
	if meta.UX != nil {
		uxType = meta.UX.Type
	}
	if strings.TrimSpace(uxType) == "" {
		uxType = "{cli|tui|web|api|doc|error}"
	}

	var useCases string
	if meta.UXState != nil && len(meta.UXState.UseCases) > 0 {
		var lines []string
		for _, uc := range meta.UXState.UseCases {
			label := strings.TrimSpace(uc.ID)
			if label == "" {
				label = "UC?"
			}
			if strings.TrimSpace(uc.Actor) != "" {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", label, uc.Actor, uc.Goal))
			} else {
				lines = append(lines, fmt.Sprintf("- [%s] %s", label, uc.Goal))
			}
		}
		useCases = strings.Join(lines, "\n")
	} else {
		useCases = "- TODO (ranked list)\n- TODO"
	}

	tests := "{TEST_COMMANDS}"
	if len(meta.DoD.Tests) > 0 {
		tests = strings.Join(meta.DoD.Tests, "\n")
	}
	manual := meta.DoD.Manual
	if strings.TrimSpace(manual) == "" {
		manual = "{MANUAL_VALIDATION}"
	}

	return fmt.Sprintf(`# UX Preflight: %s

- Task ID: %s
- UX Type: %s
- Created: %s
- Canonical file: %s

## Design interview output (required)

### Ranked use cases (top N)
%s

### Pit-of-success paths (make the happy path obvious)
- Primary flow: TODO
- Secondary flows: TODO

### Footguns / failure modes (what users will do wrong)
- TODO

### Defaults, constraints, and invariants
- TODO

## Validation plan (evidence > assertions)

### Tests
`+"`"+`sh
%s
`+"`"+`

### Manual checks
%s

### Artifacts to capture
- outputs/<task-id>/... (logs/screenshots/reports)

<!-- pt-meta: %s -->
`, iss.Title, iss.ID, uxType, now.Format(time.RFC3339), outPath, useCases, tests, manual, string(metaJSON))
}
