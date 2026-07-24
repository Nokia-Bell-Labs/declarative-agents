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
	writeAuditFixture(t, filepath.Join(root, "agent", "machine.yaml"), "states: [Idle, Done]\n")
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
              contains: [Idle]
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
	writeAuditFixture(t, filepath.Join(root, "fixtures", "profile.yaml"), "name: approval\n")
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
            - path: fixtures/profile.yaml
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
`)

	err := auditReferenceImplementationEvidence(language, root)
	if err == nil || !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("error = %v, want path escape finding", err)
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
