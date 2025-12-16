package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pulse/flow"
	"pulse/impact"
)

func main() {
	flags := flag.NewFlagSet("pulse", flag.ExitOnError)
	dryRun := flags.Bool("dry-run", false, "dry-run: load flows, select impacted set, print plan (no browser)")
	diffRange := flags.String("diff", "", "git diff range (e.g. HEAD~1..HEAD) used for impact selection")
	repoDir := flags.String("repo", "", "override git repo root (default: git rev-parse --show-toplevel)")
	pulseRoot := flags.String("pulse-root", "", "override pulse project root (default: <repo>/projects/pulse)")
	flowsDir := flags.String("flows-dir", "", "override flows directory (default: <pulse-root>/testdata/flows/valid)")
	impactConfig := flags.String("impact-config", "", "override impact config path (default: <pulse-root>/impact/map.pulse.toml)")
	jsonOut := flags.Bool("json", false, "emit JSON (reserved; not implemented)")
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pulse --dry-run --diff=RANGE [--repo=DIR] [--pulse-root=DIR] [--flows-dir=DIR] [--impact-config=FILE]")
	}
	_ = jsonOut // reserved for a follow-up task
	flags.Parse(os.Args[1:])

	if !*dryRun {
		fmt.Fprintln(os.Stderr, "pulse: only --dry-run is implemented in MVP")
		os.Exit(2)
	}
	if strings.TrimSpace(*diffRange) == "" {
		flags.Usage()
		fmt.Fprintln(os.Stderr, "error: --diff is required for --dry-run")
		os.Exit(2)
	}

	root, err := discoverRepoRoot(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: repo root: %v\n", err)
		os.Exit(1)
	}
	pRoot, err := discoverPulseRoot(root, *pulseRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: pulse root: %v\n", err)
		os.Exit(1)
	}

	fDir := *flowsDir
	if strings.TrimSpace(fDir) == "" {
		fDir = filepath.Join(pRoot, "testdata", "flows", "valid")
	} else if !filepath.IsAbs(fDir) {
		fDir = filepath.Join(pRoot, fDir)
	}

	cfgPath := *impactConfig
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = filepath.Join(pRoot, "impact", "map.pulse.toml")
	} else if !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Join(pRoot, cfgPath)
	}

	cfg, err := impact.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load impact config: %v\n", err)
		os.Exit(1)
	}

	flows, err := loadFlows(fDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load flows: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	changed, err := impact.ChangedFilesFromGitDiff(ctx, root, *diffRange)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: git diff: %v\n", err)
		os.Exit(1)
	}

	tags := impact.TagsForChangedPaths(cfg, changed)
	selectedIDs := impact.SelectFlowIDs(flows, tags, cfg.P0Tag)

	printPlan(root, pRoot, *diffRange, changed, tags, flows, selectedIDs)
}

func discoverRepoRoot(override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		return filepath.Abs(override)
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return filepath.Abs(strings.TrimSpace(string(out)))
}

func discoverPulseRoot(repoRoot string, override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		return filepath.Abs(override)
	}
	if strings.TrimSpace(repoRoot) == "" {
		return "", errors.New("repoRoot is empty")
	}
	p := filepath.Join(repoRoot, "projects", "pulse")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("expected pulse root at %s: %w", p, err)
	}
	return p, nil
}

func loadFlows(dir string) ([]flow.Flow, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("flows dir is empty")
	}
	var flows []flow.Flow
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".toml" {
			return nil
		}
		f, err := flow.ParseFile(path)
		if err != nil {
			return err
		}
		flows = append(flows, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(flows, func(i, j int) bool { return flows[i].ID < flows[j].ID })
	return flows, nil
}

func printPlan(repoRoot, pulseRoot, diffRange string, changed []string, tags []string, flows []flow.Flow, selectedIDs []string) {
	fmt.Printf("Pulse dry-run (MVP)\n")
	fmt.Printf("repo=%s\n", repoRoot)
	fmt.Printf("pulse_root=%s\n", pulseRoot)
	fmt.Printf("diff=%s\n", diffRange)
	fmt.Printf("changed_files=%d\n", len(changed))
	fmt.Printf("selected_tags=%s\n", strings.Join(tags, ","))

	byID := make(map[string]flow.Flow, len(flows))
	for _, f := range flows {
		byID[f.ID] = f
	}

	fmt.Printf("selected_flows=%d\n", len(selectedIDs))
	for _, id := range selectedIDs {
		f, ok := byID[id]
		if !ok {
			continue
		}
		reasons := reasonsForFlow(f, tags)
		fmt.Printf("  - %s  (reasons=%s)\n", f.ID, strings.Join(reasons, ","))
	}
}

func reasonsForFlow(f flow.Flow, selectedTags []string) []string {
	tagSet := make(map[string]struct{}, len(selectedTags))
	for _, t := range selectedTags {
		tagSet[t] = struct{}{}
	}
	var reasons []string
	for _, t := range f.Tags {
		if _, ok := tagSet[t]; ok {
			reasons = append(reasons, t)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "p0")
	}
	sort.Strings(reasons)
	return reasons
}
