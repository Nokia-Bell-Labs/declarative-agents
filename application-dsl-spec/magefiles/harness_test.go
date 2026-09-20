// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func coverageLanguage() languageFile {
	language := validLanguage()
	language.Statements[0].Acceptance = []acceptanceEntry{
		{Assertion: "fixture", Path: "fixtures/document/suite/valid.yaml", Check: "yaml_mapping", Verdict: "valid"},
		{Assertion: "fixture", Path: "fixtures/document/suite/invalid.yaml", Check: "yaml_mapping", Verdict: "invalid"},
	}
	return language
}

func TestFixtureCoverageAcceptsBothVerdicts(t *testing.T) {
	if err := checkFixtureCoverage(coverageLanguage()); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureCoverageRequiresBothVerdicts(t *testing.T) {
	language := coverageLanguage()
	language.Statements[0].Acceptance = language.Statements[0].Acceptance[:1]
	err := checkFixtureCoverage(language)
	if err == nil || !strings.Contains(err.Error(), "no invalid-verdict fixture") {
		t.Fatalf("coverage error = %v, want missing invalid fixture", err)
	}
}

func TestFixtureCoverageIgnoresNonDocumentTargets(t *testing.T) {
	language := coverageLanguage()
	language.Statements[0].Target = "runtime"
	language.Statements[0].Acceptance = []acceptanceEntry{
		{Assertion: "rig", Path: "x_test.go", Test: "TestX"},
	}
	if err := checkFixtureCoverage(language); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureOwnershipReportsOrphansAndSharedFixtures(t *testing.T) {
	module := t.TempDir()
	for _, name := range []string{
		"fixtures/document/suite/valid.yaml",
		"fixtures/document/suite/invalid.yaml",
		"fixtures/document/suite/orphan.yaml",
	} {
		writeEvidenceFile(t, module, name, "name: x\n")
	}
	language := coverageLanguage()
	err := checkFixtureOwnership(module, language)
	if err == nil || !strings.Contains(err.Error(), "orphan.yaml is referenced by no statement") {
		t.Fatalf("ownership error = %v, want orphan report", err)
	}

	second := language.Statements[0]
	second.ID = "R-INTRO-002"
	language.Statements = append(language.Statements, second)
	err = checkFixtureOwnership(module, language)
	if err == nil || !strings.Contains(err.Error(), "owned by 2 statements") {
		t.Fatalf("ownership error = %v, want shared-fixture report", err)
	}
}

// TestRepositoryFixtureSuitesAreCoveredAndOwned gates the tracked suites, so
// a suite regression fails go test as well as mage audit.
func TestRepositoryFixtureSuitesAreCoveredAndOwned(t *testing.T) {
	language, err := loadLanguage(filepath.Join("..", languagePath))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFixtureCoverage(language); err != nil {
		t.Error(err)
	}
	if err := checkFixtureOwnership("..", language); err != nil {
		t.Error(err)
	}
}
