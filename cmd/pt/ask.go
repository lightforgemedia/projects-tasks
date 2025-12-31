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

func cmdAsk(args []string) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	dryRun := fs.Bool("dry-run", false, "print output path without writing")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Println("Usage: pt ask <task-id> [--dry-run]")
	}
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing task id")
	}
	taskID := fs.Arg(0)

	storePath := strings.TrimSpace(*dbPath)
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
	}
	reviewsDir := reviewsDirFromStorePath(storePath)
	askPath := filepath.Join(reviewsDir, fmt.Sprintf("ASK-%s.md", taskID))

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iss, meta, err := client.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	now := time.Now()
	content := renderAskTemplate(iss, meta, now, askPath)
	if *dryRun {
		if *jsonOut {
			return printJSON(map[string]any{
				"status":  "ok",
				"written": false,
				"path":    askPath,
				"content": content,
			})
		}
		fmt.Printf("ASK path: %s\n", askPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(askPath), 0o755); err != nil {
		return fmt.Errorf("create reviews dir: %w", err)
	}
	archivedPath, archived, err := archiveExistingCanonical(askPath, now)
	if err != nil {
		return fmt.Errorf("archive existing ask file: %w", err)
	}
	if err := os.WriteFile(askPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write ask file: %w", err)
	}

	comment := fmt.Sprintf("ask-file: %s", askPath)
	if archived {
		comment += fmt.Sprintf("\nask-archived: %s", archivedPath)
	}
	if err := client.AddComment(ctx, taskID, comment); err != nil {
		return fmt.Errorf("link ask via comment: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]any{
			"status":        "ok",
			"written":       true,
			"path":          askPath,
			"archived":      archived,
			"archived_path": archivedPath,
		})
	}
	fmt.Printf("ASK written: %s\n", askPath)
	if archived {
		fmt.Printf("ASK archived: %s\n", archivedPath)
	}
	return nil
}

func renderAskTemplate(iss pt.Issue, meta pt.TaskMeta, now time.Time, askPath string) string {
	metaJSON, _ := json.Marshal(meta)

	var tests string
	if len(meta.DoD.Tests) > 0 {
		tests = strings.Join(meta.DoD.Tests, "\n")
	} else {
		tests = "{TEST_COMMANDS}"
	}

	manual := meta.DoD.Manual
	if strings.TrimSpace(manual) == "" {
		manual = "{MANUAL_VALIDATION}"
	}

	criteria := "{ACCEPTANCE_CRITERIA}"
	if len(meta.DoD.Criteria) > 0 {
		criteria = strings.Join(meta.DoD.Criteria, "\n- ")
		criteria = "- " + criteria
	}

	inputs := "{FILES_AND_DIRS}"
	if len(meta.Inputs) > 0 {
		var lines []string
		for _, p := range meta.Inputs {
			lines = append(lines, fmt.Sprintf("- %s", p))
		}
		inputs = strings.Join(lines, "\n")
	}

	context := meta.Context
	if strings.TrimSpace(context) == "" {
		context = "{WHY_THIS_TASK_EXISTS}"
	}

	scope := meta.Scope
	if strings.TrimSpace(scope) == "" {
		scope = "{IN_SCOPE_AND_OUT_OF_SCOPE}"
	}

	ref := meta.Reference
	if strings.TrimSpace(ref) == "" {
		ref = "{LINKS_DOCS_PRIOR_WORK}"
	}

	groundingFiles := "{TODO}"
	groundingSymbols := "{TODO}"
	groundingCommands := "{TODO}"
	if meta.Grounding != nil {
		if len(meta.Grounding.Files) > 0 {
			groundingFiles = strings.Join(meta.Grounding.Files, "\n- ")
			groundingFiles = "- " + groundingFiles
		}
		if len(meta.Grounding.Symbols) > 0 {
			groundingSymbols = strings.Join(meta.Grounding.Symbols, "\n- ")
			groundingSymbols = "- " + groundingSymbols
		}
		if len(meta.Grounding.Commands) > 0 {
			groundingCommands = strings.Join(meta.Grounding.Commands, "\n- ")
			groundingCommands = "- " + groundingCommands
		}
	}

	return fmt.Sprintf(`# ASK Packet: %s

- Task ID: %s
- Role: %s
- Template: %s
- Artifact: %s
- Created: %s
- Canonical file: %s

## Goal (1–3 bullets)
- TODO

## Task context (WHY)
%s

## Scope (BOUNDS)
%s

## Inputs (WHERE to read/modify)
%s

## Grounding pack (STRICT for implementation)

Add task-level grounding here to prevent drift:

### Files
%s

### Symbols
%s

### sg-agent commands
%s

## DoD commands (proof plan)

### Tests (run EXACT commands)
`+"`"+`sh
%s
`+"`"+`

### Manual verification
%s

### Acceptance criteria (must be checkable)
%s

## Rubric / constraints (reviewer + agent)
- Match spec pack and artifact
- Evidence > assertions (logs, outputs, screenshots)
- No silent skips; document failures + remediation

<!-- pt-meta: %s -->
`, iss.Title, iss.ID, meta.Role, meta.Template, meta.Artifact, now.Format(time.RFC3339), askPath, context, scope, inputs, groundingFiles, groundingSymbols, groundingCommands, tests, manual, criteria, string(metaJSON))
}
