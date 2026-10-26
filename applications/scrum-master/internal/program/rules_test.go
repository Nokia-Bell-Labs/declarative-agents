// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func workIssue() tracker.Issue {
	return tracker.Issue{
		Labels: []string{"plane::data", "kind::build", "component::po-1"},
		Description: "## Requirements\n\n1. Do it.\n\n## Acceptance Criteria\n\n" +
			"- It is done (test: done; environment: local)\n",
	}
}

// A rule removed from the policy's admission.rules no longer applies
// (GH-130; srd013 R1.1, config format policy).
func TestRuleRemovedFromPolicyNoLongerApplies(t *testing.T) {
	p := Defaults()
	i := workIssue() // no milestone: data-milestone fails under the defaults
	if reasons := Lint(i, p); !slices.ContainsFunc(reasons, func(r string) bool { return strings.Contains(r, "milestone") }) {
		t.Fatalf("defaults must require the milestone, got %v", reasons)
	}
	p.AdmissionRules = slices.DeleteFunc(RuleNames(), func(n string) bool { return n == "data-milestone" })
	if reasons := Lint(i, p); len(reasons) != 0 {
		t.Fatalf("with data-milestone removed, reasons = %v, want none", reasons)
	}
}

// The declared default rules produce the same verdicts as the built-in
// list, so declaring them in control/policy.md changes nothing (GH-130).
func TestDeclaredDefaultRulesMatchTheBuiltInList(t *testing.T) {
	p := Defaults()
	declared := Defaults()
	declared.AdmissionRules = RuleNames()
	for name, i := range map[string]tracker.Issue{
		"admitted":       func() tracker.Issue { i := workIssue(); i.Milestone = "rig-01.0"; return i }(),
		"two planes":     {Labels: []string{"plane::data", "plane::control", "kind::build", "component::po-1"}},
		"kind off-plane": {Labels: []string{"plane::management", "kind::build", "component::po-1"}},
		"bad criterion": {Labels: []string{"plane::data", "kind::build", "component::po-1"},
			Description: "## Requirements\n\n1. Do.\n\n## Acceptance Criteria\n\n- vague\n"},
	} {
		got, want := Lint(i, declared), Lint(i, p)
		if !slices.Equal(got, want) {
			t.Errorf("%s: declared %v != built-in %v", name, got, want)
		}
	}
}

// A rule name scrum-master does not implement is reported, not silently skipped.
func TestUnknownRuleIsReported(t *testing.T) {
	p := Defaults()
	p.AdmissionRules = []string{"one-plane", "no-such-rule"}
	reasons := Lint(workIssue(), p)
	if !slices.ContainsFunc(reasons, func(r string) bool { return strings.Contains(r, "no-such-rule") }) {
		t.Fatalf("reasons = %v, want the unknown rule named", reasons)
	}
}
