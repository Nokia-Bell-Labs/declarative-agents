// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const languagePath = "language.yaml"

// statementIDPattern is the shape of a stable statement identifier. Identifiers
// are never renumbered or reused: constitutions, tests, and commits cite them.
var statementIDPattern = regexp.MustCompile(`^R-[A-Z][A-Z0-9]*-[0-9]{3}$`)

// rfc2119Levels are the requirement levels a statement may carry.
var rfc2119Levels = map[string]bool{
	"MUST":       true,
	"MUST NOT":   true,
	"SHOULD":     true,
	"SHOULD NOT": true,
	"MAY":        true,
}

type languageFile struct {
	Language   languageMeta        `yaml:"language"`
	Statements []languageStatement `yaml:"statements"`
}

type languageMeta struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Version            string   `yaml:"version"`
	ConformanceTargets []string `yaml:"conformance_targets"`
}

type languageStatement struct {
	ID         string            `yaml:"id"`
	Target     string            `yaml:"target"`
	Level      string            `yaml:"level"`
	Statement  string            `yaml:"statement"`
	Acceptance []acceptanceEntry `yaml:"acceptance"`
}

// acceptanceEntry is one piece of evidence that a statement is checked. The
// assertion selects which fields apply: a fixture entry names a document under
// the module with the check applied and the expected verdict; a go_test entry
// names a test function that must exist and pass in the file at path.
type acceptanceEntry struct {
	Assertion string `yaml:"assertion"`
	Path      string `yaml:"path"`
	Check     string `yaml:"check"`
	Verdict   string `yaml:"verdict"`
	Test      string `yaml:"test"`
}

func loadLanguage(path string) (languageFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return languageFile{}, fmt.Errorf("read language: %w", err)
	}
	var language languageFile
	if err := yaml.Unmarshal(data, &language); err != nil {
		return languageFile{}, fmt.Errorf("parse language: %w", err)
	}
	return language, nil
}

// validateLanguage checks the statement schema: stable well-formed identifiers,
// declared conformance targets, RFC 2119 levels, normative text, and at least
// one acceptance entry per statement with the fields its assertion requires.
func validateLanguage(language languageFile) error {
	var findings []error
	targets := make(map[string]bool, len(language.Language.ConformanceTargets))
	for _, target := range language.Language.ConformanceTargets {
		if targets[target] {
			findings = append(findings, fmt.Errorf("duplicate conformance target %q", target))
		}
		targets[target] = true
	}
	if len(targets) == 0 {
		findings = append(findings, errors.New("language declares no conformance targets"))
	}
	if len(language.Statements) == 0 {
		findings = append(findings, errors.New("language has no statements"))
	}

	seen := make(map[string]bool, len(language.Statements))
	for _, statement := range language.Statements {
		label := statement.ID
		if label == "" {
			label = "(statement without id)"
		}
		switch {
		case !statementIDPattern.MatchString(statement.ID):
			findings = append(findings, fmt.Errorf("%s: id must match %s", label, statementIDPattern))
		case seen[statement.ID]:
			findings = append(findings, fmt.Errorf("%s: duplicate statement id", label))
		}
		seen[statement.ID] = true
		if !targets[statement.Target] {
			findings = append(findings, fmt.Errorf("%s: unknown conformance target %q", label, statement.Target))
		}
		if !rfc2119Levels[statement.Level] {
			findings = append(findings, fmt.Errorf("%s: unknown RFC 2119 level %q", label, statement.Level))
		}
		if strings.TrimSpace(statement.Statement) == "" {
			findings = append(findings, fmt.Errorf("%s: statement text is required", label))
		}
		if len(statement.Acceptance) == 0 {
			findings = append(findings, fmt.Errorf("%s: at least one acceptance entry is required", label))
		}
		for index, entry := range statement.Acceptance {
			if err := validateAcceptanceEntry(entry); err != nil {
				findings = append(findings, fmt.Errorf("%s acceptance[%d]: %w", label, index, err))
			}
		}
	}
	return errors.Join(findings...)
}

func validateAcceptanceEntry(entry acceptanceEntry) error {
	if strings.TrimSpace(entry.Path) == "" {
		return errors.New("path is required")
	}
	switch entry.Assertion {
	case "fixture":
		if entry.Check == "" {
			return errors.New("fixture entries require a check")
		}
		if entry.Verdict != "valid" && entry.Verdict != "invalid" {
			return fmt.Errorf("fixture verdict must be valid or invalid, got %q", entry.Verdict)
		}
	case "go_test":
		if !strings.HasPrefix(entry.Test, "Test") {
			return fmt.Errorf("go_test entries require a Test* function, got %q", entry.Test)
		}
	default:
		return fmt.Errorf("unknown acceptance assertion %q", entry.Assertion)
	}
	return nil
}
