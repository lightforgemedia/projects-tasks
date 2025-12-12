package pt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ValidationRunner executes DoD checks for a task.
type ValidationRunner struct {
	Runner CommandRunner
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
		vr.Runner = ExecRunner{}
	}
	var outputs []string

	runCmd := func(cmdStr string) error {
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

// WithTimeout returns a context with default timeout if none provided.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 15 * time.Second
	}
	return context.WithTimeout(parent, d)
}
