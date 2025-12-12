package pt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectInfo holds project-level metadata for agent onboarding.
type ProjectInfo struct {
	Summary   string   `json:"summary,omitempty"`   // Brief description of what the project does
	Structure []string `json:"structure,omitempty"` // Key directories/modules to know about
}

// Manifest represents a phase bundle containing multiple tasks.
type Manifest struct {
	Title   string      `json:"title"`
	Owner   string      `json:"owner"`
	Project ProjectInfo `json:"project,omitempty"` // Project metadata for agent context
	Tasks   []Task      `json:"tasks"`
}

// Task describes a unit of work to be synced into Beads.
type Task struct {
	Template        string            `json:"template"`
	Title           string            `json:"title"`
	Role            string            `json:"role"`
	Artifact        string            `json:"artifact,omitempty"`
	Deps            []string          `json:"deps,omitempty"`
	NextHint        string            `json:"next_hint,omitempty"`
	EstimatedEffort string            `json:"estimated_effort,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	DoD             DefinitionOfDone  `json:"dod"`

	// Handoff fields - help agents understand the task without project context
	Context   string   `json:"context,omitempty"`   // WHY: problem being solved, motivation
	Inputs    []string `json:"inputs,omitempty"`    // WHERE: files/directories to read/modify
	Scope     string   `json:"scope,omitempty"`     // BOUNDS: IN-scope and OUT-of-scope
	Reference string   `json:"reference,omitempty"` // RELATED: links to docs, issues, prior work
}

// DefinitionOfDone describes required checks before a task can advance.
type DefinitionOfDone struct {
	Tests         []string `json:"tests,omitempty"`
	ValidationCmd string   `json:"validation_cmd,omitempty"`
	Manual        string   `json:"manual,omitempty"`
	Criteria      []string `json:"criteria,omitempty"`
	OnFailure     string   `json:"on_failure,omitempty"`
}

var allowedTemplates = map[string]struct{}{
	"backend_endpoint":   {},
	"frontend_component": {},
	"migration":          {},
	"bug_fix":            {},
	"refactor":           {},
	"observability_hook": {},
	"discovery":          {},
	"spike":              {},
}

// ParseManifest reads and validates a manifest file (JSON or subset TOML).
func ParseManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var manifest Manifest

	switch ext {
	case ".json":
		manifest, err = parseJSONManifest(data)
	case ".toml":
		manifest, err = parseTOMLManifest(data)
	default:
		return Manifest{}, fmt.Errorf("unsupported manifest extension %q (use .json or .toml)", ext)
	}
	if err != nil {
		return Manifest{}, err
	}

	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func parseJSONManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse json: %w", err)
	}
	return manifest, nil
}

// parseTOMLManifest implements a narrow TOML reader for the documented manifest shape.
// It supports:
// - top-level key/value pairs
// - [project] table for project metadata
// - [[tasks]] array of tables
// - [tasks.params] and [tasks.dod] subtables
// - string values and string arrays
func parseTOMLManifest(data []byte) (Manifest, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	manifest := Manifest{}
	var tasks []Task
	var current *Task
	context := "root" // root | project | task | params | dod

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := strings.TrimSpace(stripInlineComment(scanner.Text()))
		if raw == "" {
			continue
		}

		switch raw {
		case "[project]":
			context = "project"
			continue
		case "[[tasks]]":
			// start new task
			if current != nil {
				tasks = append(tasks, *current)
			}
			current = &Task{Params: map[string]string{}}
			context = "task"
			continue
		case "[tasks.params]":
			if current == nil {
				return Manifest{}, fmt.Errorf("line %d: params table before any task", lineNum)
			}
			if current.Params == nil {
				current.Params = map[string]string{}
			}
			context = "params"
			continue
		case "[tasks.dod]":
			if current == nil {
				return Manifest{}, fmt.Errorf("line %d: dod table before any task", lineNum)
			}
			context = "dod"
			continue
		}

		key, value, err := parseKV(raw)
		if err != nil {
			return Manifest{}, fmt.Errorf("line %d: %w", lineNum, err)
		}

		switch context {
		case "root":
			assignRootKV(&manifest, key, value)
		case "project":
			assignProjectKV(&manifest.Project, key, value)
		case "task":
			if current == nil {
				return Manifest{}, fmt.Errorf("line %d: value without task context", lineNum)
			}
			if err := assignTaskKV(current, key, value); err != nil {
				return Manifest{}, fmt.Errorf("line %d: %w", lineNum, err)
			}
		case "params":
			if current == nil {
				return Manifest{}, fmt.Errorf("line %d: params without task context", lineNum)
			}
			if current.Params == nil {
				current.Params = map[string]string{}
			}
			current.Params[key] = value.asString()
		case "dod":
			if current == nil {
				return Manifest{}, fmt.Errorf("line %d: dod without task context", lineNum)
			}
			if err := assignDoDKV(&current.DoD, key, value); err != nil {
				return Manifest{}, fmt.Errorf("line %d: %w", lineNum, err)
			}
		default:
			return Manifest{}, fmt.Errorf("line %d: unknown context %q", lineNum, context)
		}
	}
	if err := scanner.Err(); err != nil {
		return Manifest{}, fmt.Errorf("scan toml: %w", err)
	}

	if current != nil {
		tasks = append(tasks, *current)
	}
	manifest.Tasks = tasks
	return manifest, nil
}

type value struct {
	kind string   // "string" or "array"
	str  string   // when kind == string
	arr  []string // when kind == array
}

func (v value) asString() string {
	if v.kind == "string" {
		return v.str
	}
	return strings.Join(v.arr, ",")
}

func parseKV(line string) (string, value, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", value{}, fmt.Errorf("expected key = value, got %q", line)
	}
	key := strings.TrimSpace(parts[0])
	rawVal := strings.TrimSpace(parts[1])
	if key == "" {
		return "", value{}, errors.New("empty key")
	}
	if strings.HasPrefix(rawVal, "[") {
		arr, err := parseStringArray(rawVal)
		if err != nil {
			return "", value{}, err
		}
		return key, value{kind: "array", arr: arr}, nil
	}
	strVal, err := parseString(rawVal)
	if err != nil {
		return "", value{}, err
	}
	return key, value{kind: "string", str: strVal}, nil
}

func parseString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		return strings.Trim(raw, `"`), nil
	}
	return raw, nil
}

