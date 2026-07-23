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

// The evidence-run tests (GH-713). Resolution proves a named test exists;
// `go test -list` compiles the test binaries and runs none of them. So a suite
// could claim status: implemented for a test that fails and the gate stayed
// green -- which is how the LLM preload gate case (GH-701) sat behind a passing
// audit from the day it was written.

// TestRunTestEvidenceFailsOnAFailingClaim is the case that motivated the issue:
// a suite claims implemented evidence, the named test fails, and the audit must
// fail with it rather than reporting the evidence validated.
func TestRunTestEvidenceFailsOnAFailingClaim(t *testing.T) {
	root := writeSuite(t, `
id: test-rel09.0-example
test_cases:
  - name: A claim that does not hold
    status: implemented
    go_test: go test ./magefiles -run '^TestFails$'
`)
	run := func(_ string, _ ...string) ([]byte, error) {
		return []byte("--- FAIL: TestFails (0.00s)\n    thing_test.go:12: boom"), fmt.Errorf("exit status 1")
	}
	err := runTestEvidence(run, root)
	if err == nil {
		t.Fatal("a failing implemented claim must fail the audit")
	}
	for _, want := range []string{"test-rel09.0-example", "A claim that does not hold", "--- FAIL: TestFails", "boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%s", want, err)
		}
	}
}

// TestRunTestEvidenceRunsOnlyImplementedClaims proves a planned case is not run:
// it names no evidence it has to honor yet.
func TestRunTestEvidenceRunsOnlyImplementedClaims(t *testing.T) {
	root := writeSuite(t, `
id: test-rel09.0-example
test_cases:
  - name: Implemented
    status: implemented
    go_test: go test ./magefiles -run '^TestA$'
  - name: Done
    status: done
    go_test: go test ./magefiles -run '^TestB$'
  - name: Planned
    status: planned
    go_test: go test ./magefiles -run '^TestNotYetWritten$'
  - name: Integration only
    status: implemented
    integration_target: mage integration:thing
`)
	var ran []string
	run := func(_ string, args ...string) ([]byte, error) {
		ran = append(ran, strings.Join(args, " "))
		return nil, nil
	}
	if err := runTestEvidence(run, root); err != nil {
		t.Fatalf("passing claims should pass: %v", err)
	}
	want := []string{"test ./magefiles -run ^TestA$", "test ./magefiles -run ^TestB$"}
	if strings.Join(ran, "\n") != strings.Join(want, "\n") {
		t.Errorf("ran:\n%s\nwant:\n%s", strings.Join(ran, "\n"), strings.Join(want, "\n"))
	}
}

// TestRunTestEvidenceReportsEveryFailure proves one failing suite does not hide
// the rest, the same contract the boot smoke holds.
func TestRunTestEvidenceReportsEveryFailure(t *testing.T) {
	root := writeSuite(t, `
id: test-rel09.0-example
test_cases:
  - name: First
    status: implemented
    go_test: go test ./magefiles -run '^TestA$'
  - name: Second
    status: implemented
    go_test: go test ./magefiles -run '^TestB$'
`)
	run := func(_ string, _ ...string) ([]byte, error) {
		return []byte("failed"), fmt.Errorf("exit status 1")
	}
	err := runTestEvidence(run, root)
	if err == nil {
		t.Fatal("failing claims must fail the audit")
	}
	if !strings.Contains(err.Error(), "2 of 2 go_test evidence command(s) failed") {
		t.Errorf("error does not report both failures: %v", err)
	}
}

// TestRunTestEvidenceFailsWhenNothingIsRunnable guards the silent-success shape:
// a glob that matches nothing, or suites whose layout moved, must fail rather
// than report a vacuous pass over zero commands.
func TestRunTestEvidenceFailsWhenNothingIsRunnable(t *testing.T) {
	root := t.TempDir()
	err := runTestEvidence(func(string, ...string) ([]byte, error) { return nil, nil }, root)
	if err == nil || !strings.Contains(err.Error(), "no runnable go_test evidence") {
		t.Fatalf("an empty corpus must fail, got %v", err)
	}
}

