// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// Overview is the program's home on GitLab Pages: the interactive timeline,
// a tile per initiative, and the releases, generated each run (GH-128).
const Overview = Dir + "/overview.md"

// overviewPlaceholder is the overview before its word has run, so the
// shell's home opens on a page that says what fills it.
const overviewPlaceholder = "# Overview\n\nNo overview yet: the site's overview word writes it, with the program's timeline and a tile per initiative.\n"

// OverviewPage is the overview word's page, drawn from the timeline facts
// the timeline word wrote into the reports checkout dir: the program's
// name, the interactive timeline embedded, a tile per initiative with its
// state, points, horizon, target release, and epics, linking its page, and
// the releases. The tiles are HTML in the Markdown: the shell styles them,
// and the forge, which strips the styles, shows them as plain blocks.
func OverviewPage(dir, name string) ([]byte, error) {
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
	nodes, _ := hierarchyNodes(f)
	var page strings.Builder
	page.WriteString(generated("overview", "the timeline facts"))
	fmt.Fprintf(&page, "# %s\n\n", name)
	fmt.Fprintf(&page, "As of %s. [Status](status/index.md) · [Road map](status/road-map.md) · [Plan](%s/index.md) · [Timeline](timeline/index.md) · [Requirement history](history/index.md)\n\n", f.Today, HierarchyDir)
	page.WriteString("## Timeline\n\n")
	page.WriteString(embed("timeline", "The program's interactive timeline") + "\n")
	page.WriteString("[Open the interactive timeline](timeline/gantt/program.html) on its own page, with the plan as it stood at each corpus tag to compare.\n\n")
	page.WriteString("## Initiatives\n\n")
	var initiatives []*Node
	for _, id := range sortedNodes(nodes) {
		if nodes[id].Kind == "initiative" {
			initiatives = append(initiatives, nodes[id])
		}
	}
	if len(initiatives) == 0 {
		page.WriteString("No initiative yet.\n\n")
	} else {
		page.WriteString(`<div class="tiles">` + "\n")
		for _, n := range initiatives {
			page.WriteString(tile(n, nodes))
		}
		page.WriteString("</div>\n\n")
	}
	page.WriteString("## Releases\n\n")
	if len(f.Releases) == 0 {
		page.WriteString("No release on the road map yet.\n")
	} else {
		page.WriteString("| release | status | sprint | tag |\n| --- | --- | --- | --- |\n")
		for _, r := range f.Releases {
			sprint := ""
			if r.Sprint != nil {
				sprint = r.Sprint.Start + " to " + r.Sprint.End
			}
			fmt.Fprintf(&page, "| %s | %s | %s | %s |\n", r.Version, r.Status, sprint, r.Tag)
		}
	}
	return []byte(page.String()), nil
}

// tile is one initiative's card: a link to its page around its title, its
// state, a bar of points done over total, horizon, target release, and the
// number of epics under it.
func tile(n *Node, nodes map[string]*Node) string {
	epics := 0
	for _, c := range n.Children {
		if nodes[c].Kind == "epic" {
			epics++
		}
	}
	pct := 0
	if n.Points > 0 {
		pct = n.PointsDone * 100 / n.Points
	}
	var facts []string
	for _, f := range []string{n.Horizon, release(n.Item), fmt.Sprintf("%d epics", epics)} {
		if f != "" {
			facts = append(facts, html.EscapeString(f))
		}
	}
	return fmt.Sprintf(`<a class="tile state-%s" href="%s">
<span class="tile-title">%s</span>
<span class="tile-state">%s</span>
<span class="tile-bar"><span style="width:%d%%"></span></span>
<span class="tile-points">%d of %d points</span>
<span class="tile-facts">%s</span>
</a>
`, html.EscapeString(n.Status), n.Page, html.EscapeString(n.Title), html.EscapeString(n.Status), pct, n.PointsDone, n.Points, strings.Join(facts, " · "))
}
