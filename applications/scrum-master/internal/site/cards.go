// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// level is how the site names one hierarchy level to a reader: its label,
// the letter the navigator shows, and what the level is, its timeframe, and
// its owner, as the planning hierarchy table init writes into the README
// says them.
type level struct{ Label, Letter, What, Timeframe, Owner string }

// levels are the hierarchy's levels in the order the tree nests them. The
// shell colours each kind as the interactive timeline does.
var levels = map[string]level{
	"theme":      {"Theme", "T", "A strategic area: the why", "Months to years", "Technical program manager"},
	"initiative": {"Initiative", "I", "An outcome several teams deliver: the program's Product Goal", "Several quarters", "Technical program manager"},
	"epic":       {"Epic", "E", "A large body of work under an initiative", "One to three months", "A Product Owner"},
	"feature":    {"Feature", "F", "A capability inside an epic", "Two to four sprints", "A Product Owner"},
	"story":      {"Story", "S", "The unit of value a team finishes in one sprint", "One sprint", "The team's Product Owner"},
}

// badge is a kind's labelled chip, as the plan, a page's facts, and its
// breadcrumb show it.
func badge(kind string) string {
	l, ok := levels[kind]
	if !ok {
		return html.EscapeString(kind)
	}
	return fmt.Sprintf(`<span class="kind kind-%s">%s</span>`, kind, l.Label)
}

// navBadge is a kind's one-letter chip in the navigator, named in full
// on hover.
func navBadge(kind string) string {
	l, ok := levels[kind]
	if !ok {
		return ""
	}
	return fmt.Sprintf(`<span class="kb kb-%s" title="%s">%s</span> `, kind, l.Label, l.Letter)
}

var stateClass = regexp.MustCompile(`[^a-z0-9]+`)

// state is the CSS class of an item's state.
func state(status string) string {
	return "state-" + strings.Trim(stateClass.ReplaceAllString(strings.ToLower(status), "-"), "-")
}

// shown is a state as a reader reads it: to_be_agreed is "to be agreed".
func shown(status string) string { return html.EscapeString(strings.ReplaceAll(status, "_", " ")) }

// bar is a points bar, empty without points.
func bar(n *Node) string {
	if n.Points == 0 {
		return ""
	}
	return fmt.Sprintf(`<span class="bar"><span style="width:%d%%"></span></span>`, n.PointsDone*100/n.Points)
}

// legend names each level with its chip: what it is and its timeframe.
func legend() string {
	var b strings.Builder
	b.WriteString(`<div class="legend">` + "\n")
	for _, k := range hierarchyKinds {
		l := levels[k]
		fmt.Fprintf(&b, `<span class="legend-item">%s <span class="legend-what">%s · %s</span></span>`+"\n", badge(k), html.EscapeString(l.What), strings.ToLower(l.Timeframe))
	}
	b.WriteString("</div>\n")
	return b.String()
}

// meta joins a node's facts with its points, dropping the empty ones.
func meta(facts ...string) string {
	var out []string
	for _, f := range facts {
		if f != "" {
			out = append(out, html.EscapeString(f))
		}
	}
	return strings.Join(out, " · ")
}

// below counts the documents under a node by kind, deepest last: "1
// feature · 3 stories".
func below(n *Node, nodes map[string]*Node) string {
	counts := map[string]int{}
	var walk func(id string)
	walk = func(id string) {
		for _, c := range nodes[id].Children {
			counts[nodes[c].Kind]++
			walk(c)
		}
	}
	walk(n.ID)
	var out []string
	for _, k := range hierarchyKinds {
		if c := counts[k]; c > 0 {
			name := strings.ToLower(levels[k].Label)
			if c > 1 {
				name = plural(name)
			}
			out = append(out, fmt.Sprintf("%d %s", c, name))
		}
	}
	return strings.Join(out, " · ")
}

func plural(s string) string {
	if strings.HasSuffix(s, "y") {
		return strings.TrimSuffix(s, "y") + "ies"
	}
	return s + "s"
}

// planHref is a node's link from the plan's index, beside the pages.
func planHref(n *Node) string {
	return html.EscapeString(strings.TrimPrefix(n.Page, HierarchyDir+"/"))
}

// subcard is a document on a board: its chip, title, state, horizon and
// owner, what lies under it, and its points; href links its page from the
// board's.
func subcard(n *Node, nodes map[string]*Node, href func(*Node) string) string {
	return fmt.Sprintf(`<a class="subcard subcard-%s %s" href="%s"><span class="card-head">%s<span class="card-state">%s</span></span><span class="subcard-title">%s</span><span class="card-meta">%s</span><span class="card-meta">%s</span>%s</a>`+"\n",
		n.Kind, state(n.Status), href(n), badge(n.Kind), shown(n.Status), html.EscapeString(n.Title),
		meta(n.Horizon, n.Owner), meta(below(n, nodes), points(n)), bar(n))
}

// columns are a board's columns, the states the timeline gives an item, in
// the order work moves through them.
var columns = []struct{ State, Label string }{
	{timeline.Future, "Future"}, {timeline.Active, "Active"}, {timeline.Done, "Done"},
}

// board lays documents out as a kanban board on the plan's index.
func board(ids []string, nodes map[string]*Node) string { return boardFrom(ids, nodes, planHref) }

