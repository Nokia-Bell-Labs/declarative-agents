// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
// Test-only subprocess helpers inventory temporary fixture modules. Production
// pkg/spec only parses outputs supplied by declared profile exec words.

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

type moduleTestRunner func(string) (map[testRef]testResult, error)

func runGoTestEvidenceWith(root string, run moduleTestRunner) ([]Finding, error) {
	inv, err := BuildGoTestInventory(root)
	if err != nil {
		return nil, err
	}
	suites, err := discoverAndParseTestSuites(root)
	if err != nil {
		return nil, err
	}
	_, findings := collectEvidenceClaims(inv, suites)
	if len(findings) > 0 {
		return findings, nil
	}
	results, err := run(root)
	if err != nil {
		return nil, err
	}
	var events strings.Builder
	for ref, result := range results {
		for _, line := range result.output {
			data, _ := json.Marshal(goTestEvent{Action: "output", Package: ref.pkg, Test: ref.name, Output: line})
			events.Write(data)
			events.WriteByte('\n')
		}
		data, _ := json.Marshal(goTestEvent{Action: result.action, Package: ref.pkg, Test: ref.name})
		events.Write(data)
		events.WriteByte('\n')
	}
	return ReduceGoTestEvidenceRun(inv, suites, events.String())
}

func BuildGoTestInventory(root string) (*GoTestInventory, error) {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, out)
		}
		return string(out), nil
	}
	module, err := run("list", "-m")
	if err != nil {
		return nil, err
	}
	packages, err := run("list", "./...")
	if err != nil {
		return nil, err
	}
	tests, err := run("test", "-json", "-list", "^Test", "./...")
	if err != nil {
		return nil, err
	}
	return ParseGoTestInventory(module, packages, tests)
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
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{"TestOther": "pass"}))
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

func TestRunEvidenceRequiresEveryAmbiguousBareNameMatchToPass(t *testing.T) {
	inv := &GoTestInventory{
		modulePath: "example.test/fixture",
		packages: map[string]bool{
			"example.test/fixture/one": true,
			"example.test/fixture/two": true,
		},
		byPackage: map[string]map[string]bool{
			"example.test/fixture/one": {"TestShared": true},
			"example.test/fixture/two": {"TestShared": true},
		},
		allTests: map[string]bool{"TestShared": true},
	}
	suites := map[string]TestSuite{"shared": {
		ID: "shared", TestCases: []TestCase{{Name: "ambiguous proof", GoTest: "TestShared"}},
	}}
	events := `{"Action":"pass","Package":"example.test/fixture/one","Test":"TestShared"}
{"Action":"fail","Package":"example.test/fixture/two","Test":"TestShared"}
`
	findings, err := ReduceGoTestEvidenceRun(inv, suites, events)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "TestShared failed (example.test/fixture/two)") {
		t.Fatalf("ambiguous bare name must require both package matches: %+v", findings)
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

// TestRunEvidenceAllowsNothingClaimed preserves the audit behavior for modules
// whose only executable evidence is handled by another gate.
func TestRunEvidenceAllowsNothingClaimed(t *testing.T) {
	suite := `
id: test-rel09.0-example
title: Example
test_cases:
  - name: Mage only
    go_test: mage integration:thing
`
	root := evidenceFixture(t, suite, map[string]string{"TestClaimed": ""})
	findings, err := runGoTestEvidenceWith(root, staticResults(map[string]string{}))
	if err != nil || len(findings) != 0 {
		t.Fatalf("an empty claim set should be neutral, got findings=%v err=%v", findings, err)
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
