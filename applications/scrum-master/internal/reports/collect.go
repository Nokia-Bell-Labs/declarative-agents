// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/repofiles"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Where a collected file was read: from its host without a checkout, from a
// checkout of its repository, or from a central reports repository kept
// readable for one release (GH-2725).
const (
	SourceHost     = repofiles.SourceHost
	SourceCheckout = repofiles.SourceCheckout
	SourceCentral  = "central"
)

// Collected is one report or rollup a collection read: where it lives, the
// commit that last changed it, which a rollup cites, and its text.
type Collected struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	Week       string `json:"week"`
	// Name is a report's author, or a rollup's subsystem or program.
	Name    string `json:"name"`
	Source  string `json:"source"`
	Content string `json:"content"`
}

// Cite names the file as a rollup cites it.
func (c Collected) Cite() string { return Cite(c.Repository, c.Commit) }

// OpenIssue is a status or rollup issue of the period still open: the
// report or rollup it asks for has not merged.
type OpenIssue struct {
	Repository string `json:"repository"`
	IID        int    `json:"iid"`
	Kind       string `json:"kind"` // status, team-rollup, program-rollup
	// Name is the person a status issue is for, or the rollup's subsystem
	// or program.
	Name    string `json:"name"`
	Week    string `json:"week"`
	DueDate string `json:"due_date,omitempty"`
	WebURL  string `json:"web_url,omitempty"`
}

// Unread is a repository a collection could not read, and why.
type Unread struct {
	Repository string `json:"repository"`
	Reason     string `json:"reason"`
}

// Collection is what scrum-master reports collect returns: for a level and
// a period, the repositories it read, the merged reports and rollups, and
// the status and rollup issues still open (GH-2725).
type Collection struct {
	Level        string      `json:"level"`
	Subsystem    string      `json:"subsystem,omitempty"`
	From         string      `json:"from"`
	To           string      `json:"to"`
	System       string      `json:"system"`
	Repositories []string    `json:"repositories"`
	Reports      []Collected `json:"reports"`
	Rollups      []Collected `json:"rollups"`
	Open         []OpenIssue `json:"open"`
	Unread       []Unread    `json:"unread,omitempty"`
}

// CollectInput is one collection.
type CollectInput struct {
	Config   Config
	System   string // the system repository
	Ref      string // ref of its subsystem SRDs
	Level    string // team or program
	From, To string // YYYY-Www, inclusive
	// Subsystem names a team; Project, the repository the collection runs
	// in, names it when Subsystem is empty.
	Subsystem, Project string
}

var rollupPattern = regexp.MustCompile(`<!-- scrum-master reports rollup week=(\S+) level=(\S+) -->`)

func rollupMarker(week, level string) string {
	return fmt.Sprintf("<!-- scrum-master reports rollup week=%s level=%s -->", week, level)
}

