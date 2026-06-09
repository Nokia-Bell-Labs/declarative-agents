// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type statsRecord struct {
	GoSrc   int `yaml:"src"`
	GoTest  int `yaml:"test"`
	GoTotal int `yaml:"total"`
	YAML    int `yaml:"yaml"`
}

// Stats prints lines-of-code and YAML config counts.
func Stats() error {
	var rec statsRecord
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
			rec.YAML += n
		}
		return nil
	})
	if err != nil {
		return err
	}
	rec.GoTotal = rec.GoSrc + rec.GoTest

	fmt.Printf("loc src=%d test=%d total=%d\n", rec.GoSrc, rec.GoTest, rec.GoTotal)
	fmt.Printf("yaml lines=%d\n", rec.YAML)
	return nil
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
