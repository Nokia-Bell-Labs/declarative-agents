// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const renderOutputDir = "generated-files"

// citationPattern matches a statement citation placeholder. A citation is the
// statement's definition site: the render expands it in place with a stable
// anchor, and each statement has exactly one.
var citationPattern = regexp.MustCompile(`\{\{statement ([A-Z0-9-]+)\}\}`)

// All renders the chapters (default entry point, mirroring design-patterns).
func All() error {
	return Render()
}

// Build compiles the rendered specification artifacts.
func Build() error {
	return Render()
}

// Render expands every statement citation in every chapter and writes the
// result under generated-files/. It fails when a citation names no statement,
// when a statement is cited other than exactly once, or when no chapters
// exist — the same invariants Audit enforces, so prose and statements cannot
// drift apart.
func Render() error {
	language, err := loadLanguage(languagePath)
	if err != nil {
		return err
	}
	if err := validateLanguage(language); err != nil {
		return err
	}
	return renderChapters(".", renderOutputDir, language)
}

func renderChapters(sourceDir, outputDir string, language languageFile) error {
	chapters, err := discoverChapters(sourceDir)
	if err != nil {
		return err
	}
	if len(chapters) == 0 {
		return fmt.Errorf("no [0-9][0-9]-*.md chapter files found in %s", sourceDir)
	}

	statements := make(map[string]languageStatement, len(language.Statements))
	for _, statement := range language.Statements {
		statements[statement.ID] = statement
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	cited := map[string]int{}
	var findings []error
	for _, chapter := range chapters {
		source := filepath.Join(sourceDir, chapter)
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		rendered := citationPattern.ReplaceAllStringFunc(string(data), func(match string) string {
			id := citationPattern.FindStringSubmatch(match)[1]
			statement, ok := statements[id]
			if !ok {
				findings = append(findings, fmt.Errorf("%s: citation %s names no statement", chapter, id))
				return match
			}
			cited[id]++
			return renderStatement(statement)
		})
		target := filepath.Join(outputDir, chapter)
		if err := os.WriteFile(target, []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		fmt.Printf("rendered %s\n", target)
	}

	for _, statement := range language.Statements {
		switch cited[statement.ID] {
		case 0:
			findings = append(findings, fmt.Errorf("statement %s is not cited by any chapter", statement.ID))
		case 1:
		default:
			findings = append(findings, fmt.Errorf("statement %s is cited %d times, want exactly one definition site",
				statement.ID, cited[statement.ID]))
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("render failed: %w", errors.Join(findings...))
	}
	return nil
}

// renderStatement formats a statement as a blockquote under a stable HTML
// anchor, so a rendered page links to it as #<id> (chapter 08 of the epic:
// stable statement anchors on the published site).
func renderStatement(statement languageStatement) string {
	return fmt.Sprintf("<a id=%q></a>\n\n> **%s** (%s, %s) — %s",
		statement.ID, statement.ID, statement.Target, statement.Level,
		strings.TrimSpace(statement.Statement))
}

func discoverChapters(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var chapters []string
	for _, entry := range entries {
		name := entry.Name()
		if len(name) >= 3 &&
			name[0] >= '0' && name[0] <= '9' &&
			name[1] >= '0' && name[1] <= '9' &&
			name[2] == '-' && strings.HasSuffix(name, ".md") {
			chapters = append(chapters, name)
		}
	}
	sort.Strings(chapters)
	return chapters, nil
}
