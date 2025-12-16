package impact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pulse/flow"
)

func TestTagsForChangedPaths(t *testing.T) {
	cfg := Config{
		P0Tag:      "p0",
		AlwaysTags: []string{"p0"},
		Rules: []Rule{
			{Prefix: "frontend/product/", Tags: []string{"area:product_card"}},
			{Prefix: "frontend/settings/", Tags: []string{"area:settings"}},
		},
	}

	tags := TagsForChangedPaths(cfg, []string{"frontend/product/card.tsx"})
	if len(tags) == 0 {
		t.Fatalf("expected tags")
	}
	if !contains(tags, "area:product_card") {
		t.Fatalf("expected area:product_card, got %v", tags)
	}
	if !contains(tags, "p0") {
		t.Fatalf("expected p0 always included, got %v", tags)
	}
}

func TestExampleConfigAndDiff_SelectsExpectedTags(t *testing.T) {
	rawCfg, err := os.ReadFile(filepath.Join("testdata", "map.example.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	cfg, err := ParseConfig(rawCfg)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	rawDiff, err := os.ReadFile(filepath.Join("testdata", "diff_name_only.txt"))
	if err != nil {
		t.Fatalf("read diff: %v", err)
	}
	var changed []string
	for _, l := range strings.Split(string(rawDiff), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			changed = append(changed, l)
		}
	}

	tags := TagsForChangedPaths(cfg, changed)
	if !contains(tags, "area:product_card") || !contains(tags, "p0") {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestSelectFlowIDs_IncludesP0(t *testing.T) {
	flows := mustLoadValidFlows(t)
	ids := SelectFlowIDs(flows, []string{"area:settings"}, "p0")
	if len(ids) == 0 {
		t.Fatalf("expected ids")
	}

	foundP0 := false
	for _, id := range ids {
		if id == "auth_login_basic" {
			foundP0 = true
		}
	}
	if !foundP0 {
		t.Fatalf("expected p0 flow auth_login_basic included, got %v", ids)
	}
}

func mustLoadValidFlows(t *testing.T) []flow.Flow {
	t.Helper()
	dir := filepath.Join("..", "testdata", "flows", "valid")
	paths := []string{
		filepath.Join(dir, "product_card_quickadd.toml"),
		filepath.Join(dir, "settings_profile_save.toml"),
		filepath.Join(dir, "auth_login_basic.toml"),
	}
	out := make([]flow.Flow, 0, len(paths))
	for _, p := range paths {
		f, err := flow.ParseFile(p)
		if err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		out = append(out, f)
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
