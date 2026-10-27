// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package timeline computes the planned and actual spans the site's Gantt
// views draw (srd048 R3): one item per initiative, epic, feature, story,
// and Sprint Goal, on the sprint axis. The computation is a plain function
// over loaded inputs; the site word loads them from the system repository's
// checkout, its local git, and the tracker.
package timeline

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// The states an item is in on the timeline.
const (
	Done   = "done"
	Active = "active"
	Future = "future"
)

// KindSprintGoal is the kind of a Sprint Goal issue on the timeline, and
// KindIssue of any other tracker issue.
const (
	KindSprintGoal = "sprint_goal"
	KindIssue      = "issue"
)

// Span is a stretch of days, start and end inclusive, as YYYY-MM-DD; an
// open end is empty.
type Span struct {
	Start string `json:"start"`
	End   string `json:"end,omitempty"`
}

// Release is one road-map release on the sprint axis: its sprint's days,
// from the ISO weeks the road map or its milestone names.
type Release struct {
	Version   string   `json:"version"`
	Status    string   `json:"status"`
	Tag       string   `json:"tag,omitempty"`
	Milestone string   `json:"milestone,omitempty"`
	Items     []string `json:"items,omitempty"`
	Sprint    *Span    `json:"sprint,omitempty"`
}

// Goal is a Sprint Goal issue: the release it plans, the stories its
// Sprint Backlog selected, and when it opened and closed.
type Goal struct {
	Ref, Title, Release, Created, Closed string
	// URL is the issue's page in the tracker, which the timeline links.
	URL     string
	Stories []string
}

// Issue is a tracker issue other than a Sprint Goal: the release its
// milestone names, the story it implements, and when it opened and closed.
type Issue struct {
	Ref, Title, Milestone, Story, Created, Closed, URL string
	// DependsOn is the issues its Depends-on and Blocked-by lines name,
	// as project#iid (GH-135).
	DependsOn []string
}

// Item is one row of the timeline.
type Item struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Parent       string   `json:"parent,omitempty"`
	Status       string   `json:"status"`
	Architecture []string `json:"architecture,omitempty"`
	Release      string   `json:"release,omitempty"`
	Tag          string   `json:"tag,omitempty"`
	Planned      *Span    `json:"planned,omitempty"`
	Actual       *Span    `json:"actual,omitempty"`
	Points       int      `json:"points"`
	PointsDone   int      `json:"points_done"`
	// URL is where the item is read: a Sprint Goal's issue page.
	URL string `json:"url,omitempty"`
	// Horizon, Owner, and Rank are the document's own; Statement is a
	// story's "as a, I want, so that"; Source is the document's page on
	// its repository's forge and Path its path in that repository;
	// Selects is the stories a Sprint Goal's backlog lists, named as the
	// facts name them (GH-126).
	Horizon   string   `json:"horizon,omitempty"`
	Owner     string   `json:"owner,omitempty"`
	Rank      string   `json:"rank,omitempty"`
	Statement string   `json:"statement,omitempty"`
	Source    string   `json:"source,omitempty"`
	Path      string   `json:"path,omitempty"`
	Selects   []string `json:"selects,omitempty"`
	// Program names the sub-program whose document this is, taken whole
	// from its published facts; its work is on that program's site.
	Program string `json:"program,omitempty"`
	// DependsOn is the items it waits on: a planning document's
	// depends_on and blocked_by links, an issue's Depends-on and
	// Blocked-by lines (GH-135).
	DependsOn []string `json:"depends_on,omitempty"`
	// Due is an initiative's requirement groups, each with the day it
	// falls due, in group order: what the initiative promises (GH-136).
	Due []Due `json:"due,omitempty"`
}

// Due is one requirement group and the day it falls due.
type Due struct {
	Group string `json:"group"`
	Date  string `json:"date"`
}

