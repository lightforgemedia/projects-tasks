package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func archiveExistingCanonical(path string, now time.Time) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("expected file at %s, found directory", path)
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	archiveDir := filepath.Join(filepath.Dir(path), "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", false, err
	}
	archivedPath := filepath.Join(archiveDir, fmt.Sprintf("%s.%s%s", base, now.Format("2006-01-02-150405"), ext))

	if err := os.Rename(path, archivedPath); err == nil {
		return archivedPath, true, nil
	}
	if err := copyFile(path, archivedPath); err != nil {
		return "", false, err
	}
	if err := os.Remove(path); err != nil {
		return "", false, err
	}
	return archivedPath, true, nil
}
