package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

func cmdReview(args []string) error {
	if len(args) == 0 {
		return reviewUsage()
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "write":
		return cmdReviewWrite(subArgs)
	case "check":
		return cmdReviewCheck(subArgs)
	case "-h", "--help", "help":
		return reviewUsage()
	default:
		_ = reviewUsage()
		return fmt.Errorf("unknown review subcommand %q", subcmd)
	}
}

func reviewUsage() error {
	fmt.Print(`pt review - Save phase/project reviews as markdown (stored alongside the project DB)

Subcommands:
  write <task-id>         Create a review markdown file and link it to a task via comment
  check <task-id>         Verify the linked review file exists (used by DoD tests)

Examples:
  pt review write pt-12 --kind=pre --phase=prove --desc="kickoff plan"
  pt review check pt-12 --kind=pre

Notes:
  - Files are written under <project-root>/.pt/reviews/ by default.
  - This is a "rail", not a hard gate: you can proceed without it, but pt next will recommend it at phase boundaries.
`)
	return errors.New("")
}

type reviewKind string

const (
	reviewKindPre  reviewKind = "pre"
	reviewKindPost reviewKind = "post"
	reviewKindDemo reviewKind = "demo"
)

func (k reviewKind) valid() bool {
	return k == reviewKindPre || k == reviewKindPost || k == reviewKindDemo
}

func reviewsDirFromStorePath(storePath string) string {
	root := projectRootFromStorePath(storePath)
	if strings.TrimSpace(root) == "" {
		return filepath.Join(".pt", "reviews")
	}
	return filepath.Join(root, ".pt", "reviews")
}

func reviewFilename(taskID string, kind reviewKind, desc string, now time.Time) string {
	slug := feedbackSlug(desc)
	if slug == "review" {
		slug = string(kind)
	}
	return fmt.Sprintf("%s.%s.%s.%s.md", now.Format("2006-01-02-1504"), taskID, kind, slug)
}

func cmdReviewWrite(args []string) error {
	fs := flag.NewFlagSet("review write", flag.ContinueOnError)
	kind := fs.String("kind", "pre", "review kind: pre|post|demo")
	phase := fs.String("phase", "", "phase id (optional; used for context)")
	desc := fs.String("desc", "", "short description for filename (recommended)")
	outPath := fs.String("out", "", "override output file path")
	dryRun := fs.Bool("dry-run", false, "print output path without writing")
	jsonOut := fs.Bool("json", false, "output JSON")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() {
		fmt.Println("Usage: pt review write <task-id> --kind=pre|post|demo [--phase=ID] [--desc=TEXT] [--out=PATH] [--dry-run]")
	}
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing task id")
	}
	taskID := fs.Arg(0)

	k := reviewKind(strings.ToLower(strings.TrimSpace(*kind)))
	if !k.valid() {
		return fmt.Errorf("invalid --kind %q (expected pre|post|demo)", *kind)
	}

	storePath := strings.TrimSpace(*dbPath)
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
	}
	baseDir := reviewsDirFromStorePath(storePath)

	now := time.Now()
	path := strings.TrimSpace(*outPath)
	if path == "" {
		path = filepath.Join(baseDir, reviewFilename(taskID, k, *desc, now))
	}

	content := renderReviewTemplate(k, strings.TrimSpace(*phase), taskID, now)
	if *dryRun {
		if *jsonOut {
			return printJSON(map[string]any{
				"status":  "ok",
				"written": false,
				"path":    path,
				"kind":    string(k),
				"phase":   strings.TrimSpace(*phase),
				"content": content,
			})
		}
		fmt.Printf("Review path: %s\n", path)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create reviews dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("review file already exists: %s", path)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write review file: %w", err)
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	comment := fmt.Sprintf("review-file: %s\nreview-kind: %s\nreview-phase: %s", path, k, strings.TrimSpace(*phase))
	if err := client.AddComment(ctx, taskID, comment); err != nil {
		return fmt.Errorf("link review via comment: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]any{
			"status":  "ok",
			"written": true,
			"path":    path,
			"kind":    string(k),
			"phase":   strings.TrimSpace(*phase),
		})
	}
	fmt.Printf("Review written: %s\n", path)
	fmt.Printf("ASK packet: %s (run: pt ask %s)\n", filepath.Join(baseDir, fmt.Sprintf("ASK-%s.md", taskID)), taskID)
	return nil
}

