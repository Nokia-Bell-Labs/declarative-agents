// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// chartOf renders the page for the facts and a tag's plan, and reads back
// the data it inlines.
func chartOf(t *testing.T, now timeline.Facts, tags map[string]timeline.Facts) (string, chartData) {
	t.Helper()
	page, err := TimelineHTML(now, tags)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)<script type="application/json" id="chart-data">(.*?)</script>`).FindStringSubmatch(string(page))
	if m == nil {
		t.Fatal("no inlined chart data")
	}
	var d chartData
	if err := json.Unmarshal([]byte(m[1]), &d); err != nil {
		t.Fatal(err)
	}
	return string(page), d
}

// The page draws the plan itself: no library, no request of its own, and
// the plan now and each tag's inlined (srd048 R5.3, GH-136).
func TestTimelineHTMLIsSelfContained(t *testing.T) {
	now := goldenFacts(t)
	earlier := goldenFacts(t)
	earlier.AsOf = "corpus/v0.1.0"
	for i, it := range earlier.Items {
		if it.ID == "ep002-scoring" {
			earlier.Items[i].Planned = &timeline.Span{Start: "2026-11-09", End: "2026-11-20"}
		}
	}
	html, d := chartOf(t, now, map[string]timeline.Facts{"corpus/v0.1.0": earlier})
	if !strings.HasPrefix(html, "<!doctype html>") {
		t.Fatal("the page lacks its doctype")
	}
	for _, fetch := range []string{`src="http`, `href="http`, "url(http", "url(//", "fetch(", "XMLHttpRequest", "@import", "<link", "frappe", "new Gantt("} {
		if strings.Contains(html, fetch) {
			t.Errorf("the page carries %q", fetch)
		}
	}
	if urls := regexp.MustCompile(`https?://[^"'\s)]+`).FindAllString(html, -1); len(urls) != 1 || urls[0] != "http://www.w3.org/2000/svg" {
		t.Errorf("the page names URLs beyond the SVG namespace: %v", urls)
	}
	if d.Today != now.Today || d.Origin != "2026-10-19" || len(d.Items) != len(now.Items) || len(d.Releases) != len(now.Releases) {
		t.Fatalf("the data: today %s, origin %s, %d items, %d releases", d.Today, d.Origin, len(d.Items), len(d.Releases))
	}
	if len(d.Plans) != 1 || d.Plans[0].Label != "corpus/v0.1.0" || d.Plans[0].Planned["ep002-scoring"].End != "2026-11-20" {
		t.Fatalf("the tag's plan: %+v", d.Plans)
	}
}

// The data carries what each shape needs: an initiative's due dates, a
// Sprint Goal's stories and release, an issue's story and actual days, and
// each item's dependencies (GH-136).
func TestTimelineHTMLCarriesWhatItDraws(t *testing.T) {
	f := goldenFacts(t)
	for i := range f.Items {
		switch f.Items[i].ID {
		case "in001-reliable-agents":
			f.Items[i].Due = []timeline.Due{{Group: "R1", Date: "2026-11-06"}, {Group: "R2", Date: "2026-11-27"}}
		case "ep002-scoring":
			f.Items[i].DependsOn = []string{"ep001-grading"}
		}
	}
	f.Items = append(f.Items, timeline.Item{ID: "g/po-1#9", Kind: timeline.KindIssue, Title: "Grade one", Parent: "st001-grade-trajectories",
		Status: timeline.Done, Actual: &timeline.Span{Start: "2026-10-20", End: "2026-10-28"}})
	_, d := chartOf(t, f, nil)
	by := map[string]chartItem{}
	for _, it := range d.Items {
		by[it.ID] = it
	}
	if due := by["in001-reliable-agents"].Due; len(due) != 2 || due[1].Group != "R2" || due[1].Date != "2026-11-27" {
		t.Errorf("the initiative's due dates: %+v", due)
	}
	if issue := by["g/po-1#9"]; issue.Parent != "st001-grade-trajectories" || issue.Actual == nil || issue.Actual.End != "2026-10-28" {
		t.Errorf("the issue: %+v", issue)
	}
	if deps := by["ep002-scoring"].DependsOn; len(deps) != 1 || deps[0] != "ep001-grading" {
		t.Errorf("the epic's dependencies: %v", deps)
	}
	for _, it := range d.Items {
		if it.Kind == timeline.KindSprintGoal && it.Release == "" {
			t.Errorf("a Sprint Goal without its release: %+v", it)
		}
	}
}

// The page draws each item as what it is: the plan's tree as rows,
// releases as lines, due dates as diamonds, Sprint Goals as flags on their
// epics, actual days beneath the plan, a tag's plan as ghosts, and
// dependencies as arrows; Sprint Goals and issues are no rows (GH-136).
func TestTimelineHTMLDrawsEachItemAsWhatItIs(t *testing.T) {
	html, _ := chartOf(t, goldenFacts(t), nil)
	for _, want := range []string{
		`var PLAN = ["theme", "initiative", "epic", "feature", "story"];`,                 // the only rows
		`it.kind === "sprint_goal"`, `goals.push(it)`, `it.kind === "issue" && it.parent`, // no rows of their own
		"// Releases: a line at each sprint's end", "// An initiative's due dates.", "// The Sprint Goals serving the epic, as flags",
		"// The actual days, a thin bar beneath the plan.", `"stroke-dasharray": "4 3"`, "// Dependencies.",
		`Week: { px:`, `Sprint: { px:`, `Quarter: { px:`, `addDays(lo, -off - 21)`,
		`state.view === "roadmap" ? it.kind === "theme"`, "function drawTree(list)", "window.top.location.href",
		`Compare with the plan at <select id="plan">`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}

// Embedded in the overview with ?embed=1, the timeline drops its heading
// and margin (GH-128).
func TestTimelineHTMLEmbeds(t *testing.T) {
	html, _ := chartOf(t, goldenFacts(t), nil)
	for _, want := range []string{`/[?&]embed=1(&|$)/.test(location.search)`, "html.embed h1 { display: none; }", "html.embed body { margin: 0; }",
		// Embedded, it tells the page its height after each draw, so its
		// frame fits it and nothing scrolls inside the frame (GH-136).
		`window.parent.postMessage({ timelineHeight: document.documentElement.scrollHeight }, "*")`} {
		if !strings.Contains(html, want) {
			t.Errorf("the timeline lacks %q", want)
		}
	}
}
