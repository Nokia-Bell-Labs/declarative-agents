// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realCounter counts lines of a real file so the valid-total tests exercise the
// production dpCountLines through dpCollectStats.
func realCounter(path string) (int, error) { return dpCountLines(path) }

func TestDPCollectStatsTalliesCategoriesAndIgnoresGenerated(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("intro.md", "a\nb\nc\n")                        // markdown: 3 lines
	writeFile("spec.yaml", "x\ny\n")                          // yaml: 2 lines
	writeFile("other.yml", "one\n")                           // yaml: 1 line
	writeFile("diagram.puml", "@startuml\n@enduml\n")         // puml: 2 lines
	writeFile("templates/report.tpl", "L1\nL2\nL3\nL4\n")     // templates: 4 lines
	writeFile("templates/nested.md", "only\n")                // markdown (suffix wins): 1 line
	writeFile("generated-files/skip.md", "no\ncount\nhere\n") // ignored dir
	writeFile("node_modules/dep.yaml", "ignored\n")           // ignored dir

	rec, err := dpCollectStats(root, filepath.Walk, realCounter)
	if err != nil {
		t.Fatalf("dpCollectStats: %v", err)
	}
	if rec.Markdown.Files != 2 || rec.Markdown.Lines != 4 {
		t.Errorf("markdown = %+v, want {Files:2 Lines:4}", rec.Markdown)
	}
	if rec.YAML.Files != 2 || rec.YAML.Lines != 3 {
		t.Errorf("yaml = %+v, want {Files:2 Lines:3}", rec.YAML)
	}
	if rec.PlantUML.Files != 1 || rec.PlantUML.Lines != 2 {
		t.Errorf("puml = %+v, want {Files:1 Lines:2}", rec.PlantUML)
	}
	if rec.Templates.Files != 1 || rec.Templates.Lines != 4 {
		t.Errorf("templates = %+v, want {Files:1 Lines:4}", rec.Templates)
	}
}

func TestDPCollectStatsPropagatesWalkError(t *testing.T) {
	walkErr := errors.New("permission denied traversing")
	// A walker that hands the callback an error for a path, as filepath.Walk
	// does when a directory cannot be read.
	walk := func(_ string, fn filepath.WalkFunc) error {
		return fn("unreadable/dir", nil, walkErr)
	}
	_, err := dpCollectStats(".", walk, realCounter)
	if err == nil || !strings.Contains(err.Error(), "walk unreadable/dir") ||
		!errors.Is(err, walkErr) {
		t.Fatalf("err = %v, want wrapped walk error", err)
	}
}

func TestDPCollectStatsPropagatesLineCountError(t *testing.T) {
	countErr := errors.New("scanner exploded")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.md"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failing := func(path string) (int, error) {
		if strings.HasSuffix(path, ".md") {
			return 0, countErr
		}
		return 0, nil
	}
	_, err := dpCollectStats(root, filepath.Walk, failing)
	if err == nil || !strings.Contains(err.Error(), "count lines") || !errors.Is(err, countErr) {
		t.Fatalf("err = %v, want wrapped count-lines error", err)
	}
}

func TestDPCollectStatsDoesNotCountUntrackedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "binary.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An untracked file must never be opened for counting; a counter that always
	// errors proves it is skipped.
	strict := func(path string) (int, error) {
		return 0, errors.New("must not count untracked file " + path)
	}
	rec, err := dpCollectStats(root, filepath.Walk, strict)
	if err != nil {
		t.Fatalf("untracked file caused an error: %v", err)
	}
	if rec != (dpStatsOutput{}) {
		t.Fatalf("untracked file was tallied: %+v", rec)
	}
}

func TestDPCountLinesCountsOpenErrorAndScannerError(t *testing.T) {
	t.Run("counts lines of a real file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "f.txt")
		if err := os.WriteFile(path, []byte("l1\nl2\nl3\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		n, err := dpCountLines(path)
		if err != nil {
			t.Fatalf("dpCountLines: %v", err)
		}
		if n != 3 {
			t.Fatalf("lines = %d, want 3", n)
		}
	})

	t.Run("open error is path-qualified", func(t *testing.T) {
		_, err := dpCountLines(filepath.Join(t.TempDir(), "missing.txt"))
		if err == nil || !strings.Contains(err.Error(), "open ") {
			t.Fatalf("err = %v, want path-qualified open error", err)
		}
	})

	t.Run("scanner error surfaces", func(t *testing.T) {
		// A single token longer than bufio.MaxScanTokenSize with no newline
		// makes the scanner fail with bufio.ErrTooLong.
		path := filepath.Join(t.TempDir(), "toolong.txt")
		huge := strings.Repeat("x", bufio.MaxScanTokenSize+1)
		if err := os.WriteFile(path, []byte(huge), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := dpCountLines(path)
		if err == nil || !strings.Contains(err.Error(), "scan ") {
			t.Fatalf("err = %v, want path-qualified scan error", err)
		}
		if !errors.Is(err, bufio.ErrTooLong) {
			t.Fatalf("err = %v, want wrapped bufio.ErrTooLong", err)
		}
	})
}
