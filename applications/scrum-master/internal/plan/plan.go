// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package plan is the scrum-master side of plan in the system repository
// (srd031): it ranks the objective items the next release could prove and
// writes the chosen release into the road map. Nothing here is a model
// step (design.md D10).
package plan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/objectives"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// RoadMap is the road map's path in a repository checkout.
const RoadMap = "docs/road-map.md"

// periodDays is the length of a period: three weeks (srd031 R3.2).
const periodDays = 21

// Item is one objective item a release can prove.
type Item struct {
	ID      string   `json:"id"`    // objNNN-short-name#Rn.m
	Group   string   `json:"group"` // objNNN-short-name#Rn
	Title   string   `json:"title"` // the objective's title
	Text    string   `json:"text"`
	Due     string   `json:"due,omitempty"`
	Slipped bool     `json:"slipped,omitempty"`
	Placed  string   `json:"placed_in,omitempty"` // the release that already lists it
	Missing []string `json:"missing,omitempty"`   // test, environment
}

// Candidates is what scrum-master plan candidates returns (srd031 R2, R3).
type Candidates struct {
	Latest          string `json:"latest_release,omitempty"`
	Version         string `json:"recommended_version"`
	EndDate         string `json:"recommended_end_date,omitempty"`
	Tag             string `json:"recommended_tag,omitempty"`
	Items           []Item `json:"items"`
	WithoutEvidence []Item `json:"without_evidence,omitempty"`
	// Backlog is the team's Product Backlog, when the checkout holds stories
	// (srd031 R6.4).
	Backlog *Backlog `json:"backlog,omitempty"`

	agreed map[string]bool // every item of an agreed objective document
}

// Input is where candidates come from: the system repository's checkout,
// the tracker for slipped objective issues and the last milestone, and
// the period's aggregate, if the caller has one.
type Input struct {
	Dir       string
	System    string
	Tracker   tracker.Tracker
	Aggregate *reports.Aggregation
	Releases  program.Releases
	Artifact  string
	Tags      []string
	Now       time.Time
}

// MakeCandidates ranks the agreed objective items not yet placed in a
// release, plus the placed ones whose objective slipped: slipped first,
// then by due date, then in document order (srd031 R3.1). Items whose
// criteria lack a test or an environment come back apart (R4.4, R5.1).
func MakeCandidates(in Input) (Candidates, error) {
	var c Candidates
	releases, err := readRoadMap(in.Dir)
	if err != nil {
		return c, err
	}
	placed := map[string]string{}
	for _, r := range releases {
		for _, id := range r.Objectives {
			placed[id] = r.Version
		}
	}
	slipped, err := slippedOutcomes(in)
	if err != nil {
		return c, err
	}
	docs, err := objectives.LoadAll(in.Dir)
	if err != nil {
		return c, err
	}
	items := []Item{}
	c.agreed = map[string]bool{}
	for _, d := range docs {
		if d.Status != "agreed" {
			continue
		}
		for _, gid := range d.ObjectiveIDs() {
			o := d.Requirements[gid]
			group := d.ID + "#" + gid
			for _, it := range o.ItemsOf() {
				item := Item{ID: d.ID + "#" + it.ID, Group: group, Title: o.Title, Text: it.Text, Due: o.Due,
					Slipped: slipped[group] || slipped[d.ID+"#"+it.ID], Placed: placed[d.ID+"#"+it.ID], Missing: missing(d, gid, it.ID)}
				c.agreed[item.ID] = true
				switch {
				case item.Placed != "" && !item.Slipped:
				case len(item.Missing) > 0:
					c.WithoutEvidence = append(c.WithoutEvidence, item)
				default:
					items = append(items, item)
				}
			}
		}
	}
	slices.SortStableFunc(items, func(a, b Item) int {
		switch {
		case a.Slipped != b.Slipped:
			if a.Slipped {
				return -1
			}
			return 1
		case a.Due != b.Due:
			if a.Due == "" {
				return 1
			}
			if b.Due == "" {
				return -1
			}
			return strings.Compare(a.Due, b.Due)
		}
		return 0
	})
	c.Items = items
	if n := len(releases); n > 0 && unplanned(releases[n-1]) {
		// scrum-master init lays down 01.0 with no items and no tag: that record is
		// the next release, not a released one.
		c.Version = releases[n-1].Version
		if n > 1 {
			c.Latest = releases[n-2].Version
		}
	} else {
		if n > 0 {
			c.Latest = releases[n-1].Version
		}
		c.Version = nextVersion(c.Latest)
	}
	c.EndDate = endDate(in, releases, items)
	next := release.Next(in.Releases, in.Artifact, in.Tags)
	for _, cand := range next.Candidates {
		if cand.Bump == "minor" {
			c.Tag = cand.Tag
		}
	}
	return c, nil
}