// TestGoTestArgsParsesTheEvidenceForms covers the two runnable forms the
// resolver accepts, the forms it deliberately skips, and the forms that must
// fail rather than skip. The -run regex must survive as one argument: its '|'
// is an alternation, not a shell pipe.
func TestGoTestArgsParsesTheEvidenceForms(t *testing.T) {
	tests := []struct {
		name     string
		evidence string
		want     []string
		wantErr  string
	}{
		{
			name:     "command with a quoted alternation",
			evidence: `go test ./magefiles -run '^TestA$|^TestB$'`,
			want:     []string{"test", "./magefiles", "-run", "^TestA$|^TestB$"},
		},
		{
			name:     "command with no -run",
			evidence: "go test ./magefiles",
			want:     []string{"test", "./magefiles"},
		},
		{
			name:     "bare comma-separated names",
			evidence: "TestOne, TestTwo",
			want:     []string{"test", "./...", "-run", "^TestOne$|^TestTwo$"},
		},
		{name: "mage target", evidence: "mage integration:chatbot"},
		{name: "prose", evidence: "covered by the kind smoke test"},
		{name: "empty", evidence: "  "},
		// A go-test command that cannot be parsed must not pass as a skip: that
		// is the silent-pass shape this whole step exists to close.
		{
			name:     "unterminated quote",
			evidence: `go test ./magefiles -run '^TestA$`,
			wantErr:  "unterminated quote",
		},
		{name: "no package or test", evidence: "go test", wantErr: "names no package or test"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := goTestArgs(tc.evidence)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("args = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestImplementedEvidenceCommandsFailsOnUnparseableClaim proves an implemented
// case whose command cannot be parsed fails the audit rather than being skipped
// into a vacuous pass.
func TestImplementedEvidenceCommandsFailsOnUnparseableClaim(t *testing.T) {
	root := writeSuite(t, `
id: test-rel09.0-example
test_cases:
  - name: Unrunnable claim
    status: implemented
    go_test: "go test ./magefiles -run \'^TestA$"
`)
	_, err := implementedEvidenceCommands(root)
	if err == nil {
		t.Fatal("an unparseable implemented claim must fail, not skip")
	}
	for _, want := range []string{"test-rel09.0-example", "Unrunnable claim", "unterminated quote"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

// TestImplementedEvidenceCommandsKeepsEveryClaimant proves a command cited by
// two cases runs once and reports both. Attributing a failure to whichever
// suite sorted first would send the reader to the wrong claim.
func TestImplementedEvidenceCommandsKeepsEveryClaimant(t *testing.T) {
	root := writeSuite(t, `
id: test-rel09.0-example
test_cases:
  - name: First claim
    status: implemented
    go_test: go test ./magefiles -run '^TestShared$'
  - name: Second claim
    status: done
    go_test: go test ./magefiles -run '^TestShared$'
`)
	commands, err := implementedEvidenceCommands(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %d, want 1 (the shared command runs once)", len(commands))
	}
	if len(commands[0].claimants) != 2 {
		t.Fatalf("claimants = %v, want both cases", commands[0].claimants)
	}
}

// TestImplementedEvidenceCommandsCoversThisCorpus runs against the real suites,
// so the parsing is proven on the corpus it gates rather than on fixtures alone.
func TestImplementedEvidenceCommandsCoversThisCorpus(t *testing.T) {
	// agentDir walks up to the mesh root, so the corpus sits two levels above it.
	root := filepath.Dir(filepath.Dir(agentDir(t, "executor")))
	commands, err := implementedEvidenceCommands(root)
	if err != nil {
		t.Fatalf("read this example's suites: %v", err)
	}
	if len(commands) == 0 {
		t.Fatal("no runnable evidence in this corpus; the suites or their layout changed")
	}
	for _, command := range commands {
		if len(command.claimants) == 0 {
			t.Errorf("command %v carries no claimant; a failure could not be attributed", command.args)
		}
		if len(command.args) < 2 || command.args[0] != "test" {
			t.Errorf("command %v is not a go test invocation", command.args)
		}
	}
}

// writeSuite lays out a corpus with one test-suite file and returns its root.
func writeSuite(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "specs", "test-suites")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "suite.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
