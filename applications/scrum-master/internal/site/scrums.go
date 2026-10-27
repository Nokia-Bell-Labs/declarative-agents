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
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// ScrumsDir is the folder of the scrum pages, one per team the subsystem
// SRDs declare.
const ScrumsDir = "scrums"

// scrum is one team with what it works on: its own documents, the Sprint
// Goals filed in its repositories, and the epics its documents serve.
type scrum struct {
	timeline.Team
	Page    string
	Docs    []string // its features and stories, in kind and rank order
	Serves  []string // a sub-program's documents this plan shows, its interior apart
	Goals   []timeline.Item
	Epics   []string
	Done    int // its story points done
	Points  int // its story points
	Stories int
}

// owns reports whether a document is one of the team's: the facts name a
// team's documents <repository name>/<id>.
func (s scrum) owns(id string) bool {
	for _, r := range s.Repositories {
		if strings.HasPrefix(id, path.Base(r)+"/") {
			return true
		}
	}
	return false
}

// files reports whether a Sprint Goal was filed in one of the team's
// repositories: its id is project#iid.
func (s scrum) files(ref string) bool {
	project, _, _ := strings.Cut(ref, "#")
	return slices.Contains(s.Repositories, project)
}

// scrumsIn gathers each team's work from the facts.
func scrumsIn(f timeline.Facts, nodes map[string]*Node) []scrum {
	var out []scrum
	for _, t := range f.Teams {
		s := scrum{Team: t, Page: ScrumsDir + "/" + t.Name + ".md"}
		for _, id := range sortedNodes(nodes) {
			n := nodes[id]
			if !s.owns(id) {
				continue
			}
			if n.Program != "" {
				s.Serves = append(s.Serves, id)
				continue
			}
			if n.Kind == "story" || n.Kind == "feature" {
				s.Docs = append(s.Docs, id)
			}
			if n.Kind == "story" {
				s.Stories++
				s.Points += n.Points
				if n.Status == timeline.Done {
					s.Done += n.Points
				}
			}
			seen := map[string]bool{}
			for e := n; e != nil && !seen[e.ID]; e = nodes[e.Parent] {
				seen[e.ID] = true
				if e.Kind == "epic" {
					if !slices.Contains(s.Epics, e.ID) {
						s.Epics = append(s.Epics, e.ID)
					}
					break
				}
			}
		}
		slices.SortStableFunc(s.Docs, func(a, b string) int { return byRank(nodes[a], nodes[b]) })
		for _, id := range s.Serves {
			s.Points += nodes[id].Points
			s.Done += nodes[id].PointsDone
		}
		slices.SortStableFunc(s.Epics, func(a, b string) int { return byRank(nodes[a], nodes[b]) })
		for _, it := range f.Items {
			if it.Kind == timeline.KindSprintGoal && s.files(it.ID) {
				s.Goals = append(s.Goals, it)
			}
		}
		// Open Sprint Goals first, the latest release first within each.
		slices.SortStableFunc(s.Goals, func(a, b timeline.Item) int {
			if (a.Status == timeline.Done) != (b.Status == timeline.Done) {
				if a.Status == timeline.Done {
					return 1
				}
				return -1
			}
			return strings.Compare(b.Release, a.Release)
		})
		out = append(out, s)
	}
	return out
}

// ScrumPages are a page per scrum and the folder's index, drawn from the
// timeline facts the timeline word wrote into the reports checkout dir:
// each team's Product Owner, its area, its open Sprint Goals with the
// stories they selected, the epics its documents serve, cross-team ones
// included, and its own features and stories, each a board by state.
func ScrumPages(dir string) (map[string][]byte, error) {
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
	head := generated("hierarchy", "the timeline facts and the subsystem SRDs")
	scrums := scrumsIn(f, nodes)
	pages := map[string][]byte{}
	var index strings.Builder
	index.WriteString(head + "# Scrums\n\n")
	index.WriteString("Each team owns an area of the system, a subsystem and the components its SRD lists, and works the stories in its own repository. Planned work is a separate tree: an epic may span several scrums, each writing the stories for its part. Each scrum's page shows what it works on now.\n\n")
	if len(scrums) == 0 {
		index.WriteString("No subsystem SRD declares a team yet.\n")
	} else {
		index.WriteString(`<div class="tiles">` + "\n")
		for _, s := range scrums {
			index.WriteString(scrumTile(s))
		}
		index.WriteString("</div>\n")
	}
	pages[Dir+"/"+ScrumsDir+"/index.md"] = []byte(index.String())
	for _, s := range scrums {
		pages[Dir+"/"+s.Page] = []byte(scrumPage(s, nodes, head))
	}
	return pages, nil
}

