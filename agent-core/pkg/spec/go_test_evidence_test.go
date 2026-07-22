// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"strings"
	"testing"
)

// testInventory is a hand-built inventory so the validator is exercised without
// running `go` or touching the filesystem. Two packages carry tests and a third
// resolves with none, so package-only evidence and cross-package isolation are
// both covered.
func testInventory() *GoTestInventory {
	const mod = "example.com/mod"
	spec := mod + "/pkg/spec"
	stl := mod + "/internal/tools/stl"
	empty := mod + "/internal/empty"
	return &GoTestInventory{
		modulePath: mod,
		packages:   map[string]bool{spec: true, stl: true, empty: true},
		byPackage: map[string]map[string]bool{
			spec: {"TestFoo": true, "TestLoop_RunsToCompletion": true, "TestBar": true},
			stl:  {"TestA": true, "TestB": true, "TestOnlyInStl": true},
		},
		allTests: map[string]bool{
			"TestFoo": true, "TestLoop_RunsToCompletion": true, "TestBar": true,
			"TestA": true, "TestB": true, "TestOnlyInStl": true,
		},
	}
}

func TestCheckEvidenceResolvesAndReports(t *testing.T) {
	inv := testInventory()
	tests := []struct {
		name     string
		evidence string
		wantErr  string // substring the problem must contain; "" means resolves/skips
	}{
		// Bare names, anchored to exact membership.
		{"bare name present", "TestFoo", ""},
		{"bare name absent", "TestMissing", "no Go test named TestMissing"},
		{"bare name cannot match by prefix", "TestLoop", "no Go test named TestLoop"},

		// Comma-separated names validate every member.
		{"comma list all present", "TestFoo, TestBar", ""},
		{"comma list one missing", "TestFoo, TestGone", "TestGone"},

		// Package-scoped -run.
		{"package run present", "go test ./pkg/spec -run TestFoo", ""},
		{"package run equals form", "go test ./pkg/spec -run=TestFoo", ""},
		{"regex alternation matches", "go test ./internal/tools/stl -run 'Test(A|B)'", ""},
		{"run cannot match another package", "go test ./pkg/spec -run TestOnlyInStl", "matches no test"},
		{"zero match despite exit zero", "go test ./pkg/spec -run TestNope", "matches no test"},
		{"invalid regex", "go test ./pkg/spec -run 'Test('", "invalid -run regex"},
		{"missing package", "go test ./pkg/nope -run TestFoo", "unknown package ./pkg/nope"},
		{"recursive run matches", "go test ./... -run TestOnlyInStl", ""},

		// Package-only commands: packages must resolve, nothing to match.
		{"package only present", "go test ./pkg/spec", ""},
		{"package only with no tests still resolves", "go test ./internal/empty", ""},
		{"package only missing", "go test ./pkg/nope", "unknown package ./pkg/nope"},
		{"recursive package only", "go test ./...", ""},

		// Skipped forms produce no problem.
		{"mage target skipped", "mage integration:uc001", ""},
		{"mage audit skipped", "mage audit", ""},
		{"cross-module pipeline skipped", "cd magefiles && go test ./... -run 'TestX'", ""},
		{"descriptive skipped", "Manual verification against the demo", ""},
		{"empty skipped", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inv.checkEvidence(tt.evidence)
			switch {
			case tt.wantErr == "" && got != "":
				t.Fatalf("checkEvidence(%q) = %q, want resolved/skipped", tt.evidence, got)
			case tt.wantErr != "" && !strings.Contains(got, tt.wantErr):
				t.Fatalf("checkEvidence(%q) = %q, want it to contain %q", tt.evidence, got, tt.wantErr)
			}
		})
	}
}

