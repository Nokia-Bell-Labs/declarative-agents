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
