// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// GanttDir is the gantt word's folder, under the timeline's.
const GanttDir = "timeline/gantt"

// ChartRows is the most tasks one chart draws: past it a chart stops being
// legible, so the view is split instead (srd048 R5.2).
const ChartRows = 30

// GanttPages are the gantt word's pages, drawn from the timeline facts the
// timeline word wrote into the reports checkout dir (srd048 R5): the
// program's chart of initiatives, and one page per initiative with its
// epics and features as sections and their stories as tasks, each split
// into charts of at most ChartRows tasks.
func GanttPages(dir string) (map[string][]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", "facts.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s holds no timeline facts; run the site's timeline word first", dir)
	} else if err != nil {
		return nil, err
	}
	var f timeline.Facts
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s/timeline/facts.json: %w", Dir, err)
	}
	pages := ganttPages(f)
	html, err := timelineHTMLPage(dir)
	if err != nil {
		return nil, err
	}
	pages[Dir+"/"+GanttDir+"/program.html"] = html
	return pages, nil
}

func ganttPages(f timeline.Facts) map[string][]byte {
	pages := map[string][]byte{}
	byID := map[string]timeline.Item{}
	children := map[string][]timeline.Item{}
	var initiatives []timeline.Item
	for _, it := range f.Items {
		byID[it.ID] = it
	}
	for _, it := range f.Items {
		if it.Parent != "" {
			children[it.Parent] = append(children[it.Parent], it)
		}
		if it.Kind == "initiative" {
			initiatives = append(initiatives, it)
		}
	}
	head := generated("gantt", "the timeline facts") + "# %s\n\n%s\n\n"
	var program strings.Builder
	fmt.Fprintf(&program, head, "Program timeline", "Each initiative's planned days on the sprint axis, with each release at the end of its sprint. Done, active, and future items are drawn as such; "+
		"each initiative's own page draws its epics, features, and stories.")
	program.WriteString(charts("Initiatives", f.Releases, []section{{"Initiatives", initiatives}}))
	program.WriteString("\nThe [interactive timeline](program.html) draws every item, with the plan as it stood at each corpus tag to compare.\n")
	if len(initiatives) > 0 {
		program.WriteString("\n## Initiatives\n\n")
	}
	for _, in := range initiatives {
		fmt.Fprintf(&program, "- [%s](%s.md): %s\n", in.ID, in.ID, in.Title)
	}
	pages[Dir+"/"+GanttDir+"/program.md"] = []byte(program.String())
	for _, in := range initiatives {
		var sections []section
		for _, top := range children[in.ID] {
			s := section{title: top.ID, items: []timeline.Item{top}}
			var walk func(id string)
			walk = func(id string) {
				for _, c := range children[id] {
					s.items = append(s.items, c)
					walk(c.ID)
				}
			}
			walk(top.ID)
			sections = append(sections, s)
		}
		var page strings.Builder
		fmt.Fprintf(&page, head, in.Title, "The planned days of "+in.ID+"'s epics, features, and stories on the sprint axis, each epic a section, with each release at the end of its sprint. [The program's timeline](program.md).")
		page.WriteString(charts(in.ID, f.Releases, sections))
		pages[Dir+"/"+GanttDir+"/"+in.ID+".md"] = []byte(page.String())
	}
	return pages
}

// section is one section of a chart: its title and its tasks.
type section struct {
	title string
	items []timeline.Item
}

// charts draws sections as gantt charts of at most ChartRows tasks each,
// keeping a section in one chart where it fits and splitting one that does
// not; each chart carries the release milestones.
func charts(title string, releases []timeline.Release, sections []section) string {
	var groups [][]section
	var cur []section
	rows := 0
	flush := func() {
		if len(cur) > 0 {
			groups = append(groups, cur)
		}
		cur, rows = nil, 0
	}
	for _, s := range sections {
		items := drawable(s.items)
		for len(items) > 0 {
			if rows > 0 && rows+len(items) > ChartRows {
				flush()
			}
			take := min(len(items), ChartRows-rows)
			cur = append(cur, section{s.title, items[:take]})
			rows += take
			items = items[take:]
			if rows == ChartRows {
				flush()
			}
		}
	}
	flush()
	if len(groups) == 0 {
		return "Nothing on the timeline has planned days yet.\n"
	}
	var b strings.Builder
	for i, g := range groups {
		name := title
		if len(groups) > 1 {
			name = fmt.Sprintf("%s, part %d of %d", title, i+1, len(groups))
			fmt.Fprintf(&b, "\n## Part %d of %d\n\n", i+1, len(groups))
		}
		b.WriteString(gantt(name, releases, g))
	}
	return b.String()
}

