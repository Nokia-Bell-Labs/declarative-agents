// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func day(offset int) string { return time.Now().AddDate(0, 0, offset).Format("2006-01-02") }

// fileWith runs scrum-master issue file for a data issue against f and returns
// the result and the filed issue.
func fileWith(t *testing.T, f *fake.Fake, extra ...string) (FileResult, tracker.Issue) {
	t.Helper()
	body := filepath.Join(t.TempDir(), "issue.md")
	os.WriteFile(body, []byte(goodBody), 0o644)
	args := append([]string{"issue", "file", "--json", "--project", "g/po-1", "--title", "Score one agent",
		"--body-file", body, "--plane", "data", "--kind", "build", "--component", "po-1"}, extra...)
	var out bytes.Buffer
	if err := cli.Default.Run(args, &out, func() (tracker.Tracker, error) { return f, nil }); err != nil {
		t.Fatalf("file: %v", err)
	}
	var r FileResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	i, err := f.GetIssue("g/po-1", r.IID)
	if err != nil {
		t.Fatal(err)
	}
	return r, i
}

func TestFileSetsActiveSprintMilestone(t *testing.T) {
	f := fake.New()
	f.Milestones["sprint-41"] = tracker.Milestone{Title: "sprint-41", StartDate: day(-20), DueDate: day(-6)}
	f.Milestones["sprint-42"] = tracker.Milestone{Title: "sprint-42", StartDate: day(-5), DueDate: day(9)}
	f.Milestones["no-dates"] = tracker.Milestone{Title: "no-dates"}
	r, i := fileWith(t, f)
	if r.Milestone != "sprint-42" || i.Milestone != "sprint-42" || r.MilestoneReason != "" {
		t.Fatalf("result %+v, issue milestone %q", r, i.Milestone)
	}
	if !r.Lint.Admitted {
		t.Fatalf("not admitted: %v", r.Lint.Reasons)
	}
}

func TestFileLeavesMilestoneWhenNoneOrSeveralActive(t *testing.T) {
	f := fake.New()
	r, i := fileWith(t, f)
	if r.Milestone != "" || i.Milestone != "" || !strings.Contains(r.MilestoneReason, "no active milestone") {
		t.Fatalf("none: %+v", r)
	}
	if r.Lint.Admitted {
		t.Fatal("admitted a data issue without a sprint")
	}

	f.Milestones["a"] = tracker.Milestone{Title: "a", StartDate: day(-3), DueDate: day(3)}
	f.Milestones["b"] = tracker.Milestone{Title: "b", StartDate: day(-1), DueDate: day(10)}
	r, i = fileWith(t, f)
	if r.Milestone != "" || i.Milestone != "" || !strings.Contains(r.MilestoneReason, "several active milestones cover") || !strings.Contains(r.MilestoneReason, "a, b") {
		t.Fatalf("several: %+v", r)
	}
}

func TestFileExplicitMilestoneWins(t *testing.T) {
	f := fake.New()
	f.Milestones["sprint-42"] = tracker.Milestone{Title: "sprint-42", StartDate: day(-5), DueDate: day(9)}
	f.Milestones["later"] = tracker.Milestone{Title: "later"}
	r, i := fileWith(t, f, "--milestone", "later")
	if r.Milestone != "later" || i.Milestone != "later" || r.MilestoneReason != "" {
		t.Fatalf("result %+v, issue milestone %q", r, i.Milestone)
	}
}