// Facts are the timeline as of the working tree, or as planned at AsOf, a
// corpus tag.
type Facts struct {
	AsOf     string    `json:"as_of,omitempty"`
	Today    string    `json:"today"`
	Releases []Release `json:"releases"`
	Items    []Item    `json:"items"`
	// Teams are the program's scrums, as the system's subsystem SRDs
	// declare them; a tag's facts hold none.
	Teams []Team `json:"teams,omitempty"`
	// Parent is the larger program this one is a team of, when its
	// control/program.md names one (GH-2740).
	Parent *ProgramRef `json:"parent,omitempty"`
}

// ProgramRef names another program: its system repository and its site.
type ProgramRef struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Site       string `json:"site,omitempty"`
}

// Program is a document a sub-program serves this plan with, as its own
// published facts give it.
type Program struct {
	Name       string
	Points     int
	PointsDone int
	Status     string
	Planned    *Span
	Actual     *Span
}

// Team is one scrum: the subsystem it owns, its Product Owner, the
// components of its area, and the repositories they live in. Name is what
// its documents carry as <name>/<id>, its scrum repository's name.
type Team struct {
	Name         string   `json:"name"`
	Subsystem    string   `json:"subsystem"`
	Owner        string   `json:"owner"`
	Components   []string `json:"components"`
	Repositories []string `json:"repositories"`
	// Scrum is its scrum repository, where it plans: the one its SRD
	// names apart from its code, else its first component's (GH-2738).
	Scrum string `json:"scrum"`
	// Apart says the SRD names the scrum repository, apart from the code.
	Apart bool `json:"apart,omitempty"`
	// Program says its scrum repository is a program's system
	// repository: a sub-program, whose own site Site is (GH-2740).
	Program bool   `json:"program,omitempty"`
	Site    string `json:"site,omitempty"`
	// URL is its scrum repository's page on the forge, when the SRD names
	// one a browser opens.
	URL string `json:"url,omitempty"`
}

// Input is what the facts are computed from.
type Input struct {
	AsOf     string
	Today    time.Time
	Docs     []*hierarchy.Doc
	Releases []Release
	// Done maps each done story to the release whose sprint-end summary
	// records it; Landed maps a release to the day its summary merged,
	// the actual end of its done stories.
	Done   map[string]string
	Landed map[string]string
	Goals  []Goal
	Issues []Issue
	// Programs are the documents a sub-program serves this plan with,
	// taken whole from its published facts: their points, state, and days
	// are its own, never recomputed from children this plan does not read
	// (GH-2740).
	Programs map[string]Program
	// Sources maps a document id to its page on its repository's forge,
	// Paths to its path within that repository.
	Sources, Paths map[string]string
}

func day(t time.Time) string { return t.Format(time.DateOnly) }

