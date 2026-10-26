// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// SprintsDir is the folder of the sprint pages, one per release sprint
// (GH-139).
const SprintsDir = "sprints"

// sprint is one release's sprint: its release, its days, and its title by
// the ISO weeks it spans.
type sprint struct {
	Release timeline.Release
	Page    string
	Title   string
}

// sprintsOf is the program's sprints, one per release with a sprint, in
// road-map order.
func sprintsOf(f timeline.Facts) []sprint {
	var out []sprint
	for _, r := range f.Releases {
		if r.Sprint == nil || r.Sprint.Start == "" || r.Sprint.End == "" {
			continue
		}
		out = append(out, sprint{Release: r, Page: SprintsDir + "/" + r.Version + ".md",
			Title: "Sprint " + weeks(r.Sprint.Start, r.Sprint.End) + ", release " + r.Version})
	}
	return out
}

// weeks names a span by its ISO weeks: W43–45, or W52–2027-W02 across a
// year's end.
func weeks(start, end string) string {
	s, err1 := time.Parse(time.DateOnly, start)
	e, err2 := time.Parse(time.DateOnly, end)
	if err1 != nil || err2 != nil {
		return start + " to " + end
	}
	sy, sw := s.ISOWeek()
	ey, ew := e.ISOWeek()
	if sy == ey {
		return fmt.Sprintf("W%02d–%02d", sw, ew)
	}
	return fmt.Sprintf("%d-W%02d–%d-W%02d", sy, sw, ey, ew)
}

// overlaps reports whether a planned span shares a day with the sprint.
func overlaps(p *timeline.Span, s *timeline.Span) bool {
	return p != nil && p.Start != "" && p.End != "" && p.Start <= s.End && p.End >= s.Start
}

// within reports whether a planned span lies inside the sprint.
func within(p *timeline.Span, s *timeline.Span) bool {
	return p != nil && p.Start != "" && p.End != "" && p.Start >= s.Start && p.End <= s.End
}

// SprintPages are a page per release sprint and the folder's index (GH-139):
// each sprint's days and release, the epics active in it, the stories
// planned into it, and the Sprint Goals committed for its release, linked
// to their pages and trackers.
func SprintPages(f timeline.Facts, nodes map[string]*Node) map[string][]byte {
	pages := map[string][]byte{}
	head := generated("hierarchy", "the timeline facts")
	sprints := sprintsOf(f)
	var index strings.Builder
	index.WriteString(head + "# Sprints\n\nEach sprint of the program, three weeks ending in a release, with the epics and stories planned into it and the Sprint Goals committed for its release.\n\n")
	if len(sprints) == 0 {
		index.WriteString("No release has a sprint yet.\n")
	} else {
		index.WriteString("| sprint | days | release | status |\n| --- | --- | --- | --- |\n")
	}
	for _, sp := range sprints {
		r := sp.Release
		fmt.Fprintf(&index, "| [%s](%s.md) | %s to %s | %s | %s |\n", sp.Title, r.Version, r.Sprint.Start, r.Sprint.End, r.Version, r.Status)
		pages[Dir+"/"+sp.Page] = []byte(sprintPage(sp, f, nodes, head))
	}
	pages[Dir+"/"+SprintsDir+"/index.md"] = []byte(index.String())
	return pages
}

func sprintPage(sp sprint, f timeline.Facts, nodes map[string]*Node, head string) string {
	r := sp.Release
	link := func(n *Node) string { return "[" + n.Title + "](../" + n.Page + ")" }
	var b strings.Builder
	fmt.Fprintf(&b, "%s# %s\n\n%s to %s. Release %s, %s", head, sp.Title, r.Sprint.Start, r.Sprint.End, r.Version, r.Status)
	if r.Tag != "" {
		fmt.Fprintf(&b, ", tag %s", r.Tag)
	}
	b.WriteString(". [The program's timeline](../timeline/index.md) draws it among the others.\n\n")

	// The stories planned into the sprint: inside its days, or placed in
	// its release.
	var stories []*Node
	inSprint := map[string]int{}
	for _, id := range sortedNodes(nodes) {
		n := nodes[id]
		if n.Kind == "story" && (within(n.Planned, r.Sprint) || n.Release == r.Version) {
			stories = append(stories, n)
			for p := nodes[n.Parent]; p != nil; p = nodes[p.Parent] {
				if p.Kind == "epic" {
					inSprint[p.ID]++
					break
				}
			}
		}
	}
	// The epics active in it: their days overlap the sprint, or a story of
	// theirs is in it.
	b.WriteString("## Epics\n\n")
	var epics []*Node
	for _, id := range sortedNodes(nodes) {
		n := nodes[id]
		if n.Kind == "epic" && (overlaps(n.Planned, r.Sprint) || inSprint[n.ID] > 0) {
			epics = append(epics, n)
		}
	}
	if len(epics) == 0 {
		b.WriteString("No epic is planned in this sprint.\n\n")
	} else {
		b.WriteString("| epic | state | points | planned | stories in this sprint |\n| --- | --- | --- | --- | --- |\n")
		for _, n := range epics {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d |\n", link(n), n.Status, pointsCell(n), spanCell(n.Planned), inSprint[n.ID])
		}
		b.WriteString("\n")
	}
	b.WriteString("## Stories\n\n")
	if len(stories) == 0 {
		b.WriteString("No story is planned into this sprint yet.\n\n")
	} else {
		b.WriteString("| story | epic | state | points |\n| --- | --- | --- | --- |\n")
		for _, n := range stories {
			epic := ""
			for p := nodes[n.Parent]; p != nil; p = nodes[p.Parent] {
				if p.Kind == "epic" {
					epic = link(p)
					break
				}
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", link(n), epic, n.Status, pointsCell(n))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Sprint Goals\n\n")
	var goals []timeline.Item
	for _, it := range f.Items {
		if it.Kind == timeline.KindSprintGoal && it.Release == r.Version {
			goals = append(goals, it)
		}
	}
	if len(goals) == 0 {
		b.WriteString("No Sprint Goal is committed for this release yet.\n")
	} else {
		b.WriteString("| Sprint Goal | state | stories |\n| --- | --- | --- |\n")
		for _, g := range goals {
			var selected []string
			for _, s := range g.Selects {
				if n, ok := nodes[s]; ok {
					selected = append(selected, link(n))
				} else {
					selected = append(selected, s)
				}
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", itemLink(g), g.Status, strings.Join(selected, ", "))
		}
	}
	return b.String()
}

func pointsCell(n *Node) string {
	if n.Points == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d", n.PointsDone, n.Points)
}

func spanCell(s *timeline.Span) string {
	if s == nil {
		return ""
	}
	return s.Start + " to " + s.End
}
