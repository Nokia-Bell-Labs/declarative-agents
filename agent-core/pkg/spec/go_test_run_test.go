// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the execution half of the go_test evidence gate (GH-717).
// Resolution proves a named test exists -- `go test -list` compiles the test
// binaries and runs none of them -- so a suite could claim evidence for a test
// that fails and the audit stayed green. That is how a chatbot-mesh case claimed
// a proof that had never passed (GH-701).
//
// Every test here injects a module runner. None shells out to `go test`: the
// runner spawns `go test ./...` over its own module, so a test that let it run
// for real would recurse into itself.

// evidenceFixture lays out a module with one test suite and returns its root.
// The go.mod and package give BuildGoTestInventory a real module to inventory.
func evidenceFixture(t *testing.T, suite string, tests map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/fixture\n\ngo 1.24\n")
	write(filepath.Join(TSSubdir, "test-rel09.0-example.yaml"), suite)
	var body strings.Builder
	body.WriteString("package subject\n\nimport \"testing\"\n")
	for name, code := range tests {
		fmt.Fprintf(&body, "\nfunc %s(t *testing.T) {%s}\n", name, code)
	}
	write(filepath.Join("subject", "subject_test.go"), body.String())
	return root
}

// staticResults answers every lookup with one outcome, keyed by test name so a
// fixture does not have to know its own import paths.
func staticResults(byName map[string]string) moduleTestRunner {
	return func(string) (map[testRef]testResult, error) {
		results := map[testRef]testResult{}
		for name, action := range byName {
			results[testRef{pkg: "example.test/fixture/subject", name: name}] = testResult{
				action: action,
				output: []string{"    subject_test.go:9: " + name + " " + action},
			}
		}
		return results, nil
	}
}

const passingSuite = `
id: test-rel09.0-example
title: Example
test_cases:
  - name: A claim
    go_test: TestClaimed
`

// TestRunEvidenceFailsOnAFailingClaim is the case that motivated the issue: the
// suite claims a test as its proof, the test fails, and the audit fails with it
// rather than reporting the evidence validated.
func TestRunEvidenceFailsOnAFailingClaim(t *testing.T) {
	root := evidenceFixture(t, passingSuite, map[string]string{"TestClaimed": ""})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{"TestClaimed": "fail"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1: %v", len(findings), findings)
	}
	for _, want := range []string{"test-rel09.0-example", "A claim", "TestClaimed failed"} {
		if !strings.Contains(findings[0].Message+findings[0].SuiteID, want) {
			t.Errorf("finding missing %q: %+v", want, findings[0])
		}
	}
}

// TestRunEvidenceFailsOnASkippedClaim proves a skipped test is not evidence.
// This is the case only a single run can see: `go test -run X` exits zero
// whether X passed or skipped, so neither resolution nor a per-claim invocation
// can tell the difference.
func TestRunEvidenceFailsOnASkippedClaim(t *testing.T) {
	root := evidenceFixture(t, passingSuite, map[string]string{"TestClaimed": ""})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{"TestClaimed": "skip"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "was skipped") {
		t.Fatalf("a skipped claim must fail: %+v", findings)
	}
}

// TestRunEvidenceFailsOnAClaimThatNeverRan covers a test that resolves but the
// run never reached -- a build tag, a filtered package. It proves nothing.
func TestRunEvidenceFailsOnAClaimThatNeverRan(t *testing.T) {
	root := evidenceFixture(t, passingSuite, map[string]string{"TestClaimed": ""})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "did not run") {
		t.Fatalf("a claim that never ran must fail: %+v", findings)
	}
}

// TestRunEvidencePassesWhenEveryClaimPasses is the green path.
func TestRunEvidencePassesWhenEveryClaimPasses(t *testing.T) {
	root := evidenceFixture(t, passingSuite, map[string]string{"TestClaimed": ""})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{"TestClaimed": "pass"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("a passing claim should raise nothing: %+v", findings)
	}
}

// TestRunEvidenceTreatsAbsentStatusAsAClaim locks the rule that differs from the
// chatbot-mesh runner: the status field is optional in the test_suite format
// rule and most cases here omit it, so an absent status must not silently
// withhold the claim. Only planned does.
func TestRunEvidenceTreatsAbsentStatusAsAClaim(t *testing.T) {
	suite := `
id: test-rel09.0-example
title: Example
test_cases:
  - name: No status
    go_test: TestNoStatus
  - name: Implemented
    status: implemented
    go_test: TestImplemented
  - name: Done
    status: done
    go_test: TestDone
  - name: Planned
    status: planned
    go_test: TestPlanned
`
	root := evidenceFixture(t, suite, map[string]string{
		"TestNoStatus": "", "TestImplemented": "", "TestDone": "", "TestPlanned": "",
	})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{
		"TestNoStatus": "fail", "TestImplemented": "fail", "TestDone": "fail", "TestPlanned": "fail",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("findings = %d, want 3 (planned withholds its claim, nothing else does): %+v",
			len(findings), findings)
	}
	for _, f := range findings {
		if strings.Contains(f.Message, "Planned") {
			t.Errorf("a planned case must not be run: %s", f.Message)
		}
	}
}

