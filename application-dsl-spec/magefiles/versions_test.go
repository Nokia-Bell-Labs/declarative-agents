// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func pendingEntry() acceptanceEntry {
	return acceptanceEntry{Assertion: assertionPending, Note: "No test anchors this statement."}
}

func TestSchemaAcceptsPendingWithNote(t *testing.T) {
	language := validLanguage()
	language.Statements[0].Acceptance = append(language.Statements[0].Acceptance, pendingEntry())
	if err := validateLanguageSchema(language); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRejectsPendingWithoutNote(t *testing.T) {
	language := validLanguage()
	language.Statements[0].Acceptance = []acceptanceEntry{{Assertion: assertionPending}}
	err := validateLanguageSchema(language)
	if err == nil || !strings.Contains(err.Error(), "require a note") {
		t.Fatalf("error = %v, want pending note required", err)
	}
}

// TestFullGateRejectsPending holds the engine's version to evidence on every
// entry: a pending gap is legal only where the engine does not implement the
// version.
func TestFullGateRejectsPending(t *testing.T) {
	language := validLanguage()
	language.Statements[0].Acceptance = append(language.Statements[0].Acceptance, pendingEntry())
	err := validateLanguage(language)
	if err == nil || !strings.Contains(err.Error(), "pending evidence in a version the engine implements") {
		t.Fatalf("error = %v, want pending rejected", err)
	}
}

// TestSchemaLeavesTerminologyToTheFullGate proves the chapter 02 bare-noun
// rule binds only the version whose chapters define it.
func TestSchemaLeavesTerminologyToTheFullGate(t *testing.T) {
	language := validLanguage()
	language.Statements[0].Statement = "An agent MUST bind one workspace."
	if err := validateLanguageSchema(language); err != nil {
		t.Fatalf("shared schema applied the terminology rule: %v", err)
	}
	if err := validateLanguage(language); err == nil {
		t.Fatal("full gate accepted a bare noun")
	}
}

func TestSchemaChecksRetirementRecord(t *testing.T) {
	tests := map[string]struct {
		status, retired, want string
	}{
		"retired without note": {status: statusRetired, want: "must name its retirement"},
		"note without status":  {retired: "Replaced by R-X-002.", want: "retired note on a statement in force"},
		"unknown status":       {status: "draft", want: `unknown status "draft"`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			language := validLanguage()
			language.Statements[0].Status = test.status
			language.Statements[0].Retired = test.retired
			err := validateLanguageSchema(language)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	language := validLanguage()
	language.Statements[0].Status = statusRetired
	language.Statements[0].Retired = "The 2.0 revision retired this rule."
	if err := validateLanguageSchema(language); err != nil {
		t.Fatalf("well-formed retirement rejected: %v", err)
	}
}

func TestFixtureCoverageExemptsRetiredAndPending(t *testing.T) {
	language := coverageLanguage()
	language.Statements[0].Acceptance = language.Statements[0].Acceptance[:1]
	if err := checkFixtureCoverage(language, "document"); err == nil {
		t.Fatal("one-verdict statement passed coverage")
	}
	pending := language
	pending.Statements = []languageStatement{language.Statements[0]}
	pending.Statements[0].Acceptance = append(pending.Statements[0].Acceptance, pendingEntry())
	if err := checkFixtureCoverage(pending, "document"); err != nil {
		t.Fatalf("statement recording its gap failed coverage: %v", err)
	}
	retired := coverageLanguage()
	retired.Statements[0].Acceptance = retired.Statements[0].Acceptance[:1]
	retired.Statements[0].Status = statusRetired
	if err := checkFixtureCoverage(retired, "document"); err != nil {
		t.Fatalf("retired statement failed coverage: %v", err)
	}
}

func TestFixtureOwnershipExemptsUnits(t *testing.T) {
	version := t.TempDir()
	writeEvidenceFile(t, version, "fixtures/units/template.yaml", "name: t\n")
	language := languageFile{}
	if err := checkFixtureOwnership(version, language); err != nil {
		t.Fatalf("unit fixture needed an owner: %v", err)
	}
	writeEvidenceFile(t, version, "fixtures/specification/stray.yaml", "name: s\n")
	if err := checkFixtureOwnership(version, language); err == nil {
		t.Fatal("unowned specification fixture passed ownership")
	}
}

func TestRetiredFixturePlacement(t *testing.T) {
	statement := languageStatement{
		ID: "R-X-001", Status: statusRetired, Retired: "Retired.",
		Acceptance: []acceptanceEntry{{Assertion: "fixture", Path: "fixtures/specification/x/valid.yaml"}},
	}
	err := checkRetiredFixturePlacement(languageFile{Statements: []languageStatement{statement}})
	if err == nil || !strings.Contains(err.Error(), "lies outside fixtures/retired/") {
		t.Fatalf("error = %v, want retired fixture outside retired suite", err)
	}
	statement.Status, statement.Retired = "", ""
	statement.Acceptance[0].Path = "fixtures/retired/x/valid.yaml"
	err = checkRetiredFixturePlacement(languageFile{Statements: []languageStatement{statement}})
	if err == nil || !strings.Contains(err.Error(), "draws on retired fixture") {
		t.Fatalf("error = %v, want in-force statement on retired fixture", err)
	}
}

func TestFixturesParseRejectsMalformedYAML(t *testing.T) {
	version := t.TempDir()
	writeEvidenceFile(t, version, "fixtures/specification/ok.yaml", "a: 1\n---\nb: 2\n")
	if err := checkFixturesParse(version); err != nil {
		t.Fatal(err)
	}
	writeEvidenceFile(t, version, "fixtures/specification/bad.yaml", "a: [1\n")
	if err := checkFixturesParse(version); err == nil {
		t.Fatal("malformed fixture parsed")
	}
}

func TestEvidenceReferencesResolveWithoutRunning(t *testing.T) {
	repo := t.TempDir()
	module := filepath.Join(repo, "application-dsl-spec")
	version := specVersion{Dir: "2.0", AuthoredTarget: "specification", Gate: gateReduced}
	roots := rootsFor(module, version)
	writeEvidenceFile(t, roots.Version, "fixtures/specification/x/valid.yaml", "a: 1\n")
	writeEvidenceFile(t, repo, "agent-core/x_test.go",
		"package x\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Fatal(\"never run\") }\n")
	language := languageFile{Statements: []languageStatement{{
		ID: "R-X-001",
		Acceptance: []acceptanceEntry{
			// The check name has no implementation; a reference check must
			// not try to execute it.
			{Assertion: "fixture", Path: "fixtures/specification/x/valid.yaml", Check: "unimplemented", Verdict: "valid"},
			{Assertion: "rig", Path: "agent-core/x_test.go", Test: "TestX"},
			pendingEntry(),
		},
	}}}
	if err := checkEvidenceReferences(roots, language); err != nil {
		t.Fatal(err)
	}
	language.Statements[0].Acceptance[1].Test = "TestGone"
	if err := checkEvidenceReferences(roots, language); err == nil {
		t.Fatal("rig reference to a missing test resolved")
	}
	language.Statements[0].Acceptance[1].Test = "TestX"
	language.Statements[0].Acceptance[0].Path = "fixtures/specification/x/missing.yaml"
	if err := checkEvidenceReferences(roots, language); err == nil {
		t.Fatal("missing fixture resolved")
	}
}

// TestRepositoryCurrentVersionPassesReducedGate gates the tracked 2.0
// directory, so a schema, ownership, or reference break fails go test as well
// as mage audit.
func TestRepositoryCurrentVersionPassesReducedGate(t *testing.T) {
	language, err := loadLanguage(currentVersion.languagePath(".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := auditReduced("..", currentVersion, language); err != nil {
		t.Fatal(err)
	}
}
