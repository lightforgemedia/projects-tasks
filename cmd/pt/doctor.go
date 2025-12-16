package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"projects-tasks/pkg/pt"
)

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	dbPath := fs.String("db", "", "store path to check (default: .pt/db.json)")
	fix := fs.Bool("fix", false, "attempt to fix issues (creates backup first)")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt doctor [--db=PATH] [--fix] [--json]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	storePath := *dbPath
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
	}

	// Check store file exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		if *jsonOut {
			return printJSON(map[string]interface{}{
				"status": "error",
				"error":  fmt.Sprintf("store file not found: %s", storePath),
			})
		}
		return fmt.Errorf("store file not found: %s", storePath)
	}

	// Run checks
	checks := runDoctorChecks(storePath)

	// Count issues
	issueCount := 0
	for _, c := range checks {
		if c.Status == "error" || c.Status == "warning" {
			issueCount++
		}
	}

	// If fix requested and there are fixable issues, create backup and fix
	fixedCount := 0
	if *fix && issueCount > 0 {
		// Create backup
		backupPath := storePath + ".backup." + time.Now().UTC().Format("20060102-150405")
		if err := copyFileForBackup(storePath, backupPath); err != nil {
			if *jsonOut {
				return printJSON(map[string]interface{}{
					"status": "error",
					"error":  fmt.Sprintf("failed to create backup: %v", err),
				})
			}
			return fmt.Errorf("failed to create backup: %w", err)
		}

		// Apply fixes
		fixedCount = applyDoctorFixes(storePath, checks)

		// Re-run checks after fix
		checks = runDoctorChecks(storePath)
		issueCount = 0
		for _, c := range checks {
			if c.Status == "error" || c.Status == "warning" {
				issueCount++
			}
		}
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"store_path":  storePath,
			"checks":      checks,
			"issue_count": issueCount,
			"fixed_count": fixedCount,
			"fix_applied": *fix && fixedCount > 0,
		})
	}

	// Human-readable output
	fmt.Printf("PT Doctor - Store: %s\n\n", storePath)
	for _, c := range checks {
		icon := "✓"
		if c.Status == "error" {
			icon = "✗"
		} else if c.Status == "warning" {
			icon = "⚠"
		}
		fmt.Printf("%s %s: %s\n", icon, c.Name, c.Message)
		if c.Details != "" {
			fmt.Printf("  %s\n", c.Details)
		}
	}
	fmt.Println()
	if issueCount == 0 {
		fmt.Println("All checks passed!")
	} else {
		fmt.Printf("%d issue(s) found", issueCount)
		if !*fix {
			fmt.Println(". Run with --fix to attempt repairs.")
		} else if fixedCount > 0 {
			fmt.Printf(", %d fixed.\n", fixedCount)
		} else {
			fmt.Println(" (unfixable).")
		}
	}
	return nil
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warning, error
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Fixable bool   `json:"fixable"`
	FixHint string `json:"fix_hint,omitempty"` // what fix would do
}

func runDoctorChecks(storePath string) []doctorCheck {
	var checks []doctorCheck

	// 1. Check JSON validity
	raw, err := os.ReadFile(storePath)
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "file_readable",
			Status:  "error",
			Message: fmt.Sprintf("cannot read file: %v", err),
		})
		return checks // Can't continue
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		checks = append(checks, doctorCheck{
			Name:    "json_valid",
			Status:  "error",
			Message: fmt.Sprintf("invalid JSON: %v", err),
		})
		return checks // Can't continue
	}
	checks = append(checks, doctorCheck{
		Name:    "json_valid",
		Status:  "ok",
		Message: "store is valid JSON",
	})

	// 2. Check required fields exist
	requiredFields := []string{"next_id", "issues", "labels", "deps"}
	missingFields := []string{}
	for _, f := range requiredFields {
		if _, ok := data[f]; !ok {
			missingFields = append(missingFields, f)
		}
	}
	if len(missingFields) > 0 {
		checks = append(checks, doctorCheck{
			Name:    "required_fields",
			Status:  "warning",
			Message: fmt.Sprintf("missing fields: %v", missingFields),
			Fixable: true,
			FixHint: "initialize missing fields with empty values",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "required_fields",
			Status:  "ok",
			Message: "all required fields present",
		})
	}

	// 3. Check title_map consistency
	ctx, cancel := pt.ContextWithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := pt.NewStoreClient(storePath, "pt")
	issues, _ := client.List(ctx, nil, "", 1000)

	titleMap := make(map[string]string)
	for _, iss := range issues {
		if existing, ok := titleMap[iss.Title]; ok {
			checks = append(checks, doctorCheck{
				Name:    "title_duplicates",
				Status:  "warning",
				Message: fmt.Sprintf("duplicate title: %q (IDs: %s, %s)", iss.Title, existing, iss.ID),
			})
		} else {
			titleMap[iss.Title] = iss.ID
		}
	}
	if len(checks) == 2 || checks[len(checks)-1].Name != "title_duplicates" {
		checks = append(checks, doctorCheck{
			Name:    "title_duplicates",
			Status:  "ok",
			Message: "no duplicate titles",
		})
	}

	// 4. Check for orphaned worktrees
	worktrees, _ := client.ListWorktrees(ctx)
	orphanedWTs := []string{}
	for taskID, wt := range worktrees {
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			orphanedWTs = append(orphanedWTs, fmt.Sprintf("%s:%s", taskID, wt.Path))
		}
	}
	if len(orphanedWTs) > 0 {
		checks = append(checks, doctorCheck{
			Name:    "orphaned_worktrees",
			Status:  "warning",
			Message: fmt.Sprintf("%d worktree record(s) without directory", len(orphanedWTs)),
			Details: strings.Join(orphanedWTs, ", "),
			Fixable: true,
			FixHint: "clear worktree records for missing directories",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "orphaned_worktrees",
			Status:  "ok",
			Message: "all worktree records valid",
		})
	}

	// 5. Check for blocked tasks referencing missing tasks
	blocked, _ := client.ListBlocked(ctx)
	issueIDs := make(map[string]bool)
	for _, iss := range issues {
		issueIDs[iss.ID] = true
	}
	orphanedBlocks := []string{}
	for taskID := range blocked {
		if !issueIDs[taskID] {
			orphanedBlocks = append(orphanedBlocks, taskID)
		}
	}
	if len(orphanedBlocks) > 0 {
		checks = append(checks, doctorCheck{
			Name:    "orphaned_blocks",
			Status:  "warning",
			Message: fmt.Sprintf("%d blocked record(s) for missing tasks", len(orphanedBlocks)),
			Details: strings.Join(orphanedBlocks, ", "),
			Fixable: true,
			FixHint: "remove blocked records for missing tasks",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "orphaned_blocks",
			Status:  "ok",
			Message: "all blocked records valid",
		})
	}

	// 6. Check for broken dependency references
	brokenDeps := []string{}
	for _, iss := range issues {
		deps, _ := client.Dependencies(ctx, iss.ID)
		for _, d := range deps {
			if d.Status == "unknown" {
				brokenDeps = append(brokenDeps, fmt.Sprintf("%s->%s", iss.ID, d.ID))
			}
		}
	}
	if len(brokenDeps) > 0 {
		checks = append(checks, doctorCheck{
			Name:    "broken_deps",
			Status:  "warning",
			Message: fmt.Sprintf("%d broken dependency reference(s)", len(brokenDeps)),
			Details: strings.Join(brokenDeps, ", "),
			Fixable: true,
			FixHint: "remove dependency references to missing tasks",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "broken_deps",
			Status:  "ok",
			Message: "all dependencies valid",
		})
	}

	// 7. Check lock file (just presence, not stale)
	lockPath := storePath + ".lock"
	if _, err := os.Stat(lockPath); err == nil {
		checks = append(checks, doctorCheck{
			Name:    "lock_file",
			Status:  "ok",
			Message: "lock file exists (normal)",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "lock_file",
			Status:  "ok",
			Message: "no stale lock file",
		})
	}

	return checks
}

