// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"os"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
	"path/filepath"
)

// seeded is a tracker whose group g already holds labels.
func seeded(t *testing.T, p trackertest.Program, labels ...tracker.Label) tracker.Tracker {
	t.Helper()
	f := p.Tracker()
	for _, l := range labels {
		if err := f.CreateGroupLabel("g", l); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func groupLabels(t *testing.T, f tracker.Tracker) map[string]tracker.Label {
	t.Helper()
	have, err := f.GroupLabels("g")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]tracker.Label{}
	for _, l := range have {
		m[l.Name] = l
	}
	return m
}

func actions(res []LabelResult) map[string]int {
	m := map[string]int{}
	for _, r := range res {
		m[r.Action]++
	}
	return m
}

func TestLabelsCreatesOnceAndNeverDeletes(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := seeded(t, p, tracker.Label{Name: "team-only", Color: "#000000"})
		want := DefaultLabels()

		res, err := SyncLabels(f, "g", want, false, false)
		if err != nil || actions(res)["created"] != len(want) {
			t.Fatalf("first run: %v %v", actions(res), err)
		}
		writes := p.Writes()
		res, err = SyncLabels(f, "g", want, false, false)
		if err != nil || actions(res)["unchanged"] != len(want) || p.Writes() != writes {
			t.Fatalf("second run: %v writes=%d err=%v", actions(res), p.Writes()-writes, err)
		}
		if have := groupLabels(t, f); len(have) != len(want)+1 || have["team-only"].Name == "" {
			t.Fatalf("labels: %+v", have)
		}
	})
}

func TestDifferingLabelIsReportedThenUpdatedOnRequest(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := seeded(t, p, tracker.Label{Name: "plane::data", Color: "#2CA02C", Description: "managed by terraform"})
		want := []tracker.Label{{Name: "plane::data", Color: "#1f75cb", Description: "Work"}}

		res, _ := SyncLabels(f, "g", want, false, false)
		if res[0].Action != "differs" || groupLabels(t, f)["plane::data"].Color != "#2CA02C" {
			t.Fatalf("without update: %+v %+v", res, groupLabels(t, f))
		}
		res, _ = SyncLabels(f, "g", want, true, false)
		if res[0].Action != "updated" || groupLabels(t, f)["plane::data"].Color != "#1f75cb" {
			t.Fatalf("with update: %+v %+v", res, groupLabels(t, f))
		}
	})
}

func TestLabelsColorCompareIgnoresCaseAndDryRunWritesNothing(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := seeded(t, p, tracker.Label{Name: "kind::spec", Color: "#8E7CC3", Description: "Changes requirements or design documents under docs/"})
		writes := p.Writes()
		res, err := SyncLabels(f, "g", DefaultLabels(), true, true)
		if err != nil || p.Writes() != writes {
			t.Fatalf("dry run wrote: %d %v", p.Writes()-writes, err)
		}
		for _, r := range res {
			if r.Name == "kind::spec" && r.Action != "unchanged" {
				t.Fatalf("case-different color treated as change: %+v", r)
			}
		}
	})
}

func TestLoadLabelsReadsControlFileOrDefaults(t *testing.T) {
	f := fake.New()
	if l, err := LoadLabels(f, "g/system", "main"); err != nil || len(l) != len(DefaultLabels()) {
		t.Fatalf("defaults: %d %v", len(l), err)
	}
	f.AddFile("g/system", "main", "control/labels.md", []byte(`---
id: labels
title: Labels
---

<!-- field: axes -->
## Axes

<!-- field: plane -->
### Plane

<!-- field: prefix -->
#### Prefix

plane::

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| data | #1f75cb | Work |

<!-- field: kind -->
### Kind

<!-- field: prefix -->
#### Prefix

kind::

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| spec | #8e7cc3 | Design |

<!-- field: component -->
### Component

<!-- field: prefix -->
#### Prefix

component::

<!-- field: values_from -->
#### Values from

subsystem SRDs
`))
	l, err := LoadLabels(f, "g/system", "main")
	if err != nil || len(l) != 2 || l[0].Name != "plane::data" || l[1].Name != "kind::spec" {
		t.Fatalf("control file: %+v %v", l, err)
	}
}

// TestConfigFormatExamplesDecode holds the labels and policy config formats
// to their own examples: each example is a config document scrum-master reads.
func TestConfigFormatExamplesDecode(t *testing.T) {
	f := fake.New()
	f.AddFile("g/system", "main", LabelsFile, specExample(t, "labels"))
	f.AddFile("g/system", "main", PolicyFile, specExample(t, "policy"))
	labels, err := LoadLabels(f, "g/system", "main")
	if err != nil || len(labels) == 0 || labels[0].Name != "plane::management" {
		t.Fatalf("labels = %v, %v", labels, err)
	}
	p, err := LoadPolicy(f, "g/system", "main")
	if err != nil {
		t.Fatal(err)
	}
	if p.AuditCommand != "mage audit" || p.Escalation.BlockerAgeDays != 14 || len(p.Gate("agent-profile")) != 2 || p.Gate("corpus")[0].Command != "mage audit" {
		t.Fatalf("policy = %+v", p)
	}
}

// specExample is the config document a config format shows in its
// config_file_layout field.
func specExample(t *testing.T, format string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../docs/specs/config-formats/" + format + ".md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(b), "<!-- field: config_file_layout -->")
	if !ok {
		t.Fatalf("%s has no config_file_layout", format)
	}
	_, rest, _ = strings.Cut(rest, "```markdown\n")
	doc, _, _ := strings.Cut(rest, "\n```")
	return []byte(doc + "\n")
}

// TestLabelsRegistersComponents is srd013 R6.1 with --control: each
// component the system's subsystem SRDs declare gets its component:: label
// once, so a tracker keeping a label registry holds what issues carry.
func TestLabelsRegistersComponents(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", []byte(subsystemFixture))
		f := p.Tracker()
		run := func() {
			t.Helper()
			var out bytes.Buffer
			if err := cli.Default.Run([]string{"issue", "labels", "--group", "g", "--control", "g/system", "--json"}, &out, func() (tracker.Tracker, error) { return f, nil }); err != nil {
				t.Fatalf("labels: %v: %s", err, out.String())
			}
		}
		run()
		if l := groupLabels(t, f)["component::po-1"]; l.Name == "" {
			t.Fatalf("component::po-1 not registered: %v", groupLabels(t, f))
		}
		writes := p.Writes()
		run()
		if p.Writes() != writes {
			t.Fatalf("a second run wrote %d times", p.Writes()-writes)
		}
	})
}

// issue labels without --group creates the labels in the program group
// control/trackers.md records (GH-103).
func TestLabelsDefaultToTheRecordedGroup(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "control"), 0o755)
	os.WriteFile(filepath.Join(dir, routing.File), routing.Document(routing.Config{Group: "aeop", Default: routing.System{Name: routing.Beads, TrackerRepository: "x"}}), 0o644)
	t.Chdir(dir)
	f := fake.New()
	var out bytes.Buffer
	if err := cli.Default.Run([]string{"issue", "labels", "--json"}, &out, func() (tracker.Tracker, error) { return f, nil }); err != nil {
		t.Fatalf("labels: %v: %s", err, out.String())
	}
	if labels, _ := f.GroupLabels("aeop"); len(labels) == 0 {
		t.Fatalf("no labels in the recorded group aeop: %s", out.String())
	}
}
