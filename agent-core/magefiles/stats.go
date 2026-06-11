// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type statsRecord struct {
	GoSrc   int
	GoTest  int
	GoTotal int
	YAML    yamlStats
}

type fileLineStats struct {
	Files int
	Lines int
}

type yamlStats struct {
	Total   fileLineStats
	Docs    categorizedYAMLStats
	Configs categorizedYAMLStats
	Other   fileLineStats
}

type categorizedYAMLStats struct {
	Total      fileLineStats
	Categories map[string]fileLineStats
}

// Stats prints lines-of-code and YAML file breakdowns as a YAML blob.
func Stats() error {
	rec := statsRecord{
		YAML: yamlStats{
			Docs:    categorizedYAMLStats{Categories: make(map[string]fileLineStats)},
			Configs: categorizedYAMLStats{Categories: make(map[string]fileLineStats)},
		},
	}
	skipDirs := map[string]bool{
		".git": true, "vendor": true, binDir: true,
		"magefiles": true, "node_modules": true,
	}

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasSuffix(path, "_test.go"):
			n, _ := countLines(path)
			rec.GoTest += n
		case strings.HasSuffix(path, ".go"):
			n, _ := countLines(path)
			rec.GoSrc += n
		case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
			n, _ := countLines(path)
			addYAMLStats(&rec.YAML, path, n)
		}
		return nil
	})
	if err != nil {
		return err
	}
	rec.GoTotal = rec.GoSrc + rec.GoTest

	printStatsYAML(rec)
	return nil
}

func addYAMLStats(stats *yamlStats, path string, lines int) {
	path = filepath.ToSlash(path)
	incFileLineStats(&stats.Total, lines)

	switch {
	case path == "docs.yaml" || strings.HasPrefix(path, "docs/"):
		incFileLineStats(&stats.Docs.Total, lines)
		cat := docsYAMLCategory(path)
		s := stats.Docs.Categories[cat]
		incFileLineStats(&s, lines)
		stats.Docs.Categories[cat] = s
	case strings.HasPrefix(path, "configs/") || strings.HasPrefix(path, "tools/"):
		incFileLineStats(&stats.Configs.Total, lines)
		cat := configsYAMLCategory(path)
		s := stats.Configs.Categories[cat]
		incFileLineStats(&s, lines)
		stats.Configs.Categories[cat] = s
	default:
		incFileLineStats(&stats.Other, lines)
	}
}

func incFileLineStats(stats *fileLineStats, lines int) {
	stats.Files++
	stats.Lines += lines
}

func docsYAMLCategory(path string) string {
	switch {
	case !strings.Contains(strings.TrimPrefix(path, "docs/"), "/"):
		return "top_level"
	case strings.HasPrefix(path, "docs/specs/config-formats/"):
		return "config_formats"
	case strings.HasPrefix(path, "docs/specs/semantic-models/"):
		return "semantic_models"
	case strings.HasPrefix(path, "docs/specs/software-requirements/"):
		return "software_requirements"
	case strings.HasPrefix(path, "docs/specs/use-cases/"):
		return "use_cases"
	case strings.HasPrefix(path, "docs/specs/test-suites/"):
		return "test_suites"
	case strings.HasPrefix(path, "docs/specs/"):
		return "specs_other"
	default:
		return "docs_other"
	}
}

func configsYAMLCategory(path string) string {
	switch {
	case strings.HasPrefix(path, "tools/"):
		return "shared_tools"
	case strings.Contains(path, "/llm/"):
		return "llm_configs"
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "configs_other"
	}
	switch parts[1] {
	case "generator", "planner", "evaluator", "bench", "validate":
		return parts[1]
	default:
		return "configs_other"
	}
}

func printStatsYAML(rec statsRecord) {
	fmt.Printf("go: {src_lines: %d, test_lines: %d, total_lines: %d}\n", rec.GoSrc, rec.GoTest, rec.GoTotal)
	fmt.Println("yaml:")
	printFileLineStats("  total", rec.YAML.Total)
	printCategorizedYAMLStats("  docs", rec.YAML.Docs)
	printCategorizedYAMLStats("  configs", rec.YAML.Configs)
	printFileLineStats("  other", rec.YAML.Other)
}

func printCategorizedYAMLStats(label string, stats categorizedYAMLStats) {
	fmt.Printf("%s:\n", label)
	printFileLineStats("    total", stats.Total)
	fmt.Println("    categories:")
	for _, key := range sortedKeys(stats.Categories) {
		printFileLineStats("      "+key, stats.Categories[key])
	}
}

func printFileLineStats(label string, stats fileLineStats) {
	fmt.Printf("%s: {files: %d, lines: %d}\n", label, stats.Files, stats.Lines)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n, s.Err()
}
