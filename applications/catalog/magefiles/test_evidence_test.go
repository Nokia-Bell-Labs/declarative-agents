// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestValidateTestEvidencePassesOnCleanModule asserts the audit invokes the
// agent's evidence resolver over this module root and accepts a clean result.
func TestValidateTestEvidencePassesOnCleanModule(t *testing.T) {
	var got []string
	cleaned := false
	run := func(binary string, args ...string) ([]byte, error) {
		got = append([]string{binary}, args...)
		return []byte("test evidence valid"), nil
	}
	stage := func(string) (string, func() error, error) {
		return "/staged/audit-profile.yaml", func() error {
			cleaned = true
			return nil
		}, nil
	}
	if err := validateTestEvidence(run, stage, "/tmp/agent", "/module", "/core"); err != nil {
		t.Fatalf("clean evidence should pass, got %v", err)
	}
	want := "/tmp/agent --profile /staged/audit-profile.yaml --directory /module --core-root /core"
	if strings.Join(got, " ") != want {
		t.Errorf("invocation = %q, want %q", strings.Join(got, " "), want)
	}
	if !cleaned {
		t.Error("staged evidence profile was not cleaned")
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
	err := validateTestEvidence(run, successfulEvidenceStage, "/tmp/agent", "/module", "/core")
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
	err := validateTestEvidence(run, successfulEvidenceStage, "/tmp/agent", "/module", "/core")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected the exec error to surface, got %v", err)
	}
}

func successfulEvidenceStage(string) (string, func() error, error) {
	return "/staged/audit-profile.yaml", func() error { return nil }, nil
}

func TestValidateTestEvidenceReportsCleanupFailure(t *testing.T) {
	wantErr := errors.New("cleanup failed")
	stage := func(string) (string, func() error, error) {
		return "/staged/audit-profile.yaml", func() error { return wantErr }, nil
	}
	err := validateTestEvidence(
		func(string, ...string) ([]byte, error) { return nil, nil },
		stage,
		"/tmp/agent",
		"/module",
		"/core",
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("validateTestEvidence error = %v, want cleanup failure", err)
	}
}

func TestStageCatalogTestEvidenceProfileCleansAfterBuildFailure(t *testing.T) {
	wantErr := errors.New("build failed")
	commands := &recordingCatalogRunner{failCall: 1, failErr: wantErr}
	runner := newTestCatalogRunner(t, commands)

	_, _, err := stageCatalogTestEvidenceProfileWith(runner.catalogRoot, runner)
	if !errors.Is(err, wantErr) {
		t.Fatalf("stage error = %v, want build failure", err)
	}
	calls := commands.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("commands = %d, want helper build only", len(calls))
	}
	assertRemoved(t, filepath.Dir(outputPath(t, calls[0])))
}

func TestStageCatalogTestEvidenceProfileOverridesGoBinaryLocally(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	commands := &recordingCatalogRunner{}
	runner := catalogTestRunner{
		catalogRoot: root,
		stdout:      io.Discard,
		stderr:      io.Discard,
		runCommand:  commands.run,
		mkdirTemp:   os.MkdirTemp,
		removeAll:   os.RemoveAll,
	}

	profile, cleanup, err := stageCatalogTestEvidenceProfileWith(root, runner)
	if err != nil {
		t.Fatalf("stage returned error: %v", err)
	}
	tempDir := filepath.Dir(profile)
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	helper := outputPath(t, commands.recordedCalls()[0])
	declaration, err := os.ReadFile(filepath.Join(tempDir, "go-test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(declaration), "binary: go") {
		t.Error("staged declaration retained raw Go binary")
	}
	if got := strings.Count(string(declaration), strconv.Quote(helper)); got != 3 {
		t.Errorf("staged helper references = %d, want 3", got)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	assertRemoved(t, tempDir)

	original, err := os.ReadFile(filepath.Join(root, "agents", "specification-critic", "go-test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(original), "binary: go") {
		t.Error("generic specification-critic declaration was modified")
	}
}

func TestSpecificationCriticAuditProfileDeclaresEvidencePipeline(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "agents", "specification-critic", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	profile := read("audit-profile.yaml")
	machine := read("audit-machine.yaml")
	tools := read("go-test.yaml")
	moduleArgs := "args: [GOWORK=off, go, list, -m]"
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
	failureRoute := "state: InventoryTests, signal: ToolFailed, next: ResolvingClaims, action: resolve_test_evidence"
	for _, want := range []string{
		failureRoute,
		"state: ResolvingClaims, signal: ValidationFailed, next: Reporting, action: format_report",
		"state: Reporting, signal: ToolFailed, next: Failed",
	} {
		if !strings.Contains(machine, want) {
			t.Errorf("audit machine missing governed inventory failure route %q", want)
		}
	}
	for _, want := range []string{"binary: env", "binary: go", moduleArgs, "args: [list, ./...]", "stdin_source: $from(go_packages_raw).output", "args: [test, -json, -count=1, ./...]"} {
		if !strings.Contains(tools, want) {
			t.Errorf("Go exec declarations missing %q", want)
		}
	}
	coreFixture, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "agent-core", "testdata", "integration", "profiles", "audit", "go-test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(coreFixture), moduleArgs) {
		t.Errorf("agent-core audit fixture missing %q", moduleArgs)
	}
	coreMachine, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "agent-core", "testdata", "integration", "profiles", "audit", "audit-machine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(coreMachine), failureRoute) {
		t.Errorf("agent-core audit fixture missing governed inventory failure route %q", failureRoute)
	}
	suite, err := os.ReadFile(filepath.Join(
		"..", "docs", "specs", "test-suites", "test-rel07.1-profile-boundaries.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"inventory_failure_reduction: resolve_test_evidence",
		"inventory_failure_reported: true",
		"implicit_module_download: false",
	} {
		if !strings.Contains(string(suite), want) {
			t.Errorf("formal evidence suite missing %q", want)
		}
	}
	if got := strings.Count(machine, "action: go_test_run"); got != 1 {
		t.Errorf("audit machine go_test_run actions = %d, want one shared evidence run", got)
	}
	if got := strings.Count(tools, "args: [test, -json, -count=1, ./...]"); got != 1 {
		t.Errorf("audit go test execution declarations = %d, want one shared evidence run", got)
	}
}
