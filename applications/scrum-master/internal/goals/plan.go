// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package goals

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/objectives"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Plan is what scrum-master goals plan returns and scrum-master goals push takes back once the
// person has confirmed or changed each Repository and Environment.
type Plan struct {
	System     string             `json:"system"`
	Release    string             `json:"release"`
	Tag        string             `json:"tag"`
	Milestone  string             `json:"milestone,omitempty"`
	Objectives []PlannedObjective `json:"objectives"`
	// Pushes says what push files from the confirmed plan: one Sprint
	// Goal per repository, however many entries name it (GH-66).
	Pushes string `json:"pushes"`
}

// pushes is the plan's statement of what push files: entries are items,
// one each, so an outcome's items can go to different teams, and a team's
// entries share one Sprint Goal (srd022 R4.3).
const pushes = "goals push files one Sprint Goal per repository, serving every entry confirmed for it: set each entry's repository, and entries for the same team join one Sprint Goal; do not merge entries by hand"

// PlannedObjective is one initiative item the release carries, with the
// outcome (requirement group) it belongs to, its recommendations, and the
// Sprint Goal or objective issue already filed for it. An outcome's items
// come one per entry, so the person can confirm each for a different team
// (srd022, srd034: one Cites line per item the team serves). Push files one
// Sprint Goal per team serving every item confirmed for its repository.
type PlannedObjective struct {
	Key         string   `json:"key"` // objNNN-name#Rn
	Document    string   `json:"document"`
	Group       string   `json:"group"`
	Title       string   `json:"title"`
	Items       []string `json:"items"`
	Due         string   `json:"due"`
	Repository  string   `json:"repository"`  // recommended; the person confirms or changes it
	Environment string   `json:"environment"` // recommended; the person confirms or changes it
	Candidates  []string `json:"candidates"`
	// RepositoryReason says why no repository is recommended: only an
	// outcome's delivered_by names one (srd022 R2.2, GH-92); or, when the
	// only team is, that it is recommended for being the only one (GH-109).
	RepositoryReason string `json:"repository_reason,omitempty"`
	Existing         string `json:"existing,omitempty"` // web URL of an issue already filed for the release
	// Carried is the open Sprint Goal of an earlier release that already
	// cites the item, as project#iid: the team carries it on, so push files
	// nothing for it (srd022 R2.3).
	Carried string `json:"carried,omitempty"`
}

// MakePlan computes the facts and recommendations for every initiative
// item of the release (srd022 R2, R3.1, R3.2). It asks nothing.
func MakePlan(t tracker.Tracker, s Sources) (Plan, error) {
	p := Plan{System: s.System, Release: s.Release.Version, Tag: s.Release.Tag, Milestone: s.Release.Milestone, Pushes: pushes}
	candidates := s.Subsystems.Repositories()
	carriers, err := openCarriers(t, candidates, p.Release)
	if err != nil {
		return p, err
	}
	// The system repository is a component (srd001 R21), but it serves no
	// Sprint Goal of its own: a team does. It stays a candidate only where
	// an outcome's delivered_by names it (GH-20).
	teams := slices.DeleteFunc(slices.Clone(candidates), func(r string) bool { return r == program.ProjectPath(s.System) })
	for _, id := range s.Release.Objectives {
		doc, item, _ := strings.Cut(id, "#")
		group, _, ok := strings.Cut(item, ".")
		d := s.Documents[doc]
		obj, found := d.Requirements[group]
		if !ok || !found || !hasItem(obj, item) {
			return p, fmt.Errorf("release objective %s names no item of %s", id, doc)
		}
		key := doc + "#" + group
		po := PlannedObjective{Key: key, Document: doc, Group: group, Title: obj.Title, Items: []string{item}, Due: obj.Due, Candidates: teams}
		po.Repository = recommendRepository(s, obj)
		switch {
		case po.Repository != "":
		case len(teams) == 1:
			// One team serves the program: there is nothing to choose
			// between, so it is the recommendation (GH-109).
			po.Repository = teams[0]
			po.RepositoryReason = "the only team repository a subsystem SRD declares; the outcome's delivered_by names none"
		default:
			po.RepositoryReason = "the outcome's delivered_by names no repository a subsystem SRD declares; choose one of the candidates, or name it in the initiative's delivered_by"
		}
		if po.Repository != "" && !slices.Contains(po.Candidates, po.Repository) {
			po.Candidates = append(slices.Clone(teams), po.Repository)
		}
		po.Environment = recommendEnvironment(d, group, item)
		if c, ok := carriers[id]; ok {
			po.Carried, po.Repository = c.ref, c.repo
		}
		if po.Repository != "" {
			goal, legacy, err := findExisting(t, po.Repository, p.Release)
			if err != nil {
				return p, err
			}
			if old, ok := legacy[key]; ok {
				po.Existing = old.WebURL
			} else if goal != nil {
				po.Existing = goal.WebURL
			}
		}
		p.Objectives = append(p.Objectives, po)
	}
	return p, nil
}

