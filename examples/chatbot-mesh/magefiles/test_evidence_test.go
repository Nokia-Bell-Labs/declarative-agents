// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestValidateTestEvidencePassesOnCleanModule asserts the audit invokes the
// agent's evidence resolver over this module root and accepts a clean result.
func TestValidateTestEvidencePassesOnCleanModule(t *testing.T) {
	var got []string
	run := func(binary string, args ...string) ([]byte, error) {
		got = append([]string{binary}, args...)
		return []byte("test evidence valid"), nil
	}
	if err := validateTestEvidence(run, "/tmp/agent", "/module"); err != nil {
		t.Fatalf("clean evidence should pass, got %v", err)
	}
	want := "/tmp/agent --validate-test-evidence --directory /module"
	if strings.Join(got, " ") != want {
		t.Errorf("invocation = %q, want %q", strings.Join(got, " "), want)
	}
}

// TestValidateTestEvidenceFailsAuditOnFindings asserts a zero-match proof
// command fails the audit and that the resolver's report reaches the caller.
func TestValidateTestEvidenceFailsAuditOnFindings(t *testing.T) {
	report := `Error: test evidence validation failed: 1 finding(s)
  [error] test-rel09.0: test case "x" go_test "go test ./magefiles -run TestGone": -run "TestGone" matches no test`
	run := func(_ string, _ ...string) ([]byte, error) {
		return []byte(report), fmt.Errorf("exit status 1")
	}
	err := validateTestEvidence(run, "/tmp/agent", "/module")
	if err == nil {
		t.Fatal("findings should fail the audit")
	}
	for _, want := range []string{"matches no test", "test-rel09.0", "TestGone"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestValidateTestEvidenceFallsBackToExitError asserts a failure with no output
// still reports the underlying error rather than an empty detail.
func TestValidateTestEvidenceFallsBackToExitError(t *testing.T) {
	run := func(_ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("fork/exec: permission denied")
	}
	err := validateTestEvidence(run, "/tmp/agent", "/module")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected the exec error to surface, got %v", err)
	}
}

// The evidence-run tests. Resolution proves a named test exists; `go test -list`
// compiles the test binaries and runs none of them. So a suite could claim
// evidence for a test that fails and the gate stayed green -- which is how the
// LLM preload gate case sat behind a passing audit from the day it was written
// (GH-701, GH-713).
//
// Execution now goes through agent-core's shared runner (spec.RunGoTestEvidence)
// rather than a second implementation here: this example classified evidence
// strings itself, duplicating a taxonomy pkg/spec already owned, and two copies
// of one taxonomy drift (GH-717). What is tested here is the delegation; the
// claim semantics are tested where they live, in pkg/spec.

// TestRunTestEvidenceInvokesTheSharedRunner asserts the audit reaches the shared
// runner over this module root, the same way validateTestEvidence reaches the
// resolver.
func TestRunTestEvidenceInvokesTheSharedRunner(t *testing.T) {
	var got []string
	run := func(binary string, args ...string) ([]byte, error) {
		got = append([]string{binary}, args...)
		return []byte("test evidence passed"), nil
	}
	if err := runTestEvidence(run, "/tmp/agent", "/module"); err != nil {
		t.Fatalf("passing evidence should pass, got %v", err)
	}
	want := "/tmp/agent --run-test-evidence --directory /module"
	if strings.Join(got, " ") != want {
		t.Errorf("invocation = %q, want %q", strings.Join(got, " "), want)
	}
}

// TestRunTestEvidenceFailsAuditOnAFailingClaim is the case that motivated the
// work: a suite claims evidence, the named test fails, and the audit fails with
// it rather than reporting the evidence validated. The runner's report must
// reach the caller intact -- a failure that named no suite would send the reader
// hunting.
func TestRunTestEvidenceFailsAuditOnAFailingClaim(t *testing.T) {
	report := `Error: test evidence run failed in .: 1 finding(s)
  [error] test-rel04.0-llm-tier: test case "Rendered workloads carry the preload gate" go_test "go test ./magefiles -run TestOllama" did not pass:
    TestOllamaTierRendersWithDefaults failed (chatbot-mesh/magefiles)`
	run := func(_ string, _ ...string) ([]byte, error) {
		return []byte(report), fmt.Errorf("exit status 1")
	}
	err := runTestEvidence(run, "/tmp/agent", "/module")
	if err == nil {
		t.Fatal("a failing claim must fail the audit")
	}
	for _, want := range []string{
		"test-rel04.0-llm-tier",
		"Rendered workloads carry the preload gate",
		"TestOllamaTierRendersWithDefaults failed",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%s", want, err)
		}
	}
}

// TestRunTestEvidenceFallsBackToExitError asserts a failure with no output still
// reports the underlying error rather than an empty detail.
func TestRunTestEvidenceFallsBackToExitError(t *testing.T) {
	run := func(_ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("fork/exec: permission denied")
	}
	err := runTestEvidence(run, "/tmp/agent", "/module")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected the exec error to surface, got %v", err)
	}
}

// TestNoEvidenceClassificationRemains proves the duplicate is gone rather than
// left beside the delegation. Two code paths for one job is the condition this
// change exists to remove, and a leftover would drift from pkg/spec silently.
func TestNoEvidenceClassificationRemains(t *testing.T) {
	source, err := os.ReadFile("test_evidence.go")
	if err != nil {
		t.Fatalf("read test_evidence.go: %v", err)
	}
	for _, gone := range []string{
		"func goTestArgs", "func splitCommand", "func implementedEvidenceCommands",
		"func claimsImplemented", "evidenceCommand", "defaultEvidenceRun",
	} {
		if strings.Contains(string(source), gone) {
			t.Errorf("%s still lives in the example; the shared runner owns evidence classification", gone)
		}
	}
}