func readRoadMap(dir string) ([]goals.Release, error) {
	b, err := os.ReadFile(filepath.Join(dir, RoadMap))
	if err != nil {
		return nil, err
	}
	d, err := corpus.ParseMarkdown(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", RoadMap, err)
	}
	return goals.ReleasesOf(d), nil
}

// missing names what an item's acceptance criteria lack: a criterion that
// traces it, and on each such criterion its tests and its environment.
func missing(d *objectives.Document, group, item string) []string {
	var tracing []objectives.Criterion
	for _, c := range d.CriteriaFor(group) {
		if slices.Contains(c.Traces, item) {
			tracing = append(tracing, c)
		}
	}
	if len(tracing) == 0 {
		return []string{"test", "environment"}
	}
	var out []string
	for _, c := range tracing {
		if len(c.Tests) == 0 && !slices.Contains(out, "test") {
			out = append(out, "test")
		}
		if c.Environment == "" && !slices.Contains(out, "environment") {
			out = append(out, "environment")
		}
	}
	return out
}

// slippedOutcomes maps each item whose Sprint Goal slipped: the aggregate
// names it in a slipped-commitment signal, or the period ended with it
// neither met nor reported complete, as a Sprint Goal its team carries to
// the next sprint is. The Sprint Goal's tasks marker names the outcomes it
// serves and its Cites lines the items it promised, so of each outcome only
// the cited items slip, and other goals' items of the same outcome do not;
// a goal citing no item of an outcome it serves marks the whole outcome
// (srd018 R3.2, srd031 R2.3).
func slippedOutcomes(in Input) (map[string]bool, error) {
	out := map[string]bool{}
	if in.Aggregate == nil || in.Tracker == nil {
		return out, nil
	}
	var refs []string
	for _, s := range in.Aggregate.Signals {
		if s.Kind == reports.SignalSlipped && !slices.Contains(refs, s.Subject) {
			refs = append(refs, s.Subject)
		}
	}
	for _, p := range in.Aggregate.Progress {
		if p.State != "met" && p.State != "reported complete" && !slices.Contains(refs, p.SprintGoal) {
			refs = append(refs, p.SprintGoal)
		}
	}
	for _, ref := range refs {
		project, iid, err := release.ParseIssueRef(ref)
		if err != nil {
			continue
		}
		issue, err := in.Tracker.GetIssue(project, iid)
		if err != nil {
			return nil, fmt.Errorf("slipped Sprint Goal %s: %w", ref, err)
		}
		m, ok := sprintgoal.Parse(issue.Description)
		if !ok {
			continue
		}
		cited := sprintgoal.CitedItems(issue.Description)
		for _, key := range m.Serves {
			items := slices.DeleteFunc(slices.Clone(cited), func(c string) bool { return !strings.HasPrefix(c, key+".") })
			if len(items) == 0 {
				items = []string{key}
			}
			for _, it := range items {
				out[it] = true
			}
		}
	}
	return out, nil
}

// teamRepository reports whether the checkout at dir is a team's
// repository, by the subsystem SRD scrum-master onboard writes there.
func teamRepository(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "docs", "specs", "software-requirements", "srd001-subsystem.md"))
	return err == nil
}

