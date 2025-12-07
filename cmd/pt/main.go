package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	projects_tasks_pkg_contract "projects-tasks/pkg/contract"
	"projects-tasks/pkg/pt"
)

func newClient() pt.Client {
	return pt.NewClientFromEnv()
}

func requireUser(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		return "", errors.New("no user identity set; use --as or set $USER")
	}
	return user, nil
}

func splitManualSteps(prompt string) []string {
	lines := strings.Split(prompt, "\n")
	var steps []string
	for _, l := range lines {
		s := strings.TrimSpace(l)
		if s != "" {
			steps = append(steps, s)
		}
	}
	if len(steps) == 0 && strings.TrimSpace(prompt) != "" {
		steps = append(steps, strings.TrimSpace(prompt))
	}
	return steps
}

func combineHooks(parts ...[]HookResult) []HookResult {
	var out []HookResult
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func printJSON(data interface{}) error {
	enc, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(enc))
	return nil
}

func readyBlockers(ctx context.Context, client pt.Client, iss pt.Issue) []string {
	deps, err := client.Dependencies(ctx, iss.ID)
	if err != nil {
		return nil
	}
	var blocked []string
	for _, d := range deps {
		status := strings.ToLower(strings.TrimSpace(d.Status))
		if status == "" {
			status = "unknown"
		}
		if status == "closed" || status == "done" || status == "resolved" {
			continue
		}
		blocked = append(blocked, fmt.Sprintf("%s(%s)", d.ID, status))
	}
	return blocked
}

func confirmManual(steps []string, autoYes bool) bool {
	if len(steps) == 0 {
		return true
	}
	if autoYes {
		return true
	}
	scanner := bufio.NewScanner(os.Stdin)
	for i, step := range steps {
		fmt.Printf("Manual step %d/%d: %s\nConfirm? [y/N]: ", i+1, len(steps), step)
		if !scanner.Scan() {
			return false
		}
		resp := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if resp != "y" && resp != "yes" {
			return false
		}
	}
	return true
}

func main() {
	if err := run(os.Args); err != nil {
		if errString := err.Error(); errString != "" {
			fmt.Fprintln(os.Stderr, errString)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		usage()
		return errors.New("")
	}
	if loadedHooks == nil {
		cfg, err := loadHooks()
		if err != nil {
			return err
		}
		loadedHooks = cfg
	}

	cmd := args[1]
	cmdArgs := args[2:]

	switch cmd {
	case "sync":
		return cmdSync(cmdArgs)
	case "ready":
		return cmdReady(cmdArgs)
	case "claim":
		return cmdClaim(cmdArgs)
	case "release":
		return cmdRelease(cmdArgs)
	case "validate":
		return cmdValidate(cmdArgs)
	case "approve":
		return cmdApprove(cmdArgs)
	case "reject":
		return cmdReject(cmdArgs)
	case "context":
		return cmdContext(cmdArgs)
	case "graph":
		return cmdGraph(cmdArgs)
	case "list":
		return cmdList(cmdArgs)
	case "show":
		return cmdShow(cmdArgs)
	case "add":
		return cmdAdd(cmdArgs)
	case "comment":
		return cmdComment(cmdArgs)
	case "snapshot":
		return cmdSnapshot(cmdArgs)
	case "hooks":
		return cmdHooksPrint()
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Print(`pt CLI (store-backed)

Commands (SDLC flow):
  sync <manifest>                    Apply manifest (create/update tasks, deps)
  ready [--role=ROLE] [--limit=N]    List unblocked work (status=open)
  list  [--status=...]               List tasks by status (open|in_progress|needs_review|closed)
  show  <id> [--json]                Show task details (DoD, deps, comments)
  claim <id> [--as=USER]             Mark in_progress and assign
  release <id>                       Return to open and clear assignee
  validate <id> [--yes]              Run DoD hooks; on success -> needs_review
  approve <id>                       Mark done
  reject <id> --reason="text"        Send back to in_progress with a comment
  add "Title" [flags]                Create ad-hoc task (role/template required)
  comment <id> "text"                Append a comment to a task
  snapshot [--out=...]               Copy the store to a timestamped file
  context init <id>|validate <file>  Manage agent context contracts
  graph <manifest>                   Visualize manifest dependencies (cycles shown)
  hooks                              Print merged hook configuration (global + local)

Happy-path primer:
  1) pt sync phases/<file>.toml
  2) pt ready --role=<role> --verbose
  3) pt claim <id> [--as=you]
  4) do work; pt validate <id> [--yes]
  5) pt approve <id>  |  pt reject <id> --reason="..."
  6) pt release <id> (if you’re stuck)
`)
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt sync <manifest>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing manifest argument")
	}
	path := fs.Arg(0)
	manifest, err := pt.ParseManifest(path)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	actor := strings.TrimSpace(os.Getenv("USER"))
	preHooks, err := runHooks("pre-sync", hookPayload{Actor: actor})
	if err != nil {
		return err
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	idMap, err := client.Sync(ctx, manifest)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}
	postHooks, err := runHooks("post-sync", hookPayload{Actor: actor})
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status": "ok",
			"synced": idMap,
			"hooks":  combineHooks(preHooks, postHooks),
		})
	}
	for title, id := range idMap {
		fmt.Printf("%s -> %s\n", title, id)
	}
	return nil
}

