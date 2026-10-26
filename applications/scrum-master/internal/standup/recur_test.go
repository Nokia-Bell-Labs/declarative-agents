// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package standup

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

func TestRecurReopensWithNextDueDateAndPreviousRuns(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		labels := []string{"plane::data", "type::recurring", "kind::review", "component::po-1"}
		first, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: regression run", Labels: append(slices.Clone(labels), "op::status"), DueDate: "2026-10-15", Milestone: "rig-01.0",
			Description: "Run the suite.\n\n## Recurrence\n\nWeekly.\n\n## Previous Runs\n\n- none\n"})
		now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

		r, err := Recur(f, "g/po-1", first.IID, RecurOptions{Now: now})
		if err != nil || !r.Closed || r.Existing || r.DueDate != "2026-10-22" {
			t.Fatalf("recur open issue: %+v %v", r, err)
		}
		old, _ := f.GetIssue("g/po-1", first.IID)
		notes, _ := f.ListNotes("g/po-1", first.IID)
		if old.State != "closed" || len(notes) != 1 || !strings.Contains(notes[0].Body, "Run summary") {
			t.Fatalf("previous instance %s, notes %+v", old.State, notes)
		}
		next, _ := f.GetIssue("g/po-1", r.Next)
		if next.Title != first.Title || !slices.Equal(next.Labels, append(labels, "op::recur")) || next.DueDate != "2026-10-22" || next.Milestone != "rig-01.0" {
			t.Fatalf("next instance %+v", next)
		}
		if !strings.Contains(next.Description, "## Recurrence") || !strings.Contains(next.Description, "- g/po-1#1 (due 2026-10-15): closed by the recur skill without its work.") || strings.Contains(next.Description, "- none") {
			t.Fatalf("next body:\n%s", next.Description)
		}

		again, err := Recur(f, "g/po-1", first.IID, RecurOptions{Now: now})
		all, _ := f.ListIssues("g/po-1", tracker.IssueQuery{State: "all"})
		if err != nil || !again.Existing || again.Next != r.Next || len(all) != 2 {
			t.Fatalf("second recur: %+v %v, %d issues", again, err, len(all))
		}

		f.UpdateIssue("g/po-1", r.Next, tracker.IssueUpdate{State: ptrTo("close")})
		third, err := Recur(f, "g/po-1", r.Next, RecurOptions{Now: now.AddDate(0, 0, 7)})
		if err != nil || third.Closed || third.DueDate != "2026-10-29" {
			t.Fatalf("recur after a merge closed it: %+v %v", third, err)
		}
		body, _ := f.GetIssue("g/po-1", third.Next)
		if strings.Count(body.Description, "\n- g/po-1#") != 2 || !strings.Contains(body.Description, "#2 (due 2026-10-22): closed by its merge.") ||
			strings.Count(body.Description, "<!-- scrum-master standup recurs") != 1 {
			t.Fatalf("third body:\n%s", body.Description)
		}
	})
}

func ptrTo[T any](v T) *T { return &v }

func TestRecurOpensNextInstanceOnce(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		first, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: audit", Labels: []string{"type::recurring"}, DueDate: "2026-10-15",
			Description: "Audit.\n\n## Previous Runs\n\n- none\n"})
		now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
		r, err := Recur(f, "g/po-1", first.IID, RecurOptions{Now: now})
		if err != nil || r.Existing || r.Next == 0 {
			t.Fatalf("first recur: %+v %v", r, err)
		}
		again, err := Recur(f, "g/po-1", first.IID, RecurOptions{Now: now})
		all, _ := f.ListIssues("g/po-1", tracker.IssueQuery{State: "all"})
		if err != nil || !again.Existing || again.Next != r.Next || len(all) != 2 {
			t.Fatalf("second recur opened another instance: %+v %v, %d issues", again, err, len(all))
		}
	})
}

// An instance opened before recur moved to the issue group carries the
// marker scrum-master standup recur wrote; recur must find it and open none.
func TestRecurFindsEarlierMarker(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		first, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: audit", Labels: []string{"type::recurring"}, DueDate: "2026-10-15",
			Description: "Audit.\n\n## Previous Runs\n\n- none\n"})
		earlier, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: audit", Labels: []string{"type::recurring"}, DueDate: "2026-10-22",
			Description: "Audit.\n\n## Previous Runs\n\n- g/po-1#1 (due 2026-10-15): closed by scrum-master standup recur without its work.\n\n<!-- scrum-master standup recurs g/po-1#1 -->\n"})
		r, err := Recur(f, "g/po-1", first.IID, RecurOptions{Now: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)})
		all, _ := f.ListIssues("g/po-1", tracker.IssueQuery{State: "all"})
		if err != nil || !r.Existing || r.Next != earlier.IID || len(all) != 2 {
			t.Fatalf("recur missed the earlier instance: %+v %v, %d issues", r, err, len(all))
		}
	})
}

// TestRecurCarriesTheRunsSummary closes an open instance with the run's
// summary and writes it on the next instance's Previous Runs line.
func TestRecurCarriesTheRunsSummary(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		first, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: Daily Scrum", Labels: []string{"type::recurring"}, DueDate: "2026-10-15",
			Description: "Scrum.\n\n## Previous Runs\n\n- none\n"})
		r, err := Recur(f, "g/po-1", first.IID, RecurOptions{Now: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC),
			Summary: "Day 3: the grader\nis blocked on trajectories."})
		if err != nil || !r.Closed {
			t.Fatalf("recur: %+v %v", r, err)
		}
		notes, _ := f.ListNotes("g/po-1", first.IID)
		if len(notes) != 1 || !strings.HasPrefix(notes[0].Body, "Run summary: Day 3: the grader\nis blocked on trajectories.") {
			t.Fatalf("notes %+v", notes)
		}
		next, _ := f.GetIssue("g/po-1", r.Next)
		if !strings.Contains(next.Description, ": Day 3: the grader is blocked on trajectories. Closed by the recur skill.\n") || strings.Contains(next.Description, "without its work") {
			t.Fatalf("next body:\n%s", next.Description)
		}
	})
}

// TestRecurAdmitsTheNextInstance holds recur to the admission scrum-master issue
// file runs: a recurred data issue is admitted as a freshly filed one is,
// so on beads it starts ready, not open (srd013 R8.1, srd041 R1.1).
func TestRecurAdmitsTheNextInstance(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		first, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: weekly status", DueDate: "2026-10-15",
			Labels:      []string{"plane::data", "type::recurring", "kind::review", "component::po-1"},
			Description: "Report the week.\n\n## Recurrence\n\nWeekly.\n\n## Previous Runs\n\n- none\n"})
		r, err := Recur(f, "g/po-1", first.IID, RecurOptions{Policy: program.Defaults(), Now: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)})
		if err != nil || !r.Admitted {
			t.Fatalf("recur = %+v, %v; want the next instance admitted", r, err)
		}
		next, _ := f.GetIssue("g/po-1", r.Next)
		notes, _ := f.ListNotes("g/po-1", r.Next)
		if next.HasLabel("needs-triage") || len(notes) != 0 {
			t.Fatalf("next instance labels %v, notes %+v", next.Labels, notes)
		}
		if bt, ok := f.(*beads.Tracker); ok {
			if i, _, err := bt.Store.Find("g/po-1", r.Next); err != nil || i.State != beads.Ready {
				t.Fatalf("next instance on beads is %s (%v), want ready", i.State, err)
			}
		}
	})
}
