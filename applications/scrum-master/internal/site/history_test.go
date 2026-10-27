// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const historySRD = `---
id: srd010-scoring
title: Scoring
---

<!-- req: R1 -->
## R1 - Scoring

<!-- req: R1.1 -->
### R1.1

Scores a run.

<!-- req: R1.2 -->
### R1.2

Stores a score.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A run is scored.
`

func historyRoadMap(release string) string {
	return "---\nid: road-map-fixture\ntitle: Road map fixture\n---\n\n<!-- release: 01.0 | status: specified -->\n## 01.0 - First\n\n<!-- field: deliverables -->\n### Deliverables\n\n" +
		map[bool]string{true: "- in001-reliable-agents#R1.1\n", false: "None\n"}[release == "01.0"] +
		"\n<!-- release: 01.1 | status: specified -->\n## 01.1 - Second\n\n<!-- field: deliverables -->\n### Deliverables\n\n" +
		map[bool]string{true: "- in001-reliable-agents#R1.1\n", false: "None\n"}[release == "01.1"]
}

// historyRepo is a system checkout with three corpus tags. Between the
// first two, one merge adds R1.3, rewords R1.1, and retires R1.2 of
// srd010, and another moves in001's R1.1 from release 01.0 to 01.1; between
// the last two, a merge rewords srd010's AC1. Each merge carries the
// Merged-by trailer scrum-master's merge writes.
func historyRepo(t *testing.T) string {
	t.Helper()
	system := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", system, "-c", "user.name=git-author", "-c", "user.email=a@example.invalid"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(p, s string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(system, p)), 0o755)
		if err := os.WriteFile(filepath.Join(system, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	initiative, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", "docs", "specs", "initiatives", "in001-reliable-agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	merge := func(branch, message, by string, change func()) {
		t.Helper()
		git("checkout", "-q", "-b", branch)
		change()
		git("commit", "-q", "-am", "the change")
		git("checkout", "-q", "main")
		git("merge", "-q", "--no-ff", "-m", message+"\n\nMerged-by: "+by, branch)
	}
	git("init", "-q", "-b", "main")
	write("docs/specs/software-requirements/srd010-scoring.md", historySRD)
	write("docs/specs/initiatives/in001-reliable-agents.md", string(initiative))
	write("docs/road-map.md", historyRoadMap("01.0"))
	git("add", "-A")
	git("commit", "-q", "-m", "the plan")
	git("tag", "corpus/v0.1.0")
	merge("gh-5-scoring", "Merge GH-5: rework scoring (g/system#5)", "alice", func() {
		s := strings.Replace(historySRD, "Scores a run.", "Scores a run against its rubric.", 1)
		s = strings.Replace(s, "<!-- req: R1.2 -->", "<!-- req: R1.2 | status: retired -->", 1)
		s = strings.Replace(s, "<!-- field: acceptance_criteria -->", "<!-- req: R1.3 -->\n### R1.3\n\nPublishes a score.\n\n<!-- field: acceptance_criteria -->", 1)
		write("docs/specs/software-requirements/srd010-scoring.md", s)
	})
	merge("gh-6-replan", "Merge GH-6: move R1.1 to 01.1 (g/system#6)", "bob", func() { write("docs/road-map.md", historyRoadMap("01.1")) })
	git("tag", "corpus/v0.2.0")
	merge("gh-7-criteria", "Merge GH-7: sharpen AC1 (g/system#7)", "carol", func() {
		b, _ := os.ReadFile(filepath.Join(system, "docs/specs/software-requirements/srd010-scoring.md"))
		write("docs/specs/software-requirements/srd010-scoring.md", strings.Replace(string(b), "A run is scored.", "A run is scored within a minute.", 1))
	})
	git("tag", "corpus/v0.3.0")
	return system
}

// Between two tags the history lists exactly the item added, the one
// reworded, the one retired (not removed and added), and the one moved
// between releases, each with its merge and who merged it; a document's
// page gathers its rows across both steps; everything is read from local
// git, against golden files (GH-85).
func TestHistoryBetweenCorpusTags(t *testing.T) {
	t.Setenv(CacheEnv, t.TempDir())
	system := historyRepo(t)
	steps, err := History(system)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].From != "corpus/v0.1.0" || steps[1].To != "corpus/v0.3.0" {
		t.Fatalf("steps %+v", steps)
	}
	var got []string
	for _, c := range steps[0].Changes {
		by := ""
		if len(c.Merges) == 1 {
			by = c.Merges[0].By
		}
		got = append(got, c.Document+" "+c.ID+" "+c.Kind+" "+c.From+c.To+" "+by)
	}
	want := []string{"in001-reliable-agents R1.1 moved 01.001.1 bob", "srd010-scoring R1.1 reworded  alice",
		"srd010-scoring R1.2 retired  alice", "srd010-scoring R1.3 added  alice"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("changes from v0.1.0 to v0.2.0:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if c := steps[1].Changes; len(c) != 1 || c[0].ID != "AC1" || c[0].Kind != Reworded || c[0].Merges[0].By != "carol" {
		t.Fatalf("changes from v0.2.0 to v0.3.0: %+v", c)
	}
	pages, docs, data := HistoryPages(steps)
	if _, ok := data[Dir+"/data/history/history.json"]; !ok {
		t.Fatal("no JSON copy")
	}
	srd := string(docs[Dir+"/history/docs/srd010-scoring.md"])
	if strings.Count(srd, "\n- corpus/") != 4 || strings.Index(srd, "corpus/v0.2.0 to corpus/v0.3.0") > strings.Index(srd, "corpus/v0.1.0 to corpus/v0.2.0") {
		t.Fatalf("the document's page does not gather both steps, newest first:\n%s", srd)
	}
	for name, page := range map[string][]byte{
		"step": pages[Dir+"/history/corpus-v0.1.0..corpus-v0.2.0.md"], "index": pages[Dir+"/history/index.md"], "document": docs[Dir+"/history/docs/srd010-scoring.md"],
	} {
		// The merge commits differ run to run; the golden files name them
		// by placeholder.
		text := placeholders(string(page), steps)
		golden := filepath.Join("..", "..", "testdata", "site", "history", name+".md.golden")
		if *update {
			os.MkdirAll(filepath.Dir(golden), 0o755)
			os.WriteFile(golden, []byte(text), 0o644)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != text {
			t.Errorf("%s differs from %s:\n%s", name, golden, text)
		}
	}
}

// placeholders replaces each merge commit with a placeholder naming its merger.
func placeholders(page string, steps []Step) string {
	for _, s := range steps {
		for _, c := range s.Changes {
			for _, m := range c.Merges {
				page = strings.ReplaceAll(page, m.Commit, "<merge by "+m.By+">")
			}
		}
	}
	return page
}

// A document's corpus page closes with its history across the tags,
// newest first, linking its page in the history (GH-96).
func TestCorpusPageCarriesItsHistory(t *testing.T) {
	t.Setenv(CacheEnv, t.TempDir())
	system := historyRepo(t)
	steps, err := History(system)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := CorpusPages(system, steps)
	if err != nil {
		t.Fatal(err)
	}
	page := string(pages["site/corpus/specs/software-requirements/srd010-scoring.md"])
	i := strings.Index(page, "\n## History\n")
	if i < 0 || !strings.Contains(page[i:], "(../../../history/docs/srd010-scoring.md)") || strings.Count(page[i:], "\n- corpus/") != 4 ||
		strings.Index(page, "corpus/v0.2.0 to corpus/v0.3.0") > strings.Index(page, "corpus/v0.1.0 to corpus/v0.2.0") {
		t.Fatalf("the SRD's corpus page:\n%s", page)
	}
	if strings.Contains(string(pages["site/corpus/road-map.md"]), "## History") == false {
		t.Errorf("the road map, whose move the history names, has no history")
	}
	// The move is the initiative's history too, linked from its page; the
	// road map names whose item moved and links no history of its own
	// (GH-116).
	in := string(pages["site/corpus/specs/initiatives/in001-reliable-agents.md"])
	if !strings.Contains(in, ": R1.1 moved from release 01.0 to 01.1: merge") || !strings.Contains(in, "(../../../history/docs/in001-reliable-agents.md)") {
		t.Errorf("the initiative's corpus page:\n%s", in)
	}
	roadMap := string(pages["site/corpus/road-map.md"])
	if !strings.Contains(roadMap, "in001-reliable-agents R1.1 moved from release 01.0 to 01.1") || strings.Contains(roadMap, "its history") {
		t.Errorf("the road map's corpus page:\n%s", roadMap)
	}
}

// An initiative item the road map comes to deliver, newly agreed or long
// held, is placed on its release, and one it stops delivering is taken
// off, as a move is recorded (GH-141).
func TestHistoryRecordsPlacements(t *testing.T) {
	doc := func(ids ...string) docSnapshot {
		d := docSnapshot{path: "docs/specs/initiatives/in001.md", items: map[string]entry{}}
		for _, id := range ids {
			d.items[id] = entry{text: id}
		}
		return d
	}
	a := snapshot{docs: map[string]docSnapshot{"in001": doc("R1.1", "R1.4", "R2.1")},
		releases: map[string]string{"in001#R1.1": "01.0", "in001#R1.4": "01.0"}}
	b := snapshot{docs: map[string]docSnapshot{"in001": doc("R1.1", "R1.4", "R2.1", "R2.2")},
		releases: map[string]string{"in001#R1.1": "01.1", "in001#R2.1": "01.1", "in001#R2.2": "01.1"}}
	var got []string
	for _, c := range diff(a, b) {
		got = append(got, strings.TrimSpace(c.ID+" "+strings.TrimPrefix(changeLine(c), "- "+c.ID+" ")))
	}
	want := []string{
		"R1.1 moved from release 01.0 to 01.1: no merge changed it on the main line",
		"R1.4 taken off release 01.0: no merge changed it on the main line",
		"R2.1 placed on release 01.1: no merge changed it on the main line",
		"R2.2 added: no merge changed it on the main line",
		"R2.2 placed on release 01.1: no merge changed it on the main line",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("changes:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// An item released 01.0 still lists, delivered in 01.1 too, was
	// carried into it, not moved (GH-2781).
	b.listed = map[string][]string{"in001#R1.1": {"01.0", "01.1"}, "in001#R2.1": {"01.1"}, "in001#R2.2": {"01.1"}}
	got = nil
	for _, c := range diff(a, b) {
		if c.ID == "R1.1" {
			got = append(got, strings.TrimPrefix(changeLine(c), "- "))
		}
	}
	if !slices.Equal(got, []string{"R1.1 carried from release 01.0 into 01.1: no merge changed it on the main line"}) {
		t.Fatalf("an item still listed in its released release = %v", got)
	}
}
