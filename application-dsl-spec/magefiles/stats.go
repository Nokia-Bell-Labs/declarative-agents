// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type specStatsOutput struct {
	Markdown   specFileStats `json:"markdown"`
	YAML       specFileStats `json:"yaml"`
	Fixtures   specFileStats `json:"fixtures"`
	Statements int           `json:"statements"`
}

type specFileStats struct {
	Files int `json:"files"`
	Lines int `json:"lines"`
}

// Stats outputs the specification's file and statement counts as JSON to
// stdout, in the per-module shape the root stats target aggregates.
func Stats() error {
	rec, err := specCollectStats(".")
	if err != nil {
		return err
	}
	language, err := loadLanguage(languagePath)
	if err != nil {
		return err
	}
	rec.Statements = len(language.Statements)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func specCollectStats(root string) (specStatsOutput, error) {
	var rec specStatsOutput
	skipDirs := map[string]bool{
		".git": true, "magefiles": true, "generated-files": true,
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if skipDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		bucket := specCategoryBucket(&rec, path, filepath.ToSlash(rel))
		if bucket == nil {
			return nil
		}
		n, err := specCountLines(path)
		if err != nil {
			return fmt.Errorf("count lines %s: %w", path, err)
		}
		bucket.Files++
		bucket.Lines += n
		return nil
	})
	if err != nil {
		return specStatsOutput{}, err
	}
	return rec, nil
}

// specCategoryBucket returns the stats bucket a file belongs to, or nil when
// the file is not tallied. Fixture documents count separately from the
// specification's own YAML so conformance-suite growth is visible.
func specCategoryBucket(rec *specStatsOutput, path, slug string) *specFileStats {
	switch {
	case strings.HasPrefix(slug, "fixtures/"):
		return &rec.Fixtures
	case strings.HasSuffix(path, ".md"):
		return &rec.Markdown
	case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
		return &rec.YAML
	}
	return nil
}

func specCountLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	if err := s.Err(); err != nil {
		_ = f.Close()
		return 0, fmt.Errorf("scan %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close %s: %w", path, err)
	}
	return n, nil
}
