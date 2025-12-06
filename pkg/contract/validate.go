package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidatePayload validates JSON payload bytes against a Contract.
func ValidatePayload(payload []byte, c Contract) error {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	return ValidateData(data, c)
}

// ValidateData validates a decoded payload map.
func ValidateData(data map[string]any, c Contract) error {
	var errs []string

	for _, path := range c.Requirements.MustHave {
		if _, ok := getPath(data, path); !ok {
			errs = append(errs, fmt.Sprintf("missing required path %q", path))
		}
	}

	now := time.Now()
	for sectionName, section := range c.Fields {
		sectionObj, _ := data[sectionName].(map[string]any)
		for fieldName, rule := range section.Rules {
			path := sectionName + "." + fieldName
			val, ok := getPath(data, path)
			if !ok {
				errs = append(errs, fmt.Sprintf("missing field %q", path))
				continue
			}
			if err := validateRule(path, val, rule); err != nil {
				errs = append(errs, err.Error())
			}
			if rule.MustExist {
				if err := checkMustExist(val, rule.AllowGlob); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", path, err))
				}
			}
		}

		if section.FreshnessSecMax > 0 {
			if err := checkFreshness(sectionName, sectionObj, section.FreshnessSecMax, now); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func validateRule(path string, val any, rule FieldRule) error {
	switch rule.Type {
	case "string":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("%s must be string", path)
		}
		if rule.MinLen > 0 && len(s) < rule.MinLen {
			return fmt.Errorf("%s length < %d", path, rule.MinLen)
		}
		if rule.MaxLen > 0 && len(s) > rule.MaxLen {
			return fmt.Errorf("%s length > %d", path, rule.MaxLen)
		}
	case "array:string":
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("%s must be array", path)
		}
		if rule.MinLen > 0 && len(arr) < rule.MinLen {
			return fmt.Errorf("%s requires at least %d entries", path, rule.MinLen)
		}
		for i, v := range arr {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s[%d] must be string", path, i)
			}
		}
	case "object":
		if _, ok := val.(map[string]any); !ok {
			return fmt.Errorf("%s must be object", path)
		}
	case "array:object":
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("%s must be array", path)
		}
		if rule.MinLen > 0 && len(arr) < rule.MinLen {
			return fmt.Errorf("%s requires at least %d entries", path, rule.MinLen)
		}
		for i, v := range arr {
			if _, ok := v.(map[string]any); !ok {
				return fmt.Errorf("%s[%d] must be object", path, i)
			}
		}
	default:
		return fmt.Errorf("%s has unsupported type %q", path, rule.Type)
	}
	return nil
}

func checkMustExist(val any, allowGlob bool) error {
	switch v := val.(type) {
	case string:
		return checkPath(v, allowGlob)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if err := checkPath(s, allowGlob); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkPath(p string, allowGlob bool) error {
	if strings.ContainsAny(p, "*?[") && !allowGlob {
		return fmt.Errorf("glob patterns not allowed: %s", p)
	}
	if allowGlob {
		return nil
	}
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("path missing: %s", p)
	}
	return nil
}

func checkFreshness(sectionName string, obj map[string]any, maxSec int, now time.Time) error {
	if obj == nil {
		return fmt.Errorf("%s missing for freshness check", sectionName)
	}
	issuedRaw, ok := obj["issued_at"]
	if !ok {
		return fmt.Errorf("%s.issued_at missing for freshness check", sectionName)
	}
	issuedStr, ok := issuedRaw.(string)
	if !ok {
		return fmt.Errorf("%s.issued_at must be string", sectionName)
	}
	ts, err := time.Parse(time.RFC3339, issuedStr)
	if err != nil {
		return fmt.Errorf("%s.issued_at invalid time: %v", sectionName, err)
	}
	if now.Sub(ts) > time.Duration(maxSec)*time.Second {
		return fmt.Errorf("%s issued_at is stale (> %ds)", sectionName, maxSec)
	}
	return nil
}

func getPath(data map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = data
	for _, p := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		val, ok := obj[p]
		if !ok {
			return nil, false
		}
		current = val
	}
	return current, true
}

// Resolve returns the value at a given dotted path from decoded JSON payload.
func Resolve(payload []byte, path string) (any, bool, error) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, false, err
	}
	val, ok := getPath(data, path)
	return val, ok, nil
}

// NormalizePath returns a cleaned dotted path.
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, ".")
	path = filepath.Clean(path)
	return strings.ReplaceAll(path, "/", ".")
}
