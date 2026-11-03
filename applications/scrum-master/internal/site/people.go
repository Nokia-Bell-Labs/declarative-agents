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
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/people"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// PeopleDir is the folder of the people pages (srd048 R8.8).
const PeopleDir = "people"

// peopleFacts are who the system checkout's control/people.md names, each
// name resolved against the facts; what does not resolve, or a document that
// cannot be read, is a note, never silently dropped.
func peopleFacts(system string, f timeline.Facts) (out []timeline.Person, notes []string) {
	c, err := people.Load(system)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: not read, so no people are shown: %v", people.ControlFile, err)}
	}
	var scrums []string
	for _, t := range f.Teams {
		if t.Initiative == "" {
			scrums = append(scrums, t.Name)
		}
	}
	var epics []string
	for _, it := range f.Items {
		if it.Kind == "epic" {
			epics = append(epics, it.ID)
		}
	}
	resolved := func(p, what string, names, known []string) []string {
		var ok []string
		for _, n := range names {
			if slices.Contains(known, n) {
				ok = append(ok, n)
			} else {
				notes = append(notes, fmt.Sprintf("%s: %s is on %s %s, which the program does not hold", people.ControlFile, p, what, n))
			}
		}
		return ok
	}
	for _, p := range c.People {
		out = append(out, timeline.Person{Username: p.Username, Scrums: resolved(p.Username, "scrum", p.Scrums, scrums), Epics: resolved(p.Username, "epic", p.Epics, epics),
			Reviews: resolved(p.Username, "reviewing", p.Reviews, scrums)})
	}
	return out, notes
}

// reviewFacts sets who signs off on each document, by srd013 R11.1's order from
// its own name up: its own reviewer, its parents' up to its epic, its scrum's, the
// program's default. Where someone does, it reads whether they have, from the
// document's checkout. A document naming its reviewer, an SRD among them, is on
// that person's list, and everyone named gets a row, so a page links to them.
func reviewFacts(system string, docs []*hierarchy.Doc, f *timeline.Facts) (notes []string) {
	c, err := people.Load(system)
	if err != nil {
		return nil // peopleFacts has said so
	}
	def := ""
	if b, err := os.ReadFile(filepath.Join(system, filepath.FromSlash(program.PolicyFile))); err == nil {
		if def, err = program.DefaultReviewerIn(b); err != nil {
			notes = append(notes, fmt.Sprintf("%s: not read, so its default reviewer is not shown: %v", program.PolicyFile, err))
		}
	}
	own, paths := map[string]string{}, map[string]string{}
	for _, d := range docs {
		own[d.ID], paths[d.ID] = d.Reviewer, d.Path
	}
	items := map[string]*timeline.Item{}
	for i := range f.Items {
		items[f.Items[i].ID] = &f.Items[i]
	}
	var scrums []string
	for _, tm := range f.Teams {
		scrums = append(scrums, tm.Name)
	}
	reviewer := func(it *timeline.Item) string {
		id := it.ID
		for depth := 0; depth < 8; depth++ {
			if who := own[id]; who != "" {
				return who
			}
			at := items[id]
			if at == nil || (at.Kind != "story" && at.Kind != "feature") {
				break
			}
			id = at.Parent
		}
		if team, _, ok := cutLast(it.ID, "/"); ok && slices.Contains(scrums, team) {
			if who := c.ReviewerOf(team); who != "" {
				return who
			}
		}
		return def
	}
	named := map[string]bool{}
	if def != "" {
		named[def] = true
	}
	reviews := map[string][]string{}
	for i := range f.Items {
		it := &f.Items[i]
		if it.Reviewer = reviewer(it); it.Reviewer == "" {
			continue
		}
		named[it.Reviewer] = true
		if own[it.ID] != "" {
			reviews[own[it.ID]] = append(reviews[own[it.ID]], it.ID)
		}
		if p := paths[it.ID]; p != "" {
			it.ApprovedBy, it.ApprovedAt = signoffOf(p)
		}
		for _, who := range it.ApprovedBy {
			named[who] = true
		}
	}
	srds, _ := filepath.Glob(filepath.Join(system, filepath.FromSlash(program.SRDDir), "*.md"))
	for _, p := range srds {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		front, err := corpus.Front(b)
		if err != nil {
			continue
		}
		if who, _ := front[corpus.FrontReviewer].(string); who != "" {
			named[who] = true
			reviews[who] = append(reviews[who], strings.TrimSuffix(filepath.Base(p), ".md"))
		}
	}
	for i := range f.Items {
		for _, e := range f.Items[i].Ledger {
			if e.Reviewer != "" {
				named[e.Reviewer] = true
			}
		}
	}
	for who := range named {
		if !slices.ContainsFunc(f.People, func(p timeline.Person) bool { return p.Username == who }) {
			f.People = append(f.People, timeline.Person{Username: who})
		}
	}
	for i := range f.People {
		f.People[i].Reviews = append(f.People[i].Reviews, reviews[f.People[i].Username]...)
	}
	slices.SortFunc(f.People, func(a, b timeline.Person) int { return strings.Compare(a.Username, b.Username) })
	return notes
}