func cmdReady(args []string) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 10, "max issues")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	verbose := fs.Bool("verbose", false, "show extra info (assignee, blockers)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt ready [--role=ROLE] [--limit=N] [--sort=priority|title] [--verbose]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issues, err := client.Ready(ctx, *role, *limit)
	if err != nil {
		return fmt.Errorf("ready failed: %w", err)
	}
	pt.SortIssues(issues, *sortKey)
	if *jsonOut {
		out := []map[string]interface{}{}
		for _, iss := range issues {
			if iss.Status != "open" {
				continue
			}
			blockers := readyBlockers(ctx, client, iss)
			out = append(out, map[string]interface{}{
				"id":        iss.ID,
				"title":     iss.Title,
				"status":    iss.Status,
				"assignee":  iss.Assignee,
				"labels":    iss.Labels,
				"blockers":  blockers,
				"next_hint": iss.NextHint,
			})
		}
		return printJSON(out)
	}
	for _, iss := range issues {
		if iss.Status != "open" { // hide claimed/in_progress
			continue
		}
		line := fmt.Sprintf("%s [%s] %s", iss.ID, iss.IssueType, iss.Title)
		if !*verbose && strings.TrimSpace(iss.Assignee) == "" {
			line = fmt.Sprintf("%s [unassigned]", line)
		}
		blockers := readyBlockers(ctx, client, iss)
		if *verbose {
			extra := pt.IssueExtra(iss)
			if extra != "" {
				line = fmt.Sprintf("%s %s", line, extra)
			}
			if strings.TrimSpace(iss.NextHint) != "" {
				line = fmt.Sprintf("%s next:%s", line, iss.NextHint)
			}
			if len(blockers) > 0 {
				line = fmt.Sprintf("%s blocked by %s", line, strings.Join(blockers, ","))
			}
		} else if len(blockers) > 0 {
			indicator := blockers[0]
			if len(blockers) > 1 {
				indicator = fmt.Sprintf("%s(+%d)", blockers[0], len(blockers)-1)
			}
			line = fmt.Sprintf("%s [blocked %s]", line, indicator)
		}
		fmt.Println(line)
	}
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	status := fs.String("status", "open", "comma-separated statuses (open,in_progress,needs_review,closed). Empty for all.")
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 50, "max issues")
	sortKey := fs.String("sort", "priority", "sort by priority|title")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt list [--status=open,in_progress] [--role=ROLE] [--limit=N] [--json]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	var statuses []string
	if strings.TrimSpace(*status) != "" {
		for _, s := range strings.Split(*status, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				statuses = append(statuses, s)
			}
		}
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issues, err := client.List(ctx, statuses, *role, *limit)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}
	pt.SortIssues(issues, *sortKey)
	if *jsonOut {
		return printJSON(issues)
	}
	for _, iss := range issues {
		line := fmt.Sprintf("%s [%s] %s status=%s", iss.ID, iss.IssueType, iss.Title, iss.Status)
		if strings.TrimSpace(iss.Assignee) == "" {
			line = fmt.Sprintf("%s [unassigned]", line)
		} else {
			line = fmt.Sprintf("%s @%s", line, iss.Assignee)
		}
		if strings.TrimSpace(iss.NextHint) != "" {
			line = fmt.Sprintf("%s next:%s", line, iss.NextHint)
		}
		fmt.Println(line)
	}
	return nil
}

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt show <id> [--json]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iss, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return err
	}
	deps, _ := client.Dependencies(ctx, id)
	comments, _ := client.Comments(ctx, id)
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"id":        iss.ID,
			"title":     iss.Title,
			"status":    iss.Status,
			"assignee":  iss.Assignee,
			"labels":    iss.Labels,
			"next_hint": iss.NextHint,
			"meta":      meta,
			"deps":      deps,
			"comments":  comments,
			"description": iss.Description,
		})
	}
	fmt.Printf("%s [%s]\n", iss.ID, iss.Title)
	fmt.Printf("Status: %s\n", iss.Status)
	if iss.Assignee != "" {
		fmt.Printf("Assignee: %s\n", iss.Assignee)
	} else {
		fmt.Println("Assignee: (unassigned)")
	}
	if iss.NextHint != "" {
		fmt.Printf("Next: %s\n", iss.NextHint)
	}
	fmt.Printf("Role: %s  Template: %s\n", meta.Role, meta.Template)
	if len(iss.Labels) > 0 {
		fmt.Printf("Labels: %s\n", strings.Join(iss.Labels, ","))
	}
	if len(deps) > 0 {
		var parts []string
		for _, d := range deps {
			parts = append(parts, fmt.Sprintf("%s(%s)", d.ID, d.Status))
		}
		fmt.Printf("Deps: %s\n", strings.Join(parts, ", "))
	}
	fmt.Println("DoD:")
	if len(meta.DoD.Tests) > 0 {
		fmt.Printf("  tests: %s\n", strings.Join(meta.DoD.Tests, ", "))
	}
	if meta.DoD.ValidationCmd != "" {
		fmt.Printf("  validation_cmd: %s\n", meta.DoD.ValidationCmd)
	}
	if meta.DoD.Manual != "" {
		fmt.Printf("  manual: %s\n", meta.DoD.Manual)
	}
	if len(comments) > 0 {
		fmt.Println("Comments:")
		for _, c := range comments {
			fmt.Printf("  - %s\n", c)
		}
	}
	return nil
}

