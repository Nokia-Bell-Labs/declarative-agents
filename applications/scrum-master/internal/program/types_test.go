// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

const epicLabelsFixture = `---
id: labels
title: Labels
---

<!-- field: axes -->
## Axes

<!-- field: type -->
### Type

<!-- field: values -->
#### Values

| name |
| --- |
| epic |
| sprint |
`

func TestDefaultsNameTrackingNotEpic(t *testing.T) {
	var names []string
	for _, l := range DefaultLabels() {
		names = append(names, l.Name)
	}
	if !slices.Contains(names, "type::tracking") || slices.Contains(names, "type::epic") {
		t.Fatalf("default labels: %v", names)
	}
	if types := Defaults().Types; !slices.Contains(types, "tracking") || slices.Contains(types, "epic") {
		t.Fatalf("default types: %v", types)
	}
}

func TestNewIssuesCarryTheCurrentTypeName(t *testing.T) {
	for _, typ := range []string{"epic", "tracking"} {
		if l := Labels("data", typ, "", "fleet"); !slices.Contains(l, "type::tracking") || slices.Contains(l, "type::epic") {
			t.Errorf("--type %s: %v", typ, l)
		}
	}
}

func TestLintReadsEpicAsTracking(t *testing.T) {
	issue := func(typ string) tracker.Issue {
		return tracker.Issue{Project: "g/system", IID: 1, Labels: []string{"plane::management", "type::" + typ, "component::fleet"}}
	}
	for _, typ := range []string{"epic", "tracking"} {
		if reasons := Lint(issue(typ), Defaults()); len(reasons) > 0 {
			t.Errorf("type::%s: %v", typ, reasons)
		}
	}
	if reasons := Lint(issue("bogus"), Defaults()); len(reasons) != 1 || !strings.Contains(reasons[0], "type::bogus") {
		t.Errorf("type::bogus: %v", reasons)
	}
}

func TestControlFileListingEpicKeepsWorkingAndIsReported(t *testing.T) {
	f := fake.New()
	f.AddFile("g/system", "main", LabelsFile, []byte(epicLabelsFixture))
	p, err := LoadPolicy(f, "g/system", "main")
	if err != nil || !slices.Equal(p.Types, []string{"tracking", "sprint"}) {
		t.Fatalf("types %v err %v", p.Types, err)
	}
	stale, err := StaleTypes(f, "g/system", "main")
	if err != nil || !slices.Equal(stale, []string{"epic"}) {
		t.Fatalf("stale %v err %v", stale, err)
	}
	if stale, err := StaleTypes(f, "g/other", "main"); err != nil || stale != nil {
		t.Fatalf("absent control file: %v %v", stale, err)
	}
}

// TestMigrateRelabelsOnceAcrossTheProgram relabels type::epic issues in the
// system repository and a repository its subsystem SRD declares (srd013
// R6.3): a dry run writes nothing, the run relabels each once, a second run
// finds nothing, and other labels and issues are left alone.
func TestMigrateRelabelsOnceAcrossTheProgram(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", []byte(subsystemFixture))
		p.AddFile("g/system", "main", LabelsFile, []byte(epicLabelsFixture))
		tr := p.Tracker()
		create := func(project string, labels ...string) tracker.Issue {
			i, err := tr.CreateIssue(project, tracker.NewIssue{Title: "t", Labels: labels})
			if err != nil {
				t.Fatal(err)
			}
			return i
		}
		sys := create("g/system", "plane::management", "type::epic")
		po := create("g/product-owners/po-1", "plane::data", "type::epic", "component::po-1")
		plain := create("g/product-owners/po-1", "plane::data", "kind::build", "component::po-1")

		writes := p.Writes()
		dry, err := migrate(tr, "g/system", "main", true)
		if err != nil || len(dry.Relabelled) != 2 || p.Writes() != writes {
			t.Fatalf("dry run: %+v writes=%d err=%v", dry, p.Writes()-writes, err)
		}
		res, err := migrate(tr, "g/system", "main", false)
		if err != nil || len(res.Relabelled) != 2 || !slices.Equal(res.StaleTypes, []string{"epic"}) {
			t.Fatalf("run: %+v err=%v", res, err)
		}
		for _, i := range []tracker.Issue{sys, po} {
			got, err := tr.GetIssue(i.Project, i.IID)
			if err != nil || !got.HasLabel("type::tracking") || got.HasLabel("type::epic") || !got.HasLabel(i.Labels[0]) {
				t.Errorf("%s#%d labels %v err %v", i.Project, i.IID, got.Labels, err)
			}
		}
		if got, _ := tr.GetIssue(plain.Project, plain.IID); slices.ContainsFunc(got.Labels, func(l string) bool { return strings.HasPrefix(l, "type::") }) {
			t.Errorf("plain issue relabelled: %v", got.Labels)
		}
		writes = p.Writes()
		again, err := migrate(tr, "g/system", "main", false)
		if err != nil || len(again.Relabelled) != 0 || p.Writes() != writes {
			t.Fatalf("second run: %+v writes=%d err=%v", again, p.Writes()-writes, err)
		}
	})
}
