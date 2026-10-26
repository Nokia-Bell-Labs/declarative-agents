// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/repofiles"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
)

// StatusPage is the generated page beside ARCHITECTURE that renders the
// view for a reader who runs nothing (srd045 R8.5).
const StatusPage = "docs/architecture-status.md"

// The states a component is in (srd045 R8.5).
const (
	Actual       = "actual"
	InTransition = "in_transition"
	Intended     = "intended"
)

// ArchitectureView is each component of ARCHITECTURE with its state, the
// plan against the architecture as a reader asks about it (GH-57): what
// exists, what is being built and when it lands, and what is planned beyond.
type ArchitectureView struct {
	// Tag is the latest tag reachable from HEAD, the actual state; empty
	// when the repository has none.
	Tag        string            `json:"tag,omitempty"`
	Components []ComponentStatus `json:"components"`
	// Undeclared is each citation of a component ARCHITECTURE does not
	// declare, which the gate refuses; the view names it rather than drop it
	// (GH-88).
	Undeclared []Undeclared `json:"undeclared,omitempty"`
	// Teams are the team repositories whose stories the view counted, and
	// Unread the ones the registry names that it could not read, so a
	// count short of a team says so (srd045 R3.4; GH-2726).
	Teams  []TeamSource       `json:"teams,omitempty"`
	Unread []repofiles.Unread `json:"unread,omitempty"`
}

// Undeclared is a hierarchy document citing a component ARCHITECTURE does not
// declare.
type Undeclared struct {
	Component string `json:"component"`
	Document  string `json:"document"`
}

// ComponentStatus is one component's state: actual as of Tag, unchanged
// since; in_transition, added or changed since and cited by agreed
// documents; or intended, added or changed since and cited by none.
type ComponentStatus struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Tag   string `json:"tag,omitempty"`
	Added bool   `json:"added,omitempty"`
	// Deferred is a component ARCHITECTURE's status declares deferred.
	Deferred  bool             `json:"deferred,omitempty"`
	Documents []StatusDocument `json:"documents,omitempty"`
}

// StatusDocument is an agreed hierarchy document moving a component: the
// release the road map plans it for, through the initiative it rolls up to,
// and its story points done over total (srd045 R3.3).
type StatusDocument struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Release       string `json:"release,omitempty"`
	ReleaseStatus string `json:"release_status,omitempty"`
	Done          int    `json:"done"`
	Total         int    `json:"total"`
	// NotRolledUp says the document's stories live in team repositories
	// none of which is checked out beside this one, so its points are not
	// counted rather than zero (GH-89).
	NotRolledUp bool `json:"not_rolled_up,omitempty"`
}

// Architecture is the view of the repository at dir, from its working
// tree, the tag of its road map's latest released release through local
// git, its road map, and its sprint-end summaries; it opens no tracker and
// uses no network (srd045 R8.3). A component is actual when it is
// unchanged since that release and no agreed document citing it still has
// work open, so a component the release declared but no story has built
// is not (GH-99); a deferred one never is.
func Architecture(dir string) (ArchitectureView, error) {
	return architecture(dir, false, SiblingSources(dir), nil)
}

// ArchitectureFrom is the view with its stories counted from the team
// sources, as the registry names and reads them, the unread named
// (GH-2726).
func ArchitectureFrom(dir string, sources []TeamSource, unread []repofiles.Unread) (ArchitectureView, error) {
	return architecture(dir, false, sources, unread)
}