func cmdClaim(args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee (defaults to $USER)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt claim <id> [--as=USER]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	user, err := requireUser(*as)
	if err != nil {
		return err
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   user,
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-claim", payload)
	if err != nil {
		return err
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Claim(ctx, id, user); err != nil {
		return fmt.Errorf("claim failed: %w", err)
	}
	postHooks, err := runHooks("post-claim", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":   "ok",
			"id":       id,
			"assignee": user,
			"hooks":    combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Claimed %s as %s\n", id, user)
	return nil
}

func cmdRelease(args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee check (defaults to $USER)")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt release <id> [--as=USER]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	user, err := requireUser(*as)
	if err != nil {
		return err
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   "",
		Actor:      user,
		StatusFrom: issue.Status,
		StatusTo:   "open",
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Release(ctx, id, user); err != nil {
		return fmt.Errorf("release failed: %w", err)
	}
	postHooks, err := runHooks("post-release", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":   "ok",
			"id":       id,
			"assignee": "",
			"hooks":    postHooks,
		})
	}
	fmt.Printf("Released %s\n", id)
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "auto-confirm manual checks")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt validate <id>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusNeedsReview),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-validate", payload)
	if err != nil {
		return err
	}
	manualSteps := splitManualSteps(meta.DoD.Manual)
	confirm := confirmManual(manualSteps, *yes)
	vr := pt.ValidationRunner{Runner: pt.ExecRunner{}}
	res, err := vr.ValidateDoD(ctx, meta.DoD, confirm)
	fmt.Print(res.Output)
	if err != nil || !res.Passed {
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		return errors.New("validation failed")
	}
	trans := pt.Transitioner{Client: client}
	comment := "Validation passed; ready for review"
	if len(manualSteps) > 0 {
		comment = fmt.Sprintf("Validation passed; manual steps confirmed: %s", strings.Join(manualSteps, "; "))
	}
	if err := trans.SubmitForReview(ctx, issue.ID, comment); err != nil {
		return fmt.Errorf("mark needs_review failed: %w", err)
	}
	postHooks, err := runHooks("post-validate", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":           "ok",
			"id":               issue.ID,
			"new_status":       string(pt.StatusNeedsReview),
			"manual_confirmed": confirm,
			"hooks":            combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Task %s marked needs_review\n", issue.ID)
	return nil
}

func cmdApprove(args []string) error {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt approve <id>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   "closed",
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	preHooks, err := runHooks("pre-approve", payload)
	if err != nil {
		return err
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Approve(ctx, id); err != nil {
		return fmt.Errorf("approve failed: %w", err)
	}
	postHooks, err := runHooks("post-approve", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":     "ok",
			"id":         id,
			"new_status": "closed",
			"hooks":      combineHooks(preHooks, postHooks),
		})
	}
	fmt.Printf("Approved %s\n", id)

	// Show next tasks
	issues, err := client.Ready(ctx, "", 5)
	if err == nil && len(issues) > 0 {
		fmt.Println("\nUnblocked Work:")
		for _, iss := range issues {
			if iss.Status != "open" {
				continue
			}
			fmt.Printf("%s [%s] %s\n", iss.ID, iss.IssueType, iss.Title)
		}
	}
	return nil
}

