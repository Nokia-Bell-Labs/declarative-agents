// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func validLanguage() languageFile {
	return languageFile{
		Language: languageMeta{
			ID:                 "application-dsl",
			ConformanceTargets: []string{"document", "processor", "runtime", "population"},
		},
		Statements: []languageStatement{{
			ID:        "R-INTRO-001",
			Target:    "document",
			Level:     "MUST",
			Statement: "A conforming document MUST be a mapping.",
			Acceptance: []acceptanceEntry{{
				Assertion: "fixture",
				Path:      "fixtures/document/root-mapping/valid.yaml",
				Check:     "yaml_mapping",
				Verdict:   "valid",
			}},
		}},
	}
}

func TestValidateLanguageAcceptsWellFormedLanguage(t *testing.T) {
	if err := validateLanguage(validLanguage()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateLanguageRejectsSchemaViolations(t *testing.T) {
	tests := map[string]struct {
		mutate func(*languageFile)
		want   string
	}{
		"malformed id": {
			mutate: func(l *languageFile) { l.Statements[0].ID = "PROF-1" },
			want:   "id must match",
		},
		"duplicate id": {
			mutate: func(l *languageFile) {
				l.Statements = append(l.Statements, l.Statements[0])
			},
			want: "duplicate statement id",
		},
		"unknown target": {
			mutate: func(l *languageFile) { l.Statements[0].Target = "compiler" },
			want:   `unknown conformance target "compiler"`,
		},
		"unknown level": {
			mutate: func(l *languageFile) { l.Statements[0].Level = "SHALL" },
			want:   `unknown RFC 2119 level "SHALL"`,
		},
		"empty statement text": {
			mutate: func(l *languageFile) { l.Statements[0].Statement = "  " },
			want:   "statement text is required",
		},
		"missing acceptance": {
			mutate: func(l *languageFile) { l.Statements[0].Acceptance = nil },
			want:   "at least one acceptance entry is required",
		},
		"fixture without verdict": {
			mutate: func(l *languageFile) { l.Statements[0].Acceptance[0].Verdict = "" },
			want:   "fixture verdict must be valid or invalid",
		},
		"fixture without check": {
			mutate: func(l *languageFile) { l.Statements[0].Acceptance[0].Check = "" },
			want:   "fixture entries require a check",
		},
		"go_test without test function": {
			mutate: func(l *languageFile) {
				l.Statements[0].Acceptance[0] = acceptanceEntry{
					Assertion: "go_test", Path: "magefiles/x_test.go",
				}
			},
			want: "require a Test* function",
		},
		"unknown assertion": {
			mutate: func(l *languageFile) { l.Statements[0].Acceptance[0].Assertion = "manual" },
			want:   `unknown acceptance assertion "manual"`,
		},
		"no conformance targets": {
			mutate: func(l *languageFile) { l.Language.ConformanceTargets = nil },
			want:   "no conformance targets",
		},
		"no statements": {
			mutate: func(l *languageFile) { l.Statements = nil },
			want:   "no statements",
		},
		"bare noun in normative text": {
			mutate: func(l *languageFile) {
				l.Statements[0].Statement = "An agent MUST bind one workspace."
			},
			want: `bare noun "agent"`,
		},
		"bare plural noun in normative text": {
			mutate: func(l *languageFile) {
				l.Statements[0].Statement = "All machines MUST declare states."
			},
			want: `bare noun "machines"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			language := validLanguage()
			test.mutate(&language)
			err := validateLanguage(language)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateLanguage error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestBareNounBanPermitsCompoundTerms proves the chapter 02 vocabulary passes
// the scan: hyphenated class, instance, and unit terms are not bare nouns.
func TestBareNounBanPermitsCompoundTerms(t *testing.T) {
	language := validLanguage()
	language.Statements[0].Statement = "An agent-instance MUST bind exactly one workspace; " +
		"a machine-profile document MAY expand a machine-template."
	if err := validateLanguage(language); err != nil {
		t.Fatal(err)
	}
}

// TestBareNounBanPermitsBacktickedKeynames proves a statement about the
// `application` or `machine` keyname is a keyname mention, not a bare noun.
func TestBareNounBanPermitsBacktickedKeynames(t *testing.T) {
	language := validLanguage()
	language.Statements[0].Statement = "An application-profile document MUST carry the " +
		"`application` and `machine` keynames."
	if err := validateLanguage(language); err != nil {
		t.Fatal(err)
	}
}

// TestRepositoryLanguageFileIsValid gates the tracked language.yaml itself, so
// a schema break fails go test as well as mage audit.
func TestRepositoryLanguageFileIsValid(t *testing.T) {
	language, err := loadLanguage(filepath.Join("..", languagePath))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLanguage(language); err != nil {
		t.Fatal(err)
	}
}
