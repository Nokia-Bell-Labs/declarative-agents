// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package goals

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Pushed is what push reports for one initiative outcome: the team's Sprint
// Goal that serves it, or why it was refused.
type Pushed struct {
	Key     string   `json:"key"`
	Action  string   `json:"action"` // created, updated, unchanged, refused
	Project string   `json:"project,omitempty"`
	IID     int      `json:"iid,omitempty"`
	WebURL  string   `json:"web_url,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
	// Warnings are what push filed but could not make exact, such as a
	// shared criterion's unattributed tests (GH-20).
	Warnings []string `json:"warnings,omitempty"`
	Planned  bool     `json:"planned_event_posted"`
	// Legacy is set when an objective issue filed before release 04.0
	// already serves the outcome; push leaves it as it is (srd034 R3.7).
	Legacy bool `json:"legacy,omitempty"`
}

// Push files or updates one Sprint Goal per team for the release, serving
// every outcome confirmed for the team's repository, and posts its planned
// event (srd022 R4.1, R4.3). An outcome with no repository, no environment,
// or an unpinned citation is refused with its reason (R5.1); the others
// proceed. A repository that holds objective issues for the release from
// before 04.0 keeps them and gets no Sprint Goal.
func Push(t tracker.Tracker, s Sources, p Plan, webBase string, policy program.Policy, dryRun bool) ([]Pushed, error) {
	due := ""
	if p.Milestone != "" {
		if m, err := t.FindMilestone(p.System, p.Milestone); err == nil {
			due = m.DueDate
		} else if !errors.Is(err, tracker.ErrNotFound) {
			return nil, err
		}
	}
	out := make([]Pushed, len(p.Objectives))
	teams := map[string][]int{}
	var order []string
	for i, o := range p.Objectives {
		r := Pushed{Key: o.Key, Project: o.Repository}
		if o.Carried != "" {
			// The open Sprint Goal of an earlier release carries the item;
			// a second goal would duplicate it (srd022 R2.3).
			r.Action, r.Reasons = "unchanged", []string{"carried by the open Sprint Goal " + o.Carried}
			out[i] = r
			continue
		}
		d := s.Documents[o.Document]
		if d == nil {
			return nil, fmt.Errorf("plan objective %s names a document the release does not", o.Key)
		}
		_, _, cites := Render(webBase, p, []PlannedObjective{o}, s.Documents)
		if o.Repository == "" {
			r.Reasons = append(r.Reasons, "no repository confirmed")
		}
		if o.Environment == "" {
			r.Reasons = append(r.Reasons, "no environment confirmed")
		}
		for _, c := range cites {
			if err := links.CheckPinned(c); err != nil {
				r.Reasons = append(r.Reasons, err.Error())
			}
		}
		if _, _, ok := s.ComponentFor(o.Repository); o.Repository != "" && !ok {
			r.Reasons = append(r.Reasons, fmt.Sprintf("no subsystem SRD declares %s at the plan tag %s; a subsystem declared after the plan was tagged needs the plan republished", o.Repository, p.Tag))
		}
		if len(r.Reasons) > 0 {
			r.Action = "refused"
			out[i] = r
			continue
		}
		if _, seen := teams[o.Repository]; !seen {
			order = append(order, o.Repository)
		}
		teams[o.Repository] = append(teams[o.Repository], i)
	}
	for _, repo := range order {
		if err := pushTeam(t, s, p, webBase, policy, dryRun, due, repo, teams[repo], out); err != nil {
			return out, err
		}
	}
	return distinct(out), nil
}

// distinct keeps one result per outcome and issue: a team's items of one
// outcome are one plan entry each but share its Sprint Goal, so push
// reports that Sprint Goal once (srd022 R4.3).
func distinct(results []Pushed) []Pushed {
	seen := map[string]bool{}
	var out []Pushed
	for _, r := range results {
		k := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%v", r.Key, r.Project, r.IID, r.Action, r.Reasons)
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}

// outcomes gathers a team's plan entries into one per outcome, in plan
// order, each with every item the team was confirmed for: a Sprint Goal
// serves each outcome once and cites each of its items.
func outcomes(entries []PlannedObjective, members []int) []PlannedObjective {
	var team []PlannedObjective
	index := map[string]int{}
	for _, m := range members {
		o := entries[m]
		if i, seen := index[o.Key]; seen {
			for _, it := range o.Items {
				if !slices.Contains(team[i].Items, it) {
					team[i].Items = append(team[i].Items, it)
				}
			}
			continue
		}
		index[o.Key] = len(team)
		o.Items = slices.Clone(o.Items)
		team = append(team, o)
	}
	return team
}

// pushTeam files or updates one team's Sprint Goal and records the result
// on each outcome it serves.
func pushTeam(t tracker.Tracker, s Sources, p Plan, webBase string, policy program.Policy, dryRun bool, due, repo string, members []int, out []Pushed) error {
	team := outcomes(p.Objectives, members)
	goal, legacy, err := findExisting(t, repo, p.Release)
	if err != nil {
		return err
	}
	if goal == nil && len(legacy) > 0 {
		for _, m := range members {
			r := Pushed{Key: p.Objectives[m].Key, Project: repo, Action: "unchanged", Legacy: true}
			if old, ok := legacy[r.Key]; ok {
				r.IID, r.WebURL = old.IID, old.WebURL
			}
			out[m] = r
		}
		return nil
	}
	title, body, cites := Render(webBase, p, team, s.Documents)
	issueDue := ""
	for _, o := range team {
		if o.Due != "" && (issueDue == "" || o.Due < issueDue) {
			issueDue = o.Due
		}
	}
	if issueDue == "" {
		issueDue = due
	}
	if goal != nil {
		body = KeepNotes(body, goal.Description)
	}
	comp, _, _ := s.ComponentFor(repo)
	r := Pushed{Project: repo, Warnings: SharedTestWarnings(team, s.Documents)}
	var issue tracker.Issue
	switch {
	case goal == nil:
		in := tracker.NewIssue{Title: title, Description: body, DueDate: issueDue, Milestone: p.Milestone,
			Labels: append(program.Labels("management", "", "", comp.Name), program.Op(program.OpGoals))}
		f, err := program.File(t, repo, in, policy, dryRun)
		if err != nil {
			return err
		}
		r.Action, r.IID, r.WebURL = "created", f.IID, f.WebURL
		issue = tracker.Issue{Project: repo, IID: f.IID}
	case goal.Description == body && goal.DueDate == issueDue:
		r.Action, r.IID, r.WebURL = "unchanged", goal.IID, goal.WebURL
		issue = *goal
	default:
		r.Action, r.IID, r.WebURL = "updated", goal.IID, goal.WebURL
		issue = *goal
		if !dryRun {
			u := tracker.IssueUpdate{Description: &body}
			if issueDue != "" {
				u.DueDate = &issueDue
			}
			if _, err := t.UpdateIssue(repo, goal.IID, u); err != nil {
				return err
			}
		}
	}
	if !dryRun || issue.IID != 0 {
		e, text := events.Planned(p.Release, cites, team[0].Environment)
		posted, err := events.Post(t, issue.Project, issue.IID, e, events.PlannedRequired, text, dryRun)
		if err != nil {
			return err
		}
		r.Planned = posted
	} else {
		r.Planned = true
	}
	for _, m := range members {
		rm := r
		rm.Key = p.Objectives[m].Key
		out[m] = rm
	}
	return nil
}
