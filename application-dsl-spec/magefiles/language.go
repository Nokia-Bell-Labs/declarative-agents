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

// statementIDPattern is the shape of a stable statement identifier. Identifiers
// are never renumbered or reused: constitutions, tests, and commits cite them.
var statementIDPattern = regexp.MustCompile(`^R-[A-Z][A-Z0-9]*-[0-9]{3}$`)

// bareNounWords are the nouns chapter 02 bans from normative text: a
// statement names the class or the instance (agent-profile, agent-instance),
// never the bare noun. Compound terms like machine-template pass because the
// scan matches whole hyphenated tokens.
var bareNounWords = map[string]bool{
	"application": true, "applications": true,
	"agent": true, "agents": true,
	"machine": true, "machines": true,
}

var wordPattern = regexp.MustCompile(`[A-Za-z][A-Za-z-]*`)

// backtickSpanPattern removes keyname mentions before the bare-noun scan: a
// statement about the `application` keyname names a keyname, not the concept.
var backtickSpanPattern = regexp.MustCompile("`[^`]*`")

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
	// Status is empty for a statement in force and "retired" for one the
	// language no longer makes; Retired then names the release and the
	// statements that replace it.
	Status  string `yaml:"status"`
	Retired string `yaml:"retired"`
}

// statusRetired marks a statement the language no longer makes. It keeps its
// id and text, and its acceptance stays as the record of what was checked.
const statusRetired = "retired"

func (s languageStatement) retired() bool { return s.Status == statusRetired }

// hasPending reports whether the statement records evidence it lacks.
func (s languageStatement) hasPending() bool {
	for _, entry := range s.Acceptance {
		if entry.Assertion == assertionPending {
			return true
		}
	}
	return false
}

// acceptanceEntry is one piece of evidence that a statement is checked. The
// assertion selects which fields apply: a fixture entry names a document under
// the version directory with the check applied and the expected verdict; a
// go_test or rig entry names a test function that must exist in the file at
// path; a pending entry names, under note, evidence the statement lacks.
type acceptanceEntry struct {
	Assertion string `yaml:"assertion"`
	Path      string `yaml:"path"`
	Check     string `yaml:"check"`
	Verdict   string `yaml:"verdict"`
	Test      string `yaml:"test"`
	Note      string `yaml:"note"`
}

// assertionPending records a known gap rather than a check.
const assertionPending = "pending"

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

// validateLanguage is the full-gate schema: the shared schema, the chapter 02
// terminology rule, and evidence on every entry — a version the engine
// implements records no pending gaps.
func validateLanguage(language languageFile) error {
	findings := []error{validateLanguageSchema(language)}
	for _, statement := range language.Statements {
		for _, word := range wordPattern.FindAllString(backtickSpanPattern.ReplaceAllString(statement.Statement, " "), -1) {
			if bareNounWords[strings.ToLower(word)] {
				findings = append(findings, fmt.Errorf(
					"%s: bare noun %q in normative text; use a class or instance term (chapter 02)",
					statement.ID, word))
			}
		}
		for index, entry := range statement.Acceptance {
			if entry.Assertion == assertionPending {
				findings = append(findings, fmt.Errorf(
					"%s acceptance[%d]: pending evidence in a version the engine implements", statement.ID, index))
			}
		}
	}
	return errors.Join(findings...)
}

// validateLanguageSchema checks the statement schema every version shares:
// stable well-formed identifiers, declared conformance targets, RFC 2119
// levels, normative text, a consistent retirement record, and at least one
// acceptance entry per statement with the fields its assertion requires.
func validateLanguageSchema(language languageFile) error {
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
		switch {
		case statement.Status != "" && statement.Status != statusRetired:
			findings = append(findings, fmt.Errorf("%s: unknown status %q", label, statement.Status))
		case statement.retired() && strings.TrimSpace(statement.Retired) == "":
			findings = append(findings, fmt.Errorf("%s: a retired statement must name its retirement under retired", label))
		case !statement.retired() && statement.Retired != "":
			findings = append(findings, fmt.Errorf("%s: retired note on a statement in force", label))
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
	if entry.Assertion == assertionPending {
		if strings.TrimSpace(entry.Note) == "" {
			return errors.New("pending entries require a note naming the gap")
		}
		return nil
	}
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
	case "go_test", "rig":
		if !strings.HasPrefix(entry.Test, "Test") {
			return fmt.Errorf("%s entries require a Test* function, got %q", entry.Assertion, entry.Test)
		}
	default:
		return fmt.Errorf("unknown acceptance assertion %q", entry.Assertion)
	}
	return nil
}
