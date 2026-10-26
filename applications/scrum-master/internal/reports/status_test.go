// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

const po1 = "g/product-owners/po-1"

func statusFixture(p trackertest.Program) (tracker.Tracker, StatusInput) {
	p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", []byte(`---
id: srd010-po-1
title: "Subsystem: po-1"
---

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, po-1

<!-- req: R1 -->
## R1 - Receive

<!-- req: R1.1 -->
### R1.1

Objective issues arrive.

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| po-1 | https://gitlab.example.com/g/product-owners/po-1 | / |
`))
	doc, _ := configDocument(Config{Team: []Member{{Username: "po-1", Project: po1}}, System: "g/system"})
	cfg, _ := ParseConfig([]byte(doc))
	return p.Tracker(), StatusInput{Config: cfg, Reports: "g/reports", Policy: program.Defaults(), Ref: "main", Now: monday}
}

// issueCount counts project's issues, open or closed.
func issueCount(t *testing.T, f tracker.Tracker, project string) int {
	t.Helper()
	all, err := f.ListIssues(project, tracker.IssueQuery{State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return len(all)
}

func TestStatusIssueDueAfterReminderWeekdayMentionsOwner(t *testing.T) {
	trackertest.Each(t, testStatusIssueDueAfterReminderWeekdayMentionsOwner)
}

func testStatusIssueDueAfterReminderWeekdayMentionsOwner(t *testing.T, p trackertest.Program) {
	f, in := statusFixture(p)
	res, err := StatusIssues(f, in)
	if err != nil || len(res) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	r := res[0]
	if r.Action != "filed" || r.Week != "2026-W42" || r.DueDate != "2026-10-15" || !r.Admitted {
		t.Fatalf("result %+v", r)
	}
	i, _ := f.GetIssue(po1, r.IID)
	if i.Title != "Recurring: weekly status 2026-W42" || i.DueDate != "2026-10-15" || len(i.Assignees) != 0 {
		t.Fatalf("issue %+v", i)
	}
	for _, l := range []string{"plane::data", "type::recurring", "kind::review", "component::po-1", "op::status"} {
		if !slices.Contains(i.Labels, l) {
			t.Errorf("labels %v lack %s", i.Labels, l)
		}
	}
	for _, want := range []string{"@po-1:", "## Requirements", "the status skill for " + po1 + " as po-1", "reports/2026-Q4/2026-W42-po-1.md",
		"for project g/reports with issue_project " + po1, "reports-validate skill", "test: reports_validate", "## Acceptance Criteria", "## Recurrence", "day after Wednesday", "## Previous Runs"} {
		if !strings.Contains(i.Description, want) {
			t.Errorf("body lacks %q", want)
		}
	}
	// The body names skills, never a command line (GH-65).
	for _, line := range strings.Split(i.Description, "\n") {
		if !strings.HasPrefix(line, "<!--") && (strings.Contains(line, "scrum-master ") || strings.Contains(line, "tpm")) {
			t.Errorf("the status body names a command: %s", line)
		}
	}

	friday := monday.AddDate(0, 0, 4)
	if got := TargetWeek(friday, time.Wednesday); ISOWeek(got) != "2026-W43" || DueDate(got, time.Wednesday).Format("2006-01-02") != "2026-10-22" {
		t.Fatalf("after the due date the target is next week: %s", ISOWeek(got))
	}
	if d := DueDate(WeekStart(monday), time.Sunday).Format("2006-01-02"); d != "2026-10-19" {
		t.Fatalf("a Sunday reminder is due Monday: %s", d)
	}
}

func TestStatusIssuesIdempotentPerWeek(t *testing.T) {
	trackertest.Each(t, testStatusIssuesIdempotentPerWeek)
}

func testStatusIssuesIdempotentPerWeek(t *testing.T, p trackertest.Program) {
	f, in := statusFixture(p)
	first, _ := StatusIssues(f, in)
	again, err := StatusIssues(f, in)
	if err != nil || again[0].Action != "exists" || again[0].IID != first[0].IID || issueCount(t, f, po1) != 1 {
		t.Fatalf("second run: %+v err=%v issues=%d", again, err, issueCount(t, f, po1))
	}

	f.UpdateIssue(po1, first[0].IID, tracker.IssueUpdate{State: ptr("close")})
	in.Now = monday.AddDate(0, 0, 7)
	next, err := StatusIssues(f, in)
	if err != nil || next[0].Action != "recurred" || next[0].Week != "2026-W43" || next[0].DueDate != "2026-10-22" {
		t.Fatalf("next week: %+v err=%v", next, err)
	}
	i, _ := f.GetIssue(po1, next[0].IID)
	if i.Title != "Recurring: weekly status 2026-W43" || !strings.Contains(i.Description, "reports/2026-Q4/2026-W43-po-1.md") ||
		!strings.Contains(i.Description, "- "+po1+"#1 (due 2026-10-15): closed by its merge.") ||
		!strings.Contains(i.Description, "week=2026-W43 user=po-1") || strings.Contains(i.Description, "week=2026-W42") {
		t.Fatalf("recurred issue:\n%s", i.Description)
	}
	old, _ := f.GetIssue(po1, first[0].IID)
	if old.State != "closed" {
		t.Fatal("the closed instance changed state")
	}
	if again, _ := StatusIssues(f, in); again[0].Action != "exists" || issueCount(t, f, po1) != 2 {
		t.Fatalf("second run next week: %+v", again)
	}
}

func ptr[T any](v T) *T { return &v }

func TestWeekListsOpenObjectivesAndActivity(t *testing.T) {
	trackertest.Each(t, testWeekListsOpenObjectivesAndActivity)
}

func testWeekListsOpenObjectivesAndActivity(t *testing.T, p trackertest.Program) {
	for _, mr := range []tracker.Merge{
		{IID: 1, Title: "Add grader notes", Author: "po-1", MergedAt: "2026-10-13"},
		{IID: 2, Title: "Earlier work", Author: "po-1", MergedAt: "2026-10-09"},
		{IID: 3, Title: "Someone else", Author: "po-2", MergedAt: "2026-10-13"},
	} {
		p.AddMerge(po1, mr)
	}
	f := p.Tracker()
	bt, isBeads := f.(*beads.Tracker)
	if isBeads {
		bt.Store.Now = func() time.Time { return monday.AddDate(0, 0, -7) } // the period began the week before
	}
	obj, _ := f.CreateIssue(po1, tracker.NewIssue{Title: "Score one agent", DueDate: "2026-11-06",
		Description: "Body\n\n<!-- scrum-master goals objective=obj001-a:R1 release=01.0 -->"})
	if isBeads {
		bt.Store.Now = time.Now
	}
	f.CreateIssue(po1, tracker.NewIssue{Title: "Ad hoc", Description: "not an objective"})
	done, _ := f.CreateIssue(po1, tracker.NewIssue{Title: "Grader notes"})
	if err := p.As("po-1").AssignIssue(po1, done.IID, tracker.User{Username: "po-1"}); err != nil {
		t.Fatal(err)
	}
	f.UpdateIssue(po1, done.IID, tracker.IssueUpdate{State: ptr("close")})
	// The status issue closes by a merge in the reports repository, which
	// this week's merges do not list, so it is not listed as closed (GH-24).
	status, _ := f.CreateIssue(po1, tracker.NewIssue{Title: "Recurring: weekly status 2026-W42", Labels: StatusLabels})
	if err := p.As("po-1").AssignIssue(po1, status.IID, tracker.User{Username: "po-1"}); err != nil {
		t.Fatal(err)
	}
	f.UpdateIssue(po1, status.IID, tracker.IssueUpdate{State: ptr("close")})
	r, err := Week(f, po1, "po-1", monday.AddDate(0, 0, 2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Week != "2026-W42" || r.Since != "2026-10-12" {
		t.Fatalf("week %s since %s", r.Week, r.Since)
	}
	if len(r.SprintGoals) != 1 || r.SprintGoals[0].Ref != po1+"#"+itoa(obj.IID) || r.SprintGoals[0].DueDate != "2026-11-06" {
		t.Fatalf("objectives %+v", r.SprintGoals)
	}
	if len(r.Merged) != 1 || r.Merged[0].IID != 1 || len(r.Closed) != 1 || r.Closed[0].Title != "Grader notes" {
		t.Fatalf("activity merged=%+v closed=%+v", r.Merged, r.Closed)
	}
}

// TestWeekCountsOnlyThePeriodsMerges leaves out a merge inside the week
// that landed before the period's first Sprint Goal existed, such as an
// earlier run's on the same days (#147).
func TestWeekCountsOnlyThePeriodsMerges(t *testing.T) {
	p := trackertest.NewBeads(t)
	p.AddMerge(po1, tracker.Merge{IID: 1, Title: "An earlier run's work", Author: "po-1", MergedAt: "2026-10-12"})
	p.AddMerge(po1, tracker.Merge{IID: 2, Title: "This period's work", Author: "po-1", MergedAt: "2026-10-14"})
	f := p.Tracker().(*beads.Tracker)
	f.Store.Now = func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) }
	if _, err := f.CreateIssue(po1, tracker.NewIssue{Title: "Score one agent", DueDate: "2026-11-06",
		Description: "Body\n\n<!-- scrum-master goals objective=obj001-a:R1 release=01.0 -->"}); err != nil {
		t.Fatal(err)
	}
	r, err := Week(f, po1, "po-1", monday.AddDate(0, 0, 3), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Merged) != 1 || r.Merged[0].Title != "This period's work" {
		t.Fatalf("merged %+v, want only this period's", r.Merged)
	}
}

// TestWeekOfTheSystemTakesTheTeamsPeriod is srd018 R8.1 for a project with
// no Sprint Goal of its own: the system repository's week starts at the
// teams' first Sprint Goal, so an earlier run's merges there do not count.
func TestWeekOfTheSystemTakesTheTeamsPeriod(t *testing.T) {
	const system = "g/system"
	p := trackertest.NewBeads(t)
	p.AddMerge(system, tracker.Merge{IID: 1, Title: "An earlier run's plan", Author: "scrum-master", MergedAt: "2026-10-12"})
	p.AddMerge(system, tracker.Merge{IID: 2, Title: "This period's registration", Author: "scrum-master", MergedAt: "2026-10-14"})
	f := p.Tracker().(*beads.Tracker)
	f.Store.Now = func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) }
	if _, err := f.CreateIssue(po1, tracker.NewIssue{Title: "Score one agent", DueDate: "2026-11-06",
		Description: "Body\n\n<!-- scrum-master goals objective=obj001-a:R1 release=01.0 -->"}); err != nil {
		t.Fatal(err)
	}
	r, err := Week(f, system, "scrum-master", monday.AddDate(0, 0, 3), []string{po1})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Merged) != 1 || r.Merged[0].Title != "This period's registration" {
		t.Fatalf("merged %+v, want only this period's", r.Merged)
	}
	if r, _ = Week(f, system, "scrum-master", monday.AddDate(0, 0, 3), nil); len(r.Merged) != 2 {
		t.Fatalf("without the teams, merged %+v; want the whole week", r.Merged)
	}
}

// TestWeekOfTheSystemCountsItsOwnWork is srd018 R8.1 for a system that
// holds issues but no Sprint Goal: its period starts at its own first
// issue, so its planning and registration merges count while an earlier
// run's do not, and an issue a counted merge closed is listed though nobody
// was assigned (#188).
func TestWeekOfTheSystemCountsItsOwnWork(t *testing.T) {
	const system = "g/system"
	p := trackertest.NewBeads(t)
	p.AddMerge(system, tracker.Merge{IID: 1, Title: "An earlier run's plan", Author: "scrum-master", MergedAt: "2026-10-12"})
	p.AddMerge(system, tracker.Merge{IID: 2, Title: "Register the teams", Author: "scrum-master", MergedAt: "2026-10-14", Description: "Closes g/system#2"})
	f := p.Tracker().(*beads.Tracker)
	f.Store.Now = func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) }
	if _, err := f.CreateIssue(system, tracker.NewIssue{Title: "Plan release 01.0"}); err != nil {
		t.Fatal(err)
	}
	registration, err := f.CreateIssue(system, tracker.NewIssue{Title: "Register po-1"})
	if err != nil {
		t.Fatal(err)
	}
	closed := "close"
	if _, err := f.UpdateIssue(system, registration.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	r, err := Week(f, system, "scrum-master", monday.AddDate(0, 0, 3), []string{po1})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Merged) != 1 || r.Merged[0].Title != "Register the teams" {
		t.Fatalf("merged %+v, want the period's own merge only", r.Merged)
	}
	if len(r.Closed) != 1 || r.Closed[0].Ref != fmt.Sprintf("%s#%d", system, registration.IID) {
		t.Fatalf("closed %+v, want the registration the counted merge closed", r.Closed)
	}
}
