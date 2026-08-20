// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatternInvariantRepositoryConformance(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	var ran int
	err := checkPatternInvariants(root, func(_, _, _ string) error {
		ran++
		return nil
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if ran == 0 {
		t.Fatal("repository pattern language registered no executable checks")
	}
}

func TestEveryRegisteredPatternInvariantCheckGatesItsViolation(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	language, err := loadPatternInvariants(filepath.Join(root, patternLanguagePath))
	if err != nil {
		t.Fatal(err)
	}
	checks, _, err := validatePatternInvariants(language)
	if err != nil {
		t.Fatal(err)
	}
	for _, planted := range checks {
		t.Run(planted.invariantID, func(t *testing.T) {
			want := errors.New("planted invariant violation")
			run := func(_, command, _ string) error {
				if command == planted.command {
					return want
				}
				return nil
			}
			err := checkPatternInvariants(root, run, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), planted.invariantID) ||
				!errors.Is(err, want) {
				t.Fatalf("error = %v, want %s wrapping planted violation", err, planted.invariantID)
			}
		})
	}
}

func TestPatternInvariantRejectsMissingCheckBlock(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: machine-interpreter
    invariants:
      - id: P1.1
        statement: The machine is valid.
`)
	err := checkPatternInvariants(root, passingPatternCheck, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pattern invariant P1.1 has no check block") {
		t.Fatalf("error = %v, want invariant id and missing check block", err)
	}
}

func TestPatternInvariantRejectsExecutableWithoutNegativeTest(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: machine-interpreter
    invariants:
      - id: P1.1
        statement: The machine is valid.
        check:
          kind: executable
          command: go test ./machine -run TestMachineInvalid
`)
	err := checkPatternInvariants(root, passingPatternCheck, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pattern invariant P1.1 has no negative test") {
		t.Fatalf("error = %v, want invariant id and missing negative test", err)
	}
}

func TestPatternInvariantRejectsUnselectedNegativeTest(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: machine-interpreter
    invariants:
      - id: P1.1
        statement: The machine is valid.
        check:
          kind: executable
          command: go test ./machine -run TestOther
          negative_test: TestMachineInvalid
`)
	err := checkPatternInvariants(root, passingPatternCheck, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "command does not select negative test TestMachineInvalid") {
		t.Fatalf("error = %v, want unselected negative test", err)
	}
}

func TestPatternInvariantReportsManualCount(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: tool-contract
    invariants:
      - id: P3.1
        statement: Tool contracts are complete.
        check:
          kind: executable
          issue: GH-1786
          negative_test: TestFutureCheck
      - id: P3.2
        statement: Tools do not hide workflow.
        check:
          kind: manual
          reason: Atomicity requires semantic judgment.
`)
	var output bytes.Buffer
	if err := checkPatternInvariants(root, passingPatternCheck, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "manual-invariant count: 1") {
		t.Fatalf("output = %q, want manual count", output.String())
	}
	if !strings.Contains(output.String(), "2 total, 1 executable, 1 pending") {
		t.Fatalf("output = %q, want executable and pending counts", output.String())
	}
}

func TestPatternInvariantLoadsAddedCheckWithoutCodeChange(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, executableInvariantYAML("P1.1", "TestFirst"))
	var commands []string
	run := func(_, command, _ string) error {
		commands = append(commands, command)
		return nil
	}
	if err := checkPatternInvariants(root, run, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	writePatternInvariantFixture(t, root, executableInvariantYAML("P1.1", "TestFirst")+`
  - id: agent-as-data
    invariants:
      - id: P2.1
        statement: The profile closure resolves.
        check:
          kind: executable
          command: go test ./profile -run TestSecond
          negative_test: TestSecond
`)
	commands = nil
	if err := checkPatternInvariants(root, run, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("ran %d commands, want 2 after YAML-only addition", len(commands))
	}
}

func TestPatternInvariantFailsRegisteredCheck(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, executableInvariantYAML("P1.1", "TestMachineInvalid"))
	want := errors.New("planted violation detected")
	err := checkPatternInvariants(root, func(_, _, _ string) error {
		return want
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pattern invariant P1.1") ||
		!errors.Is(err, want) {
		t.Fatalf("error = %v, want P1.1 wrapping planted violation", err)
	}
}

func TestRunPatternInvariantCommandRequiresNegativeTestEvidence(t *testing.T) {
	t.Parallel()

	const testName = "TestPlantedViolation"
	pass := "printf '=== RUN   TestPlantedViolation\\n--- PASS: TestPlantedViolation (0.00s)\\n'"
	if err := runPatternInvariantCommand(t.TempDir(), pass, testName); err != nil {
		t.Fatalf("passing negative test evidence: %v", err)
	}
	missing := "printf 'ok\\n'"
	if err := runPatternInvariantCommand(t.TempDir(), missing, testName); err == nil ||
		!strings.Contains(err.Error(), "did not run and pass") {
		t.Fatalf("missing evidence error = %v", err)
	}
	if err := runPatternInvariantCommand(t.TempDir(), "exit 1", testName); err == nil ||
		!strings.Contains(err.Error(), "check command failed") {
		t.Fatalf("failing command error = %v", err)
	}
}

func executableInvariantYAML(id, negativeTest string) string {
	return `
patterns:
  - id: machine-interpreter
    invariants:
      - id: ` + id + `
        statement: The machine is valid.
        check:
          kind: executable
          command: go test ./machine -run ` + negativeTest + `
          negative_test: ` + negativeTest + `
`
}

func patternInvariantFixture(t *testing.T, content string) string {
	t.Helper()

	root := t.TempDir()
	writePatternInvariantFixture(t, root, content)
	return root
}

func writePatternInvariantFixture(t *testing.T, root, content string) {
	t.Helper()

	path := filepath.Join(root, patternLanguagePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func passingPatternCheck(_, _, _ string) error {
	return nil
}