func cmdReject(args []string) error {
	fs := flag.NewFlagSet("reject", flag.ContinueOnError)
	reason := fs.String("reason", "", "reason for rejection")
	hookVerboseFlag := fs.Bool("hook-verbose", false, "log hook execution (same as PT_HOOK_VERBOSE=1)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt reject <id> --reason=\"text\"") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *hookVerboseFlag {
		os.Setenv("PT_HOOK_VERBOSE", "1")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("reason is required")
	}
	id := fs.Arg(0)
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	dodJSON, _ := json.Marshal(meta.DoD)
	actor := strings.TrimSpace(os.Getenv("USER"))
	payload := hookPayload{
		ID:         issue.ID,
		Title:      issue.Title,
		Assignee:   issue.Assignee,
		Actor:      actor,
		StatusFrom: issue.Status,
		StatusTo:   string(pt.StatusInProgress),
		Role:       meta.Role,
		DoDJSON:    string(dodJSON),
	}
	trans := pt.Transitioner{Client: client}
	if err := trans.Reject(ctx, id, *reason); err != nil {
		return fmt.Errorf("reject failed: %w", err)
	}
	postHooks, err := runHooks("post-reject", payload)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]interface{}{
			"status":     "ok",
			"id":         id,
			"new_status": string(pt.StatusInProgress),
			"reason":     *reason,
			"hooks":      postHooks,
		})
	}
	fmt.Printf("Rejected %s\n", id)
	return nil
}