// boardFrom lays documents out as a kanban board: a column per state, each
// document in its state's column in rank order; a state no column names
// gets a column of its own after them. href links each card's page from
// the page the board is on.
func boardFrom(ids []string, nodes map[string]*Node, href func(*Node) string) string {
	if len(ids) == 0 {
		return `<p class="board-empty">Nothing under it yet.</p>` + "\n"
	}
	cols := slices.Clone(columns)
	for _, id := range ids {
		st := nodes[id].Status
		if !slices.ContainsFunc(cols, func(c struct{ State, Label string }) bool { return c.State == st }) {
			cols = append(cols, struct{ State, Label string }{st, shown(st)})
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="board">` + "\n")
	for _, c := range cols {
		var in []string
		for _, id := range ids {
			if nodes[id].Status == c.State {
				in = append(in, id)
			}
		}
		fmt.Fprintf(&b, `<div class="column column-%s"><div class="column-head">%s <span class="column-count">%d</span></div>`+"\n", strings.TrimPrefix(state(c.State), "state-"), c.Label, len(in))
		for _, id := range in {
			b.WriteString(subcard(nodes[id], nodes, href))
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</div>\n")
	return b.String()
}

// panel is one tab of a theme: the document's summary, linking its page,
// over a board of the documents under it. The shell turns a theme's
// panels into tabs; the forge shows them one after another.
func panel(n *Node, nodes map[string]*Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section class="tab-panel %s" data-tab="%s" data-state="%s" data-points="%s">`+"\n",
		state(n.Status), html.EscapeString(n.Title), shown(n.Status), html.EscapeString(pointsShort(n)))
	fmt.Fprintf(&b, `<div class="panel-head">%s<a class="card-title" href="%s">%s</a><span class="card-state">%s</span><span class="band-meta">%s</span>%s</div>`+"\n",
		badge(n.Kind), planHref(n), html.EscapeString(n.Title), shown(n.Status), meta(n.Horizon, release(n.Item), n.Owner, below(n, nodes), points(n)), bar(n))
	b.WriteString(board(n.Children, nodes))
	b.WriteString("</section>\n")
	return b.String()
}

// pointsShort is a node's points as a tab shows them, done over total.
func pointsShort(n *Node) string {
	if n.Points == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", n.PointsDone, n.Points)
}

// band is a theme's bar across the page, a tab per document under it,
// each its own board; with no theme it gathers the documents no theme
// holds on one board.
func band(theme *Node, children []string, nodes map[string]*Node) string {
	var b strings.Builder
	if theme == nil {
		b.WriteString(`<section class="band band-none">` + "\n")
		b.WriteString(`<div class="band-head"><span class="band-title">Not under a theme</span></div>` + "\n")
		b.WriteString(board(children, nodes))
		b.WriteString("</section>\n")
		return b.String()
	}
	fmt.Fprintf(&b, `<section class="band band-theme %s">`+"\n", state(theme.Status))
	fmt.Fprintf(&b, `<div class="band-head">%s<a class="band-title" href="%s">%s</a><span class="card-state">%s</span><span class="band-meta">%s</span>%s</div>`+"\n",
		badge(theme.Kind), planHref(theme), html.EscapeString(theme.Title), shown(theme.Status),
		meta(theme.Horizon, below(theme, nodes), points(theme)), bar(theme))
	if len(children) == 0 {
		b.WriteString(`<p class="board-empty">Nothing under it yet.</p>` + "\n")
	} else {
		fmt.Fprintf(&b, `<div class="tabbed" data-tabs="%s">`+"\n", html.EscapeString(theme.ID))
		for _, c := range children {
			b.WriteString(panel(nodes[c], nodes))
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</section>\n")
	return b.String()
}

// servingBand gathers the documents that serve the parent program's plan,
// those whose parent is a document of the parent, on one board under a
// bar naming the parent and linking its site (GH-2740).
func servingBand(parent *timeline.ProgramRef, ids []string, nodes map[string]*Node) string {
	var b strings.Builder
	name := html.EscapeString(parent.Name)
	if parent.Site != "" {
		name = fmt.Sprintf(`<a class="band-title" href="%s">%s</a>`, html.EscapeString(parent.Site), name)
	}
	b.WriteString(`<section class="band band-none band-parent">` + "\n")
	fmt.Fprintf(&b, `<div class="band-head">%s<span class="band-title">Serves %s's plan</span><span class="band-meta">the program this one is part of</span></div>`+"\n", programBadge, name)
	b.WriteString(board(ids, nodes))
	b.WriteString("</section>\n")
	return b.String()
}

// crumbs is a node's place in the plan, from the plan's index down through
// each document above it, each with its chip. The links stay Markdown,
// which the shell routes and the forge follows; the blank lines inside the
// block let them parse.
func crumbs(n *Node, nodes map[string]*Node) string {
	var trail []*Node
	seen := map[string]bool{n.ID: true}
	for p, ok := nodes[n.Parent]; ok && !seen[p.ID]; p, ok = nodes[p.Parent] {
		seen[p.ID] = true
		trail = append([]*Node{p}, trail...)
	}
	parts := []string{fmt.Sprintf("[Plan](%s)", relLink(n.Page, HierarchyDir+"/index.md"))}
	for _, p := range trail {
		parts = append(parts, fmt.Sprintf("%s [%s](%s)", badge(p.Kind), p.Title, relLink(n.Page, p.Page)))
	}
	return `<div class="crumbs">` + "\n\n" + strings.Join(parts, " › ") + "\n\n</div>\n\n"
}