// scrumBadge is the scrum's chip, as its page, its tile, and the
// navigator show it; programBadge a sub-program's, a team that is a
// program of its own (GH-2740).
const (
	scrumBadge   = `<span class="kind kind-scrum">Scrum</span>`
	programBadge = `<span class="kind kind-program">Program</span>`
)

// openGoals counts the Sprint Goals not yet closed.
func openGoals(goals []timeline.Item) int {
	n := 0
	for _, g := range goals {
		if g.Status != timeline.Done {
			n++
		}
	}
	return n
}

func plurals(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, plural(one))
}

// scrumTile is a scrum's card on the index: its name, Product Owner, area,
// and how much it has in hand.
func scrumTile(s scrum) string {
	bar := ""
	if s.Points > 0 {
		bar = fmt.Sprintf(`<span class="tile-bar"><span style="width:%d%%"></span></span>`, s.Done*100/s.Points)
	}
	work := []string{plurals(openGoals(s.Goals), "open Sprint Goal"), plurals(len(s.Epics), "epic"), plurals(s.Stories, "story")}
	class, chip := "tile-scrum", scrumBadge
	if s.Program {
		work = []string{plurals(openGoals(s.Goals), "open Sprint Goal"), plurals(len(s.Serves), "document") + " serving this plan", "a program with its own site"}
		class, chip = "tile-program", programBadge
	}
	pts := ""
	if s.Points > 0 {
		pts = fmt.Sprintf("%d of %d points done", s.Done, s.Points)
	}
	return fmt.Sprintf(`<a class="tile %s" href="%s.md">
<span class="tile-kind">%s</span>
<span class="tile-title">%s</span>
<span class="tile-facts">%s</span>
<span class="tile-facts">%s</span>
<span class="tile-facts">%s</span>
<span class="tile-points">%s</span>%s
</a>
`, class, html.EscapeString(s.Name), chip, html.EscapeString(s.Name), meta("Product Owner: "+s.Owner),
		meta("Area: "+strings.Join(s.Components, ", ")), meta(strings.Join(work, " · ")), html.EscapeString(pts), bar)
}

func scrumPage(s scrum, nodes map[string]*Node, head string) string {
	if s.Program {
		return programPage(s, nodes, head)
	}
	return teamPage(s, nodes, head)
}

// programPage is a sub-program's page: what this plan asked of it and what
// it serves the plan with, and a link to its own site for its teams,
// epics, and stories, which this site does not show (GH-2740).
func programPage(s scrum, nodes map[string]*Node, head string) string {
	href := func(n *Node) string { return html.EscapeString("../" + n.Page) }
	var b strings.Builder
	b.WriteString(head)
	b.WriteString(`<div class="crumbs">` + "\n\n[Scrums](index.md)\n\n</div>\n\n")
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	b.WriteString(strings.Join([]string{programBadge, "Owner: " + s.Owner, "subsystem `" + s.Subsystem + "`", "system repository " + s.Scrum}, " · ") + "\n\n")
	if s.Site != "" {
		fmt.Fprintf(&b, "**A program of its own.** Its teams, epics, and stories are on [%s's site](%s); this page shows what this plan asked of it and what it serves the plan with.\n\n", s.Name, s.Site)
	} else {
		b.WriteString("**A program of its own.** Its teams, epics, and stories are on its own site, which its control/program.md does not name yet; this page shows what this plan asked of it and what it serves the plan with.\n\n")
	}
	fmt.Fprintf(&b, "**Area:** %s.\n\n", strings.Join(s.Components, ", "))
	if s.Points > 0 {
		fmt.Fprintf(&b, "%d of %d story points done across what it serves this plan with.\n\n", s.Done, s.Points)
	}
	b.WriteString(goalsSection(s, nodes, href))
	b.WriteString("## What it serves\n\n")
	b.WriteString("Its documents whose parent is a document of this plan, with their points as its own site counts them.\n\n")
	b.WriteString(scrumSection(boardFrom(s.Serves, nodes, href), "Nothing of its plan serves this one yet, or its site's plan is not read."))
	return b.String()
}

