// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpecCollectStatsBucketsAndSkips(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"01-introduction.md":            "a\nb\n",
		"language.yaml":                 "language: {}\n",
		"fixtures/document/valid.yaml":  "name: x\n",
		"magefiles/skipped.go":          "package main\n",
		"generated-files/01-chapter.md": "rendered\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec, err := specCollectStats(root)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Markdown.Files != 1 || rec.Markdown.Lines != 2 {
		t.Errorf("markdown = %+v, want 1 file, 2 lines", rec.Markdown)
	}
	if rec.YAML.Files != 1 || rec.YAML.Lines != 1 {
		t.Errorf("yaml = %+v, want 1 file, 1 line", rec.YAML)
	}
	if rec.Fixtures.Files != 1 || rec.Fixtures.Lines != 1 {
		t.Errorf("fixtures = %+v, want 1 file, 1 line", rec.Fixtures)
	}
}
