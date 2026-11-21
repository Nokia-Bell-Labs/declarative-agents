// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
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

func TestConstitutionCitations(t *testing.T) {
	language := validLanguage()

	t.Run("missing directory passes vacuously", func(t *testing.T) {
		if err := checkConstitutionCitations(t.TempDir(), language); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("directory without constitutions fails", func(t *testing.T) {
		repo := t.TempDir()
		dir := filepath.Join(repo, filepath.FromSlash(constitutionsDir))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("R-INTRO-001\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkConstitutionCitations(repo, language); err == nil || !strings.Contains(err.Error(), "no constitution to check") {
			t.Fatalf("error = %v, want no constitution to check", err)
		}
	})
	t.Run("markdown constitution is read", func(t *testing.T) {
		repo := t.TempDir()
		dir := filepath.Join(repo, filepath.FromSlash(constitutionsDir))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nid: sample\ntitle: Sample\n---\n\n<!-- field: purpose -->\n## Purpose\n\nCites R-NOPE-999.\n"
		if err := os.WriteFile(filepath.Join(dir, "sample.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkConstitutionCitations(repo, language); err == nil || !strings.Contains(err.Error(), "R-NOPE-999") {
			t.Fatalf("error = %v, want the unresolved citation from the Markdown constitution", err)
		}
	})
	t.Run("resolvable citation passes", func(t *testing.T) {
		repo := t.TempDir()
		writeEvidenceFile(t, repo, "docs/constitutions/agent-core.md",
			"id: agent-core\nobligations:\n  - Upholds R-INTRO-001.\n")
		if err := checkConstitutionCitations(repo, language); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("citation-free constitution fails", func(t *testing.T) {
		repo := t.TempDir()
		writeEvidenceFile(t, repo, "docs/constitutions/empty.md", "id: empty\n")
		err := checkConstitutionCitations(repo, language)
		if err == nil || !strings.Contains(err.Error(), "cites no statement identifier") {
			t.Fatalf("citation error = %v, want cites no statement identifier", err)
		}
	})
	t.Run("unresolvable citation fails", func(t *testing.T) {
		repo := t.TempDir()
		writeEvidenceFile(t, repo, "docs/constitutions/ghost.md",
			"id: ghost\nobligations:\n  - Upholds R-GHOST-999.\n")
		err := checkConstitutionCitations(repo, language)
		if err == nil || !strings.Contains(err.Error(), "R-GHOST-999 resolves to no statement") {
			t.Fatalf("citation error = %v, want unresolvable R-GHOST-999", err)
		}
	})
}

// TestRepositoryConstitutionsCiteResolvableStatements gates the tracked
// constitutions against the tracked language.
func TestRepositoryConstitutionsCiteResolvableStatements(t *testing.T) {
	language, err := loadLanguage(engineVersion.languagePath(".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkConstitutionCitations(filepath.Join("..", ".."), language); err != nil {
		t.Fatal(err)
	}
}

// TestRepositoryFixtureSuitesAreCoveredAndOwned gates the tracked suites, so
// a suite regression fails go test as well as mage audit.
func TestRepositoryFixtureSuitesAreCoveredAndOwned(t *testing.T) {
	language, err := loadLanguage(engineVersion.languagePath(".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFixtureCoverage(language); err != nil {
		t.Error(err)
	}
	if err := checkFixtureOwnership(engineVersion.root(".."), language); err != nil {
		t.Error(err)
	}
}
