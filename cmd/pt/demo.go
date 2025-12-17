package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

// demo command: run a project's demo steps and capture artifacts under outputs/demo/.
func cmdDemo(args []string) error {
	if len(args) == 0 {
		return demoUsage()
	}
	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "run":
		return cmdDemoRun(subArgs)
	case "-h", "--help", "help":
		return demoUsage()
	default:
		_ = demoUsage()
		return fmt.Errorf("unknown demo subcommand %q", sub)
	}
}

func demoUsage() error {
	fmt.Print(`pt demo - Run a demo checklist and capture proof artifacts

Subcommands:
  run <task-id>          Execute demo commands from the linked demo review file

Notes:
  - Demo commands are read from the review file linked by: pt review write <id> --kind=demo
  - Commands are parsed from fenced code blocks under: "## Demo Commands (pt demo run)".
  - Output logs are written under: outputs/demo/
  - Safety: ` + "`pt demo run`" + ` prepends a guarded ` + "`rm`" + ` to PATH to reduce accidental deletes outside the project root.
    Use ` + "`--no-rm-guard`" + ` to disable (not recommended).
`)
	return errors.New("")
}

var execCommandContext = exec.CommandContext

func cmdDemoRun(args []string) error {
	args = reorderArgs(args)
	fs := flag.NewFlagSet("demo run", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 10*time.Minute, "overall timeout for running all demo commands")
	outDirFlag := fs.String("out", "", "override output dir (default: <project-root>/outputs/demo)")
	noRMGuard := fs.Bool("no-rm-guard", false, "disable safety guard that blocks `rm` outside the project root")
	dbPath := fs.String("db", "", "override store path")
	prefix := fs.String("prefix", "", "override issue prefix")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() { fmt.Println("Usage: pt demo run <task-id> [--timeout=10m]") }
	if err := fs.Parse(args); err != nil {
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
	root := projectRootFromStorePath(storePath)
	if strings.TrimSpace(root) == "" {
		return errors.New("cannot determine project root (set PT_DB or use --db)")
	}
	outDir := strings.TrimSpace(*outDirFlag)
	if outDir == "" {
		outDir = filepath.Join(root, "outputs", "demo")
	}

	client := newClientWith(*dbPath, *prefix)
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := client.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}
	comments, err := client.Comments(ctx, taskID)
	if err != nil {
		return fmt.Errorf("read comments: %w", err)
	}
	reviewPath, kind := findLinkedReview(comments)
	if strings.TrimSpace(reviewPath) == "" || !strings.EqualFold(strings.TrimSpace(kind), "demo") {
		return fmt.Errorf("no linked demo review found for %s (run: pt review write %s --kind=demo)", taskID, taskID)
	}

	raw, err := os.ReadFile(reviewPath)
	if err != nil {
		return fmt.Errorf("read review file: %w", err)
	}
	commands := parseDemoCommands(string(raw))
	if len(commands) == 0 {
		return fmt.Errorf("no demo commands found in %s (add code blocks under '## Demo Commands (pt demo run)')", reviewPath)
	}

	runCtx, runCancel := context.WithTimeout(context.Background(), *timeout)
	defer runCancel()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	now := time.Now()

	env := os.Environ()
	env = append(env, "PT_PROJECT_ROOT="+root)
	pathPrepend := ""
	rmGuardDir := ""
	if !*noRMGuard {
		dir, err := os.MkdirTemp("", "pt-demo-guard-*")
		if err != nil {
			return fmt.Errorf("create rm-guard dir: %w", err)
		}
		rmGuardDir = dir
		pathPrepend = rmGuardDir
		if err := writeRMGuard(filepath.Join(rmGuardDir, "rm")); err != nil {
			_ = os.RemoveAll(rmGuardDir)
			return err
		}
	}
	if rmGuardDir != "" {
		defer os.RemoveAll(rmGuardDir)
	}

	type cmdResult struct {
		Command string `json:"command"`
		LogPath string `json:"log_path"`
		ExitOK  bool   `json:"exit_ok"`
	}
	var results []cmdResult
	for i, cmdStr := range commands {
		logPath := filepath.Join(outDir, fmt.Sprintf("%s.%02d.log", now.Format("2006-01-02-150405"), i+1))
		if err := runDemoCommand(runCtx, root, cmdStr, logPath, env, pathPrepend); err != nil {
			results = append(results, cmdResult{Command: cmdStr, LogPath: logPath, ExitOK: false})
			if *jsonOut {
				_ = printJSON(map[string]any{
					"status":  "error",
					"task_id": taskID,
					"out_dir": outDir,
					"failed":  cmdStr,
					"results": results,
					"error":   err.Error(),
				})
			}
			return err
		}
		results = append(results, cmdResult{Command: cmdStr, LogPath: logPath, ExitOK: true})
	}

	if *jsonOut {
		return printJSON(map[string]any{
			"status":  "ok",
			"task_id": taskID,
			"out_dir": outDir,
			"results": results,
		})
	}
	fmt.Printf("Demo completed for %s\n", taskID)
	for _, r := range results {
		fmt.Printf("- ok: %s (log: %s)\n", r.Command, r.LogPath)
	}
	return nil
}

