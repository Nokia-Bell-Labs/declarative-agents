// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"encoding/json"
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

// Stats outputs lines-of-code breakdowns for design-patterns as JSON to stdout.
func Stats() error {
	var rec dpStatsOutput
	skipDirs := map[string]bool{
		".git": true, "magefiles": true, "generated-files": true, "node_modules": true,
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

		n, _ := dpCountLines(path)
		slug := filepath.ToSlash(path)

		switch {
		case strings.HasSuffix(path, ".md"):
			rec.Markdown.Files++
			rec.Markdown.Lines += n
		case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
			rec.YAML.Files++
			rec.YAML.Lines += n
		case strings.HasSuffix(path, ".puml"):
			rec.PlantUML.Files++
			rec.PlantUML.Lines += n
		case strings.HasPrefix(slug, "templates/"):
			rec.Templates.Files++
			rec.Templates.Lines += n
		}
		return nil
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func dpCountLines(path string) (int, error) {
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
