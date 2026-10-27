// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

const (
	sys   = "program/system"
	fleet = "program/delivery/fleet"
)

// planned is a system checkout whose road map holds release 01.1, and a
// tracker with the fleet's subsystem SRD and its objective issue filed for
// release 01.0, whose IID it returns.
func planned(t *testing.T, p trackertest.Program) (PushInput, tracker.Tracker, int) {
	t.Helper()
	p.AddFile(sys, "main", "docs/specs/software-requirements/srd010-delivery.md", read(t, "../../testdata/system/srd010-delivery.md"))
	dir := system(t)
	in, f := input(t, dir, p)
	c, err := MakeCandidates(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRelease(dir, Release{Version: "01.1", Milestone: "01.1", Tag: "corpus/v0.1.0",
		Items: []string{"obj001-reliable-agents#R2.1", "obj002-fast-scoring#R1.1"}}, c, nil, "", 0); err != nil {
		t.Fatal(err)
	}
	objective, err := f.CreateIssue(fleet, tracker.NewIssue{Title: "Objective R2", Labels: []string{"plane::management"},
		Description: "Objective R2\n\n<!-- scrum-master goals objective=obj001-reliable-agents#R2 release=01.0 -->"})
	if err != nil {
		t.Fatal(err)
	}
	return PushInput{Dir: dir, System: sys, Group: "program", Version: "01.1", End: "2026-11-27",
		WebBase: "https://gitlab.example.com", Policy: program.Defaults(), Ref: "main"}, f, objective.IID
}

// issuesOf lists every issue of project, open or closed, by IID.
func issuesOf(t *testing.T, f tracker.Tracker, project string) []tracker.Issue {
	t.Helper()
	issues, err := f.ListIssues(project, tracker.IssueQuery{State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return issues
}

func describe(t *testing.T, f tracker.Tracker, project string, iid int, d string) {
	t.Helper()
	if _, err := f.UpdateIssue(project, iid, tracker.IssueUpdate{Description: &d}); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) []byte {
	t.Helper()
	return []byte(readFile(t, p))
}

func TestPushSprintUpdatesMilestoneAndChecklist(t *testing.T) {
	trackertest.Each(t, testPushSprintUpdatesMilestoneAndChecklist)
}

func testPushSprintUpdatesMilestoneAndChecklist(t *testing.T, p trackertest.Program) {
	in, f, _ := planned(t, p)
	res, err := Push(f, in)
	if err != nil {
		t.Fatal(err)
	}
	// With no repository to plan in, the lists print [], not null (GH-2781).
	none, err := postPlanned(f, in, Release{Version: "01.1"}, nil, nil, nil)
	if b, _ := json.Marshal(PushResult{Planned: none, SprintEnds: []IssueAction{}}); err != nil || !strings.Contains(string(b), `"planned_events":[]`) {
		t.Errorf("nothing planned = %s, %v", b, err)
	}
	if ends, err := pushSprintEnds(f, in, pushPlan{}); err != nil || ends == nil {
		t.Errorf("no sprint-end issue = %#v, %v; want an empty list", ends, err)
	}
	m, _ := f.FindMilestone(sys, "01.1")
	if res.Milestone != tracker.MilestoneCreated || m.DueDate != "2026-11-27" || m.StartDate != "2026-11-09" || m.Description != "Release 01.1, 2026-W46 to 2026-W48." {
		t.Fatalf("milestone %s %+v", res.Milestone, m)
	}
	sprint := issuesOf(t, f, sys)[0]
	if res.Sprint.Action != "filed" || !slices.Contains(sprint.Labels, "type::sprint") || !slices.Contains(sprint.Labels, "op::plan") || sprint.Milestone != "01.1" ||
		!strings.Contains(sprint.Description, "- [ ] obj001-reliable-agents#R2: A scorecard a product team reads\n- [ ] obj002-fast-scoring#R1: Score in an hour\n") {
		t.Fatalf("sprint issue %s:\n%s", res.Sprint.Action, sprint.Description)
	}
	// The lead ticks one objective; the period moves a week.
	describe(t, f, sys, sprint.IID, strings.Replace(sprint.Description, "- [ ] obj002", "- [x] obj002", 1))
	in.End = "2026-12-04"
	res, err = Push(f, in)
	if err != nil {
		t.Fatal(err)
	}
	after := issuesOf(t, f, sys)
	if res.Milestone != tracker.MilestoneUpdated || res.Sprint.Action != "updated" || len(after) != 1 ||
		!strings.Contains(after[0].Description, "- [x] obj002-fast-scoring#R1") || after[0].DueDate != "2026-12-04" {
		t.Fatalf("second push: %+v\n%s", res, after[0].Description)
	}
}

func TestPlanPushFilesSprintEndIssuesOnce(t *testing.T) {
	trackertest.Each(t, testPlanPushFilesSprintEndIssuesOnce)
}

func testPlanPushFilesSprintEndIssuesOnce(t *testing.T, p trackertest.Program) {
	in, f, objective := planned(t, p)
	res, err := Push(f, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SprintEnds) != 1 || res.SprintEnds[0].Project != fleet || res.SprintEnds[0].Action != "filed" || !res.SprintEnds[0].Admitted {
		t.Fatalf("sprint ends = %+v", res.SprintEnds)
	}
	end := issuesOf(t, f, fleet)[1]
	if end.DueDate != "2026-11-27" || end.Milestone != "01.1" || !slices.Contains(end.Labels, "component::fleet") ||
		!slices.Contains(end.Labels, "type::recurring") || !slices.Contains(end.Labels, "op::plan") || !strings.Contains(end.Description, "docs/sprint-ends/01.1.md") ||
		!strings.Contains(end.Description, "## Recurrence") {
		t.Fatalf("sprint-end issue %+v", end)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0], sys) {
		t.Fatalf("skipped = %v", res.Skipped)
	}
	planned := fmt.Sprintf("%s#%d", fleet, objective)
	if len(res.Planned) != 1 || res.Planned[0].Issue != planned || !res.Planned[0].Posted {
		t.Fatalf("planned = %+v", res.Planned)
	}
	notes, _ := f.ListNotes(fleet, objective)
	if len(notes) == 0 || !strings.Contains(notes[0].Body, "release=01.1") ||
		!strings.Contains(notes[0].Body, "/-/blob/corpus/v0.1.0/docs/specs/objectives/obj001-reliable-agents.md#R2.1") {
		t.Fatalf("planned event: %+v", notes)
	}
	// A tracker may store descriptions without the trailing newline.
	for _, project := range []string{sys, fleet} {
		for _, i := range issuesOf(t, f, project) {
			if d := strings.TrimRight(i.Description, "\n"); d != i.Description {
				describe(t, f, project, i.IID, d)
			}
		}
	}
	writes := p.Writes()
	res, err = Push(f, in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Writes() != writes || res.Milestone != tracker.MilestoneUnchanged || res.Sprint.Action != "unchanged" ||
		res.SprintEnds[0].Action != "unchanged" || res.Planned[0].Posted || len(issuesOf(t, f, fleet)) != 2 {
		t.Fatalf("second push wrote %d: %+v", p.Writes()-writes, res)
	}
}

// TestPushFilesTheSystemsSprintEnd files the system repository's own
// sprint-end issue once its system SRD declares it a component (srd043
// R1.1), with its component, and skips nothing.
func TestPushFilesTheSystemsSprintEnd(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		srd := strings.NewReplacer("srd010-delivery", "srd001-system", "| fleet |", "| system |",
			"/program/delivery/fleet", "/"+sys).Replace(string(read(t, "../../testdata/system/srd010-delivery.md")))
		p.AddFile(sys, "main", "docs/specs/software-requirements/srd001-system.md", []byte(srd))
		in, f, _ := planned(t, p)
		res, err := Push(f, in)
		if err != nil {
			t.Fatal(err)
		}
		var system *IssueAction
		for i := range res.SprintEnds {
			if res.SprintEnds[i].Project == sys {
				system = &res.SprintEnds[i]
			}
		}
		if len(res.Skipped) != 0 || system == nil || system.Action != "filed" || !system.Admitted {
			t.Fatalf("sprint-end issues %+v, skipped %v", res.SprintEnds, res.Skipped)
		}
	})
}

// TestPushDatesTheMilestoneForTeamsInOtherGroups gives a releasing
// repository outside the program group the milestone in its own group, so
// its issues find it (srd031 R4.2).
func TestPushDatesTheMilestoneForTeamsInOtherGroups(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		const team = "elsewhere/agent-ops"
		srd := strings.NewReplacer("srd010-delivery", "srd011-agent-ops", "| fleet |", "| agent-ops |",
			"/program/delivery/fleet", "/"+team).Replace(string(read(t, "../../testdata/system/srd010-delivery.md")))
		p.AddFile(sys, "main", "docs/specs/software-requirements/srd011-agent-ops.md", []byte(srd))
		in, f, _ := planned(t, p)
		res, err := Push(f, in)
		if err != nil || !slices.Equal(res.MilestoneGroups, []string{in.Group, "elsewhere"}) {
			t.Fatalf("push = %+v, %v", res, err)
		}
		ms, err := f.ActiveMilestones(team)
		if err != nil || !slices.ContainsFunc(ms, func(m tracker.Milestone) bool { return m.Title == "01.1" && m.StartDate == "2026-11-09" }) {
			t.Fatalf("%s finds milestones %+v, %v", team, ms, err)
		}
	})
}

