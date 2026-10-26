// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// teamCheckout copies the valid hierarchy fixture into a checkout and,
// when done is given, a sprint-end summary recording those stories done.
func teamCheckout(t *testing.T, done ...string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("..", "..", "testdata", "hierarchy", "valid")
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(done) > 0 {
		summary := "# Sprint end 01.0\n\n## Stories done\n\n- " + strings.Join(done, "\n- ") + "\n"
		if err := os.MkdirAll(filepath.Join(dir, "docs", "sprint-ends"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs", "sprint-ends", "01.0.md"), []byte(summary), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestBacklogRanksStoriesWithinVelocity lists the not-done agreed stories
// by rank and recommends, in rank order, those whose points fit the
// velocity the last sprint-end summary records (srd031 R6.4, srd045 R5).
func TestBacklogRanksStoriesWithinVelocity(t *testing.T) {
	b, err := MakeBacklog(teamCheckout(t, "st001-score-trajectories"), 0)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range b.Stories {
		ids = append(ids, s.ID)
	}
	if !slices.Equal(ids, []string{"st002-store-scores", "st003-scorecard-page", "st004-shared-rubric"}) {
		t.Fatalf("backlog %v", ids)
	}
	if !b.Measured || b.Velocity != 5 || !slices.Equal(b.Recommended, []string{"st002-store-scores"}) {
		t.Fatalf("velocity %d measured %v recommended %v", b.Velocity, b.Measured, b.Recommended)
	}
	fresh, err := MakeBacklog(teamCheckout(t), 0)
	if err != nil || fresh.Measured || len(fresh.Recommended) != 0 || len(fresh.Stories) != 4 {
		t.Fatalf("unmeasured team: %+v %v", fresh, err)
	}
	assumed, err := MakeBacklog(teamCheckout(t), 8)
	if err != nil || !slices.Equal(assumed.Recommended, []string{"st001-score-trajectories", "st002-store-scores"}) {
		t.Fatalf("assumed velocity 8: %+v %v", assumed.Recommended, err)
	}
}

// TestPushWritesSprintBacklogOnce writes the chosen stories with their
// points onto the team's Sprint Goal with one planned event naming them; a
// second push changes nothing, and a dry run writes nothing (srd031 R6.6).
func TestPushWritesSprintBacklogOnce(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) {
		f := tp.Tracker()
		body := "Sprint Goal for release 01.1: Evaluation of one agent\n\nCites: https://gitlab.example.com/program/system/-/blob/v0.1.0/docs/specs/initiatives/in001-reliable-agents.md#R1.1\nEnvironment: dev\n\n" +
			"## Requirements\n\n1. A grader scores trajectories. (R1.1)\n\n## Acceptance Criteria\n\n- Scored. (test: t; environment: dev)\n\n## Notes\n\nOwner notes.\n\n## Sprint Backlog\n\n" +
			sprintgoal.Marker("01.1", []string{"in001-reliable-agents#R1"}) + "\n"
		goal, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal: Evaluation of one agent (release 01.1)", Description: body, Labels: []string{"plane::management", "component::po-1"}})
		if err != nil {
			t.Fatal(err)
		}
		dir := teamCheckout(t)
		chosen := []string{"st001-score-trajectories", "st002-store-scores"}
		writes := tp.Writes()
		if r, err := PushBacklog(f, "g/po-1", "01.1", dir, chosen, true); err != nil || r.Action != "updated" || tp.Writes() != writes {
			t.Fatalf("dry run %+v %v writes=%d", r, err, tp.Writes()-writes)
		}
		r, err := PushBacklog(f, "g/po-1", "01.1", dir, chosen, false)
		if err != nil || r.Action != "updated" || !r.Planned {
			t.Fatalf("push %+v %v", r, err)
		}
		got, _ := f.GetIssue("g/po-1", goal.IID)
		for _, want := range []string{"Owner notes.", "## Sprint Backlog\n\n- st001-score-trajectories Score trajectories (5 points)\n- st002-store-scores Store scores (3 points)\n\n<!-- scrum-master goals sprint-goal"} {
			if !strings.Contains(got.Description, want) {
				t.Errorf("description lacks %q:\n%s", want, got.Description)
			}
		}
		notes, _ := f.ListNotes("g/po-1", goal.IID)
		e, ok := events.Parse(notes[len(notes)-1].Body)
		if !ok || e.Name != "planned" || !slices.Equal(e.Values("story"), chosen) || len(e.Values("requirement")) != 1 {
			t.Fatalf("planned event %+v", e)
		}
		writes = tp.Writes()
		if r, err := PushBacklog(f, "g/po-1", "01.1", dir, chosen, false); err != nil || r.Action != "unchanged" || r.Planned || tp.Writes() != writes {
			t.Fatalf("second push %+v %v writes=%d", r, err, tp.Writes()-writes)
		}
		if _, err := PushBacklog(f, "g/po-1", "01.1", dir, []string{"ep001-agent-evaluation"}, false); err == nil {
			t.Error("an epic was accepted as a story")
		}
	})
}
