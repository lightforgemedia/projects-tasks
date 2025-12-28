package pt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidationRunner executes DoD checks for a task.
type ValidationRunner struct {
	Runner CommandRunner
	// WorkDir is the directory to run commands in. If empty, uses current working directory.
	WorkDir string
	// TaskID is the ID of the task being validated. Used for DoD placeholder expansion.
	TaskID string
}

// ValidationResult captures outputs and pass/fail.
type ValidationResult struct {
	Passed bool
	Output string
}

// ValidateDoD runs tests, validation commands, and optional manual confirmation.
// confirmManual should be true if a human/agent confirmed manual steps when dod.Manual is non-empty.
func (vr ValidationRunner) ValidateDoD(ctx context.Context, dod DefinitionOfDone, confirmManual bool) (ValidationResult, error) {
	if vr.Runner == nil {
		vr.Runner = ExecRunner{Dir: vr.WorkDir}
	}
	var outputs []string

	runCmd := func(cmdStr string) error {
		cmdStr = normalizeDoDCmd(cmdStr)
		if cmdStr == "-" {
			return nil
		}
		cmdStr = expandTaskIDPlaceholder(cmdStr, vr.TaskID)
		cmdStr = rewritePTSelf(cmdStr)
		if strings.TrimSpace(cmdStr) == "" {
			return nil
		}
		// Execute via shell to honor quoted args; callers should pass safe/known commands.
		out, err := vr.Runner.Run(ctx, "sh", "-c", cmdStr)
		outputs = append(outputs, cmdStr, string(out))
		if err != nil {
			return fmt.Errorf("command %q failed: %w", cmdStr, err)
		}
		return nil
	}

	for _, cmd := range dod.Tests {
		if err := runCmd(cmd); err != nil {
			return ValidationResult{Passed: false, Output: strings.Join(outputs, "\n")}, err
		}
	}
	if err := runCmd(dod.ValidationCmd); err != nil {
		return ValidationResult{Passed: false, Output: strings.Join(outputs, "\n")}, err
	}
	if strings.TrimSpace(dod.Manual) != "" && !confirmManual {
		return ValidationResult{Passed: false, Output: strings.Join(outputs, "\n")}, errors.New("manual check not confirmed")
	}
	return ValidationResult{Passed: true, Output: strings.Join(outputs, "\n")}, nil
}

func expandTaskIDPlaceholder(cmd string, taskID string) string {
	if strings.TrimSpace(taskID) == "" {
		return cmd
	}
	return strings.ReplaceAll(cmd, "<this-task-id>", taskID)
}

func rewritePTSelf(cmd string) string {
	self := strings.TrimSpace(os.Getenv("PT_SELF"))
	if self == "" {
		return cmd
	}
	base := filepath.Base(self)
	if base != "pt" && base != "pt.exe" {
		return cmd
	}
	s := strings.TrimSpace(cmd)
	if s == "pt" {
		return self
	}
	if strings.HasPrefix(s, "pt ") || strings.HasPrefix(s, "pt\t") {
		rest := strings.TrimSpace(s[len("pt"):])
		if rest == "" {
			return self
		}
		return self + " " + rest
	}
	return cmd
}

func normalizeDoDCmd(cmd string) string {
	s := strings.TrimSpace(cmd)
	if strings.TrimSpace(s) == "" || s == "-" {
		return s
	}

	// Some tasks/manifests store commands as a fully-quoted string (e.g. `"echo ok"`).
	// When we pass that through `sh -c`, the extra wrapper quotes can cause parse errors.
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}

	// Treat placeholder "no-op" DoD entries as "-" so validation doesn't try to execute them.
	// We accept common variants because these often come from manifests or manual edits.
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "(N/A)", "N/A", "NA":
		return "-"
	}
	return s
}

// WithTimeout returns a context with default timeout if none provided.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 15 * time.Second
	}
	return context.WithTimeout(parent, d)
}