// A goal that cites its items counts only the release's items it cites:
// serving the outcome of a release item it does not cite, it gets no
// planned event, and one citing the item gets it (GH-92).
func TestPlanPushCountsOnlyTheItemsAGoalCites(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		in, f, objective := planned(t, p)
		marker := "<!-- scrum-master goals objective=obj001-reliable-agents#R2 release=01.0 -->"
		describe(t, f, fleet, objective, "Objective R2\n\nCites: https://gitlab.example.com/program/system/-/blob/corpus/v0.0.1/docs/specs/objectives/obj001-reliable-agents.md#R2.9\n\n"+marker)
		res, err := Push(f, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Planned) != 0 {
			t.Fatalf("a goal citing another item was counted: %+v", res.Planned)
		}
		describe(t, f, fleet, objective, "Objective R2\n\nCites: https://gitlab.example.com/program/system/-/blob/corpus/v0.0.1/docs/specs/objectives/obj001-reliable-agents.md#R2.1\n\n"+marker)
		if res, err = Push(f, in); err != nil || len(res.Planned) != 1 || !res.Planned[0].Posted {
			t.Fatalf("a goal citing the item: %+v, %v", res.Planned, err)
		}
		// The event cites the item at the goal's own pin, not at the new
		// plan's tag, so the two name the same text (GH-101).
		notes, _ := f.ListNotes(fleet, objective)
		last := notes[len(notes)-1].Body
		if !strings.Contains(last, "/-/blob/corpus/v0.0.1/docs/specs/objectives/obj001-reliable-agents.md#R2.1") || strings.Contains(last, "corpus/v0.1.0") {
			t.Fatalf("the planned event cites:\n%s", last)
		}
	})
}