// cutLast cuts s around the last sep.
func cutLast(s, sep string) (string, string, bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// reviewerLine is who signs off on the document, and whether they have, on the
// page at from; nothing where no one does.
func reviewerLine(from string, it timeline.Item) string {
	if it.Reviewer == "" {
		return ""
	}
	line := "**Reviewer:** " + personLinks(from, []string{it.Reviewer}) + ". "
	if len(it.ApprovedBy) == 0 {
		return line + "Not signed off.\n\n"
	}
	return line + "Signed off by " + personLinks(from, it.ApprovedBy) + " at `" + it.ApprovedAt + "`.\n\n"
}

// scrumReviewerLine is who signs off on a scrum, on the page at from.
func scrumReviewerLine(from, reviewer string) string {
	if reviewer == "" {
		return ""
	}
	return "**Reviewer:** " + personLinks(from, []string{reviewer}) + ".\n\n"
}

func personPage(username string) string { return PeopleDir + "/" + username + ".md" }

// personLinks is usernames each linked to their page, from the page at from.
func personLinks(from string, usernames []string) string {
	var out []string
	for _, u := range usernames {
		out = append(out, fmt.Sprintf("[%s](%s)", u, relLink(from, personPage(u))))
	}
	return strings.Join(out, ", ")
}

// peopleLine is a scrum's or epic's people, on the page at from.
func peopleLine(from string, usernames []string) string {
	if len(usernames) == 0 {
		return "**People:** none recorded.\n\n"
	}
	return "**People:** " + personLinks(from, usernames) + ".\n\n"
}

// waitingOn is the open issues in the planning documents' issues records that
// username signs off on and did not write: each issue's own line, else its
// document's reviewer.
func waitingOn(username string, f timeline.Facts) []string {
	var out []string
	for _, it := range f.Items {
		for _, e := range it.Ledger {
			who := e.Reviewer
			if who == "" {
				who = it.Reviewer
			}
			if who != username || e.State == "closed" || slices.Contains(e.Who, username) {
				continue
			}
			ref := e.Ref
			if e.URL != "" {
				ref = fmt.Sprintf("[%s](%s)", e.Ref, e.URL)
			}
			out = append(out, ref+": "+e.Title)
		}
	}
	return out
}

// PeoplePages are site/people/'s index and a page per person, drawn from the
// timeline facts the timeline word wrote into the reports checkout dir.
func PeoplePages(dir string) (map[string][]byte, error) {
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
	head := generated("hierarchy", "the timeline facts and control/people.md")
	index := PeopleDir + "/index.md"
	scrumLinks := func(from string, names []string) string {
		var out []string
		for _, n := range names {
			out = append(out, fmt.Sprintf("[%s](%s)", n, relLink(from, ScrumsDir+"/"+n+".md")))
		}
		return strings.Join(out, ", ")
	}
	epicLinks := func(from string, ids []string) string {
		var out []string
		for _, id := range ids {
			out = append(out, fmt.Sprintf("[%s](%s)", nodes[id].Title, relLink(from, nodes[id].Page)))
		}
		return strings.Join(out, ", ")
	}
	orNone := func(s string) string {
		if s == "" {
			return "none"
		}
		return s
	}
	scrumNames := map[string]bool{}
	for _, tm := range f.Teams {
		scrumNames[tm.Name] = true
	}
	reviewLinks := func(from string, names []string) string {
		var out []string
		for _, n := range names {
			switch {
			case scrumNames[n]:
				out = append(out, fmt.Sprintf("[%s](%s)", n, relLink(from, ScrumsDir+"/"+n+".md")))
			case nodes[n] != nil:
				out = append(out, fmt.Sprintf("[%s](%s)", nodes[n].Title, relLink(from, nodes[n].Page)))
			default:
				out = append(out, "`"+n+"`")
			}
		}
		return strings.Join(out, ", ")
	}
	pages := map[string][]byte{}
	var ix strings.Builder
	ix.WriteString(head + "# People\n\n")
	ix.WriteString("Who is on each scrum and epic, as control/people.md in the system repository names them. Being named here assigns no one an issue.\n\n")
	if len(f.People) == 0 {
		ix.WriteString("No one is recorded yet. `scrum-master people allocate` names a person on a scrum or an epic.\n")
	} else {
		ix.WriteString("| person | scrums | epics | reviews |\n| --- | --- | --- | --- |\n")
	}
	for _, p := range f.People {
		page := personPage(p.Username)
		fmt.Fprintf(&ix, "| [%s](%s) | %s | %s | %s |\n", p.Username, relLink(index, page), orNone(scrumLinks(index, p.Scrums)), orNone(epicLinks(index, p.Epics)), orNone(reviewLinks(index, p.Reviews)))
		var b strings.Builder
		b.WriteString(head)
		b.WriteString(`<div class="crumbs">` + "\n\n[People](" + relLink(page, index) + ")\n\n</div>\n\n")
		fmt.Fprintf(&b, "# %s\n\n", p.Username)
		fmt.Fprintf(&b, "**Scrums:** %s.\n\n**Epics:** %s.\n\n**Reviews:** %s.\n", orNone(scrumLinks(page, p.Scrums)), orNone(epicLinks(page, p.Epics)), orNone(reviewLinks(page, p.Reviews)))
		if waiting := waitingOn(p.Username, f); len(waiting) > 0 {
			b.WriteString("\n## Waiting on your sign-off\n\nOpen issues you sign off on, as the issues records in the planning documents hold them.\n\n")
			for _, w := range waiting {
				b.WriteString("- " + w + "\n")
			}
		}
		pages[Dir+"/"+page] = []byte(b.String())
	}
	pages[Dir+"/"+index] = []byte(ix.String())
	return pages, nil
}
