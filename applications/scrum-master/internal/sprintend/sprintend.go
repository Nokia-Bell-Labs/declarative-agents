// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package sprintend is the scrum-master side of a sprint-end issue (srd038 R1, R6):
// the facts its Requirements summarize, so the model drafts the period's
// summary from computed state and asks only what scrum-master cannot know.
package sprintend

import (
	"fmt"
	"slices"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// SprintGoal is one Sprint Goal issue of the repository, or an objective
// issue read as one (srd034 R3.7), and its state.
type SprintGoal struct {
	Issue string `json:"issue"`
	// Serves lists the initiative outcomes from tasks' marker, doc#Rn.
	Serves           []string `json:"serves"`
	Legacy           bool     `json:"legacy,omitempty"`
	Release          string   `json:"release"`
	Title            string   `json:"title"`
	State            string   `json:"state"` // opened or closed
	Reported         string   `json:"reported,omitempty"`
	ReportedComplete bool     `json:"reported_complete"`
	Met              bool     `json:"met"`
	// MetNeeds says what a goal reported complete still lacks to be met:
	// met is posted once its evidence counts (srd038 R2.2, GH-92).
	MetNeeds string `json:"met_needs,omitempty"`
	// Outcome names which holds: met (evidence stands behind it), reported
	// complete (the owner's word, not yet met), or not complete (srd038
	// R2.2).
	Outcome string   `json:"outcome"`
	Ledger  []string `json:"ledger"` // the events posted on it, in order: each by name, a planned event that lists the Sprint Backlog with its story count
	// Stories lists the stories its Sprint Backlog section names, as
	// project#story.
	Stories []string `json:"stories"`
}

// Facts is what scrum-master sprint-end facts returns.
type Facts struct {
	Project     string              `json:"project"`
	Aggregate   reports.Aggregation `json:"aggregate"`
	SprintGoals []SprintGoal        `json:"sprint_goals"`
	Next        release.NextResult  `json:"next"`
	// Recommended is the version release next recommends from the
	// checkout's commits since its latest tag, Next.Reason saying why
	// (srd015 R3.1); empty when nothing changed since that tag.
	Recommended string `json:"recommended_version"`
}

// Input is one sprint-end facts run.
type Input struct {
	Tracker tracker.Tracker
	Project string
	// Teams are the program's team repositories, read beside Project when
	// it is the system repository: every Sprint Goal lives in a team's
	// repository, and the program's period closes on all of them.
	Teams     []string
	Aggregate reports.Aggregation
	Releases  program.Releases
	Artifact  string
	Tags      []string
	// Dir is the artifact's checkout, whose commits since its latest tag
	// the recommendation reads; empty recommends the next minor.
	Dir string
}

// MakeFacts lists the repository's Sprint Goals with their ledger and
// reported status, beside the period's aggregate and the version scrum-master
// release next recommends.
func MakeFacts(in Input) (Facts, error) {
	f := Facts{Project: in.Project, Aggregate: in.Aggregate, SprintGoals: []SprintGoal{}}
	progress := map[string]reports.Progress{}
	for _, p := range in.Aggregate.Progress {
		progress[p.SprintGoal] = p
	}
	projects := []string{in.Project}
	for _, team := range in.Teams {
		if !slices.Contains(projects, team) {
			projects = append(projects, team)
		}
	}
	for _, project := range projects {
		goals, err := sprintGoals(in.Tracker, project, progress)
		if err != nil {
			return f, err
		}
		f.SprintGoals = append(f.SprintGoals, goals...)
	}
	f.Next = release.Next(in.Releases, in.Artifact, in.Tags)
	if in.Dir != "" {
		next, err := release.Recommendation(in.Releases, in.Artifact, in.Dir, in.Tags)
		if err != nil {
			return f, err
		}
		f.Next = next
	} else {
		f.Next.Recommended, f.Next.Reason = "minor", "no checkout to read commits from: a period's release is minor"
	}
	// An artifact never released whose period planned no Sprint Goal, such
	// as a team that has not started, has nothing to release (GH-2779).
	if f.Next.Latest == "" && len(f.SprintGoals) == 0 {
		f.Next.Recommended, f.Next.RecommendedVersion = "", ""
		f.Next.Reason = "no " + in.Artifact + " tag yet and no Sprint Goal this period: nothing to release"
	}
	for _, c := range f.Next.Candidates {
		if c.Bump == f.Next.Recommended {
			f.Recommended = c.Version
		}
	}
	return f, nil
}

// sprintGoals lists project's Sprint Goals with their ledgers; each story
// they list is named project#story, since story ids repeat across teams.
func sprintGoals(t tracker.Tracker, project string, progress map[string]reports.Progress) ([]SprintGoal, error) {
	// A Sprint Goal is known by tasks' marker, not by a label.
	issues, err := t.ListIssues(project, tracker.IssueQuery{State: "all"})
	if err != nil {
		return nil, err
	}
	var out []SprintGoal
	for _, i := range issues {
		m, ok := sprintgoal.Parse(i.Description)
		if !ok {
			continue
		}
		ref := fmt.Sprintf("%s#%d", project, i.IID)
		o := SprintGoal{Issue: ref, Serves: m.Serves, Legacy: m.Legacy, Release: m.Release, Title: i.Title, State: i.State,
			Ledger: []string{}, Stories: []string{}}
		for _, s := range sprintgoal.SprintBacklog(i.Description) {
			o.Stories = append(o.Stories, project+"#"+s)
		}
		notes, err := t.ListNotes(project, i.IID)
		if err != nil {
			return nil, err
		}
		for _, n := range notes {
			if e, ok := events.Parse(n.Body); ok {
				o.Ledger = append(o.Ledger, ledgerEntry(e))
				o.Met = o.Met || e.Name == "met"
				o.ReportedComplete = o.ReportedComplete || e.Name == "reported_complete"
			}
		}
		if p, ok := progress[ref]; ok {
			o.Reported = p.Reported
			o.ReportedComplete = o.ReportedComplete || p.Complete || p.Reported == "done"
		}
		switch {
		case o.Met:
			o.Outcome = "met"
		case o.ReportedComplete:
			o.Outcome = "reported complete"
			o.MetNeeds = "evidence: met is posted once the evidence of the items it serves counts in its environment (srd038 R2.2)"
		default:
			o.Outcome = "not complete"
		}
		out = append(out, o)
	}
	return out, nil
}

// ledgerEntry names an event in a ledger. Filing a Sprint Goal and listing
// its stories each post a planned event (srd006 R8); the second reads
// "planned, N stories", so a ledger never shows planning twice.
func ledgerEntry(e events.Event) string {
	if stories := e.Values("story"); e.Name == "planned" && len(stories) > 0 {
		return fmt.Sprintf("planned, %d stories", len(stories))
	}
	return e.Name
}
