// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := validateTestEvidence(run, "/tmp/agent", "/module", "/core"); err != nil {
		t.Fatalf("clean evidence should pass, got %v", err)
	}
	want := "/tmp/agent --profile /module/agents/jurist/audit-profile.yaml --directory /module --core-root /core"
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
	err := validateTestEvidence(run, "/tmp/agent", "/module", "/core")
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
	err := validateTestEvidence(run, "/tmp/agent", "/module", "/core")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected the exec error to surface, got %v", err)
	}
}

func TestJuristAuditProfileDeclaresEvidencePipeline(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "agents", "jurist", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	profile := read("audit-profile.yaml")
	machine := read("audit-machine.yaml")
	tools := read("go-test.yaml")
	for _, want := range []string{"audit-machine.yaml", "audit-tools.yaml", "go-test.yaml"} {
		if !strings.Contains(profile, want) {
			t.Errorf("audit profile missing %q", want)
		}
	}
	for _, want := range []string{
		"action: load_test_claims", "action: go_module", "action: go_packages_raw", "action: go_packages", "action: go_test_inventory",
		"action: resolve_test_evidence", "action: go_test_run",
		"action: reduce_test_evidence_run", "action: format_report",
	} {
		if !strings.Contains(machine, want) {
			t.Errorf("audit machine missing %q", want)
		}
	}
	for _, want := range []string{"binary: go", "args: [list, -m]", "args: [list, ./...]", "stdin_source: $from(go_packages_raw).output", "args: [test, -json, -count=1, ./...]"} {
		if !strings.Contains(tools, want) {
			t.Errorf("Go exec declarations missing %q", want)
		}
	}
}