// architecture is the view, from every document on disk, or, when
// trackedOnly, from the documents git tracks: the stale check compares the
// committed page with what is committed, and an untracked document is the
// untracked finding's alone (GH-114).
func architecture(dir string, trackedOnly bool, sources []TeamSource, unread []repofiles.Unread) (ArchitectureView, error) {
	ids, now, err := workingComponents(dir)
	if err != nil {
		return ArchitectureView{}, err
	}
	v := ArchitectureView{Tag: baselineTag(dir), Components: []ComponentStatus{}, Unread: unread}
	for _, src := range sources {
		if src.Source != SourceSibling {
			v.Teams = append(v.Teams, src)
		}
	}
	shipped, err := taggedComponents(dir, v.Tag)
	if err != nil {
		return v, err
	}
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return v, err
	}
	if trackedOnly {
		docs = withoutUntracked(dir, docs)
	}
	done, err := sprintgoal.StoriesDone(dir)
	if err != nil {
		return v, err
	}
	doneSet := map[string]bool{}
	for id := range done.Release {
		doneSet[id] = true
	}
	progress, _ := hierarchy.RollUp(docs, doneSet)
	points := map[string]hierarchy.Progress{}
	for _, p := range progress {
		points[p.ID] = p
	}
	teams, read := TeamProgress(sources, docs)
	for id, p := range teams {
		sum := points[id]
		sum.ID, sum.Kind = id, p.Kind
		sum.Done += p.Done
		sum.Total += p.Total
		points[id] = sum
	}
	releases, err := roadMapReleases(dir)
	if err != nil {
		return v, err
	}
	byID := map[string]*hierarchy.Doc{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	deferred, err := deferredComponents(dir)
	if err != nil {
		return v, err
	}
	for _, d := range docs {
		for _, a := range d.Architecture {
			if _, ok := now[a]; !ok && !strings.Contains(a, "://") {
				v.Undeclared = append(v.Undeclared, Undeclared{Component: a, Document: d.ID})
			}
		}
	}
	released := releasedDue(releases, byID)
	for _, id := range ids {
		before, wasShipped := shipped[id]
		unchanged := wasShipped && before == now[id]
		c := ComponentStatus{ID: id, Added: !wasShipped, Deferred: deferred[id]}
		open := false
		for _, d := range docs {
			if d.Status != "agreed" || !slices.Contains(d.Architecture, id) {
				continue
			}
			sd := StatusDocument{ID: d.ID, Kind: string(d.Kind), Title: d.Title}
			if p, ok := points[d.ID]; ok {
				sd.Done, sd.Total = p.Done, p.Total
			} else if d.Kind == hierarchy.Story {
				sd.Total = d.PointsValue()
				if doneSet[d.ID] {
					sd.Done = sd.Total
				}
			}
			if r, ok := targetRelease(d, byID, releases); ok {
				sd.Release, sd.ReleaseStatus = r.version, r.status
			}
			sd.NotRolledUp = sd.Total == 0 && !read && d.Kind != hierarchy.Story
			c.Documents = append(c.Documents, sd)
			open = open || openWork(d, sd, byID, released)
		}
		switch {
		case c.Deferred:
			c.State, c.Documents = Intended, nil
		case unchanged && !open:
			c.State, c.Tag, c.Documents = Actual, v.Tag, nil
		case len(c.Documents) > 0:
			c.State = InTransition
		default:
			c.State = Intended
		}
		v.Components = append(v.Components, c)
	}
	return v, nil
}

type release struct {
	version, status, tag string
	initiatives          map[string]bool
	// items is the initiative items the release delivers, as
	// initiative#Rn.m.
	items []string
}

