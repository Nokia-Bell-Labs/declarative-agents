// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

const goodBody = `Satisfy the evaluation objective

Cites: https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/objectives/obj001-a.md#R1.1
Environment: dev

## Requirements

1. Score one agent's trajectories.

## Acceptance Criteria

- One agent is scored (test: grader_scores_fixture_agent; environment: dev)
`

func objective(body string) tracker.Issue {
	return tracker.Issue{Project: "g/po-1", IID: 1, Description: body, Milestone: "2026-W43..W45",
		Labels: []string{"plane::data", "kind::build", "component::fleet"}}
}

func TestAdmittedObjectiveHasNoReasons(t *testing.T) {
	if r := Lint(objective(goodBody), Defaults()); len(r) != 0 {
		t.Fatalf("reasons: %v", r)
	}
}

func TestLintReportsEveryFailure(t *testing.T) {
	body := strings.Replace(goodBody, "/-/blob/v1.0.0/", "/-/blob/main/", 1)
	body = strings.Replace(body, " (test: grader_scores_fixture_agent; environment: dev)", "", 1)
	i := objective(body)
	i.Labels = []string{"plane::data", "plane::control", "kind::nonsense"}
	i.Milestone = ""
	got := strings.Join(Lint(i, Defaults()), "\n")
	for _, want := range []string{"exactly one plane:: label, has 2", "exactly one component:: label, has 0", "kind::nonsense is not a listed kind",
		"needs a sprint milestone", "names no (test: T; environment: E)", `branch "main"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestComponentRequiredOnceDeclared needs no component on an issue while
// the program's subsystem SRDs declare none, and once they declare one,
// treats a missing or undeclared component as a failure (srd001 R21.2,
// R21.5).
func TestComponentRequiredOnceDeclared(t *testing.T) {
	componentReasons := func(i tracker.Issue, p Policy) []string {
		var out []string
		for _, r := range Lint(i, p) {
			if strings.Contains(r, "component") {
				out = append(out, r)
			}
		}
		return out
	}
	i := objective(goodBody)
	i.Labels = []string{"plane::management"}

	fresh, err := LoadPolicy(fake.New(), "g/system", "main")
	if err != nil || !fresh.ComponentsRead || len(fresh.Components) != 0 {
		t.Fatalf("fresh program policy = %+v, %v", fresh, err)
	}
	if got := componentReasons(i, fresh); len(got) != 0 {
		t.Errorf("fresh program: %v, want no component reason", got)
	}
	if got := componentReasons(i, Defaults()); len(got) != 1 {
		t.Errorf("no control project read: %v, want the missing component", got)
	}

	f := fake.New()
	f.AddFile("g/system", "main", SRDDir+"/srd010-po-1.md", []byte(subsystemFixture))
	declared, err := LoadPolicy(f, "g/system", "main")
	if err != nil || !slices.Equal(declared.Components, []string{"po-1"}) {
		t.Fatalf("declared policy = %+v, %v", declared, err)
	}
	for labels, want := range map[string]string{"": "exactly one component:: label, has 0", "component::bogus": "component::bogus is not a listed component", "component::po-1": ""} {
		i.Labels = []string{"plane::management"}
		if labels != "" {
			i.Labels = append(i.Labels, labels)
		}
		got := strings.Join(componentReasons(i, declared), "; ")
		if (want == "" && got != "") || !strings.Contains(got, want) {
			t.Errorf("%q: %q, want %q", labels, got, want)
		}
	}
}

// TestLintAllowsAtMostOneOp admits an issue with no op:: label, as every
// issue filed before the axis has, and fails one with two (srd001 R34).
func TestLintAllowsAtMostOneOp(t *testing.T) {
	i := objective(goodBody)
	if r := Lint(i, Defaults()); len(r) != 0 {
		t.Fatalf("no op:: label: %v", r)
	}
	i.Labels = append(i.Labels, Op(OpFile))
	if r := Lint(i, Defaults()); len(r) != 0 {
		t.Fatalf("one op:: label: %v", r)
	}
	i.Labels = append(i.Labels, Op(OpPlan))
	if r := strings.Join(Lint(i, Defaults()), "; "); !strings.Contains(r, "has 2 op:: labels, at most one") {
		t.Fatalf("two op:: labels: %q", r)
	}
}

// TestLintFlagsMalformedLinks admits well-formed agile link lines and flags
// a malformed or unpinned target and a second Duplicates or Implements line
// (srd007 R8.1).
func TestLintFlagsMalformedLinks(t *testing.T) {
	good := goodBody + "\nBlocked-by: g/po-1#3\nRelates-to: ep001-agent-evaluation\nImplements: st001-score-trajectories\n"
	if r := Lint(objective(good), Defaults()); len(r) != 0 {
		t.Fatalf("well-formed links refused: %v", r)
	}
	bad := goodBody + strings.Join([]string{"",
		"Depends-on: somewhere",
		"Blocks: https://gitlab.example.com/g/po-2/-/blob/main/docs/specs/stories/st002-a.md#R1",
		"Duplicates: g/po-1#1",
		"Duplicates: g/po-1#2",
		"Implements: st001-score-trajectories",
		"Implements: ep001-agent-evaluation",
	}, "\n")
	got := strings.Join(Lint(objective(bad), Defaults()), "\n")
	for _, want := range []string{`Depends-on target "somewhere"`, `branch "main"`, "has 2 Duplicates lines", "has 2 Implements lines", "Implements names ep001-agent-evaluation, not a story"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSprintIssueNeedsNoComponent(t *testing.T) {
	i := tracker.Issue{Labels: []string{"plane::management", "type::sprint"}}
	if r := Lint(i, Defaults()); len(r) != 0 {
		t.Fatalf("reasons: %v", r)
	}
}

func TestManagementFilersAreEnforcedWhenListed(t *testing.T) {
	p := Defaults()
	p.ManagementFilers = []string{"scrum-master-lead"}
	i := tracker.Issue{Author: "someone", Labels: []string{"plane::management", "component::system"}}
	if r := Lint(i, p); len(r) != 1 || !strings.Contains(r[0], "someone may not file a management issue") {
		t.Fatalf("reasons: %v", r)
	}
}

func TestApplySetsAndClearsNeedsTriageAndNothingElse(t *testing.T) {
	f := fake.New()
	p := Defaults()
	bad := objective(strings.Replace(goodBody, "/-/blob/v1.0.0/", "/-/blob/main/", 1))
	created, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "o", Description: bad.Description, Labels: bad.Labels, Milestone: bad.Milestone})

	r, err := Apply(f, created, Lint(created, p), p, false)
	if err != nil || r.Admitted || !r.Changed {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	got, _ := f.GetIssue("g/po-1", created.IID)
	if !got.HasLabel("needs-triage") || len(got.Labels) != len(created.Labels)+1 || len(got.Assignees) != 0 {
		t.Fatalf("labels=%v assignees=%v", got.Labels, got.Assignees)
	}
	notes, _ := f.ListNotes("g/po-1", created.IID)
	if len(notes) != 1 || !strings.Contains(notes[0].Body, "not pinned") {
		t.Fatalf("notes=%+v", notes)
	}

	if r, _ := Apply(f, got, Lint(got, p), p, false); r.Changed {
		t.Fatal("second identical lint changed something")
	}

	fixed := got
	fixed.Description = goodBody
	if _, err := Apply(f, fixed, Lint(fixed, p), p, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.GetIssue("g/po-1", created.IID); got.HasLabel("needs-triage") {
		t.Fatalf("needs-triage not cleared: %v", got.Labels)
	}
}

func TestFileCreatesUnassignedLintedIssueAndDryRunWritesNothing(t *testing.T) {
	f := fake.New()
	in := tracker.NewIssue{Title: "objective", Description: goodBody, Milestone: "2026-W43..W45", Labels: Labels("data", "", "build", "fleet")}
	r, err := File(f, "g/po-1", in, Defaults(), true)
	if err != nil || !r.DryRun || !r.Lint.Admitted || f.Writes != 0 {
		t.Fatalf("dry run: r=%+v err=%v writes=%d", r, err, f.Writes)
	}
	r, err = File(f, "g/po-1", in, Defaults(), false)
	if err != nil || r.IID != 1 || !r.Lint.Admitted {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	got, _ := f.GetIssue("g/po-1", 1)
	if len(got.Assignees) != 0 || !got.HasLabel("component::fleet") {
		t.Fatalf("%+v", got)
	}
}

func TestLoadPolicyReadsControlFiles(t *testing.T) {
	f := fake.New()
	f.AddFile("g/system", "main", "control/policy.md", []byte(`---
id: policy
title: Policy
---

<!-- field: admission -->
## Admission

<!-- field: require_milestone -->
### Require milestone

false

<!-- field: require_approval -->
### Require approval

true

<!-- field: management_filers -->
### Management filers

- lead-a
`))
	f.AddFile("g/system", "main", "control/labels.md", []byte(`---
id: labels
title: Labels
---

<!-- field: axes -->
## Axes

<!-- field: kind -->
### Kind

<!-- field: values -->
#### Values

| name |
| --- |
| build |
`))
	p, err := LoadPolicy(f, "g/system", "main")
	if err != nil || p.RequireMilestone || !p.RequireApproval || p.ManagementFilers[0] != "lead-a" || len(p.Kinds) != 1 {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if p, err := LoadPolicy(f, "g/other", "main"); err != nil || !p.RequireMilestone || p.RequireApproval {
		t.Fatalf("absent control files: p=%+v err=%v", p, err)
	}
}