// Make computes the timeline (srd048 R3.1). A story is planned in the
// sprint of the release whose Sprint Goal selected it, else the release
// whose summary records it done, else its nearest horizon; it is done when
// a summary records it, its actual end the day that summary merged. Any
// other document is planned over its horizon, else over its stories'
// planned days; it is done when every agreed story under it is. An agreed
// item not done is active once its planned days have begun, past their end
// included, since the work is still open; any other item is future.
func Make(in Input) Facts {
	f := Facts{AsOf: in.AsOf, Today: day(in.Today), Releases: in.Releases, Items: []Item{}}
	if f.Releases == nil {
		f.Releases = []Release{}
	}
	byVersion := map[string]Release{}
	for _, r := range in.Releases {
		byVersion[r.Version] = r
	}
	byID := map[string]*hierarchy.Doc{}
	children := map[string][]*hierarchy.Doc{}
	for _, d := range in.Docs {
		byID[d.ID] = d
	}
	for _, d := range in.Docs {
		if byID[d.Parent] != nil {
			children[d.Parent] = append(children[d.Parent], d)
		}
	}
	selected := map[string]string{}
	for _, g := range in.Goals {
		for _, s := range g.Stories {
			if _, ok := selected[s]; !ok {
				selected[s] = g.Release
			}
		}
	}
	horizon := func(d *hierarchy.Doc) *Span {
		seen := map[string]bool{}
		for cur := d; cur != nil && !seen[cur.ID]; cur = byID[cur.Parent] {
			seen[cur.ID] = true
			if s, e, err := ParseWeekRange(cur.Horizon); err == nil {
				return &Span{day(s), day(e)}
			}
		}
		return nil
	}
	items := map[string]*Item{}
	var item func(d *hierarchy.Doc) *Item
	item = func(d *hierarchy.Doc) *Item {
		if it, ok := items[d.ID]; ok {
			return it
		}
		it := &Item{ID: d.ID, Kind: string(d.Kind), Title: d.Title, Parent: d.Parent, Status: Future, Architecture: d.Architecture,
			Horizon: d.Horizon, Owner: d.Owner, Rank: d.Rank, Source: in.Sources[d.ID], Path: in.Paths[d.ID], DependsOn: docDeps(d), Due: dues(d)}
		if d.Kind == hierarchy.Story && d.IWant != "" {
			it.Statement = fmt.Sprintf("As %s %s, I want %s, so that %s.", article(d.AsA), d.AsA, d.IWant, d.SoThat)
		}
		items[d.ID] = it
		if d.Kind == hierarchy.Story {
			it.Points = d.PointsValue()
			release := selected[d.ID]
			if v, ok := in.Done[d.ID]; ok {
				it.Status, it.PointsDone = Done, it.Points
				if release == "" {
					release = v
				}
				end := in.Landed[v]
				if end == "" && byVersion[v].Sprint != nil {
					end = byVersion[v].Sprint.End
				}
				if r := byVersion[release]; r.Sprint != nil {
					it.Actual = &Span{r.Sprint.Start, end}
				} else if end != "" {
					it.Actual = &Span{end, end}
				}
			}
			if r, ok := byVersion[release]; ok {
				it.Release, it.Tag, it.Planned = r.Version, r.Tag, r.Sprint
			}
			if it.Planned == nil {
				it.Planned = horizon(d)
			}
		} else if p, ok := in.Programs[d.ID]; ok {
			it.Program, it.Points, it.PointsDone, it.Status, it.Planned, it.Actual = p.Name, p.Points, p.PointsDone, p.Status, p.Planned, p.Actual
			if it.Status == "" {
				it.Status = Future
			}
		} else {
			var own, under *Span
			if s, e, err := ParseWeekRange(d.Horizon); err == nil {
				own = &Span{day(s), day(e)}
			}
			kids := children[d.ID]
			done, agreed := 0, 0
			for _, k := range kids {
				c := item(k)
				it.Points += c.Points
				it.PointsDone += c.PointsDone
				if k.Status == "agreed" {
					agreed++
					if c.Status == Done {
						done++
					}
				}
				if c.Planned != nil {
					under = widen(under, c.Planned)
				}
				if c.Actual != nil {
					it.Actual = widen(it.Actual, c.Actual)
				}
			}
			switch {
			case own != nil:
				it.Planned = own
			case under != nil:
				it.Planned = under
			default:
				it.Planned = horizon(d)
			}
			if agreed > 0 && done == agreed {
				it.Status = Done
			} else if it.Actual != nil {
				it.Actual = &Span{Start: it.Actual.Start}
			}
			if it.Planned != nil {
				if r, ok := landsIn(in.Releases, it.Planned.End); ok {
					it.Release, it.Tag = r.Version, r.Tag
				}
			}
		}
		// A story is active only once a release takes it: one no Sprint
		// Goal selects is unplanned, however early its horizon begins
		// (GH-141).
		placed := d.Kind != hierarchy.Story || it.Release != ""
		if it.Status != Done && d.Status == "agreed" && placed && it.Planned != nil && it.Planned.Start <= f.Today {
			it.Status = Active
		}
		return it
	}
	for _, d := range in.Docs {
		item(d)
	}
	for _, d := range in.Docs {
		f.Items = append(f.Items, *items[d.ID])
	}
	for _, g := range in.Goals {
		it := Item{ID: g.Ref, Kind: KindSprintGoal, Title: g.Title, Release: g.Release, Status: Future, URL: g.URL, Selects: g.Stories}
		if r, ok := byVersion[g.Release]; ok {
			it.Tag, it.Planned = r.Tag, r.Sprint
		}
		if g.Created != "" {
			it.Actual = &Span{Start: g.Created[:min(10, len(g.Created))]}
		}
		if g.Closed != "" {
			it.Status = Done
			if it.Actual == nil {
				it.Actual = &Span{}
			}
			it.Actual.End = g.Closed[:min(10, len(g.Closed))]
		} else if it.Planned != nil && it.Planned.Start <= f.Today {
			it.Status = Active
		}
		f.Items = append(f.Items, it)
	}
	// An issue is planned in the sprint of the release its milestone names
	// and runs from the day it opened to the day it closed (GH-96).
	byMilestone := map[string]Release{}
	for _, r := range in.Releases {
		byMilestone[r.Version] = r
		if r.Milestone != "" {
			byMilestone[r.Milestone] = r
		}
	}
	for _, i := range in.Issues {
		it := Item{ID: i.Ref, Kind: KindIssue, Title: i.Title, Parent: i.Story, Status: Future, URL: i.URL, DependsOn: i.DependsOn}
		if r, ok := byMilestone[i.Milestone]; ok {
			it.Release, it.Tag, it.Planned = r.Version, r.Tag, r.Sprint
		}
		if i.Created != "" {
			it.Actual = &Span{Start: i.Created[:min(10, len(i.Created))]}
		}
		switch {
		case i.Closed != "":
			it.Status = Done
			if it.Actual == nil {
				it.Actual = &Span{}
			}
			it.Actual.End = i.Closed[:min(10, len(i.Closed))]
		case it.Actual != nil && it.Actual.Start <= f.Today:
			it.Status = Active
		}
		f.Items = append(f.Items, it)
	}
	sort.SliceStable(f.Items, func(i, j int) bool { return f.Items[i].ID < f.Items[j].ID })
	return f
}

