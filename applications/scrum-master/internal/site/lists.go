// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// EpicsDir and StoriesDir are the hierarchy word's lists: every epic and
// every story of the plan on a page of its own, so a reader reaches them
// from the navigator without unfolding the plan's tree (srd048 R8.12).
const (
	EpicsDir   = "epics"
	StoriesDir = "stories"
)

// The list pages filter the same rows in the browser, as the backlog's board
// does, with no library and no request of their own.
//
//go:embed templates/lists.html.tmpl
var listsHTML string

var listsPage = template.Must(template.New("lists").Parse(listsHTML))

// listRow is one epic or story as its list draws it.
type listRow struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Repository string   `json:"repository"`
	State      string   `json:"state"`
	Release    string   `json:"release,omitempty"`
	Horizon    string   `json:"horizon,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Who        []string `json:"who,omitempty"`
	Points     int      `json:"points"`
	PointsDone int      `json:"points_done"`
	Below      string   `json:"below,omitempty"`
	Statement  string   `json:"statement,omitempty"`
	OpenIssues int      `json:"open_issues"`
	DoneIssues int      `json:"closed_issues"`
	Epic       string   `json:"epic,omitempty"`
	EpicTitle  string   `json:"epic_title,omitempty"`
}

// ListPages are the epics' and the stories' pages, drawn from the timeline
// facts the timeline word wrote into the reports checkout dir (srd048
// R8.12): each folder's index with the embedded board above an open and a
// closed table, the forge's view, and the embedded page that filters the
// same rows. The word asks the tracker nothing.
func ListPages(dir string) (epics, stories map[string][]byte, err error) {
	b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", "facts.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("%s holds no timeline facts; run the site's timeline word first", dir)
	} else if err != nil {
		return nil, nil, err
	}
	var f timeline.Facts
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil, fmt.Errorf("%s/timeline/facts.json: %w", Dir, err)
	}
	nodes, _ := hierarchyNodes(f)
	if epics, err = listPages(EpicsDir, "epic", "Epics", nodes, f); err != nil {
		return nil, nil, err
	}
	if stories, err = listPages(StoriesDir, "story", "Stories", nodes, f); err != nil {
		return nil, nil, err
	}
	return epics, stories, nil
}

func listPages(folder, kind, title string, nodes map[string]*Node, f timeline.Facts) (map[string][]byte, error) {
	rows := listRows(kind, nodes)
	data, err := json.Marshal(struct {
		Today string    `json:"today"`
		Kind  string    `json:"kind"`
		Rows  []listRow `json:"rows"`
	}{f.Today, kind, rows})
	if err != nil {
		return nil, err
	}
	var page bytes.Buffer
	if err := listsPage.Execute(&page, struct {
		Title, Noun, Data string
		Stories           bool
	}{title, strings.ToLower(title), string(data), kind == "story"}); err != nil {
		return nil, err
	}
	return map[string][]byte{
		Dir + "/" + folder + "/index.md":            []byte(listIndex(folder, kind, title, rows, f)),
		Dir + "/" + folder + "/" + folder + ".html": page.Bytes(),
	}, nil
}

// listRows are the documents of one level, ordered by repository, then rank
// where a repository ranks them, then id.
func listRows(kind string, nodes map[string]*Node) []listRow {
	var kept []*Node
	for _, n := range nodes {
		if n.Kind == kind {
			kept = append(kept, n)
		}
	}
	slices.SortFunc(kept, func(a, b *Node) int {
		if c := strings.Compare(docRepository(a), docRepository(b)); c != 0 {
			return c
		}
		return byRank(a, b)
	})
	rows := make([]listRow, 0, len(kept))
	for _, n := range kept {
		row := listRow{ID: n.ID, Title: n.Title, Repository: docRepository(n), State: n.Status, Release: n.Release,
			Horizon: n.Horizon, Owner: n.Owner, Points: n.Points, PointsDone: n.PointsDone,
			Below: below(n, nodes), Statement: n.Statement}
		who := slices.Clone(n.People)
		walkIssues(n, nodes, func(e timeline.IssueRecord) {
			if e.State == "closed" {
				row.DoneIssues++
			} else {
				row.OpenIssues++
			}
			who = append(who, e.Who...)
		})
		slices.Sort(who)
		row.Who = slices.Compact(who)
		if e := epicOf(n, nodes); e != nil && e != n {
			row.Epic, row.EpicTitle = e.ID, e.Title
		}
		rows = append(rows, row)
	}
	return rows
}

// docRepository is the repository a document lives in, as a reader names it:
// the facts name a team's documents <repository>/<id>, and a system document
// is named by its source on the forge, else simply the system.
func docRepository(n *Node) string {
	if d := path.Dir(n.ID); d != "." {
		return d
	}
	if ref, err := links.Parse(n.Source); err == nil && ref.Project != "" {
		return path.Base(ref.Project)
	}
	return "system"
}

// walkIssues visits the issues a document and everything under it record, so
// an epic counts the issues its stories hold.
func walkIssues(n *Node, nodes map[string]*Node, visit func(timeline.IssueRecord)) {
	for _, e := range n.Ledger {
		visit(e)
	}
	for _, c := range n.Children {
		walkIssues(nodes[c], nodes, visit)
	}
}

// epicOf is the epic a document serves, found up its parents, itself when it
// is one, and nil when none holds it.
func epicOf(n *Node, nodes map[string]*Node) *Node {
	seen := map[string]bool{}
	for cur := n; cur != nil && !seen[cur.ID]; cur = nodes[cur.Parent] {
		seen[cur.ID] = true
		if cur.Kind == "epic" {
			return cur
		}
	}
	return nil
}

// listIndex is a list's index: the embedded board above an open and a closed
// table, the forge's view of the same rows, each document linking its page.
func listIndex(folder, kind, title string, rows []listRow, f timeline.Facts) string {
	noun := strings.ToLower(levels[kind].Label)
	var b strings.Builder
	b.WriteString(generated("hierarchy", "the timeline facts") + "# " + title + "\n\n")
	fmt.Fprintf(&b, "As the planning documents held them on %s, when this page was generated.\n\n", f.Today)
	if len(f.Unread) > 0 {
		fmt.Fprintf(&b, "No checkout of %s here, so what they hold may be missing; scrum-master checkouts lays them down.\n\n", strings.Join(f.Unread, ", "))
	}
	if len(rows) == 0 {
		fmt.Fprintf(&b, "No %s yet. A %s appears here once its document is in a repository with a checkout beside the system's, and the timeline word has read it.\n\n", noun, noun)
	}
	fmt.Fprintf(&b, "Every %s of the plan, read from the checkouts, never the tracker. The board's columns are the plan's states: future, active, and done. Open is anything not done, closed is done, and it filters by repository, state, release, owner, and who; the tables are the forge's view of the same rows.\n\n", noun)
	fmt.Fprintf(&b, "<iframe class=\"timeline\" src=\"%s/%s.html?embed=1\" title=\"The %s, filterable\"></iframe>\n", folder, folder, strings.ToLower(title))
	var open, closed []listRow
	for _, r := range rows {
		if r.State == timeline.Done {
			closed = append(closed, r)
		} else {
			open = append(open, r)
		}
	}
	table := func(heading, none string, rows []listRow) {
		fmt.Fprintf(&b, "\n## %s\n\n", heading)
		if len(rows) == 0 {
			b.WriteString(none + "\n")
			return
		}
		b.WriteString("| " + noun + " | state | repository | release | owner | points | issues | ")
		if kind == "story" {
			b.WriteString("epic |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		} else {
			b.WriteString("under it |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		}
		from := folder + "/index.md"
		for _, r := range rows {
			last := r.Below
			if kind == "story" {
				last = ""
				if r.Epic != "" {
					last = fmt.Sprintf("[%s](%s)", r.EpicTitle, relLink(from, HierarchyDir+"/"+r.Epic+".md"))
				}
			}
			fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s | %s | %d of %d | %d open, %d closed | %s |\n",
				r.Title, relLink(from, HierarchyDir+"/"+r.ID+".md"), r.State, r.Repository, r.Release, r.Owner, r.PointsDone, r.Points, r.OpenIssues, r.DoneIssues, last)
		}
	}
	table("Open", "No "+noun+" is open.", open)
	table("Closed", "No "+noun+" is closed.", closed)
	return b.String()
}
