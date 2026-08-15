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

type dpStatsOutput struct {
	Markdown  dpFileStats `json:"markdown"`
	YAML      dpFileStats `json:"yaml"`
	PlantUML  dpFileStats `json:"puml"`
	Templates dpFileStats `json:"templates"`
}

type dpFileStats struct {
	Files int `json:"files"`
	Lines int `json:"lines"`
}

// dpWalkFunc and dpLineCounter are injected so the traversal and per-file line
// counting are exercised in tests -- including walk, open, scanner, and
// line-read failures -- without depending on the real filesystem layout.
type dpWalkFunc func(root string, fn filepath.WalkFunc) error
type dpLineCounter func(path string) (int, error)

// Stats outputs lines-of-code breakdowns for design-patterns as JSON to stdout.
func Stats() error {
	rec, err := dpCollectStats(".", filepath.Walk, dpCountLines)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

// dpCollectStats walks root, tallying files and lines per category. Traversal
// errors and the line-read errors of files it tallies are propagated
// path-qualified so incomplete counts can never be published as valid output.
func dpCollectStats(root string, walk dpWalkFunc, countLines dpLineCounter) (dpStatsOutput, error) {
	var rec dpStatsOutput
	skipDirs := map[string]bool{
		".git": true, "magefiles": true, "generated-files": true, "node_modules": true,
	}

	err := walk(root, func(path string, info os.FileInfo, err error) error {
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
		slug := filepath.ToSlash(rel)
		bucket := dpCategoryBucket(&rec, path, slug)
		if bucket == nil {
			return nil
		}
		n, err := countLines(path)
		if err != nil {
			return fmt.Errorf("count lines %s: %w", path, err)
		}
		bucket.Files++
		bucket.Lines += n
		return nil
	})
	if err != nil {
		return dpStatsOutput{}, err
	}
	return rec, nil
}

// dpCategoryBucket returns the stats bucket a file belongs to, or nil when the
// file is not tallied. Suffix categories take precedence over the templates
// directory, matching the historical classification order.
func dpCategoryBucket(rec *dpStatsOutput, path, slug string) *dpFileStats {
	switch {
	case strings.HasSuffix(path, ".md"):
		return &rec.Markdown
	case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
		return &rec.YAML
	case strings.HasSuffix(path, ".puml"):
		return &rec.PlantUML
	case strings.HasPrefix(slug, "templates/"):
		return &rec.Templates
	}
	return nil
}

func dpCountLines(path string) (int, error) {
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
