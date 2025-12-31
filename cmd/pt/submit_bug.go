package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func cmdSubmitBug(args []string) error {
	fs := flag.NewFlagSet("submit-bug", flag.ContinueOnError)
	label := fs.String("label", "", "short label for the bug (used in filename)")
	description := fs.String("description", "", "what is wrong (symptoms + impact)")
	foundIn := fs.String("found_in", "", "where it was found (command/path/version)")
	repro := fs.String("repro", "", "reproduction steps (exact commands)")
	dir := fs.String("dir", "", "override base directory (default PT_BUG_DIR or ~/.pt/bugs/<YYYY-MM-DD>)")
	dryRun := fs.Bool("dry-run", false, "print output path without writing")
	fs.Usage = func() {
		fmt.Println("Usage: pt submit-bug --label=TEXT --description=TEXT --found_in=TEXT --repro=TEXT [--dir=PATH] [--dry-run]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*label) == "" || strings.TrimSpace(*description) == "" || strings.TrimSpace(*foundIn) == "" || strings.TrimSpace(*repro) == "" {
		fs.Usage()
		return errors.New("missing required flags (label, description, found_in, repro)")
	}

	now := time.Now()
	baseDir, err := bugBaseDir(*dir, now)
	if err != nil {
		return err
	}
	path := filepath.Join(baseDir, fmt.Sprintf("bug-%s.md", bugSlug(*label)))
	template := bugTemplate(*label, *description, *foundIn, *repro, now)

	if *dryRun {
		fmt.Printf("Bug report path: %s\n", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create bug dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("bug report already exists: %s", path)
	}
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		return fmt.Errorf("write bug report: %w", err)
	}

	fmt.Printf("Bug report saved: %s\n", path)
	fmt.Println("STOP: If PT itself is behaving incorrectly, do not continue project work until this bug is triaged/fixed.")
	return nil
}

func bugBaseDir(explicit string, now time.Time) (string, error) {
	if v := strings.TrimSpace(explicit); v != "" {
		return v, nil
	}
	if env := strings.TrimSpace(os.Getenv("PT_BUG_DIR")); env != "" {
		return filepath.Join(env, now.Format("2006-01-02")), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".pt", "bugs", now.Format("2006-01-02")), nil
}

func bugSlug(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	if s == "" {
		return "bug"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "bug"
	}
	return out
}

func bugTemplate(label, description, foundIn, repro string, now time.Time) string {
	return fmt.Sprintf(`# PT Bug Report

- Created: %s
- Label: %s
- Found In: %s

## Description

%s

## Reproduction

%s

## Expected

- TODO: What should have happened?

## Actual

- TODO: What actually happened? (include exact output/errors)

## Notes

- If this bug blocks progress, stop work and fix PT first.
`, now.Format(time.RFC3339), strings.TrimSpace(label), strings.TrimSpace(foundIn), strings.TrimSpace(description), strings.TrimSpace(repro))
}
