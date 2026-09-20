// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderLanguage() languageFile {
	language := validLanguage()
	language.Statements[0].Statement = "A conforming document MUST be a mapping."
	return language
}

func writeChapter(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenderExpandsCitationsWithStableAnchors(t *testing.T) {
	source := t.TempDir()
	output := t.TempDir()
	writeChapter(t, source, "01-introduction.md", "# Intro\n\n{{statement R-INTRO-001}}\n")

	if err := renderChapters(source, output, renderLanguage()); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(filepath.Join(output, "01-introduction.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<a id="R-INTRO-001"></a>`,
		"**R-INTRO-001** (document, MUST)",
		"A conforming document MUST be a mapping.",
	} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("rendered chapter missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderRejectsUnresolvedCitation(t *testing.T) {
	source := t.TempDir()
	writeChapter(t, source, "01-introduction.md",
		"{{statement R-INTRO-001}}\n{{statement R-GHOST-999}}\n")
	err := renderChapters(source, t.TempDir(), renderLanguage())
	if err == nil || !strings.Contains(err.Error(), "R-GHOST-999 names no statement") {
		t.Fatalf("render error = %v, want unresolved R-GHOST-999", err)
	}
}

func TestRenderRejectsUncitedStatement(t *testing.T) {
	source := t.TempDir()
	writeChapter(t, source, "01-introduction.md", "# Intro without citations\n")
	err := renderChapters(source, t.TempDir(), renderLanguage())
	if err == nil || !strings.Contains(err.Error(), "R-INTRO-001 is not cited") {
		t.Fatalf("render error = %v, want uncited R-INTRO-001", err)
	}
}

func TestRenderRejectsDuplicateDefinitionSites(t *testing.T) {
	source := t.TempDir()
	writeChapter(t, source, "01-introduction.md", "{{statement R-INTRO-001}}\n")
	writeChapter(t, source, "02-terminology.md", "{{statement R-INTRO-001}}\n")
	err := renderChapters(source, t.TempDir(), renderLanguage())
	if err == nil || !strings.Contains(err.Error(), "cited 2 times") {
		t.Fatalf("render error = %v, want duplicate citation", err)
	}
}

func TestRenderRequiresChapters(t *testing.T) {
	err := renderChapters(t.TempDir(), t.TempDir(), renderLanguage())
	if err == nil || !strings.Contains(err.Error(), "no [0-9][0-9]-*.md chapter files") {
		t.Fatalf("render error = %v, want no chapters", err)
	}
}

// TestRepositoryChaptersRender proves the tracked chapters and language.yaml
// agree: every citation resolves and every statement has its definition site.
func TestRepositoryChaptersRender(t *testing.T) {
	language, err := loadLanguage(filepath.Join("..", languagePath))
	if err != nil {
		t.Fatal(err)
	}
	if err := renderChapters("..", t.TempDir(), language); err != nil {
		t.Fatal(err)
	}
}
