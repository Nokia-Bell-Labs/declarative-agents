// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

func listFacts(t *testing.T, f timeline.Facts) string {
	t.Helper()
	dir := t.TempDir()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, Dir, "timeline", "facts.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The lists draw the facts (srd048 R8.12): each index tables its documents as
// open or closed, each linking its page, a story naming its epic and an epic
// counting the issues of the stories under it; each embedded page inlines the
// same rows with a column per state and filters them.
func TestListPages(t *testing.T) {
	dir := listFacts(t, timeline.Facts{Today: "2026-11-04", Unread: []string{"g/lost"}, People: []timeline.Person{{Username: "ana", Epics: []string{"ep001-x"}}},
		Items: []timeline.Item{
			{ID: "ep001-x", Kind: "epic", Title: "The epic", Status: "active", Owner: "A Product Owner", Horizon: "2026-W44..2026-W50", Points: 8, PointsDone: 3,
				Source: "https://gitlab.example.com/g/program/-/blob/HEAD/docs/specs/epics/ep001-x.md"},
			{ID: "ep002-y", Kind: "epic", Title: "The finished epic", Status: "done"},
			{ID: "team/ft001-y", Kind: "feature", Title: "The feature", Parent: "ep001-x", Status: "active"},
			{ID: "team/st001-z", Kind: "story", Title: "The story", Parent: "team/ft001-y", Status: "active", Release: "01.0", Owner: "A Product Owner", Points: 5, PointsDone: 0,
				Statement: "As a reader, I want it, so that I can.",
				Ledger: []timeline.IssueRecord{
					{Ref: "g/team#3", Who: []string{"bo"}, State: "open"},
					{Ref: "g/team#4", Who: []string{"bo", "cy"}, State: "closed"},
				}},
			{ID: "team/st002-w", Kind: "story", Title: "The finished story", Parent: "team/ft001-y", Status: "done", Points: 3, PointsDone: 3},
		}})
	epics, stories, err := ListPages(dir)
	if err != nil {
		t.Fatal(err)
	}
	index := string(epics[Dir+"/"+EpicsDir+"/index.md"])
	for _, want := range []string{
		`<iframe class="timeline" src="epics/epics.html?embed=1"`,
		"No checkout of g/lost here",
		"| [The epic](../hierarchy/ep001-x.md) | active | program |  | A Product Owner | 3 of 8 | 1 open, 1 closed | 1 feature · 2 stories |",
		"## Closed\n\n| epic |",
		"| [The finished epic](../hierarchy/ep002-y.md) | done | system |",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the epics' index lacks %q:\n%s", want, index)
		}
	}
	index = string(stories[Dir+"/"+StoriesDir+"/index.md"])
	for _, want := range []string{
		`<iframe class="timeline" src="stories/stories.html?embed=1"`,
		"| [The story](../hierarchy/team/st001-z.md) | active | team | 01.0 | A Product Owner | 0 of 5 | 1 open, 1 closed | [The epic](../hierarchy/ep001-x.md) |",
		"| [The finished story](../hierarchy/team/st002-w.md) | done | team |",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the stories' index lacks %q:\n%s", want, index)
		}
	}
	html := string(epics[Dir+"/"+EpicsDir+"/epics.html"])
	for _, want := range []string{
		`id="board"`, `id="repo"`, `id="state"`, `id="release"`, `id="owner"`, `id="who"`, `timelineHeight`,
		`data-col="future"`, `data-col="active"`, `data-col="done"`,
		`"id":"ep001-x"`, `"who":["ana","bo","cy"]`, `"open_issues":1`, `"closed_issues":1`, `" epics"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the epics' page lacks %q", want)
		}
	}
	if strings.Contains(html, `<select id="epic"`) {
		t.Error("the epics' page must not filter by epic")
	}
	html = string(stories[Dir+"/"+StoriesDir+"/stories.html"])
	for _, want := range []string{
		`<select id="epic"`, `"id":"team/st001-z"`, `"epic":"ep001-x"`, `"epic_title":"The epic"`, `"who":["bo","cy"]`, `"statement":"As a reader`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the stories' page lacks %q", want)
		}
	}
}

// A list with no document says so and how one comes to appear, and its board
// reads an empty list.
func TestAnEmptyListSaysSo(t *testing.T) {
	epics, stories, err := ListPages(listFacts(t, timeline.Facts{Today: "2026-11-04"}))
	if err != nil {
		t.Fatal(err)
	}
	for page, want := range map[string]string{
		string(epics[Dir+"/"+EpicsDir+"/index.md"]):         "No epic yet.",
		string(stories[Dir+"/"+StoriesDir+"/index.md"]):     "No story yet.",
		string(epics[Dir+"/"+EpicsDir+"/epics.html"]):       `"rows":[]`,
		string(stories[Dir+"/"+StoriesDir+"/stories.html"]): `"rows":[]`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q:\n%s", want, page)
		}
	}
}

// A title holding markup reaches the board as data, never as a closing script
// tag that ends the inlined rows early.
func TestAListsRowsCannotCloseTheirScript(t *testing.T) {
	epics, _, err := ListPages(listFacts(t, timeline.Facts{Items: []timeline.Item{
		{ID: "ep001-x", Kind: "epic", Title: "</script><b>bold</b>", Status: "future"},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(epics[Dir+"/"+EpicsDir+"/epics.html"]), "</script><b>") {
		t.Error("the title reached the page unescaped")
	}
}