func hasItem(o objectives.Objective, id string) bool {
	for _, it := range o.ItemsOf() {
		if it.ID == id {
			return true
		}
	}
	return false
}

// recommendRepository is the repository of a delivered_by reference that a
// subsystem SRD declares; empty when none does, so the person chooses.
func recommendRepository(s Sources, o objectives.Objective) string {
	for _, ref := range o.DeliveredBy {
		r, err := links.Parse(ref)
		if err != nil {
			continue
		}
		if c, _, ok := s.ComponentFor(r.Project); ok {
			return projectPath(c.Repository)
		}
	}
	return ""
}

// recommendEnvironment is the environment of the first acceptance
// criterion tracing item, else of the group's first criterion, else dev
// (srd022 R3.2).
func recommendEnvironment(d *objectives.Document, group, item string) string {
	criteria := d.CriteriaFor(group)
	for _, c := range criteria {
		if slices.Contains(c.Traces, item) && c.Environment != "" {
			return c.Environment
		}
	}
	if len(criteria) > 0 && criteria[0].Environment != "" {
		return criteria[0].Environment
	}
	return "dev"
}

// carrier is an open Sprint Goal of another release that cites an item.
type carrier struct{ repo, ref string }

// openCarriers maps each item an open Sprint Goal of a release other than
// this one cites, in any team repository, to that goal: a slipped item a
// team carries on is the open goal's work, not a new goal's (srd022 R2.3).
func openCarriers(t tracker.Tracker, repos []string, release string) (map[string]carrier, error) {
	out := map[string]carrier{}
	for _, repo := range repos {
		issues, err := t.ListIssues(repo, tracker.IssueQuery{State: "opened", Labels: []string{"plane::management"}})
		if errors.Is(err, tracker.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, i := range issues {
			r, ok := sprintgoal.Parse(i.Description)
			if !ok || r.Release == release {
				continue
			}
			for _, item := range sprintgoal.CitedItems(i.Description) {
				if _, seen := out[item]; !seen {
					out[item] = carrier{repo: repo, ref: fmt.Sprintf("%s#%d", repo, i.IID)}
				}
			}
		}
	}
	return out, nil
}

// findExisting returns the team's Sprint Goal for the release in repo, and
// the objective issues filed there for the release before 04.0, keyed by the
// outcome each serves (srd022 R2.3, srd034 R3.7).
func findExisting(t tracker.Tracker, repo, release string) (*tracker.Issue, map[string]tracker.Issue, error) {
	issues, err := t.ListIssues(repo, tracker.IssueQuery{State: "opened", Labels: []string{"plane::management"}})
	if err != nil {
		if errors.Is(err, tracker.ErrNotFound) {
			return nil, nil, fmt.Errorf("repository %s does not exist or is not readable", repo)
		}
		return nil, nil, err
	}
	var goal *tracker.Issue
	legacy := map[string]tracker.Issue{}
	for i := range issues {
		r, ok := sprintgoal.Parse(issues[i].Description)
		if !ok || r.Release != release {
			continue
		}
		if r.Legacy {
			legacy[r.Serves[0]] = issues[i]
		} else if goal == nil {
			goal = &issues[i]
		}
	}
	return goal, legacy, nil
}