func cmdReviewCheck(args []string) error {
	fs := flag.NewFlagSet("review check", flag.ContinueOnError)
	kind := fs.String("kind", "", "review kind to check (pre|post|demo); if empty, any linked review-file passes")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	fs.Usage = func() { fmt.Println("Usage: pt review check <task-id> [--kind=pre|post|demo]") }
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing task id")
	}
	taskID := fs.Arg(0)
	wantKind := strings.ToLower(strings.TrimSpace(*kind))
	if wantKind != "" && !reviewKind(wantKind).valid() {
		return fmt.Errorf("invalid --kind %q (expected pre|post|demo)", *kind)
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	comments, err := client.Comments(ctx, taskID)
	if err != nil {
		return fmt.Errorf("read comments: %w", err)
	}

	var linkedPaths []string
	currentPath := ""
	currentKind := ""
	for _, c := range comments {
		for _, line := range strings.Split(c, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "review-file:") {
				currentPath = strings.TrimSpace(strings.TrimPrefix(line, "review-file:"))
				currentKind = ""
				continue
			}
			if strings.HasPrefix(line, "review-kind:") {
				currentKind = strings.TrimSpace(strings.TrimPrefix(line, "review-kind:"))
				continue
			}
		}
		if strings.TrimSpace(currentPath) != "" {
			if wantKind == "" || strings.EqualFold(strings.TrimSpace(currentKind), wantKind) {
				linkedPaths = append(linkedPaths, currentPath)
			}
			currentPath = ""
			currentKind = ""
		}
	}

	if len(linkedPaths) == 0 {
		if wantKind == "" {
			return fmt.Errorf("no linked review-file comment found for %s", taskID)
		}
		return fmt.Errorf("no linked review-file comment found for %s kind=%s", taskID, wantKind)
	}
	for _, p := range linkedPaths {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("linked review file missing: %s", p)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read linked review file: %s", p)
		}
		if err := reviewValidateFilled(string(raw)); err != nil {
			return fmt.Errorf("linked review file appears unfilled: %s (%v)", p, err)
		}
	}
	return nil
}

func reviewValidateFilled(markdown string) error {
	// The generated templates contain placeholder TODOs; require authors to replace them.
	// Keep this check small and deterministic (no markdown parsing).
	var offenders []string
	for i, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- TODO") || trimmed == "TODO" {
			offenders = append(offenders, fmt.Sprintf("line %d: %s", i+1, trimmed))
			if len(offenders) >= 5 {
				break
			}
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("contains TODO placeholders (%s)", strings.Join(offenders, "; "))
	}
	return nil
}

func renderReviewTemplate(kind reviewKind, phaseID, taskID string, now time.Time) string {
	header := fmt.Sprintf(
		"# Review (%s)\n\n- Task: %s\n- Phase: %s\n- Created: %s\n- ASK: .pt/reviews/ASK-%s.md (generate/update: `pt ask %s`)\n\n",
		kind,
		taskID,
		phaseID,
		now.Format(time.RFC3339),
		taskID,
		taskID,
	)
	switch kind {
	case reviewKindPre:
		return header + `## Kickoff (what will be done)

Goals (2–5 bullets):
- TODO

Non-goals / out of scope:
- TODO

Spec pack (what this work must match):
- TODO Links: PRD/UX flows/contracts/examples (paths or URLs)

Options considered (2–3) + decision:
1. Option A: ...
2. Option B: ...
Decision: ...
Why: ...

Risks / unknowns + spikes (if needed):
- TODO

Evidence plan (what will be shown as proof):
- Tests: ` + "`go test ./...`" + ` (or targeted commands)
- Artifacts: outputs/... (logs, snapshots, reports)

Final demo plan (make this concrete NOW):
- Command(s) the user can run:
  - ` + "`go run ...`" + `
- Expected output/screenshots/log paths:
  - outputs/...

One question to sanity-check (or “No questions”):
- TODO
`
	case reviewKindPost:
		return header + `## Result (required)

- TODO Result: PASS|FAIL|BLOCKED
- TODO Summary (1–3 sentences)

Spec pack reviewed (what you compared against):
- TODO Links: PRD/UX flows/contracts/examples (paths or URLs)

Evidence (must be checkable):
- TODO Tests run (exact commands + result summary)
- TODO Evidence pointers (paths): outputs/... logs/screenshots/reports

Change summary (what shipped; links/paths):
- TODO

Deltas / tradeoffs (what changed vs plan + why):
- TODO

AI provenance (if used):
- TODO What was AI-assisted? What did you change/review manually?

Gaps / follow-ups (new tasks to create):
- TODO

Next recommended action:
- TODO
`
	case reviewKindDemo:
		return header + `## Demo (user-facing walkthrough)

Goal: Make it easy to run and *see* the product working.

Result (required):
- TODO Result: PASS|FAIL|BLOCKED
- TODO What should the user see? (1–3 bullets)

How to run:
1. Run tests (must pass):
   - ` + "`go test ./...`" + `
2. Create a demo output directory and capture logs (avoid “it worked for me”):
   - ` + "`mkdir -p outputs/demo`" + `
3. Run the demo command and tee output to a file:
   - ` + "`<command> 2>&1 | tee outputs/demo/<name>.log`" + `
4. If the demo can hang (agents/servers), always use an explicit timeout:
   - Prefer a tool flag (e.g., ` + "`--timeout 30s`" + `) if available.
   - Otherwise wrap with a shell timeout where available.

What to look for (expected behavior):
- TODO

Artifacts captured (paths):
- TODO (screenshots/logs/reports under outputs/...)

Known limitations / constraints:
- TODO

## Demo Commands (pt demo run)

Add one or more commands as fenced code blocks. ` + "`pt demo run <task-id>`" + ` will execute each block from the project root and capture logs.

` + "```sh" + `
echo "replace-me"
` + "```" + `
`
	default:
		return header + "TODO\n"
	}
}
