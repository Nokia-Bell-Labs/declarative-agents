// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package sprintend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

const po1 = "g/po-1"

// period is a reports checkout with one week in which po-1's objective
// g/po-1#1 is reported done and complete, and a tracker holding that
// objective issue and the repository's sprint-end issue g/po-1#2.
func period(t *testing.T, p trackertest.Program) (string, tracker.Tracker, reports.Config) {
	t.Helper()
	dir := t.TempDir()
	start, _ := reports.ParseWeek("2026-W42")
	path := filepath.Join(dir, reports.ReportPath(start, "lead"))
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(reports.Render("lead", "2026-W42", []reports.Entry{
		{"objective": po1 + "#1", "component": "grader", "status": "done", "complete": "yes", "problems": "none"},
	}, nil)), 0o644)
	p.AddRepository(po1)
	f := p.Tracker()
	f.CreateIssue(po1, tracker.NewIssue{Title: "Objective R1", Labels: []string{"plane::management"},
		Description: "Objective R1\n\n<!-- scrum-master goals objective=obj001-reliable-agents#R1 release=01.0 -->"})
	f.CreateIssue(po1, tracker.NewIssue{Title: "Recurring: sprint end 01.0", Labels: []string{"type::recurring"},
		Description: plan.SprintEndBody("01.0", "2026-11-06")})
	cfg := reports.Config{Team: []reports.Member{{Username: "lead", Project: po1}}, ReminderWeekday: "wednesday", StatusIssueTitle: "x"}
	return dir, f, cfg
}

func aggregate(t *testing.T, dir string, f tracker.Tracker, cfg reports.Config) reports.Aggregation {
	t.Helper()
	a, err := reports.Aggregate(f, reports.AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W42", To: "2026-W42"})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func contradictions(a reports.Aggregation) int {
	n := 0
	for _, s := range a.Signals {
		if s.Kind == reports.SignalContradiction {
			n++
		}
	}
	return n
}

func TestReportedCompleteClosesWithoutMet(t *testing.T) {
	trackertest.Each(t, testReportedCompleteClosesWithoutMet)
}

func testReportedCompleteClosesWithoutMet(t *testing.T, p trackertest.Program) {
	dir, f, cfg := period(t, p)
	before := aggregate(t, dir, f, cfg)
	facts, err := MakeFacts(Input{Tracker: f, Project: po1, Aggregate: before, Releases: program.Defaults().Releases, Artifact: "grader"})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.SprintGoals) != 1 || !facts.SprintGoals[0].ReportedComplete || facts.SprintGoals[0].Met || facts.SprintGoals[0].Outcome != "reported complete" || !strings.Contains(facts.SprintGoals[0].MetNeeds, "evidence") || !facts.SprintGoals[0].Legacy || len(facts.SprintGoals[0].Serves) != 1 || facts.SprintGoals[0].Serves[0] != "obj001-reliable-agents#R1" {
		t.Fatalf("facts = %+v", facts.SprintGoals)
	}
	if contradictions(before) != 1 {
		t.Fatalf("an open objective reported complete is a contradiction until sprint end closes it: %+v", before.Signals)
	}

	// The sprint-end delivery closes the sprint-end issue and the objective
	// the owner reported complete, and no met event is posted.
	if checkout := p.Checkout(po1); checkout == "" {
		closeBoth(t, f)
	} else {
		localMergeCloses(t, f, checkout)
	}
	after := aggregate(t, dir, f, cfg)
	if contradictions(after) != 0 || after.Progress[0].State != "reported complete" || after.Progress[0].Met {
		t.Fatalf("after the merge: %+v %+v", after.Progress, after.Signals)
	}
	facts, _ = MakeFacts(Input{Tracker: f, Project: po1, Aggregate: after, Releases: program.Defaults().Releases, Artifact: "grader"})
	if facts.SprintGoals[0].State != "closed" || facts.SprintGoals[0].Met || slices.Contains(facts.SprintGoals[0].Ledger, "met") {
		t.Fatalf("closed objective = %+v", facts.SprintGoals[0])
	}
}

