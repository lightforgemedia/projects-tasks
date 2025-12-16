package flow

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Flow struct {
	ID            string        `json:"id"`
	Version       int           `json:"version"`
	Tags          []string      `json:"tags,omitempty"`
	Description   string        `json:"description,omitempty"`
	Preconditions Preconditions `json:"preconditions,omitempty"`
	Steps         []Step        `json:"steps"`
	Assertions    []Assertion   `json:"assertions,omitempty"`
}

type Preconditions struct {
	Route string `json:"route,omitempty"`
	Login string `json:"login,omitempty"`
}

type Step struct {
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
	Value     string `json:"value,omitempty"`
	Key       string `json:"key,omitempty"`
	MS        int    `json:"ms,omitempty"`
	Until     string `json:"until,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	Retry     int    `json:"retry,omitempty"`
	Note      string `json:"note,omitempty"`
}

type Assertion struct {
	Type        string `json:"type"`
	Target      string `json:"target,omitempty"`
	Value       string `json:"value,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type ValidationError struct {
	Path string
	Msg  string
}

func (e ValidationError) Error() string {
	p := strings.TrimSpace(e.Path)
	if p == "" {
		return strings.TrimSpace(e.Msg)
	}
	return fmt.Sprintf("%s: %s", p, strings.TrimSpace(e.Msg))
}