// openRelease is the system's release a team's sprint serves, as a team's
// repository has none of its own to plan (srd031 R6.4): the first release
// the system's road map has not released, read from the system's checkout
// when system is a path, else from its project's default branch. Empty
// when no system is given, its road map cannot be read, or every release
// is released (GH-141).
func openRelease(t tracker.Tracker, system string) string {
	var b []byte
	if program.CheckProject("system", system) != nil {
		b, _ = os.ReadFile(filepath.Join(system, RoadMap))
	} else if t != nil && system != "" {
		if branch, err := t.DefaultBranch(system); err == nil {
			b, _ = t.GetFile(system, RoadMap, branch)
		}
	}
	d, err := corpus.ParseMarkdown(b)
	if len(b) == 0 || err != nil {
		return ""
	}
	for _, r := range d.Records("release") {
		if r.Attrs["status"] != "released" {
			return r.Key
		}
	}
	return ""
}

// unplanned reports whether a release record names no items and no tag:
// a release laid down to be planned, not one planning has written.
func unplanned(r goals.Release) bool { return len(r.Objectives) == 0 && r.Tag == "" }

// nextVersion is the road map's next minor version: 01.1 is followed by
// 01.2, and an empty road map starts at 01.0.
func nextVersion(latest string) string {
	major, minor, ok := strings.Cut(latest, ".")
	n, err := strconv.Atoi(minor)
	if !ok || err != nil {
		return "01.0"
	}
	return fmt.Sprintf("%s.%0*d", major, len(minor), n+1)
}

// endDate recommends the end of the next three-week period: the Friday of
// its third ISO week, the last working day the period has. The period
// starts on the Monday after the latest release's milestone ends, else on
// the Monday of the current week, or of the next one on a weekend (srd031
// R3.2); periodStart reads it back.
func endDate(in Input, releases []goals.Release, items []Item) string {
	weekend := in.Now.Weekday() == time.Saturday || in.Now.Weekday() == time.Sunday
	start, fromMilestone := monday(in.Now, weekend), false
	if len(releases) > 0 {
		name := releases[len(releases)-1].Milestone
		if project := systemProject(in.System); name != "" && in.Tracker != nil && project != "" {
			if m, err := in.Tracker.FindMilestone(project, name); err == nil && m.DueDate != "" {
				if due, err := time.Parse(time.DateOnly, m.DueDate); err == nil {
					start, fromMilestone = monday(due.AddDate(0, 0, 1), true), true
				}
			}
		}
		// A milestone the tracker does not find in the system's project,
		// as a program group's is not, is still dated by its name, the ISO
		// weeks it spans (GH-117).
		if _, end, err := timeline.ParseWeekRange(name); !fromMilestone && err == nil {
			start, fromMilestone = monday(end.AddDate(0, 0, 1), true), true
		}
	}
	end := start.AddDate(0, 0, periodDays-3)
	// The first release, with no milestone to follow, would start this
	// week, or the next on a weekend; but when the unplaced items fall due
	// on the Friday ending a period already begun, that is the period their
	// owners agreed, and it ends then, not a week or two later, whatever
	// day of the period the plan is made (GH-66, GH-92). It is still a
	// whole period, so periodStart reads it back.
	first := !slices.ContainsFunc(releases, func(r goals.Release) bool { return !unplanned(r) })
	if !fromMilestone && first {
		today := time.Date(in.Now.Year(), in.Now.Month(), in.Now.Day(), 0, 0, 0, 0, time.UTC)
		for _, it := range items {
			due, err := time.Parse(time.DateOnly, it.Due)
			if it.Placed == "" && err == nil && due.Weekday() == time.Friday && !due.Before(today) && !periodStart(due).After(start) {
				return it.Due
			}
		}
	}
	return end.Format(time.DateOnly)
}

// systemProject is the tracker's project for system: system itself when it
// names one, and for a checkout's path, such as a worktree the plan is
// written in, the project its origin names, so the milestone is found as
// from the project path (GH-141). Empty when a path's origin names none.
func systemProject(system string) string {
	if program.CheckProject("system", system) == nil {
		return system
	}
	out, err := exec.Command("git", "-C", system, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return program.ProjectPath(strings.TrimSpace(string(out)))
}

// monday is the Monday of d's ISO week or, with after, the first Monday on
// or after d.
func monday(d time.Time, after bool) time.Time {
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	back := (int(d.Weekday()) + 6) % 7 // days since Monday
	if after && back > 0 {
		return d.AddDate(0, 0, 7-back)
	}
	return d.AddDate(0, 0, -back)
}