// TestSprintEndReleasesOnlyAfterConfirmation holds the sprint-end issue's
// steps to the order srd038 R1.2 and R5.3 state: the summary merges, then
// the owner confirms, then the gate and tag run on main; without
// confirmation the merge alone closes the issue.
// closeBoth stands in for delivery on the fake, which has no repositories
// to merge in: both issues close.
func closeBoth(t *testing.T, f tracker.Tracker) {
	t.Helper()
	closed := "close"
	for _, iid := range []int{1, 2} {
		if _, err := f.UpdateIssue(po1, iid, tracker.IssueUpdate{State: &closed}); err != nil {
			t.Fatal(err)
		}
	}
}

// localMergeCloses is the beads delivery (srd042 R2.3): the sprint-end
// branch merges locally with a message citing both issues, and close
// closes each on that merge commit.
func localMergeCloses(t *testing.T, f tracker.Tracker, checkout string) {
	t.Helper()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = checkout
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	branch := "2-" + program.Slug("Recurring: sprint end 01.0")
	git("checkout", "-q", "-b", branch, "main")
	if err := os.MkdirAll(filepath.Join(checkout, "docs", "sprint-ends"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "docs", "sprint-ends", "01.0.md"), []byte("# Sprint end 01.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "Sprint end 01.0")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", branch, "-m", fmt.Sprintf("Merge %s (%s#2, closes %s#1)", branch, po1, po1))
	merge := git("rev-parse", "HEAD")
	for _, iid := range []int{2, 1} {
		c, err := program.Close(f, program.ExecGit{}, po1, iid, checkout, "", false)
		if err != nil || !c.Closed || c.Commit != merge {
			t.Fatalf("close %s#%d = %+v, %v, want closed on %s", po1, iid, c, err, merge)
		}
	}
}

func TestSprintEndReleasesOnlyAfterConfirmation(t *testing.T) {
	body := plan.SprintEndBody("01.1", "2026-11-27")
	// The text names skills; the marker comment is the word's own key.
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "<!--") && strings.Contains(line, "scrum-master ") {
			t.Errorf("the sprint-end body names a command line (GH-65): %s", line)
		}
	}
	steps := []string{"the sprint-end skill", "docs/sprint-ends/01.1.md", "the merge skill", "closes", "confirm the release and its version", "the merge alone closes this issue", "Once the summary merges", "the release skill"}
	at := -1
	for _, s := range steps {
		i := strings.Index(body, s)
		if i < 0 || i < at {
			t.Fatalf("%q is missing or out of order in:\n%s", s, body)
		}
		at = i
	}
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir, f, cfg := period(t, p)
		facts, err := MakeFacts(Input{Tracker: f, Project: po1, Aggregate: aggregate(t, dir, f, cfg), Releases: program.Defaults().Releases,
			Artifact: "grader", Tags: []string{"grader/v0.1.0"}})
		if err != nil || facts.Recommended != "0.2.0" || facts.Next.LatestTag != "grader/v0.1.0" || facts.Next.Reason == "" {
			t.Fatalf("recommendation = %q %+v %v", facts.Recommended, facts.Next, err)
		}
	})
}

// TestFactsRecommendFromTheArtifactsCommits reads the artifact's checkout:
// fix-only commits since its latest tag recommend a patch over that tag,
// with the reason, not the first-release minor (#151).
func TestFactsRecommendFromTheArtifactsCommits(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "Score"},
		{"tag", "grader/v0.3.1"}, {"commit", "-q", "--allow-empty", "-m", "Fix the rounding"}} {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	facts, err := MakeFacts(Input{Tracker: fake.New(), Project: po1, Releases: program.Defaults().Releases,
		Artifact: "grader", Tags: []string{"grader/v0.3.1"}, Dir: dir})
	if err != nil || facts.Recommended != "0.3.2" || !strings.Contains(facts.Next.Reason, "fix") {
		t.Fatalf("recommendation = %q, reason %q, %v", facts.Recommended, facts.Next.Reason, err)
	}
	// A team never released whose period planned no Sprint Goal releases
	// nothing; with a Sprint Goal its first release is minor (GH-2779).
	f := fake.New()
	facts, err = MakeFacts(Input{Tracker: f, Project: po1, Releases: program.Defaults().Releases, Artifact: "harness", Dir: dir})
	if err != nil || facts.Recommended != "" || facts.Next.Recommended != "" || !strings.Contains(facts.Next.Reason, "no Sprint Goal this period: nothing to release") {
		t.Fatalf("an empty sprint, never released = %q, reason %q, %v", facts.Recommended, facts.Next.Reason, err)
	}
	f.CreateIssue(po1, tracker.NewIssue{Title: "Sprint Goal: harness", Description: "Goal\n\n<!-- scrum-master goals sprint-goal release=01.0 serves=in001-x#R1 -->\n"})
	facts, err = MakeFacts(Input{Tracker: f, Project: po1, Releases: program.Defaults().Releases, Artifact: "harness", Dir: dir})
	if err != nil || facts.Recommended != "0.1.0" {
		t.Fatalf("a first sprint with a Sprint Goal = %q, reason %q, %v", facts.Recommended, facts.Next.Reason, err)
	}
}

