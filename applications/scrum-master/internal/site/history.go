// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
)

// The ways a requirement or criterion changes between two corpus tags.
const (
	Added    = "added"
	Reworded = "reworded"
	Retired  = "retired"
	Removed  = "removed"
	Moved    = "moved"
	// Placed and Unplaced are an initiative item the road map comes to
	// deliver in a release, or stops delivering in any (GH-141).
	Placed   = "placed"
	Unplaced = "unplaced"
)

// entry is one requirement item or acceptance criterion as a tag holds it.
type entry struct {
	text    string
	retired bool
}

// docSnapshot is one corpus document's items at a tag.
type docSnapshot struct {
	path  string
	items map[string]entry
}

// snapshot is the corpus at one tag: each document's items, and the
// release each initiative item is delivered in, by document#item.
type snapshot struct {
	docs     map[string]docSnapshot
	releases map[string]string
}

// Merge is a merge commit that changed a document between two tags.
type Merge struct {
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
	By      string `json:"by"`
}

// Change is one requirement item or acceptance criterion that changed
// between two corpus tags, with the merges that changed its document.
type Change struct {
	Document string `json:"document"`
	Path     string `json:"path"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	// Via is the file the change was made in when it is not the
	// document's own: a move between releases is made in the road map,
	// but it is the document's history (GH-116).
	Via    string  `json:"via,omitempty"`
	Merges []Merge `json:"merges"`
}

// Step is the changes from one corpus tag to the next.
type Step struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Changes []Change `json:"changes"`
}

var spaces = regexp.MustCompile(`\s+`)

// normal is an item's text with its whitespace collapsed, so rewrapping a
// paragraph is no rewording.
func normal(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }

// snapshotOf reads the corpus as tag holds it in the system checkout: every
// requirement item (a req record whose id has a dot) and acceptance
// criterion of every document under docs/, and the road map's deliverables.
func snapshotOf(system, tag string) (snapshot, error) {
	s := snapshot{docs: map[string]docSnapshot{}, releases: map[string]string{}}
	dir, cleanup, err := AtTag(system, tag)
	if err != nil {
		return s, err
	}
	defer cleanup()
	err = filepath.WalkDir(filepath.Join(dir, "docs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		md, err := corpus.ParseMarkdown(b)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "docs/road-map.md" {
			for _, r := range goals.ReleasesOf(md) {
				for _, item := range r.Objectives {
					s.releases[strings.TrimSpace(item)] = r.Version
				}
			}
		}
		doc := docSnapshot{path: rel, items: map[string]entry{}}
		for _, r := range md.Records("req") {
			if strings.Contains(r.Key, ".") {
				doc.items[r.Key] = entry{normal(r.Text()), r.Attrs["status"] == "retired" || r.Attrs["retired_in"] != ""}
			}
		}
		if ac := md.Field("acceptance_criteria"); ac != nil {
			for _, c := range ac.Children {
				if c.Kind == "ac" {
					doc.items[c.Key] = entry{normal(c.Text()), c.Attrs["status"] == "retired" || c.Attrs["retired_in"] != ""}
				}
			}
		}
		if len(doc.items) == 0 {
			return nil
		}
		id := md.ID()
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(rel), ".md")
		}
		s.docs[id] = doc
		return nil
	})
	return s, err
}

// diff is what changed from a to b: an item b holds and a does not is
// added; one a holds and b does not is removed; one b marks retired that a
// did not is retired, not removed and added; one whose text differs is
// reworded; and an initiative item the road map delivers in another
// release is moved, one it comes to deliver in a release placed, and one
// it stops delivering unplaced, beside any rewording (GH-141).
func diff(a, b snapshot) []Change {
	var out []Change
	ids := map[string]bool{}
	for id := range a.docs {
		ids[id] = true
	}
	for id := range b.docs {
		ids[id] = true
	}
	docIDs := mapKeys(ids)
	slices.Sort(docIDs)
	for _, doc := range docIDs {
		before, after := a.docs[doc], b.docs[doc]
		path := after.path
		if path == "" {
			path = before.path
		}
		items := map[string]bool{}
		for id := range before.items {
			items[id] = true
		}
		for id := range after.items {
			items[id] = true
		}
		for _, id := range sortItems(mapKeys(items)) {
			was, had := before.items[id]
			now, has := after.items[id]
			change := func(kind, from, to string) {
				out = append(out, Change{Document: doc, Path: path, ID: id, Kind: kind, From: from, To: to, Merges: []Merge{}})
			}
			switch {
			case has && !had:
				change(Added, "", "")
			case had && !has:
				change(Removed, "", "")
			case now.retired && !was.retired:
				change(Retired, "", "")
			case now.text != was.text:
				change(Reworded, "", "")
			}
			key := doc + "#" + id
			from, to := a.releases[key], b.releases[key]
			switch {
			case from == to || !has:
				continue
			case had && from != "" && to != "":
				change(Moved, from, to)
			case to != "":
				change(Placed, "", to)
			case had:
				change(Unplaced, from, "")
			default:
				continue
			}
			out[len(out)-1].Via = "docs/road-map.md"
		}
	}
	return out
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// sortItems orders ids as a document does: R2.10 after R2.9, criteria
// after requirements.
func sortItems(ids []string) []string {
	key := func(id string) []int {
		var out []int
		for _, part := range strings.FieldsFunc(id, func(r rune) bool { return r < '0' || r > '9' }) {
			var n int
			fmt.Sscan(part, &n)
			out = append(out, n)
		}
		return out
	}
	sort.SliceStable(ids, func(i, j int) bool {
		pi, pj := strings.HasPrefix(ids[i], "AC"), strings.HasPrefix(ids[j], "AC")
		if pi != pj {
			return pj
		}
		return slices.Compare(key(ids[i]), key(ids[j])) < 0
	})
	return ids
}

// mergedBy is the trailer scrum-master's merge writes, naming the tracker
// user who merged (srd013).
var mergedBy = regexp.MustCompile(`(?m)^Merged-by: (\S+)$`)

// mergesTouching lists, from local git alone, the merges on the first-parent
// history between two tags that changed path: each one's commit, subject,
// and who merged it, the Merged-by trailer's user, else the git author.
func mergesTouching(system, from, to, path string) ([]Merge, error) {
	out, err := exec.Command("git", "-C", system, "log", "--first-parent", "--merges", "--format=%H%x1f%an%x1f%s%x1f%b%x1e", from+".."+to, "--", path).Output()
	if err != nil {
		return nil, fmt.Errorf("read the merges of %s from %s to %s: %w", path, from, to, err)
	}
	merges := []Merge{}
	for _, rec := range strings.Split(string(out), "\x1e") {
		f := strings.Split(strings.TrimSpace(rec), "\x1f")
		if len(f) < 4 {
			continue
		}
		m := Merge{Commit: f[0][:min(12, len(f[0]))], Subject: f[2], By: f[1]}
		if t := mergedBy.FindStringSubmatch(f[3]); t != nil {
			m.By = t[1]
		}
		merges = append(merges, m)
	}
	return merges, nil
}

// History walks each pair of consecutive corpus tags of the system checkout
// and lists what changed between them, each change with the merges that
// changed its document (srd048 R6.1). Everything is read from local git.
func History(system string) ([]Step, error) {
	tags, err := ProgramTags(system)
	if err != nil {
		return nil, err
	}
	steps := []Step{}
	var prev snapshot
	for i, tag := range tags {
		cur, err := snapshotOf(system, tag)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			st := Step{From: tags[i-1], To: tag, Changes: diff(prev, cur)}
			merges := map[string][]Merge{}
			for j, c := range st.Changes {
				made := c.Path
				if c.Via != "" {
					made = c.Via
				}
				if _, ok := merges[made]; !ok {
					if merges[made], err = mergesTouching(system, st.From, st.To, made); err != nil {
						return nil, err
					}
				}
				st.Changes[j].Merges = merges[made]
			}
			steps = append(steps, st)
		}
		prev = cur
	}
	return steps, nil
}

// stepName is a step's page name: its tags with their slashes as dashes.
func stepName(s Step) string {
	return strings.ReplaceAll(s.From, "/", "-") + ".." + strings.ReplaceAll(s.To, "/", "-")
}

func changeLine(c Change) string {
	what := c.Kind
	switch c.Kind {
	case Moved:
		what = fmt.Sprintf("moved from release %s to %s", c.From, c.To)
	case Placed:
		what = "placed on release " + c.To
	case Unplaced:
		what = "taken off release " + c.From
	}
	var by []string
	for _, m := range c.Merges {
		by = append(by, fmt.Sprintf("merge %s (%s) by %s", m.Commit, m.Subject, m.By))
	}
	if len(by) == 0 {
		by = []string{"no merge changed it on the main line"}
	}
	return fmt.Sprintf("- %s %s: %s", c.ID, what, strings.Join(by, "; "))
}

// HistoryPages are the history word's pages: an index of the steps, a page
// per step listing its changes by document, and a page per document
// accumulating its changes across the steps, beside a JSON copy.
func HistoryPages(steps []Step) (pages, docs, data map[string][]byte) {
	pages, docs, data = map[string][]byte{}, map[string][]byte{}, map[string][]byte{}
	head := generated("history", "the corpus at each corpus tag and the merges between them")
	var index strings.Builder
	index.WriteString(head + "# Requirement history\n\nWhat changed in the requirements and acceptance criteria between consecutive corpus tags, each change with the merge that made it. Each document's own history is [listed by document](docs/index.md).\n\n")
	if len(steps) == 0 {
		index.WriteString("No two corpus tags to compare yet.\n")
	}
	perDoc := map[string][]string{}
	paths := map[string]string{}
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		fmt.Fprintf(&index, "- [%s to %s](%s.md): %d changes\n", s.From, s.To, stepName(s), len(s.Changes))
		var page strings.Builder
		fmt.Fprintf(&page, "%s# %s to %s\n\nThe requirements and acceptance criteria that changed between the two corpus tags, by document.\n", head, s.From, s.To)
		if len(s.Changes) == 0 {
			page.WriteString("\nNothing changed.\n")
		}
		doc := ""
		for _, c := range s.Changes {
			if c.Document != doc {
				doc = c.Document
				fmt.Fprintf(&page, "\n## %s\n\n", doc)
			}
			page.WriteString(changeLine(c) + "\n")
			perDoc[c.Document] = append(perDoc[c.Document], fmt.Sprintf("%s to %s: %s", s.From, s.To, strings.TrimPrefix(changeLine(c), "- ")))
			paths[c.Document] = c.Path
		}
		pages[Dir+"/history/"+stepName(s)+".md"] = []byte(page.String())
	}
	pages[Dir+"/history/index.md"] = []byte(index.String())
	for doc, rows := range perDoc {
		var page strings.Builder
		fmt.Fprintf(&page, "%s# %s\n\nEach change to %s's requirements and acceptance criteria, newest first", head, doc, doc)
		if p := paths[doc]; p != "" {
			fmt.Fprintf(&page, " (%s)", p)
		}
		page.WriteString(".\n\n")
		for _, r := range rows {
			page.WriteString("- " + r + "\n")
		}
		docs[Dir+"/history/docs/"+doc+".md"] = []byte(page.String())
	}
	// The documents' folder has its own index, so a link to the list
	// lands on a page (GH-139).
	var list strings.Builder
	list.WriteString(head + "# History by document\n\nEach document whose requirements or acceptance criteria changed between corpus tags, with its changes.\n\n")
	if len(perDoc) == 0 {
		list.WriteString("No document has changed between corpus tags yet.\n")
	}
	for _, doc := range slices.Sorted(maps.Keys(perDoc)) {
		fmt.Fprintf(&list, "- [%s](%s.md): %d changes\n", doc, doc, len(perDoc[doc]))
	}
	docs[Dir+"/history/docs/index.md"] = []byte(list.String())
	b, _ := json.MarshalIndent(steps, "", "  ")
	data[Dir+"/data/history/history.json"] = append(b, '\n')
	return pages, docs, data
}