func parseStringArray(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("invalid array %q", raw)
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		val, err := parseString(part)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}
	return out, nil
}

func assignRootKV(manifest *Manifest, key string, val value) {
	switch key {
	case "title":
		manifest.Title = val.asString()
	case "owner":
		manifest.Owner = val.asString()
	}
}

func assignProjectKV(project *ProjectInfo, key string, val value) {
	switch key {
	case "summary":
		project.Summary = val.asString()
	case "structure":
		project.Structure = val.arr
	}
}

func assignTaskKV(task *Task, key string, val value) error {
	switch key {
	case "template":
		task.Template = val.asString()
	case "title":
		task.Title = val.asString()
	case "role":
		task.Role = val.asString()
	case "artifact":
		task.Artifact = val.asString()
	case "deps":
		task.Deps = val.arr
	case "next_hint":
		task.NextHint = val.asString()
	case "estimated_effort":
		task.EstimatedEffort = val.asString()
	// Handoff fields
	case "context":
		task.Context = val.asString()
	case "inputs":
		task.Inputs = val.arr
	case "scope":
		task.Scope = val.asString()
	case "reference":
		task.Reference = val.asString()
	default:
		// Treat unknown keys in task context as params for forward compatibility.
		if task.Params == nil {
			task.Params = map[string]string{}
		}
		task.Params[key] = val.asString()
	}
	return nil
}

func assignDoDKV(dod *DefinitionOfDone, key string, val value) error {
	switch key {
	case "tests":
		dod.Tests = val.arr
	case "validation_cmd":
		dod.ValidationCmd = val.asString()
	case "manual":
		dod.Manual = val.asString()
	case "criteria":
		dod.Criteria = val.arr
	case "on_failure":
		dod.OnFailure = val.asString()
	default:
		return fmt.Errorf("unsupported dod field %q", key)
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if strings.TrimSpace(manifest.Title) == "" {
		return errors.New("manifest title is required")
	}
	titleIndex := make(map[string]int)
	for i, task := range manifest.Tasks {
		if err := validateTask(task); err != nil {
			return fmt.Errorf("task %d (%s): %w", i+1, task.Title, err)
		}
		if _, exists := titleIndex[task.Title]; exists {
			return fmt.Errorf("duplicate task title %q", task.Title)
		}
		titleIndex[task.Title] = i
	}
	// Dependency validation
	for i, task := range manifest.Tasks {
		for _, dep := range task.Deps {
			if _, ok := titleIndex[dep]; !ok {
				return fmt.Errorf("task %d (%s): unknown dependency %q", i+1, task.Title, dep)
			}
		}
	}
	return nil
}

func validateTask(task Task) error {
	if strings.TrimSpace(task.Title) == "" {
		return errors.New("task title is required")
	}
	if strings.TrimSpace(task.Template) == "" {
		return errors.New("task template is required")
	}
	if _, ok := allowedTemplates[task.Template]; !ok {
		return fmt.Errorf("template %q is not allowed", task.Template)
	}
	if strings.TrimSpace(task.Role) == "" {
		return errors.New("task role is required")
	}
	if strings.TrimSpace(task.Artifact) == "" {
		return errors.New("task artifact is required (link to API/IDL/UI spec)")
	}
	if task.DoD.OnFailure != "" && task.DoD.OnFailure != "block" && task.DoD.OnFailure != "flag" {
		return fmt.Errorf("dod.on_failure must be one of: block, flag, or empty")
	}
	if len(task.DoD.Tests) == 0 {
		return errors.New("definition of done requires tests (at least one command)")
	}
	if strings.TrimSpace(task.DoD.Manual) == "" {
		return errors.New("definition of done requires manual instructions (human validation)")
	}
	if len(task.DoD.Criteria) == 0 {
		return errors.New("definition of done requires acceptance criteria")
	}
	return nil
}

func stripInlineComment(line string) string {
	var out strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '"' {
			inQuote = !inQuote
		}
		if ch == '#' && !inQuote {
			break
		}
		out.WriteByte(ch)
	}
	return out.String()
}