func findLinkedReview(comments []string) (path, kind string) {
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
			// prefer the latest demo review
			if strings.EqualFold(strings.TrimSpace(currentKind), "demo") {
				path = currentPath
				kind = currentKind
			}
			currentPath = ""
			currentKind = ""
		}
	}
	return strings.TrimSpace(path), strings.TrimSpace(kind)
}

func parseDemoCommands(markdown string) []string {
	sc := bufio.NewScanner(strings.NewReader(markdown))
	inSection := false
	inCode := false
	var buf strings.Builder
	var out []string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			inSection = strings.TrimSpace(line) == "## Demo Commands (pt demo run)"
		}
		if !inSection {
			continue
		}
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			if !inCode {
				inCode = true
				buf.Reset()
				continue
			}
			// end code block
			inCode = false
			cmd := strings.TrimSpace(buf.String())
			if cmd != "" {
				out = append(out, cmd)
			}
			continue
		}
		if inCode {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	return out
}

func runDemoCommand(ctx context.Context, workDir, cmdStr, logPath string, env []string, pathPrepend string) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	cmd := execCommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = workDir
	if len(env) > 0 {
		cmd.Env = env
	}
	if strings.TrimSpace(pathPrepend) != "" {
		cmd.Env = prependPath(cmd.Env, pathPrepend)
	}
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("demo command failed: %s (log: %s): %w", cmdStr, logPath, err)
	}
	return nil
}

func prependPath(env []string, dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	set := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			set = true
			out = append(out, "PATH="+dir+string(os.PathListSeparator)+strings.TrimPrefix(e, "PATH="))
			continue
		}
		out = append(out, e)
	}
	if !set {
		out = append(out, "PATH="+dir)
	}
	return out
}

func writeRMGuard(path string) error {
	script := `#!/bin/sh
set -eu

ROOT="${PT_PROJECT_ROOT:-}"
if [ -z "$ROOT" ]; then
  echo "rm blocked: PT_PROJECT_ROOT not set" >&2
  exit 1
fi

for arg in "$@"; do
  case "$arg" in
    -*)
      ;;
    /*)
      case "$arg" in
        "$ROOT"/*|"$ROOT")
          ;;
        *)
          echo "rm blocked: absolute path outside project root: $arg" >&2
          exit 1
          ;;
      esac
      ;;
    *".."*|*"/.."*|*"../"*|*".."/*)
      echo "rm blocked: parent-path usage is not allowed in demos: $arg" >&2
      exit 1
      ;;
    *)
      ;;
  esac
done

exec /bin/rm "$@"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write rm-guard: %w", err)
	}
	return nil
}
