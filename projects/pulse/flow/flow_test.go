package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_ValidFixtures(t *testing.T) {
	dir := filepath.Join("..", "testdata", "flows", "valid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no fixtures found in %s", dir)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			f, err := Parse(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if f.ID == "" {
				t.Fatalf("missing id")
			}
			if len(f.Steps) == 0 {
				t.Fatalf("missing steps")
			}
		})
	}
}

func TestParse_InvalidCases(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing id",
			raw:  "version = 1\n[[steps]]\naction = \"click\"\ntarget = \"role=button[name='Save']\"\n",
			want: "id: required",
		},
		{
			name: "empty steps",
			raw:  "id = \"empty_steps\"\nversion = 1\n",
			want: "steps: required (must contain at least 1 step)",
		},
		{
			name: "unsupported action",
			raw:  "id = \"bad_action\"\n[[steps]]\naction = \"tap\"\ntarget = \"role=button[name='Save']\"\n",
			want: "steps[0].action: unsupported value \"tap\" (allowed: click,type,press,wait)",
		},
		{
			name: "type missing value",
			raw:  "id = \"type_missing_value\"\n[[steps]]\naction = \"type\"\ntarget = \"role=textbox[name='Email']\"\n",
			want: "steps[0].value: required for action \"type\"",
		},
		{
			name: "assert missing value",
			raw:  "id = \"assert_missing_value\"\n[[steps]]\naction = \"click\"\ntarget = \"role=button[name='Save']\"\n[[assertions]]\ntype = \"text_contains\"\ntarget = \"data-testid=toast\"\n",
			want: "assertions[0].value: required for type \"text_contains\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			if err == nil {
				t.Fatalf("expected error")
			}
			if err.Error() != tc.want {
				t.Fatalf("error mismatch:\n got: %q\nwant: %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseFile_PrefixesFilenameAndField(t *testing.T) {
	bad := filepath.Join("..", "testdata", "flows", "invalid_missing_id.toml")
	if err := os.WriteFile(bad, []byte("version = 1\n[[steps]]\naction = \"click\"\ntarget = \"role=button[name='Save']\"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(bad) })

	_, err := ParseFile(bad)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got, wantSub := err.Error(), "invalid_missing_id.toml: id: required"; !strings.Contains(got, wantSub) {
		t.Fatalf("expected error to contain %q, got %q", wantSub, got)
	}
}
