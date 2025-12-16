package impact

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"pulse/flow"
)

// Config is a minimal, deterministic mapping from changed file paths to flow tags.
//
// TOML subset supported:
//
//	p0_tag = "p0"              # optional (default: "p0")
//	always_tags = ["p0"]       # optional (default: ["p0"])
//
//	[[rules]]
//	prefix = "frontend/product/"
//	tags = ["area:product_card"]
type Config struct {
	P0Tag      string   `json:"p0_tag"`
	AlwaysTags []string `json:"always_tags,omitempty"`
	Rules      []Rule   `json:"rules,omitempty"`
}

type Rule struct {
	Prefix string   `json:"prefix"`
	Tags   []string `json:"tags"`
}

// ParseConfig parses the supported TOML subset (no external deps).
func ParseConfig(data []byte) (Config, error) {
	cfg := Config{
		P0Tag:      "p0",
		AlwaysTags: []string{"p0"},
	}

	sc := bufio.NewScanner(bytes.NewReader(data))
	context := "root" // root | rule
	var curRule *Rule

	lineNum := 0
	for sc.Scan() {
		lineNum++
		raw := strings.TrimSpace(stripInlineComment(sc.Text()))
		if raw == "" {
			continue
		}
		if raw == "[[rules]]" {
			if curRule != nil {
				cfg.Rules = append(cfg.Rules, *curRule)
			}
			curRule = &Rule{}
			context = "rule"
			continue
		}
		key, val, err := parseKV(raw)
		if err != nil {
			return Config{}, fmt.Errorf("line %d: %w", lineNum, err)
		}
		switch context {
		case "root":
			switch key {
			case "p0_tag":
				cfg.P0Tag = parseString(val)
			case "always_tags":
				cfg.AlwaysTags = parseStringArray(val)
			}
		case "rule":
			if curRule == nil {
				curRule = &Rule{}
			}
			switch key {
			case "prefix":
				curRule.Prefix = parseString(val)
			case "tags":
				curRule.Tags = parseStringArray(val)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, fmt.Errorf("scan: %w", err)
	}
	if curRule != nil {
		cfg.Rules = append(cfg.Rules, *curRule)
	}

	if strings.TrimSpace(cfg.P0Tag) == "" {
		cfg.P0Tag = "p0"
	}
	if len(cfg.AlwaysTags) == 0 && cfg.P0Tag != "" {
		cfg.AlwaysTags = []string{cfg.P0Tag}
	}
	return cfg, nil
}

func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// TagsForChangedPaths returns the union of tags implied by changed paths and AlwaysTags.
func TagsForChangedPaths(cfg Config, changedPaths []string) []string {
	set := make(map[string]struct{})
	for _, t := range cfg.AlwaysTags {
		t = strings.TrimSpace(t)
		if t != "" {
			set[t] = struct{}{}
		}
	}
	for _, p := range changedPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		for _, r := range cfg.Rules {
			if strings.TrimSpace(r.Prefix) == "" {
				continue
			}
			if strings.HasPrefix(p, r.Prefix) {
				for _, t := range r.Tags {
					t = strings.TrimSpace(t)
					if t != "" {
						set[t] = struct{}{}
					}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// SelectFlowIDs returns flow IDs that match any requested tag; p0 flows are always included.
func SelectFlowIDs(flows []flow.Flow, selectedTags []string, p0Tag string) []string {
	want := make(map[string]struct{}, len(selectedTags))
	for _, t := range selectedTags {
		t = strings.TrimSpace(t)
		if t != "" {
			want[t] = struct{}{}
		}
	}
	outSet := make(map[string]struct{})

	for _, f := range flows {
		id := strings.TrimSpace(f.ID)
		if id == "" {
			continue
		}
		if p0Tag != "" && hasTag(f.Tags, p0Tag) {
			outSet[id] = struct{}{}
			continue
		}
		for _, t := range f.Tags {
			if _, ok := want[t]; ok {
				outSet[id] = struct{}{}
				break
			}
		}
	}

	out := make([]string, 0, len(outSet))
	for id := range outSet {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ChangedFilesFromGitDiff returns repo-relative paths from a git diff range (e.g. "HEAD~1..HEAD").
func ChangedFilesFromGitDiff(ctx context.Context, repoDir string, diffRange string) ([]string, error) {
	repoDir = strings.TrimSpace(repoDir)
	if repoDir == "" {
		return nil, fmt.Errorf("repoDir is empty")
	}
	diffRange = strings.TrimSpace(diffRange)
	if diffRange == "" {
		return nil, fmt.Errorf("diffRange is empty")
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "diff", "--name-only", diffRange)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only: %w", err)
	}
	lines := strings.Split(string(out), "\n")
	var paths []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			paths = append(paths, l)
		}
	}
	return paths, nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func stripInlineComment(s string) string {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		return s[:i]
	}
	return s
}

func parseKV(raw string) (string, string, error) {
	i := strings.Index(raw, "=")
	if i < 0 {
		return "", "", fmt.Errorf("expected key = value, got %q", raw)
	}
	key := strings.TrimSpace(raw[:i])
	val := strings.TrimSpace(raw[i+1:])
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}
	if val == "" {
		return "", "", fmt.Errorf("empty value for key %q", key)
	}
	return key, val, nil
}

func parseString(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func parseStringArray(raw string) []string {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]"))
	if body == "" {
		return nil
	}
	parts := strings.Split(body, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, parseString(v))
	}
	return out
}
