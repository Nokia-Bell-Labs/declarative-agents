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

// flatRoots resolves every kind of evidence against one directory, for tests
// that exercise a single entry in isolation.
func flatRoots(dir string) evidenceRoots {
	return evidenceRoots{Version: dir, Module: dir, Repository: dir}
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
	err := runAcceptanceEntry(flatRoots(dir), entry)
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
	if err := runAcceptanceEntry(flatRoots(dir), entry); err == nil {
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
	err := runAcceptanceEntry(flatRoots(t.TempDir()), entry)
	if err == nil || !strings.Contains(err.Error(), "escapes the module") {
		t.Fatalf("escape error = %v, want escapes the module", err)
	}
}

func TestRigEvidenceResolvesDeclaredRepositoryTest(t *testing.T) {
	repo := t.TempDir()
	module := filepath.Join(repo, "application-dsl-spec")
	writeEvidenceFile(t, repo, "applications/x/magefiles/rig_test.go",
		"package main\n\nimport \"testing\"\n\nfunc TestRigProves(t *testing.T) {}\n")
	entry := acceptanceEntry{
		Assertion: "rig",
		Path:      "applications/x/magefiles/rig_test.go",
		Test:      "TestRigProves",
	}
	if err := runAcceptanceEntry(rootsFor(module, engineVersion), entry); err != nil {
		t.Fatalf("rig reference failed: %v", err)
	}
	entry.Test = "TestRenamed"
	if err := runAcceptanceEntry(rootsFor(module, engineVersion), entry); err == nil ||
		!strings.Contains(err.Error(), "is not declared") {
		t.Fatalf("renamed rig test error = %v, want is not declared", err)
	}
	entry.Path = "applications/x/magefiles/missing_test.go"
	if err := runAcceptanceEntry(rootsFor(module, engineVersion), entry); err == nil {
		t.Fatal("missing rig file produced passing evidence")
	}
}

func TestRigEvidenceMayNotEscapeRepository(t *testing.T) {
	repo := t.TempDir()
	module := filepath.Join(repo, "application-dsl-spec")
	entry := acceptanceEntry{
		Assertion: "rig",
		Path:      "../outside_test.go",
		Test:      "TestOutside",
	}
	err := runAcceptanceEntry(rootsFor(module, engineVersion), entry)
	if err == nil || !strings.Contains(err.Error(), "escapes the repository") {
		t.Fatalf("escape error = %v, want escapes the repository", err)
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
	language, err := loadLanguage(engineVersion.languagePath(".."))
	if err != nil {
		t.Fatal(err)
	}
	roots := rootsFor("..", engineVersion)
	for _, statement := range language.Statements {
		for _, entry := range statement.Acceptance {
			if entry.Assertion != "fixture" {
				continue
			}
			if err := runAcceptanceEntry(roots, entry); err != nil {
				t.Errorf("%s: %v", statement.ID, err)
			}
		}
	}
}

func TestMachineExpansionShape(t *testing.T) {
	tests := map[string]struct {
		content string
		valid   bool
	}{
		"body with states and transitions": {
			content: "states: [{name: A}]\ntransitions: [{state: A, signal: S, next: A}]\n",
			valid:   true,
		},
		"body with spliced stage-fragments": {
			content: "states: [{name: A}]\ntransitions: [{state: A, signal: S, next: A}]\n" +
				"expand:\n  - fragment: u.yaml\n  - fragment: v.yaml\n",
			valid: true,
		},
		"single template expansion without body": {
			content: "unit: t\nexpand:\n  - fragment: u.yaml\n",
			valid:   true,
		},
		"states without transitions": {
			content: "states: [{name: A}]\n",
			valid:   false,
		},
		"no body and no expansion": {
			content: "name: hollow\n",
			valid:   false,
		},
		"no body and two expansions": {
			content: "expand:\n  - fragment: u.yaml\n  - fragment: v.yaml\n",
			valid:   false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeEvidenceFile(t, t.TempDir(), "machine.yaml", test.content)
			err := checkFixture("machine_expansion_shape", path)
			if test.valid && err != nil {
				t.Fatalf("check failed on a conforming document: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("check passed a non-conforming document")
			}
		})
	}
}

func TestProfileAdmitsNoExpansion(t *testing.T) {
	tests := map[string]struct {
		content string
		valid   bool
	}{
		"profile written out in full": {
			content: "name: worker\nmachine: machine.yaml\n",
			valid:   true,
		},
		"profile expanding a blueprint": {
			content: "name: chatbot\nexpand:\n  - fragment: b.yaml\n    args: {server: control}\n",
			valid:   false,
		},
	}
	runShapeCases(t, "profile_admits_no_expansion", tests)
}

func TestProfileCarriesNoBlueprintKeynames(t *testing.T) {
	tests := map[string]struct {
		content string
		valid   bool
	}{
		"agent-profile keynames only": {
			content: "name: worker\nmachine: machine.yaml\ntools: [tools.yaml]\n",
			valid:   true,
		},
		"unit":    {content: "unit: b\nmachine: machine.yaml\n", valid: false},
		"params":  {content: "params:\n  - {name: server, type: string}\nmachine: m.yaml\n", valid: false},
		"profile": {content: "profile:\n  machine: machine.yaml\n", valid: false},
	}
	runShapeCases(t, "profile_carries_no_blueprint_keynames", tests)
}

func TestNameIsOptionalButTyped(t *testing.T) {
	tests := map[string]struct {
		check, content string
		valid          bool
	}{
		"nameless profile":       {"profile_required_keynames", "machine: machine.yaml\n", true},
		"profile with list name": {"profile_required_keynames", "name: [a]\nmachine: machine.yaml\n", false},
		"nameless machine body":  {"machine_body_keynames", "states: [A]\ntransitions: []\n", true},
		"machine with map name":  {"machine_body_keynames", "name: {a: b}\nstates: [A]\ntransitions: []\n", false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeEvidenceFile(t, t.TempDir(), "doc.yaml", test.content)
			err := checkFixture(test.check, path)
			if test.valid != (err == nil) {
				t.Fatalf("check %s on %q: err = %v, want valid=%v", test.check, test.content, err, test.valid)
			}
		})
	}
}

// runShapeCases applies one fixture check to each case and compares the
// outcome with the declared validity.
func runShapeCases(t *testing.T, check string, tests map[string]struct {
	content string
	valid   bool
}) {
	t.Helper()
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeEvidenceFile(t, t.TempDir(), "doc.yaml", test.content)
			err := checkFixture(check, path)
			if test.valid && err != nil {
				t.Fatalf("check failed on a conforming document: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("check passed a non-conforming document")
			}
		})
	}
}

