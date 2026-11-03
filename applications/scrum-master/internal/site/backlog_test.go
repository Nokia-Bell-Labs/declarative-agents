// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// The timeline facts carry each story's issues record as its document
// holds it, read from the checkouts and never the tracker (srd048 R10.1).
func TestFactsCarryTheStoriesRecord(t *testing.T) {
	in, system := statusProgram(t)
	storyPath := filepath.Join(system, "docs", "specs", "stories", "st001-score-trajectories.md")
	doc, err := os.ReadFile(storyPath)
	if err != nil {
		t.Fatal(err)
	}
	entry := backlog.LedgerEntry{Ref: "g/po-1#7", URL: "https://gitlab.example.com/g/po-1/-/issues/7", Title: "Define scoring",
		Who: []string{"ana"}, Opened: "2026-10-28", Closed: "2026-10-30", State: "closed"}
	if err := os.WriteFile(storyPath, backlog.WriteLedger(doc, []backlog.LedgerEntry{entry}), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSources(system, in.Tracker, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range f.Items {
		if it.ID != "st001-score-trajectories" {
			continue
		}
		if len(it.Ledger) != 1 || it.Ledger[0].Ref != entry.Ref || it.Ledger[0].Closed != "2026-10-30" || strings.Join(it.Ledger[0].Who, ",") != "ana" {
			t.Fatalf("the story's record = %+v", it.Ledger)
		}
		return
	}
	t.Fatal("the story is not among the items")
}

// The backlog pages draw the facts (srd048 R10.2): the index's open table
// with who works on an issue, how long it has been open, and the
// projected time from its story's release sprint, the closed table with
// how long an issue took, every issue linked on its tracker where its url
// opens and named without a link where it does not (R8.5), each row
// linking its story's and its epic's pages; the embedded page inlines the
// same rows as a kanban board — done is closed, in progress is open with
// someone on it, to do is open with nobody (GH-2855) — and filters by
// repository, state, and who.
func TestBacklogPages(t *testing.T) {
	dir := t.TempDir()
	f := timeline.Facts{Today: "2026-11-04",
		Releases: []timeline.Release{{Version: "01.0", Status: "specified", Sprint: &timeline.Span{Start: "2026-10-26", End: "2026-11-13"}}},
		Items: []timeline.Item{
			{ID: "ep001-x", Kind: "epic", Title: "The epic"},
			{ID: "ft001-y", Kind: "feature", Title: "The feature", Parent: "ep001-x"},
			{ID: "st001-z", Kind: "story", Title: "The story", Parent: "ft001-y", Release: "01.0", Ledger: []timeline.IssueRecord{
				{Ref: "g/code-1#3", URL: "https://gitlab.example.com/g/code-1/-/issues/3", Title: "Wire it", Who: []string{"bo"}, Opened: "2026-11-01", State: "open"},
				{Ref: "g/po-1#7", Title: "Define it", Who: []string{"ana"}, Opened: "2026-10-27", Closed: "2026-10-30", State: "closed"},
			}},
		}}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, Dir, "timeline", "facts.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := BacklogPages(dir)
	if err != nil {
		t.Fatal(err)
	}
	index := string(pages[Dir+"/"+BacklogDir+"/index.md"])
	for _, want := range []string{
		`<iframe class="timeline" src="backlog/backlog.html?embed=1"`,
		"| [g/code-1#3](https://gitlab.example.com/g/code-1/-/issues/3) | Wire it | bo | 2026-11-01 | 3 days | release 01.0, by 2026-11-13 | [The story](../hierarchy/st001-z.md) | [The epic](../hierarchy/ep001-x.md) |",
		"| g/po-1#7 | Define it | ana | 2026-10-27 | 2026-10-30 | 3 days | [The story](../hierarchy/st001-z.md) | [The epic](../hierarchy/ep001-x.md) |",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the index lacks %q:\n%s", want, index)
		}
	}
	if strings.Contains(index, "[g/po-1#7](") {
		t.Error("an issue without a url must not link (R8.5)")
	}
	html := string(pages[Dir+"/"+BacklogDir+"/backlog.html"])
	for _, want := range []string{
		`id="repo"`, `id="state"`, `id="who"`, `"ref":"g/code-1#3"`, `"story":"st001-z"`, `"epic":"ep001-x"`,
		`timelineHeight`, `"projected":"release 01.0, by 2026-11-13"`,
		`data-col="todo"`, `data-col="progress"`, `data-col="done"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the embedded page lacks %q", want)
		}
	}
}

// An issue naming an epic directly has no story: the facts carry the epic's
// record from its document, the backlog links the epic alone, the epic's page
// lists the issue, and its points stay its stories' (srd048 R10.1, AC12).
func TestAnEpicsOwnIssues(t *testing.T) {
	in, system := statusProgram(t)
	p := filepath.Join(system, "docs", "specs", "epics", "ep001-agent-evaluation.md")
	doc, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s, err := LoadSources(system, in.Tracker, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	points := func(f timeline.Facts) (int, int, timeline.Item) {
		for _, it := range f.Items {
			if it.ID == "ep001-agent-evaluation" {
				return it.Points, it.PointsDone, it
			}
		}
		t.Fatal("the epic is not among the items")
		return 0, 0, timeline.Item{}
	}
	before, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	wantPoints, wantDone, _ := points(before)
	entry := backlog.LedgerEntry{Ref: "g/po-1#9", Title: "No story yet", Who: []string{"ana"}, Opened: "2026-10-28", State: "open"}
	if err := os.WriteFile(p, backlog.WriteLedger(doc, []backlog.LedgerEntry{entry}), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	got, done, epic := points(f)
	if got != wantPoints || done != wantDone {
		t.Fatalf("points = %d of %d, want %d of %d: an issue adds none", done, got, wantDone, wantPoints)
	}
	if len(epic.Ledger) != 1 || epic.Ledger[0].Ref != entry.Ref {
		t.Fatalf("the epic's record = %+v", epic.Ledger)
	}
	f.Items = append(f.Items, timeline.Item{ID: entry.Ref, Kind: timeline.KindIssue, Title: entry.Title, Parent: epic.ID})
	nodes, _ := hierarchyNodes(f)
	if n := nodes[epic.ID]; n == nil || !slices.Contains(n.Issues, entry.Ref) {
		t.Fatalf("the epic's node = %+v", n)
	}
	b, _ := json.Marshal(f)
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, Dir, "timeline", "facts.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := BacklogPages(dir)
	if err != nil {
		t.Fatal(err)
	}
	var row string
	for _, line := range strings.Split(string(pages[Dir+"/"+BacklogDir+"/index.md"]), "\n") {
		if strings.Contains(line, entry.Ref) {
			row = line
		}
	}
	if !strings.Contains(row, "| none |") || !strings.Contains(row, "../hierarchy/"+epic.ID+".md") {
		t.Fatalf("the backlog's row = %q", row)
	}
}
