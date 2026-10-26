// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// chartLine is every line a gantt block may hold: its header, its settings,
// a section, a milestone, or a task with its optional state, id, start, and
// end, the subset of mermaid's gantt grammar the site writes.
var chartLine = regexp.MustCompile(`^(` + "```mermaid|```" + `|---|config:|  gantt:|    leftPadding: \d+|gantt|    title [^:#;]+|    dateFormat YYYY-MM-DD|    axisFormat %G-W%V|    tickInterval \d+week|    weekday monday|    section [^:#;]+|` +
	`    [^:#;]+ :milestone, t_[A-Za-z0-9_]+, \d{4}-\d\d-\d\d, 0d|    [^:#;]+ :((done|active), )?t_[A-Za-z0-9_]+, \d{4}-\d\d-\d\d, \d{4}-\d\d-\d\d)$`)

// checkCharts fails on any line of a mermaid block outside the grammar.
func checkCharts(t *testing.T, name, page string) {
	t.Helper()
	in := false
	for _, line := range strings.Split(page, "\n") {
		if line == "```mermaid" {
			in = true
		}
		if in && !chartLine.MatchString(line) {
			t.Errorf("%s: %q is outside the gantt grammar", name, line)
		}
		if line == "```" {
			in = false
		}
	}
}

func goldenFacts(t *testing.T) timeline.Facts {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "timeline", "facts.json.golden"))
	if err != nil {
		t.Fatal(err)
	}
	var f timeline.Facts
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// The gantt views of the program with one done, one active, and one future
// epic: the program's chart and the initiative's, against golden files,
// states mapped to mermaid's, and the releases at their sprints' ends
// (GH-84).
func TestGanttGolden(t *testing.T) {
	pages := ganttPages(goldenFacts(t))
	for _, name := range []string{"program", "in001-reliable-agents"} {
		got, ok := pages[Dir+"/"+GanttDir+"/"+name+".md"]
		if !ok {
			t.Fatalf("no %s page among %v", name, keys(pages))
		}
		checkCharts(t, name, string(got))
		golden := filepath.Join("..", "..", "testdata", "site", "gantt", name+".md.golden")
		if *update {
			os.MkdirAll(filepath.Dir(golden), 0o755)
			os.WriteFile(golden, got, 0o644)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("%s differs from %s:\n%s", name, golden, got)
		}
	}
	page := string(pages[Dir+"/"+GanttDir+"/in001-reliable-agents.md"])
	for _, want := range []string{":done, t_ep001_grading, 2026-10-19, 2026-11-07", ":active, t_ep002_scoring, 2026-11-09, 2026-11-28",
		"    ep003-publishing Publishing :t_ep003_publishing, 2026-11-30, 2026-12-19", "    Release 01.0 :milestone, t_release_01_0, 2026-11-06, 0d"} {
		if !strings.Contains(page, want) {
			t.Errorf("the initiative's chart lacks %q:\n%s", want, page)
		}
	}
}

// An initiative with more stories than a chart holds is drawn as several
// charts, none over ChartRows tasks (GH-84).
func TestGanttSplitsACrowdedInitiative(t *testing.T) {
	f := timeline.Facts{Items: []timeline.Item{{ID: "in001-x", Kind: "initiative", Title: "X", Planned: &timeline.Span{Start: "2026-10-19", End: "2026-12-18"}},
		{ID: "ep001-x", Kind: "epic", Title: "X", Parent: "in001-x", Planned: &timeline.Span{Start: "2026-10-19", End: "2026-12-18"}}}}
	for i := 1; i <= 35; i++ {
		f.Items = append(f.Items, timeline.Item{ID: fmt.Sprintf("st%03d-x", i), Kind: "story", Title: "X", Parent: "ep001-x", Planned: &timeline.Span{Start: "2026-10-19", End: "2026-11-06"}})
	}
	page := string(ganttPages(f)[Dir+"/"+GanttDir+"/in001-x.md"])
	checkCharts(t, "in001-x", page)
	if n := strings.Count(page, "```mermaid"); n != 2 || !strings.Contains(page, "## Part 2 of 2") {
		t.Fatalf("%d charts for 36 tasks:\n%s", n, page)
	}
	for _, chart := range strings.Split(page, "```mermaid")[1:] {
		if tasks := regexp.MustCompile(`(?m)^    \S.* :(done, |active, )?t_`).FindAllString(chart, -1); len(tasks) > ChartRows {
			t.Errorf("a chart draws %d tasks", len(tasks))
		}
	}
}
