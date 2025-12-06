package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	projects_tasks_pkg_contract "projects-tasks/pkg/contract"
	"projects-tasks/pkg/pt"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "sync":
		cmdSync(args)
	case "ready":
		cmdReady(args)
	case "claim":
		cmdClaim(args)
	case "release":
		cmdRelease(args)
	case "validate":
		cmdValidate(args)
	case "approve":
		cmdApprove(args)
	case "reject":
		cmdReject(args)
	case "context":
		cmdContext(args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`pt CLI
Commands:
  sync <manifest>                   Apply manifest to bd (creates/updates issues and deps)
  ready [--role=ROLE] [--limit=N]   List ready work (status=open only)
  claim <id>                        Mark issue in_progress and assign current user
  release <id>                      Return issue to open (unassign)
  validate <id>                     Run DoD hooks and mark needs_review (label)
  approve <id>                      Close issue
  reject <id> --reason="text"       Send issue back to in_progress with comment
  context init <id>|validate <file> Manage agent context contracts
`)
}

func cmdSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	fs.Usage = func() { fmt.Println("Usage: pt sync <manifest>") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	path := fs.Arg(0)
	manifest, err := pt.ParseManifest(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse manifest:", err)
		os.Exit(1)
	}
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	idMap, err := client.Sync(ctx, manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync failed:", err)
		os.Exit(1)
	}
	for title, id := range idMap {
		fmt.Printf("%s -> %s\n", title, id)
	}
}

