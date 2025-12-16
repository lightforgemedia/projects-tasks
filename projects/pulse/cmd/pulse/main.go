package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"

	"pulse/flow"
	"pulse/impact"
	"pulse/internal/demo"
)

func main() {
	flags := flag.NewFlagSet("pulse", flag.ExitOnError)
	demoMode := flags.Bool("demo", false, "start the local demo server (for MVP verification)")
	demoAddr := flags.String("addr", "127.0.0.1:8085", "listen address for --demo")
	runMode := flags.Bool("run", false, "run a single flow end-to-end in a real browser (MVP)")
	flowPath := flags.String("flow", "", "flow TOML file path (relative to repo root or pulse root)")
	baseURL := flags.String("base-url", "", "base URL to run against (e.g. http://127.0.0.1:8085)")
	headless := flags.Bool("headless", true, "run browser headless (default true)")
	dryRun := flags.Bool("dry-run", false, "dry-run: load flows, select impacted set, print plan (no browser)")
	diffRange := flags.String("diff", "", "git diff range (e.g. HEAD~1..HEAD) used for impact selection")
	repoDir := flags.String("repo", "", "override git repo root (default: git rev-parse --show-toplevel)")
	pulseRoot := flags.String("pulse-root", "", "override pulse project root (default: <repo>/projects/pulse)")
	flowsDir := flags.String("flows-dir", "", "override flows directory (default: <pulse-root>/testdata/flows/valid)")
	impactConfig := flags.String("impact-config", "", "override impact config path (default: <pulse-root>/impact/map.pulse.toml)")
	jsonOut := flags.Bool("json", false, "emit JSON (reserved; not implemented)")
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  pulse --demo [--addr=127.0.0.1:8085]")
		fmt.Fprintln(os.Stderr, "  pulse --run --flow=PATH --base-url=URL [--headless=true]")
		fmt.Fprintln(os.Stderr, "  pulse --dry-run --diff=RANGE [--repo=DIR] [--pulse-root=DIR] [--flows-dir=DIR] [--impact-config=FILE]")
	}
	_ = jsonOut // reserved for a follow-up task
	flags.Parse(os.Args[1:])

	if *demoMode {
		if err := runDemoServer(*demoAddr); err != nil {
			fmt.Fprintf(os.Stderr, "error: demo server: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *runMode {
		if err := runOneFlow(*repoDir, *pulseRoot, *flowPath, *baseURL, *headless); err != nil {
			fmt.Fprintf(os.Stderr, "error: run: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if !*dryRun {
		fmt.Fprintln(os.Stderr, "pulse: use --demo, --run, or --dry-run (MVP)")
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

func runOneFlow(repoOverride string, pulseOverride string, flowArg string, base string, headless bool) error {
	flowArg = strings.TrimSpace(flowArg)
	base = strings.TrimSpace(base)
	if flowArg == "" {
		return errors.New("--flow is required")
	}
	if base == "" {
		return errors.New("--base-url is required")
	}
	if !strings.HasSuffix(strings.ToLower(flowArg), ".toml") {
		return fmt.Errorf("--flow must be a .toml file path, got %q", flowArg)
	}

	root, err := discoverRepoRoot(repoOverride)
	if err != nil {
		return fmt.Errorf("repo root: %w", err)
	}
	pRoot, err := discoverPulseRoot(root, pulseOverride)
	if err != nil {
		return fmt.Errorf("pulse root: %w", err)
	}

	flowPath, err := resolvePath(root, pRoot, flowArg)
	if err != nil {
		return fmt.Errorf("flow path: %w", err)
	}
	f, err := flow.ParseFile(flowPath)
	if err != nil {
		return fmt.Errorf("parse flow: %w", err)
	}

	runtimePath := filepath.Join(pRoot, "runtime", "pulse_runtime.js")
	runtimeJS, err := os.ReadFile(runtimePath)
	if err != nil {
		return fmt.Errorf("read runtime: %w", err)
	}

	targetURL, err := joinBaseURL(base, f.Preconditions.Route)
	if err != nil {
		return err
	}

	u := launcher.New().Headless(headless).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("about:blank")
	page.MustEvalOnNewDocument(string(runtimeJS))
	page.MustNavigate(targetURL)
	page.MustWaitLoad()

	if !page.MustEval(`() => typeof window.__pulse !== "undefined"`).Bool() {
		return errors.New("runtime not injected (window.__pulse missing)")
	}

	fmt.Printf("==> %s  START\n", f.ID)
	for i, s := range f.Steps {
		if err := runStep(page, i, s); err != nil {
			return err
		}
	}
	for i, a := range f.Assertions {
		if err := runAssertion(page, i, a); err != nil {
			return err
		}
	}
	fmt.Printf("==> %s  PASS\n", f.ID)
	return nil
}

func resolvePath(repoRoot string, pulseRoot string, relOrAbs string) (string, error) {
	if filepath.IsAbs(relOrAbs) {
		if _, err := os.Stat(relOrAbs); err != nil {
			return "", err
		}
		return relOrAbs, nil
	}
	candidates := []string{
		filepath.Join(repoRoot, relOrAbs),
		filepath.Join(pulseRoot, relOrAbs),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("file not found: %q (tried repo root and pulse root)", relOrAbs)
}

func joinBaseURL(base string, route string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", fmt.Errorf("invalid --base-url %q: %w", base, err)
	}
	route = strings.TrimSpace(route)
	if route == "" {
		return u.String(), nil
	}
	out, err := u.Parse(route)
	if err != nil {
		return "", fmt.Errorf("invalid route %q: %w", route, err)
	}
	return out.String(), nil
}

func runStep(page *rod.Page, idx int, s flow.Step) error {
	switch s.Action {
	case "click":
		fmt.Printf("  step[%d] click  target=%s\n", idx, s.Target)
		return pulseAct(page, "click", s.Target, "")
	case "type":
		fmt.Printf("  step[%d] type   target=%s\n", idx, s.Target)
		return pulseAct(page, "type", s.Target, s.Value)
	case "wait":
		if s.MS <= 0 {
			return fmt.Errorf("step[%d]: wait requires ms > 0", idx)
		}
		fmt.Printf("  step[%d] wait   ms=%d\n", idx, s.MS)
		time.Sleep(time.Duration(s.MS) * time.Millisecond)
		return nil
	default:
		return fmt.Errorf("step[%d]: unsupported action %q", idx, s.Action)
	}
}

func pulseAct(page *rod.Page, actType string, target string, value string) error {
	cmd := map[string]any{
		"type":   actType,
		"target": target,
	}
	if actType == "type" {
		cmd["args"] = map[string]any{"value": value}
	}
	res := page.MustEval(`cmd => window.__pulse.Act(cmd)`, cmd)
	if !res.Get("ok").Bool() {
		return fmt.Errorf("act failed (%s %s): %s", actType, target, res.Get("error").String())
	}
	return nil
}

func runAssertion(page *rod.Page, idx int, a flow.Assertion) error {
	switch a.Type {
	case "visible":
		found, visible, _, err := evalTarget(page, a.Target)
		if err != nil {
			return err
		}
		if !found || !visible {
			return fmt.Errorf("assertion[%d] visible failed: target=%s", idx, a.Target)
		}
		fmt.Printf("  assertion[%d] visible OK  target=%s\n", idx, a.Target)
		return nil
	case "hidden":
		found, visible, _, err := evalTarget(page, a.Target)
		if err != nil {
			return err
		}
		if found && visible {
			return fmt.Errorf("assertion[%d] hidden failed: target=%s", idx, a.Target)
		}
		fmt.Printf("  assertion[%d] hidden OK  target=%s\n", idx, a.Target)
		return nil
	case "text_contains":
		found, visible, text, err := evalTarget(page, a.Target)
		if err != nil {
			return err
		}
		if !found || !visible || !strings.Contains(text, a.Value) {
			return fmt.Errorf("assertion[%d] text_contains failed: target=%s want=%q got=%q", idx, a.Target, a.Value, text)
		}
		fmt.Printf("  assertion[%d] text_contains OK  target=%s\n", idx, a.Target)
		return nil
	case "url_is":
		path := page.MustEval(`() => location.pathname`).String()
		if path != a.Value {
			return fmt.Errorf("assertion[%d] url_is failed: want=%q got=%q", idx, a.Value, path)
		}
		fmt.Printf("  assertion[%d] url_is OK  value=%s\n", idx, a.Value)
		return nil
	default:
		return fmt.Errorf("assertion[%d]: unsupported type %q", idx, a.Type)
	}
}

func evalTarget(page *rod.Page, target string) (found bool, visible bool, text string, err error) {
	js := `(target) => {
		function norm(s) { return String(s || "").replace(/\s+/g, " ").trim(); }
		function labelText(el) {
			const id = el && el.id;
			if (id) {
				const lab = document.querySelector('label[for="' + CSS.escape(id) + '"]');
				if (lab) return norm(lab.textContent || "");
			}
			const labelledBy = el.getAttribute && el.getAttribute("aria-labelledby");
			if (labelledBy) {
				const ref = document.getElementById(labelledBy);
				if (ref) return norm(ref.textContent || "");
			}
			return "";
		}
		function roleOf(el) {
			const r = el.getAttribute && el.getAttribute("role");
			if (r) return r;
			const tag = (el.tagName || "").toLowerCase();
			if (tag === "a") return "link";
			if (tag === "button") return "button";
			if (tag === "input" || tag === "textarea") return "textbox";
			if (tag === "select") return "combobox";
			return tag || "unknown";
		}
		function nameOf(el) {
			const aria = el.getAttribute && el.getAttribute("aria-label");
			if (aria) return norm(aria);
			const testid = el.getAttribute && el.getAttribute("data-testid");
			if (testid) return norm(testid);
			const tag = (el.tagName || "").toLowerCase();
			if (tag === "input" || tag === "textarea" || tag === "select") {
				const lab = labelText(el);
				if (lab) return lab;
			}
			return norm(el.innerText || el.textContent || "");
		}
		function parseRoleLocator(target) {
			const raw = String(target || "");
			const rolePart = raw.slice("role=".length);
			const bracket = rolePart.indexOf("[");
			const role = (bracket >= 0 ? rolePart.slice(0, bracket) : rolePart).trim();
			let name = "";
			if (bracket >= 0) {
				const attrs = rolePart.slice(bracket);
				const k = "name=";
				const i = attrs.indexOf(k);
				if (i >= 0) {
					const after = attrs.slice(i + k.length);
					const q = after[0];
					if (q === "'" || q === "\"") {
						const end = after.indexOf(q, 1);
						if (end > 0) name = after.slice(1, end);
					}
				}
			}
			return { role, name: norm(name) };
		}
		function find(target) {
			const t = String(target || "");
			if (t.startsWith("css=")) return document.querySelector(t.slice("css=".length));
			if (t.startsWith("data-testid=")) {
				const v = t.slice("data-testid=".length);
				return document.querySelector('[data-testid="' + CSS.escape(v) + '"]');
			}
			if (t.startsWith("role=")) {
				const q = parseRoleLocator(t);
				if (!q.role) return null;
				const nodes = document.querySelectorAll("button,a,input,select,textarea,[role]");
				for (const el of nodes) {
					if (!el || !el.getBoundingClientRect) continue;
					const r = el.getBoundingClientRect();
					if (!r || r.width <= 0 || r.height <= 0) continue;
					if (roleOf(el) !== q.role) continue;
					if (q.name && nameOf(el) !== q.name) continue;
					return el;
				}
				return null;
			}
			return null;
		}
		function isVisible(el) {
			if (!el) return false;
			const ariaHidden = el.getAttribute && el.getAttribute("aria-hidden");
			if (ariaHidden === "true") return false;
			const st = window.getComputedStyle(el);
			if (!st || st.display === "none" || st.visibility === "hidden") return false;
			const r = el.getBoundingClientRect();
			return !!r && r.width > 0 && r.height > 0;
		}
		const el = find(target);
		return {
			found: !!el,
			visible: el ? isVisible(el) : false,
			text: el ? norm(el.innerText || el.textContent || "") : ""
		};
	}`

	res := page.MustEval(js, target)
	return res.Get("found").Bool(), res.Get("visible").Bool(), res.Get("text").String(), nil
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

func runDemoServer(addr string) error {
	s, err := demo.Start(addr)
	if err != nil {
		return err
	}
	fmt.Printf("Pulse demo server listening: %s\n", s.BaseURL)
	fmt.Printf("Routes: /products?query=socks  /login  /settings/profile\n")
	fmt.Printf("Ctrl+C to stop.\n")

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.Close(ctx)
}
