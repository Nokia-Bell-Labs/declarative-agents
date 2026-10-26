// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// teamCheckout puts po-1's checkout beside the status program's system:
// its own st001, five points, pinned to the system's ep001 and recorded
// done in its 01.0 summary. The system has an st001 of its own, so the two
// must stay apart.
func teamCheckout(t *testing.T, system string) {
	t.Helper()
	team := filepath.Join(filepath.Dir(system), "po-1")
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", "docs", "specs", "stories", "st001-score-trajectories.md"))
	if err != nil {
		t.Fatal(err)
	}
	story := strings.Replace(string(b), "- parent: ft001-grader", "- parent: https://gitlab.example.com/g/system/-/blob/corpus/v0.1.0/docs/specs/epics/ep001-agent-evaluation.md", 1)
	for p, s := range map[string]string{
		"docs/specs/stories/st001-score-trajectories.md": story,
		"docs/sprint-ends/01.0.md":                       "# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score-trajectories\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(team, p)), 0o755)
		if err := os.WriteFile(filepath.Join(team, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "init", "-q", "-b", "main", team).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

// The roll-up page counts a team checkout's stories in the system documents
// they serve, as backlog status does (GH-109), up to the theme (GH-141).
func TestStatusRollUpCountsTeamCheckouts(t *testing.T) {
	in, system := statusProgram(t)
	row := func(id string) string {
		t.Helper()
		pages, _, _, err := StatusPages(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(pages[Dir+"/status/roll-up.md"]), "\n") {
			if strings.HasPrefix(line, "| "+id+" |") {
				return line
			}
		}
		t.Fatalf("no %s row", id)
		return ""
	}
	if before := row("ep001-agent-evaluation"); !strings.HasSuffix(before, "| 5 | 16 |") {
		t.Fatalf("without the team: %s", before)
	}
	theme := row("th001-dependable-agents")
	teamCheckout(t, system)
	if after := row("ep001-agent-evaluation"); !strings.HasSuffix(after, "| 10 | 21 |") {
		t.Fatalf("with po-1's done 5-point story: %s", after)
	}
	var done, total int
	cells := strings.Split(strings.Trim(theme, "| "), " | ")
	fmt.Sscan(cells[len(cells)-2], &done)
	fmt.Sscan(cells[len(cells)-1], &total)
	if after := row("th001-dependable-agents"); !strings.HasSuffix(after, fmt.Sprintf("| %d | %d |", done+5, total+5)) {
		t.Fatalf("the theme, from %s, with po-1's done 5-point story: %s", theme, after)
	}
}

// The timeline draws a team checkout's stories under the system documents
// they serve, named <checkout>/<id> beside the system's own, done as the
// team's summary records; a team's Sprint Goal selects them by that name.
// A tag's facts are the system's plan alone (GH-109).
func TestTimelineDrawsTeamCheckouts(t *testing.T) {
	in, system := statusProgram(t)
	teamCheckout(t, system)
	f, err := Facts(system, "", in.Sources)
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]timeline.Item{}
	for _, it := range f.Items {
		items[it.ID] = it
	}
	team, ok := items["po-1/st001-score-trajectories"]
	if !ok || team.Parent != "ep001-agent-evaluation" || team.Status != timeline.Done || team.Points != 5 {
		t.Fatalf("po-1's story %+v (found %v)", team, ok)
	}
	if own, ok := items["st001-score-trajectories"]; !ok || own.Parent != "ft001-grader" {
		t.Fatalf("the system's own st001 %+v (found %v)", own, ok)
	}
	if f, err = Facts(system, "corpus/v0.1.0", in.Sources); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(f.Items, func(it timeline.Item) bool { return strings.HasPrefix(it.ID, "po-1/") }) {
		t.Fatal("a tag's facts drew today's team checkout")
	}

	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "system", "srd010-delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	srd := strings.ReplaceAll(string(b), "program/delivery/fleet", "g/po-1")
	os.MkdirAll(filepath.Join(system, "docs", "specs", "software-requirements"), 0o755)
	if err := os.WriteFile(filepath.Join(system, "docs", "specs", "software-requirements", "srd010-delivery.md"), []byte(srd), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Tracker.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal 01.0, selected", Labels: []string{"plane::management", "op::plan"},
		Description: "Score one agent.\n\n" + sprintgoal.Marker("01.0", []string{"in001-reliable-agents#R1"}) + "\n\n## Sprint Backlog\n\n- st001-score-trajectories\n"})
	s, err := LoadSources(system, in.Tracker, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	var selected []string
	for _, g := range s.Goals {
		selected = append(selected, g.Stories...)
	}
	if !slices.Equal(selected, []string{"po-1/st001-score-trajectories"}) {
		t.Fatalf("team Sprint Goals select %v", selected)
	}
}