func TestMachineInstanceHeader(t *testing.T) {
	tests := map[string]struct {
		content string
		valid   bool
	}{
		"header and one expansion": {
			content: "unit: i\nname: control\npurpose: Serve.\nexpand:\n  - fragment: t.yaml\n    args: {word: go}\n",
			valid:   true,
		},
		"a declared body is a splice, not an application": {
			content: "states: [{name: A}]\ntransitions: [{state: A, signal: S, next: A}]\n" +
				"expand:\n  - fragment: s.yaml\n",
			valid: true,
		},
		"no expansion at all": {
			content: "name: worker\nstates: [{name: A}]\n",
			valid:   true,
		},
		"keyname standing beside the expansion": {
			content: "unit: i\nsignals: [Seed]\nexpand:\n  - fragment: t.yaml\n",
			valid:   false,
		},
		"entry prefixes what it produces": {
			content: "unit: i\nexpand:\n  - fragment: t.yaml\n    as: x\n",
			valid:   false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeEvidenceFile(t, t.TempDir(), "machine.yaml", test.content)
			err := checkFixture("machine_instance_header", path)
			if test.valid && err != nil {
				t.Fatalf("check failed on a conforming document: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("check passed a non-conforming document")
			}
		})
	}
}

func TestExpansionNamesFragment(t *testing.T) {
	named := writeEvidenceFile(t, t.TempDir(), "named.yaml",
		"expand:\n  - fragment: u.yaml\n    args: {p: 1}\n")
	if err := checkFixture("expansion_names_fragment", named); err != nil {
		t.Fatalf("check failed on a named fragment: %v", err)
	}
	unnamed := writeEvidenceFile(t, t.TempDir(), "unnamed.yaml",
		"expand:\n  - args: {p: 1}\n")
	if err := checkFixture("expansion_names_fragment", unnamed); err == nil {
		t.Fatal("check passed an expansion entry without a fragment name")
	}
	vacuous := writeEvidenceFile(t, t.TempDir(), "vacuous.yaml", "name: x\n")
	if err := checkFixture("expansion_names_fragment", vacuous); err != nil {
		t.Fatalf("check failed on a document without expansions: %v", err)
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