func ParseFile(path string) (Flow, error) {
	if strings.TrimSpace(path) == "" {
		return Flow{}, fmt.Errorf("read flow: path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Flow{}, fmt.Errorf("read flow: %w", err)
	}
	f, err := Parse(raw)
	if err != nil {
		return Flow{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return f, nil
}

func Parse(data []byte) (Flow, error) {
	var out Flow
	out.Version = 1

	sc := bufio.NewScanner(bytes.NewReader(data))
	context := "root" // root | preconditions | step | assertion
	var curStep *Step
	var curAssertion *Assertion

	lineNum := 0
	for sc.Scan() {
		lineNum++
		raw := strings.TrimSpace(stripInlineComment(sc.Text()))
		if raw == "" {
			continue
		}

		switch raw {
		case "[preconditions]":
			flushStep(&out, &curStep)
			flushAssertion(&out, &curAssertion)
			context = "preconditions"
			continue
		case "[[steps]]":
			flushStep(&out, &curStep)
			flushAssertion(&out, &curAssertion)
			curStep = &Step{}
			context = "step"
			continue
		case "[[assertions]]":
			flushStep(&out, &curStep)
			flushAssertion(&out, &curAssertion)
			curAssertion = &Assertion{}
			context = "assertion"
			continue
		}

		key, val, err := parseKV(raw)
		if err != nil {
			return Flow{}, fmt.Errorf("line %d: %w", lineNum, err)
		}

		switch context {
		case "root":
			if err := applyRootKV(&out, key, val); err != nil {
				return Flow{}, err
			}
		case "preconditions":
			if err := applyPreconditionsKV(&out, key, val); err != nil {
				return Flow{}, err
			}
		case "step":
			if curStep == nil {
				curStep = &Step{}
			}
			if err := applyStepKV(curStep, key, val); err != nil {
				return Flow{}, err
			}
		case "assertion":
			if curAssertion == nil {
				curAssertion = &Assertion{}
			}
			if err := applyAssertionKV(curAssertion, key, val); err != nil {
				return Flow{}, err
			}
		default:
			return Flow{}, fmt.Errorf("internal parse error: unknown context %q", context)
		}
	}
	if err := sc.Err(); err != nil {
		return Flow{}, fmt.Errorf("scan: %w", err)
	}

	flushStep(&out, &curStep)
	flushAssertion(&out, &curAssertion)

	applyDefaults(&out)
	if err := validate(out); err != nil {
		return Flow{}, err
	}
	return out, nil
}

func stripInlineComment(s string) string {
	// TOML comment is "# ...". This intentionally does not attempt to handle quoted "#".
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

func applyRootKV(out *Flow, key, val string) error {
	switch key {
	case "id":
		out.ID = parseString(val)
	case "version":
		n, err := parseInt(val)
		if err != nil {
			return ValidationError{Path: "version", Msg: "must be integer"}
		}
		out.Version = n
	case "tags":
		out.Tags = parseStringArray(val)
	case "description":
		out.Description = parseString(val)
	}
	return nil
}

func applyPreconditionsKV(out *Flow, key, val string) error {
	switch key {
	case "route":
		out.Preconditions.Route = parseString(val)
	case "login":
		out.Preconditions.Login = parseString(val)
	}
	return nil
}

func applyStepKV(s *Step, key, val string) error {
	switch key {
	case "action":
		s.Action = parseString(val)
	case "target":
		s.Target = parseString(val)
	case "value":
		s.Value = parseString(val)
	case "key":
		s.Key = parseString(val)
	case "ms":
		n, err := parseInt(val)
		if err != nil {
			return ValidationError{Path: "steps[].ms", Msg: "must be integer"}
		}
		s.MS = n
	case "until":
		s.Until = parseString(val)
	case "timeout_ms":
		n, err := parseInt(val)
		if err != nil {
			return ValidationError{Path: "steps[].timeout_ms", Msg: "must be integer"}
		}
		s.TimeoutMS = n
	case "retry":
		n, err := parseInt(val)
		if err != nil {
			return ValidationError{Path: "steps[].retry", Msg: "must be integer"}
		}
		s.Retry = n
	case "note":
		s.Note = parseString(val)
	}
	return nil
}

func applyAssertionKV(a *Assertion, key, val string) error {
	switch key {
	case "type":
		a.Type = parseString(val)
	case "target":
		a.Target = parseString(val)
	case "value":
		a.Value = parseString(val)
	case "fingerprint":
		a.Fingerprint = parseString(val)
	}
	return nil
}

func parseString(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func parseInt(raw string) (int, error) {
	s := parseString(raw)
	return strconv.Atoi(strings.TrimSpace(s))
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

func flushStep(out *Flow, cur **Step) {
	if *cur != nil {
		out.Steps = append(out.Steps, **cur)
		*cur = nil
	}
}

func flushAssertion(out *Flow, cur **Assertion) {
	if *cur != nil {
		out.Assertions = append(out.Assertions, **cur)
		*cur = nil
	}
}

func applyDefaults(f *Flow) {
	if f.Version == 0 {
		f.Version = 1
	}
	for i := range f.Steps {
		if f.Steps[i].TimeoutMS == 0 {
			f.Steps[i].TimeoutMS = 5000
		}
	}
}

func validate(f Flow) error {
	if strings.TrimSpace(f.ID) == "" {
		return ValidationError{Path: "id", Msg: "required"}
	}
	if !isSnakeCase(f.ID) {
		return ValidationError{Path: "id", Msg: fmt.Sprintf("must be snake_case (got %q)", f.ID)}
	}
	if len(f.Steps) == 0 {
		return ValidationError{Path: "steps", Msg: "required (must contain at least 1 step)"}
	}

	for i, s := range f.Steps {
		path := func(field string) string { return fmt.Sprintf("steps[%d].%s", i, field) }
		switch s.Action {
		case "click":
			if strings.TrimSpace(s.Target) == "" {
				return ValidationError{Path: path("target"), Msg: "required for action \"click\""}
			}
		case "type":
			if strings.TrimSpace(s.Target) == "" {
				return ValidationError{Path: path("target"), Msg: "required for action \"type\""}
			}
			if strings.TrimSpace(s.Value) == "" {
				return ValidationError{Path: path("value"), Msg: "required for action \"type\""}
			}
		case "press":
			if strings.TrimSpace(s.Key) == "" {
				return ValidationError{Path: path("key"), Msg: "required for action \"press\""}
			}
		case "wait":
			if s.MS == 0 && strings.TrimSpace(s.Until) == "" {
				return ValidationError{Path: path("ms"), Msg: "required for action \"wait\" (set ms or until)"}
			}
			if strings.TrimSpace(s.Until) != "" && s.Until != "dom_ready" && s.Until != "network_idle" {
				return ValidationError{Path: path("until"), Msg: "unsupported value (allowed: dom_ready,network_idle)"}
			}
		default:
			return ValidationError{
				Path: path("action"),
				Msg:  fmt.Sprintf("unsupported value %q (allowed: click,type,press,wait)", s.Action),
			}
		}
		if strings.TrimSpace(s.Target) != "" && !validTarget(s.Target) {
			return ValidationError{
				Path: path("target"),
				Msg:  "invalid locator (must start with one of: role=, data-testid=, css=)",
			}
		}
	}

	for i, a := range f.Assertions {
		path := func(field string) string { return fmt.Sprintf("assertions[%d].%s", i, field) }
		switch a.Type {
		case "":
			// allow empty assertion blocks? treat as error to avoid silent no-ops
			return ValidationError{Path: path("type"), Msg: "required"}
		case "visible", "hidden":
			if strings.TrimSpace(a.Target) == "" {
				return ValidationError{Path: path("target"), Msg: fmt.Sprintf("required for type %q", a.Type)}
			}
		case "text_contains":
			if strings.TrimSpace(a.Target) == "" {
				return ValidationError{Path: path("target"), Msg: "required for type \"text_contains\""}
			}
			if strings.TrimSpace(a.Value) == "" {
				return ValidationError{Path: path("value"), Msg: "required for type \"text_contains\""}
			}
		case "url_is":
			if strings.TrimSpace(a.Value) == "" {
				return ValidationError{Path: path("value"), Msg: "required for type \"url_is\""}
			}
		default:
			return ValidationError{
				Path: path("type"),
				Msg:  fmt.Sprintf("unsupported value %q (allowed: visible,hidden,text_contains,url_is)", a.Type),
			}
		}
		if strings.TrimSpace(a.Target) != "" && !validTarget(a.Target) {
			return ValidationError{
				Path: path("target"),
				Msg:  "invalid locator (must start with one of: role=, data-testid=, css=)",
			}
		}
	}

	return nil
}

func isSnakeCase(s string) bool {
	if s == "" {
		return false
	}
	prevUnderscore := false
	for _, r := range s {
		if r == '_' {
			if prevUnderscore {
				return false
			}
			prevUnderscore = true
			continue
		}
		prevUnderscore = false
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return !strings.HasSuffix(s, "_")
}

func validTarget(t string) bool {
	return strings.HasPrefix(t, "role=") || strings.HasPrefix(t, "data-testid=") || strings.HasPrefix(t, "css=")
}
