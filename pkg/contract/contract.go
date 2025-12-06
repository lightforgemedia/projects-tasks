package contract

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Contract represents a context contract schema.
type Contract struct {
	Meta         Meta
	Requirements Requirements
	Fields       map[string]FieldSection
}

// Meta captures metadata about the contract.
type Meta struct {
	Role     string `toml:"role" json:"role"`
	Version  string `toml:"version" json:"version"`
	IssuedAt string `toml:"issued_at" json:"issued_at"`
}

// Requirements capture required paths.
type Requirements struct {
	MustHave []string `toml:"must_have" json:"must_have"`
}

// FieldRule defines validation rules for a specific path.
type FieldRule struct {
	Type        string `toml:"type" json:"type"`
	MinLen      int    `toml:"min_len" json:"min_len"`
	MaxLen      int    `toml:"max_len" json:"max_len"`
	AllowGlob   bool   `toml:"allow_glob" json:"allow_glob"`
	MustExist   bool   `toml:"must_exist" json:"must_exist"`
	Description string `toml:"description" json:"description"`
}

// FieldSection groups rules under a section (e.g., goal, scope, success).
type FieldSection struct {
	Rules           map[string]FieldRule
	FreshnessSecMax int
}

type rawContract struct {
	Meta         Meta                      `toml:"meta"`
	Requirements Requirements              `toml:"requirements"`
	Fields       map[string]map[string]any `toml:"fields"`
}

// Load reads a TOML contract file and normalizes it into a Contract.
func Load(path string) (Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, fmt.Errorf("read contract: %w", err)
	}
	var raw rawContract
	if err := toml.Unmarshal(data, &raw); err != nil {
		return Contract{}, fmt.Errorf("parse contract: %w", err)
	}
	contract := Contract{
		Meta:         raw.Meta,
		Requirements: raw.Requirements,
		Fields:       make(map[string]FieldSection, len(raw.Fields)),
	}
	for sectionName, entries := range raw.Fields {
		sec := FieldSection{Rules: map[string]FieldRule{}}
		for key, val := range entries {
			if key == "freshness_sec_max" {
				sec.FreshnessSecMax = int(asInt64(val))
				continue
			}
			ruleMap, ok := val.(map[string]any)
			if !ok {
				return Contract{}, fmt.Errorf("section %s key %s is not an object", sectionName, key)
			}
			rule, err := mapToRule(ruleMap)
			if err != nil {
				return Contract{}, fmt.Errorf("section %s key %s: %w", sectionName, key, err)
			}
			sec.Rules[key] = rule
		}
		contract.Fields[sectionName] = sec
	}
	if err := contract.validate(); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func (c Contract) validate() error {
	if strings.TrimSpace(c.Meta.Role) == "" {
		return errors.New("meta.role is required")
	}
	if len(c.Requirements.MustHave) == 0 {
		return errors.New("requirements.must_have must contain at least one path")
	}
	return nil
}

func mapToRule(m map[string]any) (FieldRule, error) {
	r := FieldRule{}
	if v, ok := m["type"]; ok {
		r.Type = asString(v)
	}
	if r.Type == "" {
		return r, errors.New("type is required")
	}
	if v, ok := m["min_len"]; ok {
		r.MinLen = int(asInt64(v))
	}
	if v, ok := m["max_len"]; ok {
		r.MaxLen = int(asInt64(v))
	}
	if v, ok := m["allow_glob"]; ok {
		r.AllowGlob = asBool(v)
	}
	if v, ok := m["must_exist"]; ok {
		r.MustExist = asBool(v)
	}
	if v, ok := m["description"]; ok {
		r.Description = asString(v)
	}
	return r, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func asInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		parsed, _ := time.ParseDuration(val)
		if parsed > 0 {
			return int64(parsed.Seconds())
		}
	}
	return 0
}

func asBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.ToLower(val) == "true"
	default:
		return false
	}
}