// roadMapReleases reads docs/road-map.md's releases in order, each with the
// initiatives its deliverables name (an item initiative#R1.1 names its
// initiative); none when the road map is absent or does not parse.
func roadMapReleases(dir string) ([]release, error) {
	b, err := os.ReadFile(filepath.Join(dir, "docs", "road-map.md"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return parseReleases(b), nil
}

// parseReleases reads a road map's releases in order; none when it does
// not parse.
func parseReleases(b []byte) []release {
	d, err := corpus.ParseMarkdown(b)
	if err != nil {
		return nil
	}
	var out []release
	for _, r := range d.Records("release") {
		rel := release{version: r.Key, status: r.Attrs["status"], tag: corpus.TreeString(r.Field("release").Field("publication").Tree(), "tag"), initiatives: map[string]bool{}}
		for _, item := range r.Field("deliverables").List() {
			id, _, _ := strings.Cut(item, "#")
			rel.initiatives[strings.TrimSpace(id)] = true
			rel.items = append(rel.items, strings.TrimSpace(item))
		}
		out = append(out, rel)
	}
	return out
}

// targetRelease is the release a document's work lands in (GH-89): of the
// road map's releases not yet released that deliver the initiative d rolls
// up to, through its parents in this repository, the first whose items fall
// due in or after the last week of d's horizon, the nearest one d or a
// parent states; the
// last of them when the horizon runs past every one; the first of them
// when no horizon or due date says more. A released release is never the
// answer: a component still changing has not shipped in it.
func targetRelease(d *hierarchy.Doc, byID map[string]*hierarchy.Doc, releases []release) (release, bool) {
	initiative, horizon := "", ""
	seen := map[string]bool{}
	for cur := d; cur != nil && !seen[cur.ID]; cur = byID[cur.Parent] {
		seen[cur.ID] = true
		if horizon == "" {
			horizon = cur.Horizon
		}
		initiative = cur.ID
		if cur.Kind == hierarchy.Initiative {
			break
		}
		if byID[cur.Parent] == nil && cur.Parent != "" {
			initiative = cur.Parent
			break
		}
	}
	var open []release
	for _, r := range releases {
		if r.initiatives[initiative] && r.status != "released" {
			open = append(open, r)
		}
	}
	if len(open) == 0 {
		return release{}, false
	}
	end, ok := horizonLastWeek(horizon)
	in := byID[initiative]
	if !ok || in == nil {
		return open[0], true
	}
	for _, r := range open {
		due, ok := releaseDue(r, initiative, in)
		if !ok {
			return open[0], true
		}
		if !due.Before(end) {
			return r, true
		}
	}
	return open[len(open)-1], true
}

// releasedDue is the due date of the work the road map's latest released
// release delivered: the latest due date of its items, through the
// initiatives the repository holds; zero when nothing is released or no
// due date says.
func releasedDue(releases []release, byID map[string]*hierarchy.Doc) time.Time {
	var due time.Time
	for _, r := range releases {
		if r.status != "released" {
			continue
		}
		for id := range r.initiatives {
			if in := byID[id]; in != nil {
				if d, ok := releaseDue(r, id, in); ok && d.After(due) {
					due = d
				}
			}
		}
	}
	return due
}

// openWork reports whether an agreed document citing a component still has
// work open: some of its story points not done, where they are known, or a
// horizon running past the work the latest released release delivered,
// since stories not yet written are work too (GH-114); with nothing
// released, any work at all (GH-99).
func openWork(d *hierarchy.Doc, sd StatusDocument, byID map[string]*hierarchy.Doc, released time.Time) bool {
	if sd.Total > 0 && sd.Done < sd.Total {
		return true
	}
	if released.IsZero() {
		return true
	}
	seen := map[string]bool{}
	for cur := d; cur != nil && !seen[cur.ID]; cur = byID[cur.Parent] {
		seen[cur.ID] = true
		if week, ok := horizonLastWeek(cur.Horizon); ok {
			return week.After(released)
		}
	}
	return false
}

// releaseDue is the latest due date of the initiative's items a release
// delivers, read from the initiative's requirement groups.
func releaseDue(r release, initiative string, in *hierarchy.Doc) (time.Time, bool) {
	var latest time.Time
	for _, item := range r.items {
		id, req, ok := strings.Cut(item, "#")
		if !ok || id != initiative {
			continue
		}
		group, _, _ := strings.Cut(req, ".")
		due, err := time.Parse(time.DateOnly, in.Due[group])
		if err != nil {
			return time.Time{}, false
		}
		if due.After(latest) {
			latest = due
		}
	}
	return latest, !latest.IsZero()
}

// horizonLastWeek is the Monday of the last week of a horizon written as
// ISO weeks, 2026-W43..2026-W51, or a single week.
func horizonLastWeek(h string) (time.Time, bool) {
	_, last, ok := strings.Cut(h, "..")
	if !ok {
		last = h
	}
	var year, week int
	if _, err := fmt.Sscanf(strings.TrimSpace(last), "%d-W%d", &year, &week); err != nil || week < 1 || week > 53 {
		return time.Time{}, false
	}
	// The Monday of ISO week 1 is the Monday on or before January 4.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
	return monday.AddDate(0, 0, 7*(week-1)), true
}

// TeamProgress is the story points of the team repositories' agreed stories
// that roll up to this repository's documents through a parent pinned by
// URL, read from the team sources with each one's sprint-end summaries,
// added to each document up the chain as RollUp adds them (GH-89, GH-2726);
// read reports whether any source holds hierarchy documents.
func TeamProgress(sources []TeamSource, docs []*hierarchy.Doc) (points map[string]hierarchy.Progress, read bool) {
	points = map[string]hierarchy.Progress{}
	ours := map[string]*hierarchy.Doc{}
	for _, d := range docs {
		ours[d.ID] = d
	}
	for _, src := range sources {
		sib := src.Dir
		theirs, err := hierarchy.Load(sib)
		if err != nil || len(theirs) == 0 {
			continue
		}
		read = true
		done, _ := sprintgoal.StoriesDone(sib)
		local := map[string]*hierarchy.Doc{}
		for _, d := range theirs {
			local[d.ID] = d
		}
		for _, s := range theirs {
			if s.Kind != hierarchy.Story || s.Status != "agreed" {
				continue
			}
			target := ""
			seen := map[string]bool{}
			for cur := s; cur != nil && !seen[cur.ID]; cur = local[cur.Parent] {
				seen[cur.ID] = true
				if strings.Contains(cur.Parent, "://") {
					target = linkedID(cur.Parent)
					break
				}
			}
			_, isDone := done.Release[s.ID]
			seen = map[string]bool{}
			for id := target; id != "" && !seen[id]; {
				seen[id] = true
				// The theme counts them too, as RollUp sums a theme from
				// its initiatives (GH-141).
				p, ok := ours[id]
				if !ok {
					break
				}
				sum := points[id]
				sum.Kind = p.Kind
				sum.Total += s.PointsValue()
				if isDone {
					sum.Done += s.PointsValue()
				}
				points[id] = sum
				id = p.Parent
			}
		}
	}
	return points, read
}

// TeamDocs are the hierarchy documents of the team sources that roll up to
// docs through a parent pinned by URL, as TeamProgress counts them, for a
// view that draws them under the documents they serve (GH-109, GH-2726).
// Each is a copy named <team>/<id>, so two teams' st001 stay apart, with
// its parent so named too, or, where it pins one, read as the document of
// docs it names. done maps each done story, so named, to the release its
// team's sprint-end summary records.
func TeamDocs(sources []TeamSource, docs []*hierarchy.Doc) (team []*hierarchy.Doc, done map[string]string) {
	done = map[string]string{}
	ours := map[string]bool{}
	for _, d := range docs {
		ours[d.ID] = true
	}
	for _, src := range sources {
		sib := src.Dir
		theirs, err := hierarchy.Load(sib)
		if err != nil || len(theirs) == 0 {
			continue
		}
		name := src.Name
		local := map[string]*hierarchy.Doc{}
		for _, d := range theirs {
			local[d.ID] = d
		}
		for _, d := range theirs {
			serves := false
			seen := map[string]bool{}
			for cur := d; cur != nil && !seen[cur.ID]; cur = local[cur.Parent] {
				seen[cur.ID] = true
				if strings.Contains(cur.Parent, "://") {
					serves = ours[linkedID(cur.Parent)]
					break
				}
			}
			if !serves {
				continue
			}
			c := *d
			c.ID = name + "/" + d.ID
			if strings.Contains(d.Parent, "://") {
				c.Parent = linkedID(d.Parent)
			} else {
				c.Parent = name + "/" + d.Parent
			}
			team = append(team, &c)
		}
		if summaries, err := sprintgoal.StoriesDone(sib); err == nil {
			for id, v := range summaries.Release {
				done[name+"/"+id] = v
			}
		}
	}
	return team, done
}

// linkedID is the document id a pinned URL names: its file's base name
// without the extension or any fragment.
func linkedID(url string) string {
	url, _, _ = strings.Cut(url, "#")
	return strings.TrimSuffix(path.Base(url), ".md")
}

// Page renders the view as the generated page a reader opens beside
// ARCHITECTURE: what exists, what is being built and when it lands, and
// what is intended with no plan yet.
func (v ArchitectureView) Page() string {
	var b strings.Builder
	b.WriteString("<!-- Generated by scrum-master backlog architecture --write from docs/ARCHITECTURE.md, the latest tag, the hierarchy documents, the road map, and the sprint-end summaries. Regenerate it; do not edit it. -->\n")
	b.WriteString("# Architecture status\n\n")
	shipped := "no release yet"
	if v.Tag != "" {
		shipped = "release " + v.Tag
	}
	fmt.Fprintf(&b, "Each component of [ARCHITECTURE](ARCHITECTURE.md), by what it is today. The actual state is %s.\n", shipped)
	section := func(title, intro string, state string, row func(ComponentStatus) []string) {
		var rows []ComponentStatus
		for _, c := range v.Components {
			if c.State == state {
				rows = append(rows, c)
			}
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n\n", title, intro)
		if len(rows) == 0 {
			b.WriteString("None.\n")
			return
		}
		for _, c := range rows {
			b.WriteString("- " + strings.Join(row(c), "; ") + "\n")
		}
	}
	section("What exists", "Components the latest release shipped and ARCHITECTURE has not changed since.", Actual, func(c ComponentStatus) []string {
		return []string{fmt.Sprintf("**%s**, as of %s", c.ID, c.Tag)}
	})
	section("What is being built", "Components added or changed since the latest release, with the agreed documents moving them, the release each is planned for, and its story points done.", InTransition, func(c ComponentStatus) []string {
		out := []string{"**" + c.ID + "**" + map[bool]string{true: " (new)", false: " (changing)"}[c.Added]}
		for _, d := range c.Documents {
			when := "no release planned yet"
			if d.Release != "" {
				when = "release " + d.Release + " (" + d.ReleaseStatus + ")"
			}
			points := fmt.Sprintf("%d of %d points done", d.Done, d.Total)
			if d.NotRolledUp {
				points = "points not rolled up: its stories live in team repositories, none checked out beside this one"
			}
			out = append(out, fmt.Sprintf("%s %s, %s, %s", d.Kind, d.ID, when, points))
		}
		return out
	})
	section("What is intended", "Components added or changed since the latest release that no agreed document plans yet.", Intended, func(c ComponentStatus) []string {
		out := "**" + c.ID + "**" + map[bool]string{true: " (new)", false: " (changing)"}[c.Added]
		if c.Deferred {
			out += ", deferred"
		}
		return []string{out}
	})
	if len(v.Unread) > 0 {
		b.WriteString("\n## Not counted\n\nTeam repositories the program's subsystem SRDs name that could not be read, so their stories are not in the points above.\n\n")
		for _, u := range v.Unread {
			fmt.Fprintf(&b, "- **%s**: %s\n", u.Repository, u.Reason)
		}
	}
	if len(v.Undeclared) > 0 {
		b.WriteString("\n## Cited but not declared\n\nComponents hierarchy documents cite that ARCHITECTURE does not declare; the corpus gate refuses them until a design change declares them.\n\n")
		for _, u := range v.Undeclared {
			fmt.Fprintf(&b, "- **%s**, cited by %s\n", u.Component, u.Document)
		}
	}
	return b.String()
}

func init() {
	var (
		dir   string
		write bool
	)
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "architecture", Summary: "each ARCHITECTURE component: actual as of the latest tag, in transition through agreed documents, or intended; with --write, the generated page beside ARCHITECTURE",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&dir, "dir", ".", "the repository's checkout")
			fs.BoolVar(&write, "write", false, "write "+StatusPage+", the page a reader opens")
		},
		Run: func(ctx *cli.Context) error {
			// The team repositories the registry names, read through the
			// tracker when one is reachable; else the checkouts beside
			// this one, offline (srd045 R8.3; GH-2726).
			t, _ := ctx.Tracker()
			scratch, err := os.MkdirTemp("", "scrum-master-teams-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(scratch)
			sources, unread, err := Sources(t, dir, "main", scratch)
			if err != nil {
				return err
			}
			v, err := ArchitectureFrom(dir, sources, unread)
			if err != nil {
				return err
			}
			page := v.Page()
			if write && !ctx.DryRun {
				if err := os.WriteFile(filepath.Join(dir, StatusPage), []byte(page), 0o644); err != nil {
					return err
				}
			}
			return ctx.Emit(v, page)
		},
	})
}

func init() {
	var dir string
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "components", Summary: "the component ids docs/ARCHITECTURE.md declares, which a hierarchy document's architecture item may cite; planning checks a component here before naming it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&dir, "dir", ".", "the repository's checkout")
		},
		Run: func(ctx *cli.Context) error {
			ids, _, err := workingComponents(dir)
			if err != nil {
				return err
			}
			if ids == nil {
				ids = []string{}
			}
			human := "docs/ARCHITECTURE.md declares no component; a new one is a design decision (the design skill)\n"
			if len(ids) > 0 {
				human = strings.Join(ids, "\n") + "\n"
			}
			return ctx.Emit(struct {
				Components []string `json:"components"`
			}{ids}, human)
		},
	})
}