func teamPage(s scrum, nodes map[string]*Node, head string) string {
	href := func(n *Node) string { return html.EscapeString("../" + n.Page) }
	var b strings.Builder
	b.WriteString(head)
	b.WriteString(`<div class="crumbs">` + "\n\n[Scrums](index.md)\n\n</div>\n\n")
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	facts := []string{scrumBadge, "Product Owner: " + s.Owner, "subsystem `" + s.Subsystem + "`"}
	scrumRepo := s.Scrum
	if s.URL != "" {
		scrumRepo = fmt.Sprintf("[%s](%s)", s.Scrum, s.URL)
	}
	b.WriteString(strings.Join(facts, " · ") + "\n\n")
	var code []string
	for _, r := range s.Repositories {
		if r != s.Scrum || !s.Apart {
			code = append(code, r)
		}
	}
	if s.Apart {
		fmt.Fprintf(&b, "**Plans in** %s: its stories, Sprint Goals, sprint-end summaries, status reports, and team rollup. **Code in** %s.\n\n", scrumRepo, strings.Join(code, ", "))
	} else {
		fmt.Fprintf(&b, "**Plans and codes in** %s", scrumRepo)
		if len(code) > 1 {
			fmt.Fprintf(&b, ", with code also in %s", strings.Join(code[1:], ", "))
		}
		b.WriteString(".\n\n")
	}
	fmt.Fprintf(&b, "**Area:** %s. A story of this scrum names only these components in its architecture; backlog check warns when one strays outside them.\n\n", strings.Join(s.Components, ", "))
	if s.Points > 0 {
		fmt.Fprintf(&b, "%d of %d story points done across %s.\n\n", s.Done, s.Points, plurals(s.Stories, "story"))
	}

	b.WriteString(goalsSection(s, nodes, href))

	b.WriteString("## Epics\n\n")
	b.WriteString("The epics this scrum's features and stories serve, its own and the cross-team epics in the system repository.\n\n")
	b.WriteString(scrumSection(boardFrom(s.Epics, nodes, href), "No feature or story of this scrum serves an epic yet."))
	if len(s.Epics) > 0 {
		b.WriteString("| epic | owner | its stories | its points done | the epic's points done |\n| --- | --- | --- | --- | --- |\n")
		for _, id := range s.Epics {
			e := nodes[id]
			stories, done, total := 0, 0, 0
			for _, d := range s.Docs {
				n := nodes[d]
				if n.Kind != "story" || !under(n, id, nodes) {
					continue
				}
				stories++
				total += n.Points
				if n.Status == timeline.Done {
					done += n.Points
				}
			}
			fmt.Fprintf(&b, "| [%s](../%s) | %s | %d | %d of %d | %s |\n", e.Title, e.Page, e.Owner, stories, done, total, points(e))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Stories\n\n")
	b.WriteString("The scrum's own features and stories, in its repository, by state.\n\n")
	b.WriteString(scrumSection(boardFrom(s.Docs, nodes, href), "No feature or story in its repository yet."))
	return b.String()
}

// scrumSection holds a board on a scrum's page, or says why there is none.
func scrumSection(board, empty string) string {
	if strings.Contains(board, `class="board-empty"`) {
		return empty + "\n\n"
	}
	return `<section class="scrum-section">` + "\n" + board + "</section>\n\n"
}

// under reports whether a node lies under the document id.
func under(n *Node, id string, nodes map[string]*Node) bool {
	seen := map[string]bool{}
	for p := nodes[n.Parent]; p != nil && !seen[p.ID]; p = nodes[p.Parent] {
		if p.ID == id {
			return true
		}
		seen[p.ID] = true
	}
	return false
}

// goalsSection is a scrum's Sprint Goals: each open one with a board of
// the stories its Sprint Backlog selects, then the closed ones listed.
func goalsSection(s scrum, nodes map[string]*Node, href func(*Node) string) string {
	var b strings.Builder
	b.WriteString("## Sprint Goals\n\n")
	open := 0
	for _, g := range s.Goals {
		if g.Status == timeline.Done {
			continue
		}
		open++
		fmt.Fprintf(&b, "### %s\n\n", itemLink(g))
		line := []string{shown(g.Status)}
		if g.Release != "" {
			line = append(line, "release "+g.Release)
		}
		b.WriteString(strings.Join(line, " · ") + "\n\n")
		var selected []string
		for _, id := range g.Selects {
			if nodes[id] != nil {
				selected = append(selected, id)
			}
		}
		b.WriteString(scrumSection(boardFrom(selected, nodes, href), "Its Sprint Backlog selects no story yet."))
	}
	if open == 0 {
		b.WriteString("No open Sprint Goal in its repositories.\n\n")
	}
	if closed := len(s.Goals) - open; closed > 0 {
		b.WriteString("Closed:\n\n")
		for _, g := range s.Goals[open:] {
			fmt.Fprintf(&b, "- %s, release %s\n", itemLink(g), g.Release)
		}
		b.WriteString("\n")
	}
	return b.String()
}
