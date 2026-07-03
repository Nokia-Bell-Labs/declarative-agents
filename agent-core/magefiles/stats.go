// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type statsOutput struct {
	Go   goStats       `json:"go"`
	YAML yamlStatsJSON `json:"yaml"`
}

type goStats struct {
	SrcLines   int `json:"src_lines"`
	TestLines  int `json:"test_lines"`
	TotalLines int `json:"total_lines"`
}

type fileLineStats struct {
	Files int `json:"files"`
	Lines int `json:"lines"`
}

type yamlStatsJSON struct {
	Total   fileLineStats            `json:"total"`
	Docs    categorizedYAMLStatsJSON `json:"docs"`
	Configs categorizedYAMLStatsJSON `json:"configs"`
	Other   fileLineStats            `json:"other"`
}

type categorizedYAMLStatsJSON struct {
	Total      fileLineStats            `json:"total"`
	Categories map[string]fileLineStats `json:"categories"`
}

// Stats outputs lines-of-code and YAML file breakdowns as JSON to stdout.
func Stats() error {
	var rec statsOutput
	rec.YAML.Docs.Categories = make(map[string]fileLineStats)
	rec.YAML.Configs.Categories = make(map[string]fileLineStats)

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
			rec.Go.TestLines += n
		case strings.HasSuffix(path, ".go"):
			n, _ := countLines(path)
			rec.Go.SrcLines += n
		case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
			n, _ := countLines(path)
			addYAMLStats(&rec.YAML, path, n)
		}
		return nil
	})
	if err != nil {
		return err
	}
	rec.Go.TotalLines = rec.Go.SrcLines + rec.Go.TestLines

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func addYAMLStats(stats *yamlStatsJSON, path string, lines int) {
	path = filepath.ToSlash(path)
	incFileLineStats(&stats.Total, lines)

	switch {
	case path == "docs.yaml" || strings.HasPrefix(path, "docs/"):
		incFileLineStats(&stats.Docs.Total, lines)
		cat := docsYAMLCategory(path)
		s := stats.Docs.Categories[cat]
		incFileLineStats(&s, lines)
		stats.Docs.Categories[cat] = s
	case strings.HasPrefix(path, "agents/") || strings.HasPrefix(path, "configs/") || strings.HasPrefix(path, "tools/"):
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
	case "generator", "planner", "evaluator", "bench", "jurist":
		return parts[1]
	default:
		return "configs_other"
	}
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
