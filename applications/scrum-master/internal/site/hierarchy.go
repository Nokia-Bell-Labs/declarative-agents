// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// HierarchyDir is the hierarchy word's folder: one page per theme,
// initiative, epic, feature, and story, a team's under its checkout's name
// (GH-126).
const HierarchyDir = "hierarchy"

// hierarchyKinds are the planning documents that get a page, in the order
// the tree nests them.
var hierarchyKinds = []string{"theme", "initiative", "epic", "feature", "story"}

// Node is one hierarchy document's page facts: the item itself, the pages
// it links up and down, and the Sprint Goals and issues that serve it.
type Node struct {
	timeline.Item
	Page     string   `json:"page"`
	Children []string `json:"children"`
	Goals    []string `json:"sprint_goals"`
	Issues   []string `json:"issues"`
}

// HierarchyPages are the hierarchy word's pages, drawn from the timeline
// facts the timeline word wrote into the reports checkout dir: a page per
// hierarchy document, linking its parent and children, the Sprint Goals
// that selected it or a story under it, and the issues that implement it,
// with its source on the forge and its corpus copy where the site holds
// one; the tree as the folder's index; the nodes as JSON (GH-126); a page
// per release sprint (GH-139); and the site's navigator over them and the
// program's repositories (GH-127).
func HierarchyPages(dir string, repos []Repo) (pages, data, sprints map[string][]byte, sidebar []byte, err error) {
	b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", "facts.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil, nil, fmt.Errorf("%s holds no timeline facts; run the site's timeline word first", dir)
	} else if err != nil {
		return nil, nil, nil, nil, err
	}
	var f timeline.Facts
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%s/timeline/facts.json: %w", Dir, err)
	}
	corpus := func(p string) bool {
		_, err := os.Stat(filepath.Join(dir, Dir, CorpusDir, filepath.FromSlash(p)))
		return err == nil
	}
	nodes, tracked := hierarchyNodes(f)
	pages = map[string][]byte{}
	for _, n := range nodes {
		pages[Dir+"/"+n.Page] = []byte(nodePage(n, nodes, tracked, corpus))
	}
	pages[Dir+"/"+HierarchyDir+"/index.md"] = []byte(treePage(nodes))
	out := make([]Node, 0, len(nodes))
	for _, id := range sortedNodes(nodes) {
		out = append(out, *nodes[id])
	}
	j, _ := json.MarshalIndent(out, "", "  ")
	return pages, map[string][]byte{Dir + "/data/hierarchy/hierarchy.json": append(j, '\n')}, SprintPages(f, nodes), []byte(sidebarPage(nodes, repos, sprintsOf(f))), nil
}

// hierarchyNodes gathers each hierarchy document with its children, the
// Sprint Goals working on it (selecting it or a story under it), and the
// issues implementing it; tracked is the Sprint Goals and issues by id.
func hierarchyNodes(f timeline.Facts) (nodes map[string]*Node, tracked map[string]timeline.Item) {
	nodes, tracked = map[string]*Node{}, map[string]timeline.Item{}
	var goals, issues []timeline.Item
	for _, it := range f.Items {
		switch {
		case slices.Contains(hierarchyKinds, it.Kind):
			nodes[it.ID] = &Node{Item: it, Page: HierarchyDir + "/" + it.ID + ".md", Children: []string{}, Goals: []string{}, Issues: []string{}}
		case it.Kind == timeline.KindSprintGoal:
			goals, tracked[it.ID] = append(goals, it), it
		case it.Kind == timeline.KindIssue:
			issues, tracked[it.ID] = append(issues, it), it
		}
	}
	for _, id := range sortedNodes(nodes) {
		if p, ok := nodes[nodes[id].Parent]; ok {
			p.Children = append(p.Children, id)
		}
	}
	for _, n := range nodes {
		slices.SortStableFunc(n.Children, func(a, b string) int { return byRank(nodes[a], nodes[b]) })
	}
	for _, g := range goals {
		for _, story := range g.Selects {
			seen := map[string]bool{}
			for id := story; id != "" && nodes[id] != nil && !seen[id]; id = nodes[id].Parent {
				seen[id] = true
				if !slices.Contains(nodes[id].Goals, g.ID) {
					nodes[id].Goals = append(nodes[id].Goals, g.ID)
				}
			}
		}
	}
	for _, i := range issues {
		if n, ok := nodes[i.Parent]; ok {
			n.Issues = append(n.Issues, i.ID)
		}
	}
	return nodes, tracked
}

// byRank orders siblings by rank where both have one, else by id.
func byRank(a, b *Node) int {
	ra, ea := strconv.Atoi(a.Rank)
	rb, eb := strconv.Atoi(b.Rank)
	if ea == nil && eb == nil && ra != rb {
		return ra - rb
	}
	return strings.Compare(a.ID, b.ID)
}

// sortedNodes is the node ids in kind order, then id.
func sortedNodes(nodes map[string]*Node) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		if c := slices.Index(hierarchyKinds, nodes[a].Kind) - slices.Index(hierarchyKinds, nodes[b].Kind); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	return ids
}

