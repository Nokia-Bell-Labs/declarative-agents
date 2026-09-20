// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKeynameSource(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenderKeynameTableFromYAMLSchema(t *testing.T) {
	repo := t.TempDir()
	writeKeynameSource(t, repo, "specs/format.yaml", `
schema:
  - field: name
    type: string
    required: true
  - field: purpose
    type: string
    required: false
`)
	table, err := renderKeynameTable(repo, "yaml", "specs/format.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| `name` | string | yes |",
		"| `purpose` | string | no |",
		"Generated at render time from",
	} {
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q:\n%s", want, table)
		}
	}
}

func TestRenderKeynameTableFromGoStruct(t *testing.T) {
	repo := t.TempDir()
	writeKeynameSource(t, repo, "pkg/manifest.go", `
package pkg

type Manifest struct {
	SchemaVersion int    `+"`yaml:\"schema_version\"`"+`
	Deployment    string `+"`yaml:\"deployment,omitempty\"`"+`
	Ignored       string
}
`)
	table, err := renderKeynameTable(repo, "go", "pkg/manifest.go", "Manifest")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| `schema_version` | int | yes |",
		"| `deployment` | string | no |",
	} {
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q:\n%s", want, table)
		}
	}
	if strings.Contains(table, "Ignored") {
		t.Errorf("table includes an untagged field:\n%s", table)
	}
}

func TestRenderKeynameTableFailures(t *testing.T) {
	repo := t.TempDir()
	writeKeynameSource(t, repo, "specs/empty.yaml", "schema: []\n")
	writeKeynameSource(t, repo, "pkg/manifest.go", "package pkg\n")
	tests := map[string]struct {
		kind, source, typeName, want string
	}{
		"escaping source":   {"yaml", "../outside.yaml", "", "escapes the repository"},
		"missing file":      {"yaml", "specs/missing.yaml", "", "no such file"},
		"empty schema":      {"yaml", "specs/empty.yaml", "", "yields no keynames"},
		"go without type":   {"go", "pkg/manifest.go", "", "requires #TypeName"},
		"undeclared struct": {"go", "pkg/manifest.go", "Ghost", "is not declared"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := renderKeynameTable(repo, test.kind, test.source, test.typeName)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRenderExpandsKeynameTablePlaceholder(t *testing.T) {
	repo := t.TempDir()
	source := filepath.Join(repo, "spec")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeKeynameSource(t, repo, "specs/format.yaml", `
schema:
  - field: name
    type: string
    required: true
`)
	writeChapter(t, source, "05-grammar.md",
		"{{statement R-INTRO-001}}\n\n{{keyname-table yaml:specs/format.yaml}}\n")
	output := t.TempDir()
	if err := renderChapters(source, output, renderLanguage()); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(filepath.Join(output, "05-grammar.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "| `name` | string | yes |") {
		t.Errorf("rendered chapter missing generated table:\n%s", rendered)
	}
}

func TestRenderRejectsUnresolvableKeynameTable(t *testing.T) {
	repo := t.TempDir()
	source := filepath.Join(repo, "spec")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeChapter(t, source, "05-grammar.md",
		"{{statement R-INTRO-001}}\n\n{{keyname-table yaml:specs/missing.yaml}}\n")
	err := renderChapters(source, t.TempDir(), renderLanguage())
	if err == nil || !strings.Contains(err.Error(), "keyname table specs/missing.yaml") {
		t.Fatalf("render error = %v, want unresolved keyname table", err)
	}
}
