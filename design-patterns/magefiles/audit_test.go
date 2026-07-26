// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditDesignPatternsChecksEvidenceBeforeBuild(t *testing.T) {
	var calls []string

	err := auditDesignPatterns(
		func() error {
			calls = append(calls, "evidence")
			return nil
		},
		func() error {
			calls = append(calls, "build")
			return nil
		},
	)

	if err != nil {
		t.Fatalf("auditDesignPatterns returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "evidence,build" {
		t.Fatalf("call order = %q, want evidence,build", got)
	}
}

func TestAuditDesignPatternsStopsOnEvidenceError(t *testing.T) {
	want := errors.New("evidence failed")
	buildCalled := false

	err := auditDesignPatterns(
		func() error { return want },
		func() error {
			buildCalled = true
			return nil
		},
	)

	if !errors.Is(err, want) {
		t.Fatalf("auditDesignPatterns error = %v, want %v", err, want)
	}
	if buildCalled {
		t.Fatal("build ran after evidence failure")
	}
}

func TestAuditDesignPatternsReturnsBuildError(t *testing.T) {
	want := errors.New("build failed")

	err := auditDesignPatterns(func() error { return nil }, func() error { return want })

	if !errors.Is(err, want) {
		t.Fatalf("auditDesignPatterns error = %v, want %v", err, want)
	}
}

func TestReferenceImplementationEvidenceAcceptsShippedChecks(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "agent", "machine.yaml"),
		"name: agent\nstates: [Idle, Done]\ntransitions: []\n")
	language := writePatternLanguage(t, root, `
patterns:
  - id: machine-interpreter
    examples:
      - system: Reference implementation
        cite: reference-implementation
        kind: internal
        note: The shipped machine declares Idle.
        evidence:
          classification: shipped
          checks:
            - path: agent/machine.yaml
              artifact: machine
              assertion: yaml_fields
              fields: [states, transitions]
`)

	if err := auditReferenceImplementationEvidence(language, root); err != nil {
		t.Fatalf("auditReferenceImplementationEvidence: %v", err)
	}
}

func TestReferenceImplementationEvidenceRejectsMissingEvidence(t *testing.T) {
	root := t.TempDir()
	language := writePatternLanguage(t, root, `
patterns:
  - id: machine-interpreter
    examples:
      - system: Unsupported claim
        cite: reference-implementation
        kind: internal
        note: Claims shipped behavior without evidence.
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), "evidence is required") {
		t.Fatalf("error = %v, want missing evidence finding", err)
	}
}

func TestReferenceImplementationEvidenceAcceptsLabeledDesignIntent(t *testing.T) {
	root := t.TempDir()
	language := writePatternLanguage(t, root, `
patterns:
  - id: approval-gate
    examples:
      - system: Design intent
        cite: reference-implementation
        kind: internal
        note: Design intent, not shipped behavior.
        evidence:
          classification: design_intent
`)

	if err := auditReferenceImplementationEvidence(language, root); err != nil {
		t.Fatalf("auditReferenceImplementationEvidence: %v", err)
	}
}

func TestReferenceImplementationEvidenceRejectsUnlabeledFixture(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "fixtures", "machine.yaml"),
		"name: approval\nstates: [Waiting, Done]\ntransitions:\n  - {state: Waiting, signal: Approved, next: Done}\n")
	language := writePatternLanguage(t, root, `
patterns:
  - id: approval-gate
    examples:
      - system: Approval example
        cite: reference-implementation
        kind: internal
        note: An approval profile exists.
        evidence:
          classification: conformance_fixture
          checks:
            - path: fixtures/machine.yaml
              artifact: machine
              assertion: yaml_transition
              match: {state: Waiting, signal: Approved, next: Done}
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), `must say "conformance fixture"`) {
		t.Fatalf("error = %v, want fixture-label finding", err)
	}
}

func TestReferenceImplementationEvidenceRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	language := writePatternLanguage(t, root, `
patterns:
  - id: machine-interpreter
    examples:
      - system: Escaping evidence
        cite: reference-implementation
        kind: internal
        note: A shipped machine exists.
        evidence:
          classification: shipped
          checks:
            - path: ../outside.yaml
              artifact: machine
              assertion: yaml_fields
              fields: [states]
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("error = %v, want path escape finding", err)
	}
}

func TestReferenceEvidenceRejectsExistingFileOfWrongArtifactType(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "profile.yaml"),
		"name: agent\nmachine: machine.yaml\n")
	check := evidenceCheck{
		Path: "profile.yaml", Artifact: "machine",
		Assertion: "yaml_fields", Fields: []string{"name"},
	}
	err := runEvidenceCheck(root, "wrong type", check)
	if err == nil || !strings.Contains(err.Error(), "requires field \"states\"") {
		t.Fatalf("error = %v, want machine artifact rejection", err)
	}
}

func TestReferenceEvidenceRejectsUnrelatedTokenOutsideGoTestDeclaration(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "claim_test.go"), `package fixture
// TestClaimedBehavior is mentioned but not implemented.
func TestSomethingElse() {}
`)
	check := evidenceCheck{
		Path: "claim_test.go", Artifact: "go_test",
		Assertion: "go_test", Test: "TestClaimedBehavior",
	}
	err := runEvidenceCheck(root, "unrelated token", check)
	if err == nil || !strings.Contains(err.Error(), "is not declared") {
		t.Fatalf("error = %v, want AST test-declaration rejection", err)
	}
}

func TestReferenceEvidenceExecutesFocusedBehaviorTest(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "go.mod"), "module fixture\n\ngo 1.26\n")
	writeAuditFixture(t, filepath.Join(root, "claim_test.go"), `package fixture
import "testing"
func TestClaimedBehavior(t *testing.T) { t.Fatal("behavior regressed") }
`)
	check := evidenceCheck{
		Path: "claim_test.go", Artifact: "go_test",
		Assertion: "go_test", Test: "TestClaimedBehavior",
	}
	err := runEvidenceCheck(root, "failing behavior", check)
	if err == nil || !strings.Contains(err.Error(), "focused Go test") ||
		!strings.Contains(err.Error(), "behavior regressed") {
		t.Fatalf("error = %v, want executed behavior failure", err)
	}
}

func TestReferenceEvidenceRejectsBehaviorallyFalseYAMLRelationship(t *testing.T) {
	root := t.TempDir()
	writeAuditFixture(t, filepath.Join(root, "a.yaml"),
		"name: a\nmachine: machine-a.yaml\ntools: [tools.yaml]\ntool_declarations: [llm/a.yaml]\n")
	writeAuditFixture(t, filepath.Join(root, "b.yaml"),
		"name: b\nmachine: machine-b.yaml\ntools: [tools.yaml]\ntool_declarations: [llm/b.yaml]\n")
	check := evidenceCheck{
		Paths: []string{"a.yaml", "b.yaml"}, Artifact: "profile",
		Assertion: "yaml_relation", SameFields: []string{"machine", "tools"},
		DifferentFields: []string{"tool_declarations"},
	}
	err := runEvidenceCheck(root, "false relationship", check)
	if err == nil || !strings.Contains(err.Error(), "machine") {
		t.Fatalf("error = %v, want false same-field rejection", err)
	}
}

func TestRepositoryReferenceImplementationEvidence(t *testing.T) {
	if err := auditReferenceImplementationEvidence("../pattern-language.yaml", "../.."); err != nil {
		t.Fatalf("repository evidence audit: %v", err)
	}
}

func writePatternLanguage(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "pattern-language.yaml")
	writeAuditFixture(t, path, content)
	return path
}

func writeAuditFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
