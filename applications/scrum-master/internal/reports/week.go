// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// SprintGoal is an open Sprint Goal issue, or an objective issue read as
// one, the person reports on.
type SprintGoal struct {
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	DueDate string `json:"due_date,omitempty"`
	WebURL  string `json:"web_url"`
}

// WeekResult is the facts the status issue's script drafts from (srd018
// R8.1).
type WeekResult struct {
	Project     string          `json:"project"`
	User        string          `json:"user"`
	Week        string          `json:"week"`
	Since       string          `json:"since"`
	SprintGoals []SprintGoal    `json:"sprint_goals"`
	Merged      []tracker.Merge `json:"merged"`
	Closed      []SprintGoal    `json:"closed"`
}

// Week returns the project's open Sprint Goals with their due dates
// and the person's activity since the ISO week began, in UTC: branches
// they merged and the issues that closed, assigned to them or closed by a
// merge counted here; a recurring issue counts only by such a merge. A merge from before the period's first Sprint Goal is
// not the period's work, so it does not count even inside the week
// (periodStart). A project holding no Sprint Goal of its own, such as the
// system repository, starts its period at its own earliest open or
// recently closed issue, so its planning and registration merges count
// while an earlier run's do not; with no issue of its own, it takes the
// Sprint Goals of teams, the program's team repositories (srd018 R8.1).
func Week(t tracker.Tracker, project, user string, now time.Time, teams []string) (WeekResult, error) {
	now = now.UTC()
	start := WeekStart(now)
	r := WeekResult{Project: project, User: user, Week: ISOWeek(now), Since: start.Format("2006-01-02"),
		SprintGoals: []SprintGoal{}, Merged: []tracker.Merge{}, Closed: []SprintGoal{}}
	open, err := t.ListIssues(project, tracker.IssueQuery{State: "opened"})
	if err != nil {
		return r, err
	}
	for _, i := range open {
		if sprintgoal.Is(i.Description) {
			r.SprintGoals = append(r.SprintGoals, sprintGoalOf(i))
		}
	}
	closed, err := t.ListIssues(project, tracker.IssueQuery{State: "closed", UpdatedAfter: r.Since})
	if err != nil {
		return r, err
	}
	own := append(slices.Clone(open), closed...)
	bound, ok := ownStart(start, own)
	if !ok {
		var goals []tracker.Issue
		for _, team := range teams {
			for _, q := range []tracker.IssueQuery{{State: "opened"}, {State: "closed", UpdatedAfter: r.Since}} {
				issues, err := t.ListIssues(team, q)
				if err != nil {
					return r, err
				}
				goals = append(goals, issues...)
			}
		}
		bound = periodStart(start, goals, sprintgoal.Is)
	}
	merged, err := t.Merges(project, user, bound.Format(time.RFC3339))
	if err != nil {
		return r, err
	}
	r.Merged = append(r.Merged, merged...)
	byMerge := map[string]bool{}
	for _, mr := range merged {
		for _, m := range closesPattern.FindAllStringSubmatch(mr.Description, -1) {
			ref := m[1]
			if strings.HasPrefix(ref, "#") {
				ref = project + ref
			}
			byMerge[ref] = true
		}
	}
	for _, i := range closed {
		// A recurring issue, such as the person's status issue, closes by a
		// merge in another repository, which Merged does not list; it counts
		// only when a merge counted here closes it, so merged and closed
		// agree (GH-24).
		assigned := slices.Contains(i.Assignees, user) && !i.HasLabel("type::recurring")
		if assigned || byMerge[i.Project+"#"+itoa(i.IID)] {
			r.Closed = append(r.Closed, sprintGoalOf(i))
		}
	}
	return r, nil
}

// periodStart is the later of start and the creation of the earliest
// issue among issues that counts (all of them when counts is nil), in UTC:
// merges before the period's first Sprint Goal, such as an earlier run's on
// the same days, deliver nothing the period planned (srd018 R3.2, R8.1).
func periodStart(start time.Time, issues []tracker.Issue, counts func(string) bool) time.Time {
	first := time.Time{}
	for _, i := range issues {
		if counts != nil && !counts(i.Description) {
			continue
		}
		if at, err := time.Parse(time.RFC3339, i.CreatedAt); err == nil && (first.IsZero() || at.Before(first)) {
			first = at
		}
	}
	if first.After(start) {
		start = first
	}
	return start.UTC()
}

// ownStart is a repository's period start from its own open and recently
// closed issues: its earliest Sprint Goal's creation, else its earliest
// issue's, no earlier than start; false when it holds no issue.
func ownStart(start time.Time, own []tracker.Issue) (time.Time, bool) {
	switch {
	case slices.ContainsFunc(own, func(i tracker.Issue) bool { return sprintgoal.Is(i.Description) }):
		return periodStart(start, own, sprintgoal.Is), true
	case len(own) > 0:
		return periodStart(start, own, nil), true
	}
	return start.UTC(), false
}

// repositoryStart is ownStart for project, from its open issues and those
// closed since start (srd018 R8.1; GH-2776).
func repositoryStart(t tracker.Tracker, project string, start time.Time) (time.Time, error) {
	var own []tracker.Issue
	for _, q := range []tracker.IssueQuery{{State: "opened"}, {State: "closed", UpdatedAfter: start.Format("2006-01-02")}} {
		issues, err := t.ListIssues(project, q)
		if err != nil {
			return start, err
		}
		own = append(own, issues...)
	}
	bound, _ := ownStart(start, own)
	return bound, nil
}

func sprintGoalOf(i tracker.Issue) SprintGoal {
	return SprintGoal{Ref: i.Project + "#" + itoa(i.IID), Title: i.Title, DueDate: i.DueDate, WebURL: i.WebURL}
}
