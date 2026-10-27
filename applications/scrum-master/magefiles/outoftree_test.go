// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The out-of-tree copy's module is renamed wholesale: go.mod, every Go
// file's imports, and their order as gofmt sorts it (GH-2754).
func TestRenameModuleRewritesEveryImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module github.com/org/repo/app\n\ngo 1.26\n")
	writeFile(t, filepath.Join(dir, "a", "a.go"), "package a\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/org/repo/app/b\"\n)\n\nvar _ = fmt.Sprint(b.X)\n")
	if err := renameModule(dir, "example.invalid"); err != nil {
		t.Fatal(err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	src, _ := os.ReadFile(filepath.Join(dir, "a", "a.go"))
	if !strings.HasPrefix(string(mod), "module example.invalid/app\n") || !strings.Contains(string(src), "\"example.invalid/app/b\"") || strings.Contains(string(src), "github.com/org/repo") {
		t.Fatalf("go.mod:\n%s\na.go:\n%s", mod, src)
	}
}