// TestRunEvidenceReportsEveryFailingClaim proves one broken suite does not hide
// the rest.
func TestRunEvidenceReportsEveryFailingClaim(t *testing.T) {
	suite := `
id: test-rel09.0-example
title: Example
test_cases:
  - name: First
    go_test: TestOne
  - name: Second
    go_test: TestTwo
`
	root := evidenceFixture(t, suite, map[string]string{"TestOne": "", "TestTwo": ""})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{
		"TestOne": "fail", "TestTwo": "fail",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2: %+v", len(findings), findings)
	}
}

// TestRunEvidenceFailsOnAnUnresolvableClaim proves a claim naming a test that
// does not exist is an error reported before anything runs, not a skip. Skipping
// it would recreate the silent pass this validator exists to close.
func TestRunEvidenceFailsOnAnUnresolvableClaim(t *testing.T) {
	suite := `
id: test-rel09.0-example
title: Example
test_cases:
  - name: Names a ghost
    go_test: TestGone
`
	root := evidenceFixture(t, suite, map[string]string{"TestClaimed": ""})
	ran := false
	findings, err := runGoTestEvidenceWith(root, func(string) (map[testRef]testResult, error) {
		ran = true
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "no Go test named TestGone") {
		t.Fatalf("an unresolvable claim must be reported: %+v", findings)
	}
	if ran {
		t.Error("the module ran despite an unreadable claim; report before spending the run")
	}
}

// TestRunEvidenceFailsWhenNothingIsClaimed guards the vacuous pass: a corpus
// whose suites resolve to no runnable claim at all is a layout change, not a
// clean bill of health.
func TestRunEvidenceFailsWhenNothingIsClaimed(t *testing.T) {
	suite := `
id: test-rel09.0-example
title: Example
test_cases:
  - name: Mage only
    go_test: mage integration:thing
`
	root := evidenceFixture(t, suite, map[string]string{"TestClaimed": ""})
	_, err := runGoTestEvidenceWith(root, staticResults(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "no runnable go_test evidence") {
		t.Fatalf("an empty claim set must fail, got %v", err)
	}
}

// TestRunEvidenceRefusesToRecurse proves the guard against the hazard that bit
// during development: the runner shells out to `go test ./...` over its own
// module, so calling it from a test in that module spawns a child that runs the
// calling test. Without the guard that hangs until something kills it.
func TestRunEvidenceRefusesToRecurse(t *testing.T) {
	t.Setenv(evidenceRunEnv, "1")
	_, err := RunGoTestEvidence(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "would recurse") {
		t.Fatalf("expected the recursion guard to fire, got %v", err)
	}
}

// TestClaimedTestsExpandsTheEvidenceForms covers what each evidence form claims.
// A command with no -run claims every test in its packages, because that is what
// running it would prove.
func TestClaimedTestsExpandsTheEvidenceForms(t *testing.T) {
	root := evidenceFixture(t, passingSuite, map[string]string{
		"TestAlpha": "", "TestBeta": "", "TestGamma": "",
	})
	inv, err := BuildGoTestInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	names := func(refs []testRef) string {
		var out []string
		for _, r := range refs {
			out = append(out, r.name)
		}
		return strings.Join(out, ",")
	}

	tests := []struct {
		name     string
		evidence string
		want     string
		problem  string
	}{
		{name: "bare name", evidence: "TestAlpha", want: "TestAlpha"},
		{name: "comma-separated names", evidence: "TestAlpha, TestBeta", want: "TestAlpha,TestBeta"},
		{
			name:     "command with -run alternation",
			evidence: "go test ./subject -run '^TestAlpha$|^TestGamma$'",
			want:     "TestAlpha,TestGamma",
		},
		{
			name:     "command with no -run claims the package",
			evidence: "go test ./subject",
			want:     "TestAlpha,TestBeta,TestGamma",
		},
		{name: "mage target claims nothing", evidence: "mage integration:thing"},
		{name: "prose claims nothing", evidence: "covered by the kind smoke test"},
		{name: "unknown test", evidence: "TestGone", problem: "no Go test named TestGone"},
		{name: "unknown package", evidence: "go test ./ghost", problem: "unknown package"},
		{
			name:     "run pattern matching nothing",
			evidence: "go test ./subject -run '^TestNothing$'",
			problem:  "matches no test",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs, problem := inv.claimedTests(tc.evidence)
			if tc.problem != "" {
				if !strings.Contains(problem, tc.problem) {
					t.Fatalf("problem = %q, want containing %q", problem, tc.problem)
				}
				return
			}
			if problem != "" {
				t.Fatalf("unexpected problem: %s", problem)
			}
			if got := names(refs); got != tc.want {
				t.Errorf("claimed = %q, want %q", got, tc.want)
			}
		})
	}
}
