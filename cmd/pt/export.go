package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"projects-tasks/pkg/pt"
)

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	dbPath := fs.String("db", "", "store path to export (default: .pt/db.json)")
	outPath := fs.String("out", "", "output file (default: stdout)")
	pretty := fs.Bool("pretty", true, "pretty-print JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt export [--db=PATH] [--out=FILE] [--pretty=true|false]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	storePath := *dbPath
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
	}

	// Read the store file
	raw, err := os.ReadFile(storePath)
	if err != nil {
		return fmt.Errorf("read store: %w", err)
	}

	// Validate it's valid JSON
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("invalid store JSON: %w", err)
	}

	// Add export metadata
	exportData := map[string]interface{}{
		"pt_export_version": 1,
		"exported_at":       time.Now().UTC().Format(time.RFC3339),
		"source_path":       storePath,
		"data":              data,
	}

	// Format output
	var output []byte
	if *pretty {
		output, err = json.MarshalIndent(exportData, "", "  ")
	} else {
		output, err = json.Marshal(exportData)
	}
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}

	// Write output
	if *outPath == "" {
		fmt.Println(string(output))
	} else {
		if err := os.WriteFile(*outPath, output, 0644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		fmt.Printf("Exported to %s\n", *outPath)
	}

	return nil
}

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	dbPath := fs.String("db", "", "target store path (default: .pt/db.json)")
	mode := fs.String("mode", "merge", "import mode: merge (add new, update existing) or replace (overwrite)")
	backup := fs.Bool("backup", true, "create backup before import")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pt import <file.json> [--db=PATH] [--mode=merge|replace] [--backup=true|false]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("import file required")
	}
	importPath := fs.Arg(0)

	storePath := *dbPath
	if storePath == "" {
		storePath = pt.DiscoveredStorePath()
	}

	// Read import file
	importRaw, err := os.ReadFile(importPath)
	if err != nil {
		return fmt.Errorf("read import file: %w", err)
	}

	// Parse import data
	var importData map[string]interface{}
	if err := json.Unmarshal(importRaw, &importData); err != nil {
		return fmt.Errorf("invalid import JSON: %w", err)
	}

	// Check if this is an export file or raw store
	var storeData map[string]interface{}
	if exportVersion, ok := importData["pt_export_version"]; ok {
		// This is an export file
		if v, ok := exportVersion.(float64); !ok || v < 1 {
			return fmt.Errorf("unsupported export version")
		}
		data, ok := importData["data"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid export format: missing data field")
		}
		storeData = data
	} else {
		// This is a raw store file
		storeData = importData
	}

	// Create backup if target exists and backup requested
	if *backup {
		if _, err := os.Stat(storePath); err == nil {
			backupPath := storePath + ".backup." + time.Now().UTC().Format("20060102-150405")
			if err := copyFileForBackup(storePath, backupPath); err != nil {
				return fmt.Errorf("create backup: %w", err)
			}
			if !*jsonOut {
				fmt.Printf("Created backup at %s\n", backupPath)
			}
		}
	}

	stats := map[string]int{
		"issues_added":   0,
		"issues_updated": 0,
		"issues_skipped": 0,
	}

	if *mode == "replace" {
		// Simple replace: write the imported data directly
		output, err := json.MarshalIndent(storeData, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal store: %w", err)
		}
		if err := os.WriteFile(storePath, output, 0644); err != nil {
			return fmt.Errorf("write store: %w", err)
		}

		// Count issues
		if issues, ok := storeData["issues"].(map[string]interface{}); ok {
			stats["issues_added"] = len(issues)
		}

		if *jsonOut {
			return printJSON(map[string]interface{}{
				"mode":  "replace",
				"stats": stats,
			})
		}
		fmt.Printf("Replaced store with %d issues\n", stats["issues_added"])
		return nil
	}

	// Merge mode: read existing, merge in new data
	var existingData map[string]interface{}
	if existingRaw, err := os.ReadFile(storePath); err == nil {
		_ = json.Unmarshal(existingRaw, &existingData)
	}
	if existingData == nil {
		existingData = map[string]interface{}{
			"next_id":   float64(0),
			"issues":    map[string]interface{}{},
			"labels":    map[string]interface{}{},
			"deps":      map[string]interface{}{},
			"comments":  map[string]interface{}{},
			"title_map": map[string]interface{}{},
			"history":   map[string]interface{}{},
			"blocked":   map[string]interface{}{},
			"worktrees": map[string]interface{}{},
		}
	}

	// Merge issues
	existingIssues, _ := existingData["issues"].(map[string]interface{})
	importIssues, _ := storeData["issues"].(map[string]interface{})
	if existingIssues == nil {
		existingIssues = make(map[string]interface{})
	}

	// Build title->id map for existing
	existingTitles := make(map[string]string)
	for id, iss := range existingIssues {
		if issMap, ok := iss.(map[string]interface{}); ok {
			if title, ok := issMap["title"].(string); ok {
				existingTitles[title] = id
			}
		}
	}

	for importID, iss := range importIssues {
		if issMap, ok := iss.(map[string]interface{}); ok {
			title, _ := issMap["title"].(string)

			// Check if issue with same title exists
			if existingID, exists := existingTitles[title]; exists {
				// Update existing issue
				existingIssues[existingID] = iss
				stats["issues_updated"]++
			} else if _, exists := existingIssues[importID]; exists {
				// ID collision but different title - skip
				stats["issues_skipped"]++
			} else {
				// New issue
				existingIssues[importID] = iss
				stats["issues_added"]++
			}
		}
	}
	existingData["issues"] = existingIssues

	// Merge other maps (labels, deps, history, etc.) - simple overwrite per key
	for _, field := range []string{"labels", "deps", "comments", "history", "blocked", "worktrees"} {
		existingMap, _ := existingData[field].(map[string]interface{})
		importMap, _ := storeData[field].(map[string]interface{})
		if existingMap == nil {
			existingMap = make(map[string]interface{})
		}
		for k, v := range importMap {
			existingMap[k] = v
		}
		existingData[field] = existingMap
	}

	// Update title_map
	titleMap := make(map[string]interface{})
	for id, iss := range existingIssues {
		if issMap, ok := iss.(map[string]interface{}); ok {
			if title, ok := issMap["title"].(string); ok {
				titleMap[title] = id
			}
		}
	}
	existingData["title_map"] = titleMap

	// Update next_id if needed
	importNextID, _ := storeData["next_id"].(float64)
	existingNextID, _ := existingData["next_id"].(float64)
	if importNextID > existingNextID {
		existingData["next_id"] = importNextID
	}

	// Write merged data
	output, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	if err := os.WriteFile(storePath, output, 0644); err != nil {
		return fmt.Errorf("write store: %w", err)
	}

	if *jsonOut {
		return printJSON(map[string]interface{}{
			"mode":  "merge",
			"stats": stats,
		})
	}
	fmt.Printf("Merged: %d added, %d updated, %d skipped\n",
		stats["issues_added"], stats["issues_updated"], stats["issues_skipped"])
	return nil
}

// copyFileForImportBackup is a local copy helper (copyFileForBackup is in doctor.go)
func copyFileForImportBackup(src, dst string) error {
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
	return err
}