// TestSystemFactsGatherTheTeams lists, for the system repository, the
// Sprint Goals of every team repository, their stories as project#story,
// and a goal a sprint-end merge reported complete from its ledger.
func TestSystemFactsGatherTheTeams(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		const team = "g/agent-ops"
		goal, err := f.CreateIssue(team, tracker.NewIssue{Title: "Sprint Goal: deploy", Labels: []string{"plane::management"},
			Description: "Goal\n\n## Sprint Backlog\n\n- st001-deploy Deploy (3 points)\n\n<!-- scrum-master goals sprint-goal release=01.0 serves=in001-x#R1 -->\n"})
		if err != nil {
			t.Fatal(err)
		}
		e, text := events.ReportedComplete("01.0", "abc123")
		if _, err := events.Post(f, team, goal.IID, e, events.ReportedCompleteRequired, text, false); err != nil {
			t.Fatal(err)
		}
		facts, err := MakeFacts(Input{Tracker: f, Project: "g/system", Teams: []string{team}, Releases: program.Defaults().Releases, Artifact: "corpus"})
		if err != nil {
			t.Fatal(err)
		}
		if len(facts.SprintGoals) != 1 {
			t.Fatalf("facts = %+v", facts.SprintGoals)
		}
		g := facts.SprintGoals[0]
		if g.Issue != fmt.Sprintf("%s#%d", team, goal.IID) || !g.ReportedComplete || g.Met || !slices.Equal(g.Stories, []string{team + "#st001-deploy"}) {
			t.Fatalf("team goal = %+v", g)
		}
	})
}

// TestEmptyListsAreEmpty holds the JSON results to [] for an empty list, not
// null: stories with no story done, and facts for a team with no Sprint Goal
// (srd038 R1.2, R2.4).
func TestEmptyListsAreEmpty(t *testing.T) {
	facts, err := MakeFacts(Input{Tracker: fake.New(), Project: po1, Releases: program.Defaults().Releases, Artifact: "grader"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(facts); !strings.Contains(string(b), `"sprint_goals":[]`) {
		t.Fatalf("facts with no Sprint Goal = %s", b)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, sprintgoal.SummaryDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sprintgoal.SummaryDir, "01.0.md"), []byte("# Sprint end 01.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := cli.Default.Run([]string{"sprint-end", "stories", "--dir", dir, "--release", "01.0", "--done", "", "--json"}, &out,
		func() (tracker.Tracker, error) { return fake.New(), nil }); err != nil {
		t.Fatalf("stories: %v: %s", err, out.String())
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["done"] == nil {
		t.Fatalf("stories with none done = %s, want \"done\": []", out.String())
	}
}

// TestLedgerTellsTheTwoPlannedEventsApart shows filing a Sprint Goal and
// listing its stories, each a planned event, read as two different steps.
func TestLedgerTellsTheTwoPlannedEventsApart(t *testing.T) {
	filed, _ := events.Planned("01.0", []string{"https://example.com/obj001.md#R1"}, "dev")
	listed, _ := events.PlannedStories("01.0", []string{"https://example.com/obj001.md#R1"}, []string{"st001-a", "st002-b"}, "dev")
	if got := []string{ledgerEntry(filed), ledgerEntry(listed)}; !slices.Equal(got, []string{"planned", "planned, 2 stories"}) {
		t.Errorf("ledger = %v", got)
	}
}
