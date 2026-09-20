// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEvidenceFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFixtureCheckRejectsNonMappingRoot is the acceptance evidence for
// R-INTRO-002: the processor obligation to reject a document whose root node
// is not a mapping, exercised through the fixture check that enforces it.
func TestFixtureCheckRejectsNonMappingRoot(t *testing.T) {
	path := writeEvidenceFile(t, t.TempDir(), "sequence-root.yaml", "- a\n- b\n")
	if err := checkFixture("yaml_mapping", path); err == nil {
		t.Fatal("yaml_mapping check passed a sequence-root document")
	}
	empty := writeEvidenceFile(t, t.TempDir(), "empty.yaml", "")
	if err := checkFixture("yaml_mapping", empty); err == nil {
		t.Fatal("yaml_mapping check passed an empty document")
	}
	mapping := writeEvidenceFile(t, t.TempDir(), "mapping.yaml", "name: x\n")
	if err := checkFixture("yaml_mapping", mapping); err != nil {
		t.Fatalf("yaml_mapping check rejected a mapping-root document: %v", err)
	}
}

func TestFixtureVerdictMismatchFails(t *testing.T) {
	dir := t.TempDir()
	writeEvidenceFile(t, dir, "mapping.yaml", "name: x\n")
	entry := acceptanceEntry{
		Assertion: "fixture",
		Path:      "mapping.yaml",
		Check:     "yaml_mapping",
		Verdict:   "invalid",
	}
	err := runAcceptanceEntry(dir, entry)
	if err == nil || !strings.Contains(err.Error(), "declared invalid but passed") {
		t.Fatalf("verdict mismatch error = %v, want declared invalid but passed", err)
	}
}

func TestFixtureUnknownCheckFails(t *testing.T) {
	dir := t.TempDir()
	writeEvidenceFile(t, dir, "mapping.yaml", "name: x\n")
	entry := acceptanceEntry{
		Assertion: "fixture",
		Path:      "mapping.yaml",
		Check:     "schema_v9",
		Verdict:   "invalid",
	}
	// An unknown check reads as a failing check, which an invalid verdict would
	// wrongly convert into passing evidence; the audit must not accept it.
	if err := runAcceptanceEntry(dir, entry); err == nil {
		t.Fatal("unknown fixture check produced passing evidence")
	}
}

func TestEvidencePathMayNotEscapeModule(t *testing.T) {
	entry := acceptanceEntry{
		Assertion: "fixture",
		Path:      "../secrets.yaml",
		Check:     "yaml_mapping",
		Verdict:   "valid",
	}
	err := runAcceptanceEntry(t.TempDir(), entry)
	if err == nil || !strings.Contains(err.Error(), "escapes the module") {
		t.Fatalf("escape error = %v, want escapes the module", err)
	}
}

func TestGoTestEvidenceRequiresDeclaredTest(t *testing.T) {
	dir := t.TempDir()
	path := writeEvidenceFile(t, dir, "sample_test.go",
		"package sample\n\nimport \"testing\"\n\nfunc TestReal(t *testing.T) {}\n")
	err := runGoTestEvidence(path, "TestMissing")
	if err == nil || !strings.Contains(err.Error(), "is not declared") {
		t.Fatalf("undeclared test error = %v, want is not declared", err)
	}
}

// TestRepositoryFixtureEvidencePasses runs every fixture acceptance entry of
// the tracked language.yaml, so a fixture regression fails go test as well as
// mage audit. go_test entries are exercised by the audit target itself; a
// test that re-entered go test here would recurse.
func TestRepositoryFixtureEvidencePasses(t *testing.T) {
	language, err := loadLanguage(filepath.Join("..", languagePath))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range language.Statements {
		for _, entry := range statement.Acceptance {
			if entry.Assertion != "fixture" {
				continue
			}
			if err := runAcceptanceEntry("..", entry); err != nil {
				t.Errorf("%s: %v", statement.ID, err)
			}
		}
	}
}

func TestUnknownFixtureCheckDoesNotClaimNonMappingRejection(t *testing.T) {
	// checkFixture must fail on unknown kinds rather than fall through to any
	// default behaviour.
	path := writeEvidenceFile(t, t.TempDir(), "mapping.yaml", "name: x\n")
	err := checkFixture("nonexistent", path)
	if err == nil || !strings.Contains(err.Error(), "unknown fixture check") {
		t.Fatalf("unknown check error = %v, want unknown fixture check", err)
	}
}