func TestFileReportsComponent(t *testing.T) {
	f := fake.New()
	r, i := fileWith(t, f)
	if r.Component != "po-1" || !i.HasLabel("component::po-1") {
		t.Fatalf("given: component %q, labels %v", r.Component, i.Labels)
	}

	f.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", []byte(subsystemFixture))
	body := filepath.Join(t.TempDir(), "issue.md")
	os.WriteFile(body, []byte(goodBody), 0o644)
	var out bytes.Buffer
	err := cli.Default.Run([]string{"issue", "file", "--json", "--project", "g/product-owners/po-1", "--system", "g/system",
		"--title", "Score one agent", "--body-file", body, "--plane", "data", "--kind", "build"},
		&out, func() (tracker.Tracker, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Component != "po-1" {
		t.Fatalf("derived: %+v %v", r, err)
	}
}

// TestFileAfterSprintEndTakesTheNextMilestone files work on the next
// milestone once the sprint's sprint-end issue has closed, though the
// ended milestone's dates still cover today (#150).
func TestFileAfterSprintEndTakesTheNextMilestone(t *testing.T) {
	f := fake.New()
	f.Milestones["01.0"] = tracker.Milestone{Title: "01.0", StartDate: day(-10), DueDate: day(2)}
	f.Milestones["01.1"] = tracker.Milestone{Title: "01.1", StartDate: day(5), DueDate: day(25)}
	f.Milestones["01.2"] = tracker.Milestone{Title: "01.2", StartDate: day(26), DueDate: day(46)}
	end, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: sprint end 01.0", Milestone: "01.0",
		Labels: []string{"plane::data", "type::recurring", "kind::release"}})
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := fileWith(t, f); r.Milestone != "01.0" {
		t.Fatalf("before the sprint ends: milestone %q, want 01.0", r.Milestone)
	}
	closed := "close"
	if _, err := f.UpdateIssue("g/po-1", end.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	r, i := fileWith(t, f)
	if r.Milestone != "01.1" || i.Milestone != "01.1" {
		t.Fatalf("after the sprint ends: %+v, issue milestone %q; want 01.1", r, i.Milestone)
	}
}

// A project with no sprint-end issue of its own, such as a reports
// repository, takes the next milestone once the system project's release
// issue for the sprint has closed: the release is cut, the sprint over
// (GH-141).
func TestSprintEndsWhenItsReleaseIsCut(t *testing.T) {
	f := fake.New()
	f.Milestones["01.0"] = tracker.Milestone{Title: "01.0", StartDate: day(-10), DueDate: day(2)}
	f.Milestones["01.1"] = tracker.Milestone{Title: "01.1", StartDate: day(5), DueDate: day(25)}
	release, err := f.CreateIssue("g/system", tracker.NewIssue{Title: "Release corpus 0.47.0", Milestone: "01.0",
		Labels: []string{"plane::data", "kind::release"}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if got, _, err := SprintMilestone(f, "g/reports", "g/system", now, false); err != nil || got != "01.0" {
		t.Fatalf("before the release: %q, %v; want 01.0", got, err)
	}
	closed := "close"
	if _, err := f.UpdateIssue("g/system", release.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	if got, _, err := SprintMilestone(f, "g/reports", "g/system", now, false); err != nil || got != "01.1" {
		t.Fatalf("after the release: %q, %v; want 01.1", got, err)
	}
	if got, _, err := SprintMilestone(f, "g/reports", "g/system", now, true); err != nil || got != "01.0" {
		t.Fatalf("an issue cutting the release: %q, %v; want 01.0", got, err)
	}
}

// TestAuditForARepositoryWithoutMage runs scrum-master backlog check where the
// built-in mage audit could not run, and keeps a policy's own command.
func TestAuditForARepositoryWithoutMage(t *testing.T) {
	dir := t.TempDir()
	if got := AuditFor(Defaults(), dir); got != BacklogAudit {
		t.Fatalf("no magefiles: %q, want %q", got, BacklogAudit)
	}
	if err := os.MkdirAll(filepath.Join(dir, "magefiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := AuditFor(Defaults(), dir); got != "mage audit" {
		t.Fatalf("with magefiles: %q, want mage audit", got)
	}
	own := Defaults()
	own.AuditCommand = "make check"
	if got := AuditFor(own, t.TempDir()); got != "make check" {
		t.Fatalf("the policy's own command: %q", got)
	}
}

// TestReleaseIssueKeepsItsEndedSprint is srd013 R3.3's exception: after a
// sprint's sprint-end merged and before a later milestone is dated, an
// issue that cuts the release (kind::release) is filed on the ended
// sprint's milestone, while other work still is not.
func TestReleaseIssueKeepsItsEndedSprint(t *testing.T) {
	f := fake.New()
	f.Milestones["01.0"] = tracker.Milestone{Title: "01.0", StartDate: day(-10), DueDate: day(2)}
	end, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: sprint end 01.0", Milestone: "01.0",
		Labels: []string{"plane::data", "type::recurring", "kind::release"}})
	if err != nil {
		t.Fatal(err)
	}
	closed := "close"
	if _, err := f.UpdateIssue("g/po-1", end.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	if r, i := fileWith(t, f, "--kind", "release"); r.Milestone != "01.0" || i.Milestone != "01.0" || !r.Lint.Admitted {
		t.Fatalf("release issue after its sprint ended: %+v, issue milestone %q; want 01.0, admitted", r, i.Milestone)
	}
	if r, i := fileWith(t, f); r.Milestone != "" || i.Milestone != "" || r.Lint.Admitted {
		t.Fatalf("build issue after the sprint ended with no later milestone: %+v; want no milestone, not admitted", r)
	}
}

// issue milestone with no milestone named puts the issue in the active
// sprint's, as the admit profile says an empty one means (GH-68).
func TestMilestoneDefaultsToTheActiveSprint(t *testing.T) {
	f := fake.New()
	f.Milestones["sprint-41"] = tracker.Milestone{Title: "sprint-41", StartDate: day(-20), DueDate: day(-6)}
	f.Milestones["sprint-42"] = tracker.Milestone{Title: "sprint-42", StartDate: day(-5), DueDate: day(9)}
	_, i := fileWith(t, f)
	var out bytes.Buffer
	if err := cli.Default.Run([]string{"issue", "milestone", "--json", "--project", "g/po-1", "--issue", fmt.Sprint(i.IID)}, &out,
		func() (tracker.Tracker, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.GetIssue("g/po-1", i.IID); got.Milestone != "sprint-42" {
		t.Fatalf("milestone %q, want the active sprint-42", got.Milestone)
	}
}