// relLink is the link from the page at from to the page at to, both under
// site/.
func relLink(from, to string) string {
	up := strings.Repeat("../", strings.Count(path.Dir(from), "/")+1)
	if path.Dir(from) == "." {
		up = ""
	}
	return up + to
}

// itemLink is a Sprint Goal's or an issue's line: its tracker page when it
// has one a browser opens, else its reference alone.
func itemLink(it timeline.Item) string {
	label := it.ID + " " + it.Title
	if strings.HasPrefix(it.URL, "https://") || strings.HasPrefix(it.URL, "http://") {
		return fmt.Sprintf("[%s](%s)", label, it.URL)
	}
	return label
}

// points is a node's points done over total, or nothing without points.
func points(n *Node) string {
	if n.Points == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d points", n.PointsDone, n.Points)
}

func nodePage(n *Node, nodes map[string]*Node, tracked map[string]timeline.Item, corpus func(string) bool) string {
	var b strings.Builder
	b.WriteString(generated("hierarchy", "the timeline facts"))
	fmt.Fprintf(&b, "# %s\n\n", n.Title)
	facts := []string{"**" + n.Kind + "** `" + n.ID + "`", n.Status}
	for _, f := range []string{n.Horizon, release(n.Item), points(n), n.Owner} {
		if f != "" {
			facts = append(facts, f)
		}
	}
	b.WriteString(strings.Join(facts, " · ") + "\n\n")
	if n.Statement != "" {
		fmt.Fprintf(&b, "> %s\n\n", n.Statement)
	}
	link := func(id string) string {
		c := nodes[id]
		line := fmt.Sprintf("[%s](%s): %s, %s", c.Title, relLink(n.Page, c.Page), c.Kind, c.Status)
		if p := points(c); p != "" {
			line += ", " + p
		}
		return line
	}
	b.WriteString("## Up\n\n")
	if _, ok := nodes[n.Parent]; ok {
		b.WriteString("- " + link(n.Parent) + "\n\n")
	} else {
		b.WriteString("Nothing above it in the plan.\n\n")
	}
	if n.Kind != "story" {
		b.WriteString("## Down\n\n")
		if len(n.Children) == 0 {
			b.WriteString("Nothing under it yet.\n\n")
		}
		for _, c := range n.Children {
			b.WriteString("- " + link(c) + "\n")
		}
		if len(n.Children) > 0 {
			b.WriteString("\n")
		}
	}
	list := func(title, none string, ids []string) {
		fmt.Fprintf(&b, "## %s\n\n", title)
		if len(ids) == 0 {
			b.WriteString(none + "\n\n")
			return
		}
		for _, id := range ids {
			it := tracked[id]
			fmt.Fprintf(&b, "- %s: %s\n", itemLink(it), it.Status)
		}
		b.WriteString("\n")
	}
	list("Sprint Goals", "No Sprint Goal has selected it or a story under it.", n.Goals)
	if n.Kind == "story" {
		list("Issues", "No issue implements it yet.", n.Issues)
	}
	b.WriteString("## Source\n\n")
	wrote := false
	if n.Source != "" {
		fmt.Fprintf(&b, "- [%s](%s) on the forge\n", sourceName(n), n.Source)
		wrote = true
	}
	if rel, ok := strings.CutPrefix(n.Path, "docs/"); ok && !strings.Contains(n.ID, "/") && corpus(rel) {
		fmt.Fprintf(&b, "- [The site's copy](%s)\n", relLink(n.Page, CorpusDir+"/"+rel))
		wrote = true
	}
	if !wrote {
		b.WriteString("No forge to link: the repository's origin is not one a browser opens.\n")
	}
	return b.String()
}

func sourceName(n *Node) string {
	if n.Path != "" {
		return n.Path
	}
	return n.ID
}

// release is a document's target release and tag, or nothing.
func release(it timeline.Item) string {
	switch {
	case it.Release != "" && it.Tag != "":
		return "release " + it.Release + " (" + it.Tag + ")"
	case it.Release != "":
		return "release " + it.Release
	}
	return ""
}

// treePage is the folder's index: every document under its parent, roots
// first, each with its kind, status, and points.
func treePage(nodes map[string]*Node) string {
	var b strings.Builder
	b.WriteString(generated("hierarchy", "the timeline facts"))
	b.WriteString("# Plan\n\nEvery theme, initiative, epic, feature, and story, each under the document it serves, with its state and points. A team's documents are named for its repository.\n\n")
	if len(nodes) == 0 {
		b.WriteString("No planning document yet.\n")
		return b.String()
	}
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		n := nodes[id]
		line := fmt.Sprintf("%s- [%s](%s) %s, %s", strings.Repeat("  ", depth), n.Title, strings.TrimPrefix(n.Page, HierarchyDir+"/"), n.Kind, n.Status)
		if p := points(n); p != "" {
			line += ", " + p
		}
		b.WriteString(line + "\n")
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, id := range sortedNodes(nodes) {
		if _, ok := nodes[nodes[id].Parent]; !ok {
			walk(id, 0)
		}
	}
	return b.String()
}