func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	role := fs.String("role", "", "role label (required)")
	template := fs.String("template", "", "task template (required)")
	manual := fs.String("manual", "", "manual DoD text")
	tests := fs.String("tests", "", "comma-separated test commands")
	validation := fs.String("validation-cmd", "", "validation command")
	deps := fs.String("deps", "", "comma-separated dependency IDs")
	nextHint := fs.String("next-hint", "", "suggested next task")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt add \"Title\" --role=... --template=... [--manual=...] [--tests=...] [--validation-cmd=...] [--deps=...] [--next-hint=...]") }
	var title string
	var flagArgs []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && title == "" {
			title = a
			continue
		}
		flagArgs = append(flagArgs, a)
	}
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if title == "" {
		if fs.NArg() > 0 {
			title = fs.Arg(0)
		} else {
			fs.Usage()
			return errors.New("missing title argument")
		}
	}
	if strings.TrimSpace(*role) == "" || strings.TrimSpace(*template) == "" {
		return errors.New("role and template are required")
	}
	var depList []string
	if strings.TrimSpace(*deps) != "" {
		for _, d := range strings.Split(*deps, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				depList = append(depList, d)
			}
		}
	}
	var testList []string
	if strings.TrimSpace(*tests) != "" {
		for _, t := range strings.Split(*tests, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				testList = append(testList, t)
			}
		}
	}
	dod := pt.DefinitionOfDone{
		Tests:         testList,
		ValidationCmd: *validation,
		Manual:        *manual,
	}
	if len(dod.Tests) == 0 && strings.TrimSpace(dod.ValidationCmd) == "" && strings.TrimSpace(dod.Manual) == "" {
		return errors.New("definition of done requires at least one of: tests, validation_cmd, manual")
	}
	task := pt.Task{
		Title:    title,
		Template: *template,
		Role:     *role,
		Deps:     depList,
		NextHint: *nextHint,
		DoD:      dod,
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := client.AddTask(ctx, task)
	if err != nil {
		return fmt.Errorf("add task failed: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]string{"status": "ok", "id": id})
	}
	fmt.Printf("Created %s\n", id)
	return nil
}

func cmdComment(args []string) error {
	fs := flag.NewFlagSet("comment", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt comment <id> \"text\"") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return errors.New("requires id and comment text")
	}
	id := fs.Arg(0)
	body := strings.TrimSpace(strings.Join(fs.Args()[1:], " "))
	if body == "" {
		return errors.New("comment text required")
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.AddComment(ctx, id, body); err != nil {
		return fmt.Errorf("comment failed: %w", err)
	}
	if *jsonOut {
		return printJSON(map[string]string{"status": "ok", "id": id})
	}
	fmt.Printf("Comment added to %s\n", id)
	return nil
}

func cmdSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	out := fs.String("out", "", "output file path (optional)")
	fs.Usage = func() { fmt.Println("Usage: pt snapshot [--out=path]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	dbPath := os.Getenv("PT_DB")
	if strings.TrimSpace(dbPath) == "" {
		dbPath = ".pt.db.json"
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("store not found at %s", dbPath)
	}
	target := *out
	if strings.TrimSpace(target) == "" {
		target = fmt.Sprintf("%s.snap.%s.json", dbPath, time.Now().Format("20060102-150405"))
	}
	if err := copyFile(dbPath, target); err != nil {
		return fmt.Errorf("snapshot failed: %w", err)
	}
	fmt.Printf("Snapshot written to %s\n", target)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func cmdContext(args []string) error {
	if len(args) < 1 {
		return errors.New("Usage: pt context <init|validate> [args]")
	}
	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "init":
		return cmdContextInit(subArgs)
	case "validate":
		return cmdContextValidate(subArgs)
	default:
		return fmt.Errorf("unknown context command: %s", sub)
	}
}

func parseContextInitArgs(args []string) (string, string, error) {
	var id string
	var role string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--role="):
			role = strings.TrimPrefix(arg, "--role=")
		case arg == "--role":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--role requires a value")
			}
			role = args[i+1]
			i++
		case strings.HasPrefix(arg, "-"):
			return "", "", fmt.Errorf("unknown flag %s", arg)
		default:
			if id == "" {
				id = arg
			} else {
				return "", "", fmt.Errorf("Usage: pt context init <id> [--role=ROLE]")
			}
		}
	}
	if id == "" {
		return "", "", fmt.Errorf("Usage: pt context init <id> [--role=ROLE]")
	}
	return id, role, nil
}

