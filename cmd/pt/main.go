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

	projects_tasks_pkg_contract "projects-tasks/pkg/contract"
	"projects-tasks/pkg/pt"
)

// bdRunner is injected in tests for the bd backend.
var bdRunner pt.CommandRunner

func newClient() pt.Client {
	return pt.NewClientFromEnv(bdRunner)
}

func userOrUnknown() string {
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	return user
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
	for i, step := range steps {
		fmt.Printf("Manual step %d/%d: %s\nConfirm? [y/N]: ", i+1, len(steps), step)
		var resp string
		fmt.Scanln(&resp)
		resp = strings.TrimSpace(strings.ToLower(resp))
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
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Print(`pt CLI
Commands:
  sync <manifest>                   Apply manifest to bd (creates/updates issues and deps)
  ready [--role=ROLE] [--limit=N]   List ready work (status=open only)
  claim <id>                        Mark issue in_progress and assign current user
  release <id>                      Return issue to open (unassign)
  validate <id> [--yes]             Run DoD hooks and mark needs_review (label)
  approve <id>                      Close issue
  reject <id> --reason="text"       Send issue back to in_progress with comment
  context init <id>|validate <file> Manage agent context contracts
  graph <manifest>                  Visualize task dependencies
`)
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.Usage = func() { fmt.Println("Usage: pt sync <manifest>") }
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
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	idMap, err := client.Sync(ctx, manifest)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
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
	fs.Usage = func() { fmt.Println("Usage: pt ready [--role=ROLE] [--limit=N] [--sort=priority|title] [--verbose]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := newClient()
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issues, err := client.Ready(ctx, *role, *limit)
	if err != nil {
		return fmt.Errorf("ready failed: %w", err)
	}
	pt.SortIssues(issues, *sortKey)
	for _, iss := range issues {
		if iss.Status != "open" { // hide claimed/in_progress
			continue
		}
		line := fmt.Sprintf("%s [%s] %s", iss.ID, iss.IssueType, iss.Title)
		blockers := readyBlockers(ctx, client, iss)
		if *verbose {
			extra := pt.IssueExtra(iss)
			if extra != "" {
				line = fmt.Sprintf("%s %s", line, extra)
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

func cmdClaim(args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee (defaults to $USER)")
	fs.Usage = func() { fmt.Println("Usage: pt claim <id> [--as=USER]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	user := *as
	if strings.TrimSpace(user) == "" {
		user = userOrUnknown()
	}
	trans := pt.Transitioner{Client: newClient()}
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := trans.Claim(ctx, id, user); err != nil {
		return fmt.Errorf("claim failed: %w", err)
	}
	fmt.Printf("Claimed %s as %s\n", id, user)
	return nil
}

func cmdRelease(args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	as := fs.String("as", "", "override assignee check (defaults to $USER)")
	fs.Usage = func() { fmt.Println("Usage: pt release <id> [--as=USER]") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	user := *as
	if strings.TrimSpace(user) == "" {
		user = userOrUnknown()
	}
	trans := pt.Transitioner{Client: newClient()}
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := trans.Release(ctx, id, user); err != nil {
		return fmt.Errorf("release failed: %w", err)
	}
	fmt.Printf("Released %s\n", id)
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "auto-confirm manual checks")
	fs.Usage = func() { fmt.Println("Usage: pt validate <id>") }
	if err := fs.Parse(args); err != nil {
		return err
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
	fmt.Printf("Task %s marked needs_review\n", issue.ID)
	return nil
}

func cmdApprove(args []string) error {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	fs.Usage = func() { fmt.Println("Usage: pt approve <id>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	id := fs.Arg(0)
	client := newClient()
	trans := pt.Transitioner{Client: client}
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := trans.Approve(ctx, id); err != nil {
		return fmt.Errorf("approve failed: %w", err)
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
	fs.Usage = func() { fmt.Println("Usage: pt reject <id> --reason=\"text\"") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("missing id argument")
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("reason is required")
	}
	id := fs.Arg(0)
	trans := pt.Transitioner{Client: newClient()}
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := trans.Reject(ctx, id, *reason); err != nil {
		return fmt.Errorf("reject failed: %w", err)
	}
	fmt.Printf("Rejected %s\n", id)
	return nil
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
				{"field": "goal.prompt", "source": fmt.Sprintf("bd:%s", id)},
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
