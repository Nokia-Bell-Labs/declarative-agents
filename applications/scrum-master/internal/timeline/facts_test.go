// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package timeline

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// program is a fixture program on 2026-11-18: an initiative with a done
// epic (its story recorded done in 01.0's summary), an active one (its
// story selected into 01.1's Sprint Goal), and a future one not yet agreed.
func program() Input {
	today, _ := time.Parse(time.DateOnly, "2026-11-18")
	return Input{
		Today: today,
		Docs: []*hierarchy.Doc{
			{ID: "in001-reliable-agents", Kind: hierarchy.Initiative, Title: "Reliable agents", Status: "agreed", Horizon: "2026-W43..2026-W51"},
			{ID: "ep001-grading", Kind: hierarchy.Epic, Title: "Grading", Status: "agreed", Parent: "in001-reliable-agents", Horizon: "2026-W43..2026-W45", Architecture: []string{"grader"}},
			{ID: "st001-grade-trajectories", Kind: hierarchy.Story, Title: "Grade trajectories", Status: "agreed", Parent: "ep001-grading", Points: "5"},
			{ID: "ep002-scoring", Kind: hierarchy.Epic, Title: "Scoring", Status: "agreed", Parent: "in001-reliable-agents", Horizon: "2026-W46..2026-W48", Architecture: []string{"scorer"}},
			{ID: "st002-score-runs", Kind: hierarchy.Story, Title: "Score runs", Status: "agreed", Parent: "ep002-scoring", Points: "3"},
			{ID: "ep003-publishing", Kind: hierarchy.Epic, Title: "Publishing", Status: "to_be_agreed", Parent: "in001-reliable-agents", Horizon: "2026-W49..2026-W51"},
		},
		Releases: []Release{
			{Version: "01.0", Status: "released", Tag: "corpus/v0.1.0", Sprint: &Span{"2026-10-19", "2026-11-06"}},
			{Version: "01.1", Status: "specified", Tag: "corpus/v0.2.0", Sprint: &Span{"2026-11-09", "2026-11-27"}},
			{Version: "01.2", Status: "specified", Sprint: &Span{"2026-11-30", "2026-12-18"}},
		},
		Done:   map[string]string{"st001-grade-trajectories": "01.0"},
		Landed: map[string]string{"01.0": "2026-11-06"},
		Goals: []Goal{
			{Ref: "g/po-1#1", Title: "Sprint Goal 01.0", Release: "01.0", Created: "2026-10-19T09:00:00Z", Closed: "2026-11-06T16:00:00Z", Stories: []string{"st001-grade-trajectories"}},
			{Ref: "g/po-1#2", Title: "Sprint Goal 01.1", Release: "01.1", Created: "2026-11-09T09:00:00Z", Stories: []string{"st002-score-runs"}},
		},
	}
}