func cmdContextInit(args []string) error {
	id, roleFlag, err := parseContextInitArgs(args)
	if err != nil {
		fmt.Println("Usage: pt context init <id> [--role=ROLE]")
		return err
	}

	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	targetRole := meta.Role
	if roleFlag != "" {
		targetRole = roleFlag
	}
	if targetRole == "" {
		return errors.New("no role found in task or flag")
	}

	contractPath := fmt.Sprintf("contracts/%s.toml", targetRole)
	contract, err := projects_tasks_pkg_contract.Load(contractPath)
	if err != nil {
		return fmt.Errorf("load contract %s: %w", contractPath, err)
	}
	_ = contract

	// Scaffold payload
	payload := map[string]any{
		"goal": map[string]any{
			"prompt":      issue.Description, // simplistic mapping
			"description": issue.Description,
		},
		"scope": map[string]any{
			"files": []string{},
		},
		"success": map[string]any{
			"criteria": meta.DoD.Tests,
			"tests":    meta.DoD.Tests,
		},
		"provenance": map[string]any{
			"inputs": []map[string]string{
				{"field": "goal.prompt", "source": fmt.Sprintf("pt:%s", id)},
			},
			"issued_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	out, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(out))
	return nil
}

func cmdContextValidate(args []string) error {
	fs := flag.NewFlagSet("context validate", flag.ContinueOnError)
	contractPath := fs.String("contract", "", "path to contract TOML (optional, defaults to contracts/<role>.toml if role key exists in payload)")
	fs.Usage = func() { fmt.Println("Usage: pt context validate <context.json> [--contract=PATH]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing context file")
	}
	payloadPath := fs.Arg(0)

	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return fmt.Errorf("read payload: %w", err)
	}

	// Determine contract path
	cPath := *contractPath
	if cPath == "" {
		role, roleErr := extractRoleFromPayload(data)
		if roleErr != nil {
			return fmt.Errorf("--contract flag is required (and role not found in payload): %w", roleErr)
		}
		cPath = fmt.Sprintf("contracts/%s.toml", role)
	}

	cPath = resolveContractPath(cPath)

	contract, err := projects_tasks_pkg_contract.Load(cPath)
	if err != nil {
		return fmt.Errorf("load contract: %w", err)
	}

	if err := projects_tasks_pkg_contract.ValidatePayload(data, contract); err != nil {
		fmt.Fprintln(os.Stderr, "VALIDATION FAILED")
		return err
	}
	fmt.Println("Context is valid.")
	return nil
}

func extractRoleFromPayload(data []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	if role, ok := payload["role"].(string); ok && strings.TrimSpace(role) != "" {
		return role, nil
	}
	if metaRaw, ok := payload["meta"]; ok {
		if metaMap, ok := metaRaw.(map[string]any); ok {
			if role, ok := metaMap["role"].(string); ok && strings.TrimSpace(role) != "" {
				return role, nil
			}
		}
	}
	return "", errors.New("role not found in payload")
}

func resolveContractPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	candidates := []string{
		path,
		filepath.Join("..", path),
		filepath.Join("..", "..", path),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return path
}

func cmdGraph(args []string) error {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: pt graph <manifest>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing manifest argument")
	}
	path := fs.Arg(0)
	manifest, err := pt.ParseManifest(path)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	fmt.Print(pt.RenderManifestTree(manifest))
	return nil
}