func cmdReady(args []string) {
	fs := flag.NewFlagSet("ready", flag.ExitOnError)
	role := fs.String("role", "", "filter by role label")
	limit := fs.Int("limit", 10, "max issues")
	fs.Usage = func() { fmt.Println("Usage: pt ready [--role=ROLE] [--limit=N]") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	issues, err := client.Ready(ctx, *role, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ready failed:", err)
		os.Exit(1)
	}
	for _, iss := range issues {
		if iss.Status != "open" { // hide claimed/in_progress
			continue
		}
		fmt.Printf("%s [%s] %s\n", iss.ID, iss.IssueType, iss.Title)
	}
}

func cmdClaim(args []string) {
	fs := flag.NewFlagSet("claim", flag.ExitOnError)
	fs.Usage = func() { fmt.Println("Usage: pt claim <id>") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	id := fs.Arg(0)
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.UpdateIssue(ctx, id, "in_progress", user); err != nil {
		fmt.Fprintln(os.Stderr, "claim failed:", err)
		os.Exit(1)
	}
	if err := client.AddLabels(ctx, id, "state:claimed"); err != nil {
		fmt.Fprintln(os.Stderr, "claim label failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Claimed %s as %s\n", id, user)
}

func cmdRelease(args []string) {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	fs.Usage = func() { fmt.Println("Usage: pt release <id>") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	id := fs.Arg(0)
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.UpdateIssue(ctx, id, "open", ""); err != nil {
		fmt.Fprintln(os.Stderr, "release failed:", err)
		os.Exit(1)
	}
	_ = client.RemoveLabels(ctx, id, "state:claimed", "state:needs_review")
	fmt.Printf("Released %s\n", id)
}

func cmdValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	fs.Usage = func() { fmt.Println("Usage: pt validate <id>") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	id := fs.Arg(0)
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 0)
	defer cancel()
	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get task failed:", err)
		os.Exit(1)
	}
	confirm := true
	if strings.TrimSpace(meta.DoD.Manual) != "" {
		fmt.Printf("Manual check required: %s\nConfirm? [y/N]: ", meta.DoD.Manual)
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			confirm = false
		}
	}
	vr := pt.ValidationRunner{Runner: pt.ExecRunner{}}
	res, err := vr.ValidateDoD(ctx, meta.DoD, confirm)
	fmt.Print(res.Output)
	if err != nil || !res.Passed {
		if err != nil {
			fmt.Fprintln(os.Stderr, "validation failed:", err)
		}
		os.Exit(1)
	}
	if err := client.UpdateIssue(ctx, issue.ID, "in_progress", ""); err != nil {
		fmt.Fprintln(os.Stderr, "mark needs_review failed:", err)
		os.Exit(1)
	}
	if err := client.AddLabels(ctx, issue.ID, "state:needs_review"); err != nil {
		fmt.Fprintln(os.Stderr, "label needs_review failed:", err)
		os.Exit(1)
	}
	if err := client.AddComment(ctx, issue.ID, "Validation passed; ready for review"); err != nil {
		fmt.Fprintln(os.Stderr, "comment failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Task %s marked needs_review\n", issue.ID)
}

func cmdApprove(args []string) {
	fs := flag.NewFlagSet("approve", flag.ExitOnError)
	fs.Usage = func() { fmt.Println("Usage: pt approve <id>") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	id := fs.Arg(0)
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.UpdateIssue(ctx, id, "closed", ""); err != nil {
		fmt.Fprintln(os.Stderr, "approve failed:", err)
		os.Exit(1)
	}
	_ = client.RemoveLabels(ctx, id, "state:needs_review", "state:claimed")
	_ = client.AddComment(ctx, id, "Approved and closed")
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
}

func cmdReject(args []string) {
	fs := flag.NewFlagSet("reject", flag.ExitOnError)
	reason := fs.String("reason", "", "reason for rejection")
	fs.Usage = func() { fmt.Println("Usage: pt reject <id> --reason=\"text\"") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	if strings.TrimSpace(*reason) == "" {
		fmt.Fprintln(os.Stderr, "reason is required")
		os.Exit(1)
	}
	id := fs.Arg(0)
	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.UpdateIssue(ctx, id, "in_progress", ""); err != nil {
		fmt.Fprintln(os.Stderr, "reject failed:", err)
		os.Exit(1)
	}
	_ = client.RemoveLabels(ctx, id, "state:needs_review")
	if err := client.AddComment(ctx, id, fmt.Sprintf("Rejected: %s", *reason)); err != nil {
		fmt.Fprintln(os.Stderr, "comment failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Rejected %s\n", id)
}

func cmdContext(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: pt context <init|validate> [args]")
		os.Exit(1)
	}
	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "init":
		cmdContextInit(subArgs)
	case "validate":
		cmdContextValidate(subArgs)
	default:
		fmt.Fprintln(os.Stderr, "unknown context command:", sub)
		os.Exit(1)
	}
}

func cmdContextInit(args []string) {
	fs := flag.NewFlagSet("context init", flag.ExitOnError)
	role := fs.String("role", "", "override role for contract selection")
	fs.Usage = func() { fmt.Println("Usage: pt context init <id> [--role=ROLE]") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	id := fs.Arg(0)

	client := pt.NewBDClient(nil)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	issue, meta, err := client.GetTask(ctx, id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get task:", err)
		os.Exit(1)
	}

	targetRole := meta.Role
	if *role != "" {
		targetRole = *role
	}
	if targetRole == "" {
		fmt.Fprintln(os.Stderr, "no role found in task or flag")
		os.Exit(1)
	}

	contractPath := fmt.Sprintf("contracts/%s.toml", targetRole)
	contract, err := projects_tasks_pkg_contract.Load(contractPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load contract %s: %v\n", contractPath, err)
		os.Exit(1)
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
}

func cmdContextValidate(args []string) {
	fs := flag.NewFlagSet("context validate", flag.ExitOnError)
	contractPath := fs.String("contract", "", "path to contract TOML (optional, defaults to contracts/<role>.toml if role key exists in payload)")
	fs.Usage = func() { fmt.Println("Usage: pt context validate <context.json> [--contract=PATH]") }
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	payloadPath := fs.Arg(0)

	data, err := os.ReadFile(payloadPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read payload:", err)
		os.Exit(1)
	}

	// Determine contract path
	cPath := *contractPath
	if cPath == "" {
		fmt.Fprintln(os.Stderr, "--contract flag is required")
		os.Exit(1)
	}

	contract, err := projects_tasks_pkg_contract.Load(cPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load contract:", err)
		os.Exit(1)
	}

	if err := projects_tasks_pkg_contract.ValidatePayload(data, contract); err != nil {
		fmt.Fprintln(os.Stderr, "VALIDATION FAILED")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Context is valid.")
}