// The facts of a program with one done, one active, and one future epic:
// spans, states, parents, citations, and points, against a golden file
// (GH-82).
func TestFactsGolden(t *testing.T) {
	f := Make(program())
	got, _ := json.MarshalIndent(f, "", "  ")
	golden := filepath.Join("..", "..", "testdata", "timeline", "facts.json.golden")
	if *update {
		os.WriteFile(golden, append(got, '\n'), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("facts differ from %s:\n%s", golden, got)
	}
	state := map[string]Item{}
	for _, it := range f.Items {
		state[it.ID] = it
	}
	for id, want := range map[string]string{"ep001-grading": Done, "ep002-scoring": Active, "ep003-publishing": Future,
		"st001-grade-trajectories": Done, "st002-score-runs": Active, "g/po-1#1": Done, "g/po-1#2": Active, "in001-reliable-agents": Active} {
		if state[id].Status != want {
			t.Errorf("%s is %s, want %s", id, state[id].Status, want)
		}
	}
	if ep := state["ep001-grading"]; ep.Release != "01.0" || ep.Actual == nil || ep.Actual.End != "2026-11-06" || ep.Points != 5 || ep.PointsDone != 5 {
		t.Errorf("the done epic %+v", ep)
	}
	if st := state["st002-score-runs"]; st.Release != "01.1" || st.Planned == nil || st.Planned.Start != "2026-11-09" || st.Actual != nil {
		t.Errorf("the active story %+v", st)
	}
	if in := state["in001-reliable-agents"]; in.Points != 8 || in.PointsDone != 5 || in.Actual == nil || in.Actual.End != "" {
		t.Errorf("the initiative %+v", in)
	}
}

// An agreed story no Sprint Goal selects is unplanned, so it stays future,
// though its epic's horizon began weeks ago (GH-141).
func TestUnplacedStoryIsNotActive(t *testing.T) {
	in := program()
	in.Docs = append(in.Docs, &hierarchy.Doc{ID: "st003-unplanned", Kind: hierarchy.Story, Title: "Unplanned", Status: "agreed", Parent: "ep002-scoring", Points: "2", Horizon: "2026-W43..2026-W51"})
	for _, it := range Make(in).Items {
		if it.ID == "st003-unplanned" && (it.Status != Future || it.Release != "") {
			t.Fatalf("the unplaced story %+v, want future with no release", it)
		}
	}
}

// An issue other than a Sprint Goal is on the timeline: planned in the
// sprint of the release its milestone names, its actual days from opening
// to closing, its parent the story it implements (GH-96).
func TestIssuesOnTheTimeline(t *testing.T) {
	in := program()
	in.Issues = []Issue{
		{Ref: "g/po-1#3", Title: "Grade one run", Milestone: "01.0", Story: "st001-grade-trajectories", Created: "2026-10-20T09:00:00Z", Closed: "2026-10-30T12:00:00Z", URL: "https://example.invalid/3"},
		{Ref: "g/po-1#4", Title: "Score one run", Milestone: "01.1", Story: "st002-score-runs", Created: "2026-11-10T09:00:00Z"},
		{Ref: "g/po-1#5", Title: "Unplanned"},
	}
	items := map[string]Item{}
	for _, it := range Make(in).Items {
		items[it.ID] = it
	}
	done, open, loose := items["g/po-1#3"], items["g/po-1#4"], items["g/po-1#5"]
	if done.Kind != KindIssue || done.Status != Done || done.Parent != "st001-grade-trajectories" || done.Release != "01.0" ||
		done.Planned == nil || done.Planned.End != "2026-11-06" || done.Actual == nil || done.Actual.Start != "2026-10-20" || done.Actual.End != "2026-10-30" || done.URL == "" {
		t.Errorf("the closed issue %+v", done)
	}
	if open.Status != Active || open.Release != "01.1" || open.Actual.End != "" {
		t.Errorf("the open issue %+v", open)
	}
	if loose.Status != Future || loose.Planned != nil {
		t.Errorf("the issue with no milestone %+v", loose)
	}
}

// A story's statement takes the article its role needs.
func TestStatementArticle(t *testing.T) {
	for role, want := range map[string]string{"evaluation owner": "an", "product team lead": "a", "": "a"} {
		if got := article(role); got != want {
			t.Errorf("article(%q) = %q, want %q", role, got, want)
		}
	}
}

// A planning document's dependencies are its depends_on and blocked_by
// links, a pinned URL read as the document it names and a team's own link
// in its team (GH-135).
func TestDocDeps(t *testing.T) {
	d := &hierarchy.Doc{ID: "po-1/st002-store", Links: map[string][]string{
		"depends_on": {"st001-grade", "https://git.example.com/g/system/-/blob/v1/docs/specs/epics/ep001-grading.md#r1"},
		"blocked_by": {"st001-grade", " "},
	}}
	if got := docDeps(d); strings.Join(got, " ") != "po-1/st001-grade ep001-grading" {
		t.Fatalf("deps %v", got)
	}
	sys := &hierarchy.Doc{ID: "ep006-eval", Links: map[string][]string{"depends_on": {"ep002-runtime", "ep003-gateway"}}}
	if got := docDeps(sys); strings.Join(got, " ") != "ep002-runtime ep003-gateway" {
		t.Fatalf("system deps %v", got)
	}
}

// An initiative's requirement groups come with their due days, R2 before
// R10 (GH-136).
func TestDues(t *testing.T) {
	d := &hierarchy.Doc{Due: map[string]string{"R10": "2027-03-26", "R2": "2026-11-27", "R1": "2026-11-06", "R3": ""}}
	var got []string
	for _, due := range dues(d) {
		got = append(got, due.Group+"="+due.Date)
	}
	if strings.Join(got, " ") != "R1=2026-11-06 R2=2026-11-27 R10=2027-03-26" {
		t.Fatalf("dues %v", got)
	}
}