func applyDoctorFixes(storePath string, checks []doctorCheck) int {
	fixedCount := 0

	// Read raw JSON for modifications
	raw, err := os.ReadFile(storePath)
	if err != nil {
		return 0
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0
	}

	for _, c := range checks {
		if !c.Fixable || c.Status == "ok" {
			continue
		}

		switch c.Name {
		case "required_fields":
			// Initialize missing fields
			if _, ok := data["next_id"]; !ok {
				data["next_id"] = 0
			}
			if _, ok := data["issues"]; !ok {
				data["issues"] = map[string]interface{}{}
			}
			if _, ok := data["labels"]; !ok {
				data["labels"] = map[string]interface{}{}
			}
			if _, ok := data["deps"]; !ok {
				data["deps"] = map[string]interface{}{}
			}
			if _, ok := data["title_map"]; !ok {
				data["title_map"] = map[string]interface{}{}
			}
			if _, ok := data["history"]; !ok {
				data["history"] = map[string]interface{}{}
			}
			if _, ok := data["comments"]; !ok {
				data["comments"] = map[string]interface{}{}
			}
			if _, ok := data["blocked"]; !ok {
				data["blocked"] = map[string]interface{}{}
			}
			if _, ok := data["worktrees"]; !ok {
				data["worktrees"] = map[string]interface{}{}
			}
			fixedCount++

		case "orphaned_worktrees":
			// Clear worktree records for missing directories
			if worktrees, ok := data["worktrees"].(map[string]interface{}); ok {
				for taskID, wt := range worktrees {
					if wtMap, ok := wt.(map[string]interface{}); ok {
						if path, ok := wtMap["path"].(string); ok {
							if _, err := os.Stat(path); os.IsNotExist(err) {
								delete(worktrees, taskID)
								fixedCount++
							}
						}
					}
				}
			}

		case "orphaned_blocks":
			// Remove blocked records for missing tasks
			if blocked, ok := data["blocked"].(map[string]interface{}); ok {
				issues := data["issues"].(map[string]interface{})
				for taskID := range blocked {
					if _, exists := issues[taskID]; !exists {
						delete(blocked, taskID)
						fixedCount++
					}
				}
			}

		case "broken_deps":
			// Remove broken dependency references
			if deps, ok := data["deps"].(map[string]interface{}); ok {
				issues := data["issues"].(map[string]interface{})
				for taskID, depList := range deps {
					if depSlice, ok := depList.([]interface{}); ok {
						validDeps := make([]interface{}, 0)
						for _, dep := range depSlice {
							if depStr, ok := dep.(string); ok {
								if _, exists := issues[depStr]; exists {
									validDeps = append(validDeps, dep)
								} else {
									fixedCount++
								}
							}
						}
						deps[taskID] = validDeps
					}
				}
			}
		}
	}

	if fixedCount > 0 {
		// Write back
		out, _ := json.MarshalIndent(data, "", "  ")
		_ = os.WriteFile(storePath, out, 0644)
	}

	return fixedCount
}

func copyFileForBackup(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Preserve permissions
	srcInfo, err := os.Stat(src)
	if err == nil {
		_ = os.Chmod(dst, srcInfo.Mode().Perm())
	}

	return nil
}