// drawable is the items with planned days, the only ones a chart can draw.
func drawable(items []timeline.Item) []timeline.Item {
	var out []timeline.Item
	for _, it := range items {
		if it.Planned != nil && it.Planned.Start != "" && it.Planned.End != "" {
			out = append(out, it)
		}
	}
	return out
}

// tickWeeks is how many weeks apart a chart's ticks fall: one, or more on
// a chart spanning past twelve weeks, so it shows about a dozen.
func tickWeeks(releases []timeline.Release, sections []section) int {
	first, last := "", ""
	see := func(start, end string) {
		if first == "" || start < first {
			first = start
		}
		if end > last {
			last = end
		}
	}
	for _, r := range releases {
		if r.Sprint != nil && r.Sprint.End != "" {
			see(r.Sprint.End, r.Sprint.End)
		}
	}
	for _, s := range sections {
		for _, it := range s.items {
			see(it.Planned.Start, it.Planned.End)
		}
	}
	a, errA := time.Parse(time.DateOnly, first)
	b, errB := time.Parse(time.DateOnly, last)
	if errA != nil || errB != nil {
		return 1
	}
	weeks := int(b.Sub(a).Hours()/24/7) + 1
	return max(1, (weeks+11)/12)
}

// dayAfter is the day after d: a gantt task's end is exclusive, so a bar
// drawn through d's last day ends the next.
func dayAfter(d string) string {
	t, err := time.Parse(time.DateOnly, d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, 1).Format(time.DateOnly)
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_]`)

// mermaidID is an item's id as a gantt task id: letters, digits, and
// underscores.
func mermaidID(id string) string { return "t_" + unsafeID.ReplaceAllString(id, "_") }

// mermaidText is a title a gantt line can hold: the grammar ends a task's
// title at a colon and reads # and ; as comments and separators.
func mermaidText(s string) string {
	return strings.NewReplacer(":", " -", "#", " ", ";", ",", "\n", " ").Replace(s)
}

// gantt is one fenced mermaid gantt chart: calendar days, weekly ticks
// labeled by ISO week so they read as sprint boundaries, the releases as
// milestones at their sprints' ends, and each task's state.
func gantt(title string, releases []timeline.Release, sections []section) string {
	var b strings.Builder
	// The label column fits the longest section title, so a title never
	// runs into its bars (mermaid's default is 75 pixels).
	width := 0
	for _, s := range sections {
		width = max(width, len(mermaidText(s.title)))
	}
	b.WriteString("```mermaid\n")
	fmt.Fprintf(&b, "---\nconfig:\n  gantt:\n    leftPadding: %d\n---\n", min(max(75, 7*width+20), 320))
	b.WriteString("gantt\n")
	// Ticks start on Monday, as ISO weeks do: mermaid's default Sunday
	// would label each tick with the week before the one it opens. They
	// fall every week, or every few on a long chart, so about a dozen
	// labels never overlap.
	fmt.Fprintf(&b, "    title %s\n    dateFormat YYYY-MM-DD\n    axisFormat %%G-W%%V\n    tickInterval %dweek\n    weekday monday\n", mermaidText(title), tickWeeks(releases, sections))
	var milestones []timeline.Release
	for _, r := range releases {
		if r.Sprint != nil && r.Sprint.End != "" {
			milestones = append(milestones, r)
		}
	}
	if len(milestones) > 0 {
		b.WriteString("    section Releases\n")
		for _, r := range milestones {
			fmt.Fprintf(&b, "    %s :milestone, %s, %s, 0d\n", mermaidText("Release "+r.Version), mermaidID("release "+r.Version), r.Sprint.End)
		}
	}
	for _, s := range sections {
		fmt.Fprintf(&b, "    section %s\n", mermaidText(s.title))
		for _, it := range s.items {
			tags := ""
			switch it.Status {
			case timeline.Done:
				tags = "done, "
			case timeline.Active:
				tags = "active, "
			}
			fmt.Fprintf(&b, "    %s :%s%s, %s, %s\n", mermaidText(it.ID+" "+it.Title), tags, mermaidID(it.ID), it.Planned.Start, dayAfter(it.Planned.End))
		}
	}
	b.WriteString("```\n")
	return b.String()
}
