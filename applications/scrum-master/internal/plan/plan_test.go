// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// obj002 is a second agreed objective document: R1 is due before obj001's
// objectives and fully evidenced; R2.1 has a criterion with no environment.
const obj002 = `---
id: obj002-fast-scoring
title: Fast scoring
status: agreed
---

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W43..2027-W03

<!-- field: problem -->
## Problem

Scoring takes a day.

<!-- field: goals -->
## Goals

- G1: A score is ready within an hour.

<!-- req: R1 -->
## R1 - Score in an hour

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-10-30"

<!-- req: R1.1 -->
### R1.1

A score is ready within an hour of a trajectory.

<!-- req: R2 -->
## R2 - Score on demand

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-12-04"

<!-- req: R2.1 -->
### R2.1

A person asks for a score and gets one.

<!-- field: non_goals -->
## Non-goals

- Scoring models.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A score is ready within an hour.

<!-- field: traces -->
#### Traces

- R1.1

<!-- field: tests -->
#### Tests

- scorer_meets_the_hour

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC2 -->
### AC2

A score comes on demand.

<!-- field: traces -->
#### Traces

- R2.1

<!-- field: tests -->
#### Tests

- scorer_on_demand
`

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// system is a system repository checkout: the testdata road map with
// release 01.0 placing obj001's items, and both objective documents.
func system(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(path string, b []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(p string) []byte { return []byte(readFile(t, p)) }
	write(RoadMap, read("../../testdata/system/road-map.md"))
	write("docs/specs/objectives/obj001-reliable-agents.md", read("../../testdata/objectives/valid/obj001-reliable-agents.md"))
	write("docs/specs/objectives/obj002-fast-scoring.md", []byte(obj002))
	return dir
}

// input is the planning input over p's tracker: one objective issue in
// g/po-1 for obj001's R2, which the aggregate reports slipped.
func input(t *testing.T, dir string, p trackertest.Program) (Input, tracker.Tracker) {
	t.Helper()
	f := p.Tracker()
	i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Objective R2",
		Description: "Objective\n\n<!-- scrum-master goals objective=obj001-reliable-agents#R2 release=01.0 -->"})
	if err != nil {
		t.Fatal(err)
	}
	agg := &reports.Aggregation{Signals: []reports.Signal{{Kind: reports.SignalSlipped, Subject: fmt.Sprintf("g/po-1#%d", i.IID)}}}
	return Input{Dir: dir, System: "g/system", Tracker: f, Aggregate: agg, Releases: program.Defaults().Releases, Artifact: "corpus", Now: now}, f
}

