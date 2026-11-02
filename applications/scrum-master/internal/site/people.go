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

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/people"
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
		out = append(out, timeline.Person{Username: p.Username, Scrums: resolved(p.Username, "scrum", p.Scrums, scrums), Epics: resolved(p.Username, "epic", p.Epics, epics)})
	}
	return out, notes
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
	pages := map[string][]byte{}
	var ix strings.Builder
	ix.WriteString(head + "# People\n\n")
	ix.WriteString("Who is on each scrum and epic, as control/people.md in the system repository names them. Being named here assigns no one an issue.\n\n")
	if len(f.People) == 0 {
		ix.WriteString("No one is recorded yet. `scrum-master people allocate` names a person on a scrum or an epic.\n")
	} else {
		ix.WriteString("| person | scrums | epics |\n| --- | --- | --- |\n")
	}
	for _, p := range f.People {
		page := personPage(p.Username)
		fmt.Fprintf(&ix, "| [%s](%s) | %s | %s |\n", p.Username, relLink(index, page), orNone(scrumLinks(index, p.Scrums)), orNone(epicLinks(index, p.Epics)))
		var b strings.Builder
		b.WriteString(head)
		b.WriteString(`<div class="crumbs">` + "\n\n[People](" + relLink(page, index) + ")\n\n</div>\n\n")
		fmt.Fprintf(&b, "# %s\n\n", p.Username)
		fmt.Fprintf(&b, "**Scrums:** %s.\n\n**Epics:** %s.\n", orNone(scrumLinks(page, p.Scrums)), orNone(epicLinks(page, p.Epics)))
		pages[Dir+"/"+page] = []byte(b.String())
	}
	pages[Dir+"/"+index] = []byte(ix.String())
	return pages, nil
}
