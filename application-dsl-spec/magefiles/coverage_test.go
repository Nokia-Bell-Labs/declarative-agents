// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

const coverageHeader = "Table 1: format.\n\n| Area | Verdict | Where |\n|---|---|---|\n"

func coverageFixture(t *testing.T, rows string) string {
	t.Helper()
	dir := t.TempDir()
	writeEvidenceFile(t, dir, coverageFileName, "# Coverage\n\n"+coverageHeader+rows+"\nAfter the table.\n")
	return dir
}

func TestCoverageMapAcceptsEveryVerdict(t *testing.T) {
	dir := coverageFixture(t,
		"| Root mapping | covered | R-INTRO-001 |\n"+
			"| Diagrams | detail | presentation, not language |\n"+
			"| Retries | gap | tracked in #42 |\n")
	if err := checkCoverageMap(dir, validLanguage()); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageMapRejectsBadRows(t *testing.T) {
	tests := map[string]struct{ row, want string }{
		"unknown id":          {"| Area | covered | R-NOPE-001 |\n", "R-NOPE-001 resolves to no statement"},
		"covered without id":  {"| Area | covered | somewhere |\n", "names no statement"},
		"detail without kind": {"| Area | detail | trust me |\n", "must name its kind"},
		"gap without issue":   {"| Area | gap | later |\n", "must name a tracking issue"},
		"unknown verdict":     {"| Area | partial | R-INTRO-001 |\n", `unknown verdict "partial"`},
		"wrong cell count":    {"| Area | covered |\n", "has 2 cells"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkCoverageMap(coverageFixture(t, test.row), validLanguage())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCoverageMapRequiresRows(t *testing.T) {
	dir := t.TempDir()
	writeEvidenceFile(t, dir, coverageFileName, "# Coverage\n\nNo tables.\n")
	if err := checkCoverageMap(dir, validLanguage()); err == nil {
		t.Fatal("empty coverage map passed")
	}
}

// TestRepositoryCoverageMapResolves gates the tracked 1.0 coverage map, so a
// renamed statement or an untracked gap fails go test as well as mage audit.
func TestRepositoryCoverageMapResolves(t *testing.T) {
	language, err := loadLanguage(engineVersion.languagePath(".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkCoverageMap(engineVersion.root(".."), language); err != nil {
		t.Fatal(err)
	}
}