func ids(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestPlanReleaseRanksSlippedFirst(t *testing.T) {
	trackertest.Each(t, testPlanReleaseRanksSlippedFirst)
}

func testPlanReleaseRanksSlippedFirst(t *testing.T, p trackertest.Program) {
	in, _ := input(t, system(t), p)
	c, err := MakeCandidates(in)
	if err != nil {
		t.Fatal(err)
	}
	// obj001's R1 items are placed in 01.0 and did not slip; its R2.1 is
	// placed but slipped, so it comes back first; obj002's R1.1 is due
	// earliest of the rest; obj002's R2.1 lacks an environment.
	if got := ids(c.Items); !slices.Equal(got, []string{"obj001-reliable-agents#R2.1", "obj002-fast-scoring#R1.1"}) {
		t.Fatalf("items = %v", got)
	}
	if !c.Items[0].Slipped || c.Items[0].Placed != "01.0" {
		t.Fatalf("slipped item = %+v", c.Items[0])
	}
	// 01.0's milestone, 2026-W43..W45, is in no tracker here: its name
	// still dates it, so 01.1 runs the next period, W46..W48 (GH-117).
	if c.Latest != "01.0" || c.Version != "01.1" || c.Tag != "corpus/v0.1.0" || c.EndDate != "2026-11-27" {
		t.Fatalf("recommendations = %s %s %s %s", c.Latest, c.Version, c.Tag, c.EndDate)
	}
}

func TestPlanReleaseReturnsItemsWithoutEvidence(t *testing.T) {
	trackertest.Each(t, testPlanReleaseReturnsItemsWithoutEvidence)
}

func testPlanReleaseReturnsItemsWithoutEvidence(t *testing.T, p trackertest.Program) {
	dir := system(t)
	in, f := input(t, dir, p)
	c, err := MakeCandidates(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.WithoutEvidence) != 1 || c.WithoutEvidence[0].ID != "obj002-fast-scoring#R2.1" || !slices.Equal(c.WithoutEvidence[0].Missing, []string{"environment"}) {
		t.Fatalf("without evidence = %+v", c.WithoutEvidence)
	}
	planning, err := f.CreateIssue("g/system", tracker.NewIssue{Title: "Plan release 01.1"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := WriteRelease(dir, Release{Version: "01.1", Milestone: "01.1", Tag: "corpus/v0.1.0",
		Items: []string{"obj002-fast-scoring#R1.1", "obj002-fast-scoring#R2.1"}}, c, f, "g/system", planning.IID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.Release.Items, []string{"obj002-fast-scoring#R1.1"}) || len(w.Returned) != 1 {
		t.Fatalf("written = %+v", w)
	}
	notes, _ := f.ListNotes("g/system", planning.IID)
	if len(notes) != 1 || !strings.Contains(notes[0].Body, "obj002-fast-scoring#R2.1: missing environment") {
		t.Fatalf("notes = %+v", notes)
	}
	if _, err := WriteRelease(dir, Release{Version: "01.1", Milestone: "01.1", Tag: "t", Items: []string{"obj009-x#R1.1"}}, c, nil, "", 0); err == nil {
		t.Fatal("an item of no agreed objective document was placed")
	}
}

func TestPlanReleaseWritesTheRoadMapRelease(t *testing.T) {
	trackertest.Each(t, testPlanReleaseWritesTheRoadMapRelease)
}

func testPlanReleaseWritesTheRoadMapRelease(t *testing.T, p trackertest.Program) {
	dir := system(t)
	in, _ := input(t, dir, p)
	c, err := MakeCandidates(in)
	if err != nil {
		t.Fatal(err)
	}
	readBack := func() []goals.Release {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, RoadMap))
		if err != nil {
			t.Fatal(err)
		}
		d, err := corpus.ParseMarkdown(b)
		if err != nil {
			t.Fatal(err)
		}
		return goals.ReleasesOf(d)
	}
	r := Release{Version: "01.1", Milestone: "01.1", Tag: "corpus/v0.1.0", Items: []string{"obj001-reliable-agents#R2.1", "obj002-fast-scoring#R1.1"}}
	if w, err := WriteRelease(dir, r, c, nil, "", 0); err != nil || w.Action != "added" {
		t.Fatalf("first write: %+v %v", w, err)
	}
	got := readBack()
	if len(got) != 2 || got[0].Version != "01.0" || got[1].Version != "01.1" || got[1].Tag != "corpus/v0.1.0" || got[1].Milestone != "01.1" ||
		!slices.Equal(got[1].Objectives, r.Items) || len(got[0].Objectives) != 3 {
		t.Fatalf("road map = %+v", got)
	}
	r.Items = []string{"obj002-fast-scoring#R1.1"}
	if w, err := WriteRelease(dir, r, c, nil, "", 0); err != nil || w.Action != "replaced" {
		t.Fatalf("second write: %+v %v", w, err)
	}
	if got := readBack(); len(got) != 2 || !slices.Equal(got[1].Objectives, r.Items) || got[0].Tag != "v0.1.0" {
		t.Fatalf("after replacing: %+v", got)
	}
	// A person's own sentence naming the tag moves with the republish too
	// (GH-20).
	b0, _ := os.ReadFile(filepath.Join(dir, RoadMap))
	b0 = []byte(strings.Replace(string(b0), "tagged corpus/v0.1.0.\n", "tagged corpus/v0.1.0.\n\nThe team reviews corpus/v0.1.0 before the demo.\n", 1))
	if err := os.WriteFile(filepath.Join(dir, RoadMap), b0, 0o644); err != nil {
		t.Fatal(err)
	}
	// A republish moves the tag; the sentence scrum-master wrote moves with it, so
	// the old tag is in neither prose nor fields (#188).
	r.Tag = "corpus/v0.1.1"
	if w, err := WriteRelease(dir, r, c, nil, "", 0); err != nil || w.Action != "replaced" {
		t.Fatalf("republish write: %+v %v", w, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, RoadMap))
	if got := readBack(); got[1].Tag != "corpus/v0.1.1" || strings.Contains(string(b), "corpus/v0.1.0") || !strings.Contains(string(b), "tagged corpus/v0.1.1.") ||
		!strings.Contains(string(b), "The team reviews corpus/v0.1.1 before the demo.") {
		t.Fatalf("after the republish write the road map still names the old tag:\n%s", b)
	}
}

// initRoadMap is the road map scrum-master init lays down: release 01.0 with prose
// and srds, and no deliverables or publication yet.
const initRoadMap = `---
id: program-roadmap
title: Program Road Map
---

<!-- field: overview -->
## Overview

The technical program manager writes here the system's releases.

<!-- release: 01.0 | status: specified -->
## 01.0 - First release

The technical program manager writes here what the first release proves, after planning it with plan.

<!-- field: srds -->
### SRDs

- srd001-system
`

// TestPlanAFreshProgram plans the release scrum-master init laid down: candidates
// read its road map without failing and recommend 01.0 itself, and plan
// write keeps its prose and srds (srd031 R3.1, R4.1).
func TestPlanAFreshProgram(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir := system(t)
		if err := os.WriteFile(filepath.Join(dir, RoadMap), []byte(initRoadMap), 0o644); err != nil {
			t.Fatal(err)
		}
		in, _ := input(t, dir, p)
		c, err := MakeCandidates(in)
		if err != nil || c.Version != "01.0" || c.Latest != "" || len(c.Items) == 0 {
			t.Fatalf("candidates = version %q latest %q items %d, %v", c.Version, c.Latest, len(c.Items), err)
		}
		r := Release{Version: "01.0", Milestone: "01.0", Tag: c.Tag, Items: []string{c.Items[0].ID}}
		if w, err := WriteRelease(dir, r, c, nil, "", 0); err != nil || w.Action != "replaced" {
			t.Fatalf("write: %+v %v", w, err)
		}
		got := readFile(t, filepath.Join(dir, RoadMap))
		for _, want := range []string{"## 01.0 - First release", "Release 01.0 delivers " + c.Items[0].ID, "<!-- field: srds -->\n### SRDs\n\n- srd001-system",
			"<!-- field: deliverables -->", "- " + c.Items[0].ID, "- tag: " + c.Tag, overviewFill} {
			if !strings.Contains(got, want) {
				t.Errorf("road map lacks %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "writes here") {
			t.Errorf("a placeholder sentence survived the write:\n%s", got)
		}
		// The parent schema's order, which scrum-master doctor checks: deliverables,
		// then srds, then release (#148).
		if strings.Index(got, "### Deliverables") > strings.Index(got, "### SRDs") || strings.Index(got, "### SRDs") > strings.Index(got, "### Release") {
			t.Errorf("field order: want deliverables, srds, release:\n%s", got)
		}
		d, err := corpus.ParseMarkdown([]byte(got))
		if err != nil {
			t.Fatal(err)
		}
		if rs := goals.ReleasesOf(d); len(rs) != 1 || rs[0].Tag != c.Tag || !slices.Equal(rs[0].Objectives, r.Items) {
			t.Fatalf("releases = %+v", rs)
		}
		again, err := MakeCandidates(in)
		if err != nil || again.Version != "01.1" || again.Latest != "01.0" {
			t.Fatalf("after planning 01.0: version %q latest %q, %v", again.Version, again.Latest, err)
		}
		// A new record is whole: status, title, prose, then deliverables,
		// the last record's srds, and release.
		next := Release{Version: "01.1", Milestone: "01.1", Tag: "corpus/v0.2.0", Items: []string{again.Items[0].ID}}
		if w, err := WriteRelease(dir, next, again, nil, "", 0); err != nil || w.Action != "added" {
			t.Fatalf("write 01.1: %+v %v", w, err)
		}
		got = readFile(t, filepath.Join(dir, RoadMap))
		added := got[strings.Index(got, "<!-- release: 01.1"):]
		for _, want := range []string{"<!-- release: 01.1 | status: specified -->\n## 01.1 - Release 01.1\n\nRelease 01.1 delivers " + again.Items[0].ID + ", on milestone 01.1, tagged corpus/v0.2.0.\n",
			"<!-- field: srds -->\n### SRDs\n\n- srd001-system\n", "<!-- field: deliverables -->", "- tag: corpus/v0.2.0"} {
			if !strings.Contains(added, want) {
				t.Errorf("the 01.1 record lacks %q:\n%s", want, added)
			}
		}
		if strings.Index(added, "### Deliverables") > strings.Index(added, "### SRDs") || strings.Index(added, "### SRDs") > strings.Index(added, "### Release") {
			t.Errorf("01.1 field order: want deliverables, srds, release:\n%s", added)
		}
	})
}

func TestPeriodStartsOnTheFirstWorkingDay(t *testing.T) {
	for end, want := range map[string]string{"2026-11-06": "2026-10-19", "2026-11-27": "2026-11-09", "2026-11-08": "2026-10-19"} {
		e, _ := time.Parse(time.DateOnly, end)
		if got := periodStart(e).Format(time.DateOnly); got != want {
			t.Errorf("periodStart(%s) = %s, want %s", end, got, want)
		}
	}
}

func TestTeamRepositoryIsKnownByItsSubsystemSRD(t *testing.T) {
	dir := t.TempDir()
	if teamRepository(dir) {
		t.Fatal("an empty checkout is a team repository")
	}
	srds := filepath.Join(dir, "docs", "specs", "software-requirements")
	os.MkdirAll(srds, 0o755)
	os.WriteFile(filepath.Join(srds, "srd001-subsystem.md"), []byte("x"), 0o644)
	if !teamRepository(dir) {
		t.Fatal("a checkout with srd001-subsystem.md is not a team repository")
	}
}

// TestEndDateIsTheLastWorkingDay recommends the Friday of the period's
// third ISO week: from the Monday after the last milestone ends, or from
// the current week, the next one on a weekend (#148).
func TestEndDateIsTheLastWorkingDay(t *testing.T) {
	for _, c := range []struct{ now, want string }{
		{"2026-10-20", "2026-11-06"}, // a Tuesday of W43: W43 to W45
		{"2026-10-19", "2026-11-06"}, // its Monday
		{"2026-10-18", "2026-11-06"}, // the Sunday before: the period starts the next day
		{"2026-10-24", "2026-11-13"}, // a Saturday: the next week
	} {
		now, _ := time.Parse(time.DateOnly, c.now)
		if got := endDate(Input{Now: now}, nil, nil); got != c.want {
			t.Errorf("end date planned on %s = %s, want %s", c.now, got, c.want)
		}
		end, _ := time.Parse(time.DateOnly, c.want)
		if start := periodStart(end); start.Weekday() != time.Monday || start.AddDate(0, 0, periodDays-3) != end {
			t.Errorf("periodStart(%s) = %s does not read the period back", c.want, start.Format(time.DateOnly))
		}
	}
}

// The system given as a checkout's path, such as the worktree a plan is
// written in, dates the next period from the last release's milestone as
// its project path does, not from the current week (GH-141).
func TestEndDateFromASystemCheckoutPath(t *testing.T) {
	f := fake.New()
	f.Milestones["01.0"] = tracker.Milestone{Title: "01.0", StartDate: "2026-10-19", DueDate: "2026-11-06"}
	wt := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://example.com/g/system.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", wt}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	now, _ := time.Parse(time.DateOnly, "2026-10-26")
	releases := []goals.Release{{Version: "01.0", Milestone: "01.0"}}
	for _, system := range []string{"g/system", wt} {
		if got := endDate(Input{Now: now, Tracker: f, System: system}, releases, nil); got != "2026-11-27" {
			t.Errorf("system %s: end %s, want 2026-11-27", system, got)
		}
	}
}

// A team's release is the system's open one, the first its road map has
// not released, read from the system's checkout path or its project, never
// the 01.0 init lays down in the team's own road map (GH-141).
func TestTeamTakesTheSystemsOpenRelease(t *testing.T) {
	roadMap := "---\nid: program-roadmap\ntitle: Program road map\n---\n<!-- release: 01.0 | status: released -->\n## 01.0\n\n<!-- release: 01.1 | status: specified -->\n## 01.1\n"
	sys := t.TempDir()
	os.MkdirAll(filepath.Join(sys, "docs"), 0o755)
	if err := os.WriteFile(filepath.Join(sys, RoadMap), []byte(roadMap), 0o644); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.AddFile("g/system", f.Default, RoadMap, []byte(roadMap))
	for _, system := range []string{sys, "g/system"} {
		if got := openRelease(f, system); got != "01.1" {
			t.Errorf("system %s: release %q, want 01.1", system, got)
		}
	}
	if got := openRelease(f, ""); got != "" {
		t.Errorf("no system: release %q, want none", got)
	}
}

// Items agreed due on the Friday that ends a period already begun end the
// recommended period then, not a week or two later, on a weekend or any
// weekday of the period; a due date that ends no whole period, or is past,
// changes nothing (GH-66, GH-92).
func TestEndDateKeepsTheItemsAgreedDue(t *testing.T) {
	sat, _ := time.Parse(time.DateOnly, "2026-10-24") // a Saturday: the next week would start the period
	for _, c := range []struct {
		due, want string
	}{
		{"2026-11-06", "2026-11-06"}, // the Friday ending W43 to W45, begun 10-19
		{"2026-11-05", "2026-11-13"}, // a Thursday ends no period
		{"2026-10-30", "2026-10-30"}, // ends W42 to W44, begun the week before and still running
		{"2026-10-23", "2026-11-13"}, // past
		{"", "2026-11-13"},
	} {
		items := []Item{{ID: "obj001-x#R1.1", Due: c.due}}
		if got := endDate(Input{Now: sat}, nil, items); got != c.want {
			t.Errorf("items due %q: end %s, want %s", c.due, got, c.want)
		}
	}
	for _, now := range []string{"2026-10-28", "2026-11-02", "2026-11-06"} { // weekdays of W44 and W45, inside W43 to W45
		day, _ := time.Parse(time.DateOnly, now)
		if got := endDate(Input{Now: day}, nil, []Item{{ID: "obj001-x#R1.1", Due: "2026-11-06"}}); got != "2026-11-06" {
			t.Errorf("planned on %s with items due 2026-11-06: end %s", now, got)
		}
	}
}

// TestOpenSprintGoalSlips ranks an outcome slipped when the period ended
// with its Sprint Goal neither met nor reported complete, though no report
// raised a slipped signal, and leaves a met one unslipped (#148).
func TestOpenSprintGoalSlips(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		in, _ := input(t, system(t), p)
		ref := in.Aggregate.Signals[0].Subject
		in.Aggregate = &reports.Aggregation{Progress: []reports.Progress{{SprintGoal: ref, State: "at-risk"}}}
		c, err := MakeCandidates(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Items) == 0 || c.Items[0].ID != "obj001-reliable-agents#R2.1" || !c.Items[0].Slipped {
			t.Fatalf("items = %+v, want obj001's R2.1 slipped first", c.Items)
		}
		in.Aggregate.Progress[0].State = "met"
		if c, err = MakeCandidates(in); err != nil {
			t.Fatal(err)
		}
		for _, it := range c.Items {
			if it.Slipped {
				t.Fatalf("a met Sprint Goal's outcome slipped: %+v", it)
			}
		}
	})
}