// widen is the span covering both a and b; an open end stays open.
func widen(a, b *Span) *Span {
	if a == nil {
		return &Span{b.Start, b.End}
	}
	out := &Span{a.Start, a.End}
	if b.Start < out.Start {
		out.Start = b.Start
	}
	if out.End != "" && (b.End == "" || b.End > out.End) {
		out.End = b.End
	}
	return out
}

// landsIn is the first release whose sprint ends on or after end: the
// release a document's planned work lands in.
func landsIn(releases []Release, end string) (Release, bool) {
	for _, r := range releases {
		if r.Sprint != nil && r.Sprint.End >= end {
			return r, true
		}
	}
	return Release{}, false
}

// article is "an" before a role that opens with a vowel, else "a".
func article(role string) string {
	if role != "" && strings.ContainsRune("aeiouAEIOU", rune(role[0])) {
		return "an"
	}
	return "a"
}

// docDeps is a planning document's depends_on and blocked_by links, each
// named as the facts name items: a link pinned by URL is the document it
// names, and a team's document's own link is in its team, under its
// checkout's name (GH-135).
func docDeps(d *hierarchy.Doc) []string {
	team, _, isTeam := strings.Cut(d.ID, "/")
	var out []string
	for _, key := range []string{"depends_on", "blocked_by"} {
		for _, t := range d.Links[key] {
			t = strings.TrimSpace(t)
			switch {
			case t == "":
				continue
			case strings.Contains(t, "://"):
				t, _, _ = strings.Cut(path.Base(t), "#")
				t = strings.TrimSuffix(t, ".md")
			case isTeam && !strings.Contains(t, "/"):
				t = team + "/" + t
			}
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// dues is a document's requirement groups with their due days, R1 before
// R2 before R10.
func dues(d *hierarchy.Doc) []Due {
	var out []Due
	for g, date := range d.Due {
		if date != "" {
			out = append(out, Due{Group: g, Date: date})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimPrefix(out[i].Group, "R"))
		b, _ := strconv.Atoi(strings.TrimPrefix(out[j].Group, "R"))
		if a != b {
			return a < b
		}
		return out[i].Group < out[j].Group
	})
	return out
}