// Collect reads what a level needs from the program's repositories, the
// subsystem SRDs' implementation targets: a team's merged reports from its
// repositories; for the program, every repository's reports and the
// teams' rollups, and the system repository's own reports. A program with
// no subsystem SRDs reads the repositories its roster names. Each file is
// read on its repository's default branch from its host when the tracker
// serves files without a checkout, else from a checkout beside this one,
// and a central reports repository the roster names is read too, for the
// release that moves reports into place. Collect writes nothing.
func Collect(t tracker.Tracker, in CollectInput) (Collection, error) {
	weeks, err := weekRange(in.From, in.To)
	if err != nil {
		return Collection{}, err
	}
	c := Collection{Level: in.Level, From: in.From, To: in.To, System: in.System,
		Repositories: []string{}, Reports: []Collected{}, Rollups: []Collected{}, Open: []OpenIssue{}}
	subs, err := program.Teams(t, in.System, in.Ref)
	if err != nil {
		return c, err
	}
	add := func(r string) {
		if r = program.ProjectPath(r); r != "" && !slices.Contains(c.Repositories, r) {
			c.Repositories = append(c.Repositories, r)
		}
	}
	switch in.Level {
	case "team":
		var sub program.Subsystem
		for _, s := range subs {
			if s.ID == in.Subsystem {
				sub = s
			}
		}
		if sub.ID == "" && in.Subsystem == "" && in.Project != "" {
			_, sub, _ = subs.ComponentFor(in.Project)
		}
		if sub.ID == "" {
			return c, fmt.Errorf("no subsystem SRD in %s names %s; a team rollup collects one subsystem's repositories", in.System, firstOf(in.Subsystem, in.Project))
		}
		c.Subsystem = sub.ID
		for _, comp := range sub.Components {
			add(comp.Repository)
		}
	case ProgramLevel:
		add(in.System)
		for _, r := range subs.Repositories() {
			add(r)
		}
		if len(subs) == 0 {
			for _, m := range in.Config.Team {
				add(m.Project)
			}
		}
	default:
		return c, fmt.Errorf("level %q is not team or program", in.Level)
	}
	quarters := quartersOf(weeks)
	for _, repo := range c.Repositories {
		var files []Collected
		var reasons []string
		for _, q := range quarters {
			for _, dir := range []string{path.Join(StatusDir, "reports", q), path.Join(StatusDir, "rollups", q)} {
				got, reason := readStatus(t, repo, dir)
				files = append(files, got...)
				if reason != "" && !slices.Contains(reasons, reason) {
					reasons = append(reasons, reason)
				}
			}
		}
		for _, r := range reasons {
			c.Unread = append(c.Unread, Unread{repo, r})
		}
		for _, f := range files {
			name, week, ok := statusName(f.Path)
			if !ok || !slices.Contains(weeks, week) {
				continue
			}
			f.Name, f.Week = name, week
			if IsRollupPath(f.Path) {
				// The program reads the teams' rollups; a team, none.
				if in.Level == ProgramLevel && name != ProgramLevel {
					c.Rollups = append(c.Rollups, f)
				}
				continue
			}
			c.Reports = append(c.Reports, f)
		}
		open, err := openStatus(t, repo, weeks)
		if err != nil {
			c.Unread = append(c.Unread, Unread{repo, "its issues: " + err.Error()})
			continue
		}
		c.Open = append(c.Open, open...)
	}
	if in.Config.Reports != "" {
		for _, q := range quarters {
			got, reason := readStatus(t, in.Config.Reports, path.Join("reports", q))
			if reason != "" {
				c.Unread = append(c.Unread, Unread{in.Config.Reports, reason})
			}
			for _, f := range got {
				name, week, ok := statusName(f.Path)
				if !ok || !slices.Contains(weeks, week) || !inLevel(in.Config, name, c.Repositories) {
					continue
				}
				f.Name, f.Week, f.Source = name, week, SourceCentral
				c.Reports = append(c.Reports, f)
			}
		}
	}
	sort.SliceStable(c.Reports, func(i, j int) bool {
		return c.Reports[i].Path+c.Reports[i].Repository < c.Reports[j].Path+c.Reports[j].Repository
	})
	sort.SliceStable(c.Rollups, func(i, j int) bool {
		return c.Rollups[i].Path+c.Rollups[i].Repository < c.Rollups[j].Path+c.Rollups[j].Repository
	})
	return c, nil
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// inLevel reports whether a central repository's report by user belongs to
// the level: its author reports in one of the level's repositories.
func inLevel(cfg Config, user string, repos []string) bool {
	return slices.ContainsFunc(cfg.Team, func(m Member) bool {
		return m.Username == user && slices.Contains(repos, program.ProjectPath(m.Project))
	})
}

// statusName reads a status file's name and week from its path,
// <week>-<name>.md.
func statusName(p string) (name, week string, ok bool) {
	base := strings.TrimSuffix(path.Base(p), ".md")
	if len(base) < 10 || base[8] != '-' {
		return "", "", false
	}
	if _, err := ParseWeek(base[:8]); err != nil {
		return "", "", false
	}
	return base[9:], base[:8], true
}

// readStatus reads every Markdown file under dir in repo, each with the
// commit that last changed it, from its host or a checkout (repofiles).
func readStatus(t tracker.Tracker, repo, dir string) ([]Collected, string) {
	files, reason := repofiles.Read(t, repo, dir, func(p string) bool { return strings.HasSuffix(p, ".md") })
	out := make([]Collected, 0, len(files))
	for _, f := range files {
		out = append(out, Collected{Repository: f.Repository, Path: f.Path, Commit: f.Commit, Source: f.Source, Content: f.Content})
	}
	return out, reason
}

// openStatus lists repo's open status and rollup issues for the weeks.
func openStatus(t tracker.Tracker, repo string, weeks []string) ([]OpenIssue, error) {
	issues, err := t.ListIssues(repo, tracker.IssueQuery{State: "opened", Labels: []string{"type::recurring"}})
	if err != nil {
		return nil, err
	}
	var out []OpenIssue
	for _, i := range issues {
		o := OpenIssue{Repository: repo, IID: i.IID, DueDate: i.DueDate, WebURL: i.WebURL}
		if m := statusPattern.FindStringSubmatch(i.Description); m != nil {
			o.Kind, o.Week, o.Name = "status", m[1], m[2]
		} else if m := rollupPattern.FindStringSubmatch(i.Description); m != nil {
			o.Kind, o.Week, o.Name = "team-rollup", m[1], m[2]
			if m[2] == ProgramLevel {
				o.Kind = "program-rollup"
			}
		} else {
			continue
		}
		if slices.Contains(weeks, o.Week) {
			out = append(out, o)
		}
	}
	return out, nil
}

// weekRange lists the ISO weeks from first to last, inclusive.
func weekRange(first, last string) ([]string, error) {
	from, err := ParseWeek(first)
	if err != nil {
		return nil, err
	}
	to, err := ParseWeek(last)
	if err != nil {
		return nil, err
	}
	if to.Before(from) {
		return nil, fmt.Errorf("the range ends at %s before it starts at %s", last, first)
	}
	var weeks []string
	for w := from; !w.After(to); w = w.AddDate(0, 0, 7) {
		weeks = append(weeks, ISOWeek(w))
	}
	return weeks, nil
}

// quartersOf lists the quarters the weeks fall in, in order.
func quartersOf(weeks []string) []string {
	var out []string
	for _, w := range weeks {
		start, _ := ParseWeek(w)
		if q := Quarter(start); !slices.Contains(out, q) {
			out = append(out, q)
		}
	}
	return out
}