// TestSlippedIsPerItem is srd031 R2.3 per item: an open Sprint Goal that
// serves an outcome and cites one of its items slips that item only, so an
// item of the same outcome another goal delivered is not carried over.
func TestSlippedIsPerItem(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		in, f := input(t, system(t), p)
		goal, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal: reliable agents",
			Description: "Goal\n\nCites: https://gitlab.example/g/system/-/blob/corpus/v0.1.0/docs/specs/objectives/obj001-reliable-agents.md#R1.1\n\n" +
				sprintgoal.Marker("01.0", []string{"obj001-reliable-agents#R1"}) + "\n"})
		if err != nil {
			t.Fatal(err)
		}
		in.Aggregate = &reports.Aggregation{Progress: []reports.Progress{{SprintGoal: fmt.Sprintf("g/po-1#%d", goal.IID), State: "at-risk"}}}
		c, err := MakeCandidates(in)
		if err != nil {
			t.Fatal(err)
		}
		slipped := map[string]bool{}
		for _, it := range c.Items {
			slipped[it.ID] = it.Slipped
		}
		if !slipped["obj001-reliable-agents#R1.1"] {
			t.Fatalf("the cited item did not slip: %+v", c.Items)
		}
		if _, back := slipped["obj001-reliable-agents#R1.2"]; back {
			t.Fatalf("an item of the same outcome the goal did not cite came back: %+v", c.Items)
		}
	})
}