func TestValidateGoTestEvidenceFindings(t *testing.T) {
	inv := testInventory()
	suites := map[string]TestSuite{
		"test-rel02.0": {
			ID: "test-rel02.0",
			TestCases: []TestCase{
				{Name: "loads corpus", GoTest: "TestFoo"},
				{Name: "missing evidence", GoTest: "TestGhost"},
			},
		},
		"test-rel01.0": {
			ID: "test-rel01.0",
			TestCases: []TestCase{
				{Name: "bad package", GoTest: "go test ./pkg/nope -run TestFoo"},
				{Name: "descriptive", GoTest: "mage audit"},
			},
		},
	}

	findings := ValidateGoTestEvidence(inv, suites)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}

	// Findings are sorted by suite ID, so rel01 precedes rel02.
	if findings[0].SuiteID != "test-rel01.0" || findings[1].SuiteID != "test-rel02.0" {
		t.Fatalf("findings not sorted by suite: %q then %q", findings[0].SuiteID, findings[1].SuiteID)
	}
	for _, f := range findings {
		if f.Level != "error" || f.Check != goTestEvidenceCheck {
			t.Errorf("finding has wrong level/check: %+v", f)
		}
	}
	// R5: the message names the case and the evidence string.
	if !strings.Contains(findings[0].Message, "bad package") ||
		!strings.Contains(findings[0].Message, "go test ./pkg/nope") {
		t.Errorf("finding message missing case or evidence: %q", findings[0].Message)
	}
	if !strings.Contains(findings[1].Message, "TestGhost") {
		t.Errorf("finding message missing the unresolved name: %q", findings[1].Message)
	}
}

func TestBareTestNames(t *testing.T) {
	if _, ok := bareTestNames("TestFoo, TestBar"); !ok {
		t.Errorf("comma-separated names should parse as bare names")
	}
	if _, ok := bareTestNames("go test ./pkg"); ok {
		t.Errorf("a go test command is not a bare-name list")
	}
	if _, ok := bareTestNames("Manual check"); ok {
		t.Errorf("descriptive prose is not a bare-name list")
	}
}

// TestTopLevelBranchesSplitsOnlyOutsideGroups asserts alternation splitting
// treats a top-level "|" as separate named proofs while leaving group
// alternation (a shared-prefix shorthand) and malformed patterns whole.
func TestTopLevelBranchesSplitsOnlyOutsideGroups(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"single name", "TestFoo", []string{"TestFoo"}},
		{"top-level alternation", "TestFoo|TestBar", []string{"TestFoo", "TestBar"}},
		{"group alternation stays whole", "Test(Foo|Bar)", []string{"Test(Foo|Bar)"}},
		{
			"mixed group and top-level",
			"TestDispatch_(A|B)|TestOther",
			[]string{"TestDispatch_(A|B)", "TestOther"},
		},
		{"character class stays whole", "Test[A|B]oo", []string{"Test[A|B]oo"}},
		{"escaped pipe is not a split", `TestFoo\|Bar`, []string{`TestFoo\|Bar`}},
		{"unbalanced group is not split", "Test(Foo|Bar", []string{"Test(Foo|Bar"}},
		{"empty branch is not split", "TestFoo|", []string{"TestFoo|"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topLevelBranches(tt.pattern)
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Errorf("topLevelBranches(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

// TestCheckRunPatternRequiresEveryBranchToMatch is the GH-592 regression guard:
// a command whose other branch still matches must not pass while one named proof
// matches nothing.
func TestCheckRunPatternRequiresEveryBranchToMatch(t *testing.T) {
	inv := testInventory()
	specPkg := "example.com/mod/pkg/spec"

	if got := inv.checkRunPattern("TestFoo|TestBar", []string{specPkg}); got != "" {
		t.Errorf("both branches exist, want pass, got %q", got)
	}

	got := inv.checkRunPattern("TestFoo|TestNotHere", []string{specPkg})
	if got == "" {
		t.Fatal("a branch matching no test must fail even when another branch matches")
	}
	for _, want := range []string{"TestNotHere", "matches no test"} {
		if !strings.Contains(got, want) {
			t.Errorf("problem %q missing %q", got, want)
		}
	}
	// The whole-pattern message is reserved for a single-branch miss, so the
	// report points at the offending name rather than the entire command.
	if !strings.Contains(got, "names") {
		t.Errorf("multi-branch miss should name the branch, got %q", got)
	}

	if got := inv.checkRunPattern("TestNotHere", []string{specPkg}); !strings.Contains(got, "matches no test") {
		t.Errorf("single-branch miss should still report, got %q", got)
	}
}

// TestCheckRunPatternGroupAlternationUnchanged asserts a shared-prefix group is
// still judged as one proof, so an existing command keeps passing.
func TestCheckRunPatternGroupAlternationUnchanged(t *testing.T) {
	inv := testInventory()
	if got := inv.checkRunPattern("Test(Foo|NotHere)", []string{"example.com/mod/pkg/spec"}); got != "" {
		t.Errorf("group alternation should match as one pattern, got %q", got)
	}
}
