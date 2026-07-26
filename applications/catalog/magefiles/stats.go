// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type profileStatsOutput struct {
	Go     profileGoStats   `json:"go"`
	YAML   profileYAMLStats `json:"yaml"`
	Agents agentsSection    `json:"agents"`
}

type profileGoStats struct {
	SrcLines   int `json:"src_lines"`
	TestLines  int `json:"test_lines"`
	TotalLines int `json:"total_lines"`
}

type profileYAMLStats struct {
	Total  fileStats `json:"total"`
	Agents fileStats `json:"agents"`
	Docs   fileStats `json:"docs"`
	Other  fileStats `json:"other"`
}

type fileStats struct {
	Files int `json:"files"`
	Lines int `json:"lines"`
}

// Stats outputs lines-of-code breakdowns for agent-profiles as JSON to stdout.
func Stats() error {
	var rec profileStatsOutput
	skipDirs := map[string]bool{
		".git": true, "vendor": true, "magefiles": true, "node_modules": true,
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
			n, _ := profileCountLines(path)
			rec.Go.TestLines += n
		case strings.HasSuffix(path, ".go"):
			n, _ := profileCountLines(path)
			rec.Go.SrcLines += n
		case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
			n, _ := profileCountLines(path)
			addProfileYAMLStats(&rec.YAML, path, n)
		}
		return nil
	})
	if err != nil {
		return err
	}
	rec.Go.TotalLines = rec.Go.SrcLines + rec.Go.TestLines

	rec.Agents, err = scanAgents("agents", profileCountLines)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func addProfileYAMLStats(stats *profileYAMLStats, path string, lines int) {
	path = filepath.ToSlash(path)
	stats.Total.Files++
	stats.Total.Lines += lines

	switch {
	case strings.HasPrefix(path, "agents/"):
		stats.Agents.Files++
		stats.Agents.Lines += lines
	case strings.HasPrefix(path, "docs/"):
		stats.Docs.Files++
		stats.Docs.Lines += lines
	default:
		stats.Other.Files++
		stats.Other.Lines += lines
	}
}

func profileCountLines(path string) (int, error) {
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