// plan write refuses a release whose items' documents cite a component
// ARCHITECTURE does not declare, naming it as a design decision to make
// first, and writes once ARCHITECTURE declares it (GH-58).
func TestPlanWriteStopsForADesignDecision(t *testing.T) {
	dir := system(t)
	obj := filepath.Join(dir, "docs/specs/objectives/obj001-reliable-agents.md")
	b, err := os.ReadFile(obj)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	i := strings.Index(text, "<!-- field: problem")
	if i < 0 {
		t.Fatal("the fixture has no problem field")
	}
	os.WriteFile(obj, []byte(strings.TrimRight(text[:i], "\n")+"\n- architecture:\n  1. scorer\n\n"+text[i:]), 0o644)
	err = DesignGaps(dir, []string{"obj001-reliable-agents#R1.1"})
	if err == nil || !strings.Contains(err.Error(), "this needs a design decision: scorer (cited by obj001-reliable-agents)") || !strings.Contains(err.Error(), "design skill") {
		t.Fatalf("a plan citing an undeclared component = %v", err)
	}
	if err := DesignGaps(dir, []string{"obj002-fast-scoring#R1.1"}); err != nil {
		t.Fatalf("a plan whose items cite nothing undeclared = %v", err)
	}
	os.WriteFile(filepath.Join(dir, "docs/ARCHITECTURE.md"), []byte("---\nid: architecture-x\ntitle: X\n---\n\n<!-- field: components -->\n## Components\n\n<!-- component: scorer -->\n### scorer\n\nScores.\n"), 0o644)
	if err := DesignGaps(dir, []string{"obj001-reliable-agents#R1.1"}); err != nil {
		t.Fatalf("once ARCHITECTURE declares the component = %v", err)
	}
}

// The design stop reads the epics under a planned item's document too: an
// epic under the objective citing an undeclared component stops the plan,
// though the objective itself cites nothing (GH-90).
func TestPlanWriteStopsForAnEpicsDesignDecision(t *testing.T) {
	dir := system(t)
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "docs", "specs", "epics", "ep001-publish-scorecards.md"))
	if err != nil {
		t.Fatal(err)
	}
	epic := strings.NewReplacer("parent: in001-reliable-agents", "parent: obj001-reliable-agents", "1. publisher", "1. spend_control").Replace(string(b))
	os.MkdirAll(filepath.Join(dir, "docs", "specs", "epics"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "docs", "specs", "epics", "ep001-publish-scorecards.md"), []byte(epic), 0o644); err != nil {
		t.Fatal(err)
	}
	err = DesignGaps(dir, []string{"obj001-reliable-agents#R1.1"})
	if err == nil || !strings.Contains(err.Error(), "spend_control (cited by ep001-publish-scorecards)") {
		t.Fatalf("a plan whose epic cites an undeclared component = %v", err)
	}
	if err := DesignGaps(dir, []string{"obj002-fast-scoring#R1.1"}); err != nil {
		t.Fatalf("a plan of another objective = %v", err)
	}
}

// plan write stops on a component the architecture moved that only a
// document not yet agreed plans, and goes on once it is agreed (GH-100).
func TestPlanWriteWaitsForTheAgreement(t *testing.T) {
	dir := system(t)
	os.WriteFile(filepath.Join(dir, "docs/ARCHITECTURE.md"), []byte("---\nid: architecture-x\ntitle: X\n---\n\n<!-- field: components -->\n## Components\n\n<!-- component: spend_control -->\n### spend_control\n\nControls spend.\n"), 0o644)
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "docs", "specs", "epics", "ep001-publish-scorecards.md"))
	if err != nil {
		t.Fatal(err)
	}
	epic := strings.NewReplacer("ep001-publish-scorecards", "ep011-spend-control", "parent: in001-reliable-agents", "parent: obj001-reliable-agents", "1. publisher", "1. spend_control", "status: agreed", "status: to_be_agreed").Replace(string(b))
	os.MkdirAll(filepath.Join(dir, "docs", "specs", "epics"), 0o755)
	path := filepath.Join(dir, "docs", "specs", "epics", "ep011-spend-control.md")
	os.WriteFile(path, []byte(epic), 0o644)
	err = DesignGaps(dir, []string{"obj001-reliable-agents#R1.1"})
	if err == nil || !strings.Contains(err.Error(), "this plan moves spend_control (cited by ep011-spend-control, to_be_agreed)") {
		t.Fatalf("a plan on an epic not agreed = %v", err)
	}
	os.WriteFile(path, []byte(strings.Replace(epic, "status: to_be_agreed", "status: agreed", 1)), 0o644)
	if err := DesignGaps(dir, []string{"obj001-reliable-agents#R1.1"}); err != nil {
		t.Fatalf("once the epic is agreed = %v", err)
	}
}
