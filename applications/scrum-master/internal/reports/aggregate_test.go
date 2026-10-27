// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"bytes"
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

func work(obj, comp, status, complete string) Entry {
	return Entry{"objective": obj, "component": comp, "status": status, "complete": complete, "problems": "none"}
}

func block(what, comp, since, waiting string) Entry {
	return Entry{"blocker": what, "component": comp, "since": since, "waiting-on": waiting}
}

// aggregateFixture is two weeks of reports from three people, a tracker
// with one met objective and two merges, and the
// configuration naming them.
// mergeRef is how the tracker names its merge of that title: a merge
// request's number, or a merge commit read from history (GH-141).
func mergeRef(t *testing.T, tr tracker.Tracker, project, title string) string {
	t.Helper()
	merged, err := tr.Merges(project, "", "2000-01-01")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range merged {
		if m.Title == title {
			return m.Ref(project)
		}
	}
	t.Fatalf("no merge %q in %s", title, project)
	return ""
}

func aggregateFixture(t *testing.T, p trackertest.Program) (string, tracker.Tracker, Config) {
	t.Helper()
	dir := t.TempDir()
	put := func(user, week string, w, b []Entry) {
		start, _ := ParseWeek(week)
		writeReport(t, dir, ReportPath(start, user), Render(user, week, w, b))
	}
	put("alice", "2026-W41", []Entry{work("g/po-1#4", "grader", "at-risk", "no")},
		[]Entry{block("stale scores", "grader", "2026-09-01", "scoring-api")})
	put("alice", "2026-W42", []Entry{work("g/po-1#4", "grader", "blocked", "no"), work("g/po-1#5", "grader", "done", "yes")},
		[]Entry{block("stale scores", "grader", "2026-09-01", "scoring-api")})
	put("bob", "2026-W42", []Entry{work("g/po-2#1", "fleet", "on-track", "no"), work("g/po-2#2", "fleet", "done", "yes")},
		[]Entry{block("no scoring quota", "fleet", "2026-10-12", "scoring-api")})

	p.AddMerge("g/po-1", tracker.Merge{IID: 7, Title: "Fix the grader cache", Author: "alice", MergedAt: "2026-10-13", Description: "Closes #9"})
	p.AddMerge("g/po-1", tracker.Merge{IID: 8, Title: "Score one agent", Author: "alice", MergedAt: "2026-10-14", Description: "Closes #4"})
	f := p.Tracker()
	for i := 0; i < 2; i++ {
		f.CreateIssue("g/po-2", tracker.NewIssue{Title: "objective"})
	}
	f.CreateNote("g/po-2", 2, "Met.\n\n<!-- scrum-master program event=met objective=obj001 -->")
	cfg := Config{Team: []Member{{"alice", "g/po-1"}, {"bob", "g/po-2"}, {"carol", "g/po-3"}}, System: "g/system"}
	cfg.defaults()
	return dir, f, cfg
}

func TestAggregateSignalsByComponentNeverByPerson(t *testing.T) {
	trackertest.Each(t, testAggregateSignalsByComponentNeverByPerson)
}

func testAggregateSignalsByComponentNeverByPerson(t *testing.T, p trackertest.Program) {
	dir, f, cfg := aggregateFixture(t, p)
	a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W42"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Reports != 3 || len(a.ByComponent["grader"]) != 3 || len(a.ByComponent["fleet"]) != 3 {
		t.Fatalf("grouped %d reports: %+v", a.Reports, a.ByComponent)
	}
	got := map[string][]Signal{}
	for _, s := range a.Signals {
		got[s.Kind] = append(got[s.Kind], s)
	}
	expect := func(kind string, n int, subject string) {
		t.Helper()
		if len(got[kind]) != n || (n > 0 && !strings.Contains(got[kind][0].Detail, subject)) {
			t.Errorf("%s: %+v", kind, got[kind])
		}
	}
	expect(SignalSharedBlocker, 1, "scoring-api blocks fleet, grader in 2 reports")
	expect(SignalAgingBlocker, 2, "grader has been blocked")
	expect(SignalSlipped, 1, "g/po-1#4 (grader)")
	expect(SignalContradiction, 1, "g/po-1#5 (grader)")
	expect(SignalMissingReport, 3, "no status report for g/po-2 in 2026-W41")
	expect(SignalUnreportedWork, 1, mergeRef(t, f, "g/po-1", "Fix the grader cache"))

	progress := map[string]string{}
	for _, p := range a.Progress {
		progress[p.SprintGoal] = p.State
	}
	if progress["g/po-1#5"] != "reported complete" || progress["g/po-2#2"] != "met" || progress["g/po-1#4"] != "blocked" {
		t.Fatalf("progress %v", progress)
	}
	all, _ := json.Marshal(a.Signals)
	for _, person := range []string{"alice", "bob", "carol"} {
		if strings.Contains(string(all), person) {
			t.Errorf("a signal names %s:\n%s", person, all)
		}
	}
}

func TestActionableSignalIsPushedAsIssue(t *testing.T) {
	trackertest.Each(t, testActionableSignalIsPushedAsIssue)
}

// systemIssues lists g/system's issues, open or closed.
func systemIssues(t *testing.T, f tracker.Tracker) []tracker.Issue {
	t.Helper()
	all, err := f.ListIssues("g/system", tracker.IssueQuery{State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func testActionableSignalIsPushedAsIssue(t *testing.T, p trackertest.Program) {
	f := p.Tracker()
	signals := []Signal{
		{SignalSharedBlocker, "scoring-api", "scoring-api", "2026-W42", "scoring-api blocks fleet, grader in 2 reports", true, []string{"fleet", "grader"}},
		{SignalMissingReport, "g/po-3", "g/po-3", "2026-W42", "no status report for g/po-3 in 2026-W42", false, nil},
	}
	res, err := PushSignals(f, "g/system", signals, program.Defaults(), false)
	if err != nil || len(res) != 1 || res[0].Action != "filed" || len(systemIssues(t, f)) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	i := systemIssues(t, f)[0]
	for _, want := range []string{"## Requirements", "## Acceptance Criteria", "(test: reports_aggregate; environment: local)", "scrum-master reports signal=shared-blocker/scoring-api/scoring-api"} {
		if !strings.Contains(i.Description, want) {
			t.Errorf("issue lacks %q:\n%s", want, i.Description)
		}
	}
	if !strings.Contains(strings.Join(i.Labels, ","), "plane::management") || !strings.Contains(strings.Join(i.Labels, ","), "component::scoring-api") ||
		!strings.Contains(strings.Join(i.Labels, ","), "op::report") {
		t.Fatalf("labels %v", i.Labels)
	}
	me, err := f.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	// Only pop sets an assignee (srd041 R9.2): the signal names the person
	// pushing it and waits, open, for whoever takes it.
	if len(i.Assignees) != 0 || i.State != "opened" || !strings.HasPrefix(i.Description, "@"+me.Username+": ") {
		t.Fatalf("the filed signal: assignees %v, state %s, body %q; want unassigned, open, mentioning %s", i.Assignees, i.State, i.Description, me.Username)
	}
	if bt, ok := f.(*beads.Tracker); ok {
		if b, _, err := bt.Store.Find("g/system", i.IID); err != nil || b.State == beads.Taken {
			t.Fatalf("the filed signal on beads is %s (%v), not taken by a pop", b.State, err)
		}
	}
	again, err := PushSignals(f, "g/system", signals, program.Defaults(), false)
	if err != nil || again[0].Action != "exists" || len(systemIssues(t, f)) != 1 {
		t.Fatalf("second push: %+v %v", again, err)
	}

	// A blocker that is not a listed component files under the first
	// component it holds up, so lint admits it.
	listed := program.Defaults()
	listed.Components, listed.ComponentsRead = []string{"fleet", "grader"}, true
	vendor := []Signal{{SignalSharedBlocker, "vendor-x", "vendor-x", "2026-W42", "vendor-x blocks fleet, grader in 2 reports", true, []string{"fleet", "grader"}}}
	res, err = PushSignals(f, "g/system", vendor, listed, false)
	if err != nil || len(res) != 1 || res[0].Action != "filed" || !res[0].Admitted {
		t.Fatalf("vendor push: %+v %v", res, err)
	}
	for _, j := range systemIssues(t, f) {
		if j.IID == res[0].IID && !slices.Contains(j.Labels, "component::fleet") {
			t.Fatalf("vendor signal labels %v, want component::fleet", j.Labels)
		}
	}
}

func TestAggregateHonoursJSONAndDryRun(t *testing.T) {
	trackertest.Each(t, testAggregateHonoursJSONAndDryRun)
}

func testAggregateHonoursJSONAndDryRun(t *testing.T, p trackertest.Program) {
	dir, f, cfg := aggregateFixture(t, p)
	b, _ := configDocument(cfg)
	writeReport(t, dir, ConfigFile, b)
	writes := p.Writes()
	var out bytes.Buffer
	err := cli.Default.Run([]string{"reports", "aggregate", "--json", "--dry-run", "--push", "--dir", dir, "--from", "2026-W41", "--to", "2026-W42"},
		&out, func() (tracker.Tracker, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	var a Aggregation
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if len(a.Signals) == 0 || len(a.Pushed) == 0 || p.Writes() != writes || len(systemIssues(t, f)) != 0 {
		t.Fatalf("dry run: %d signals, %d pushed, writes %d -> %d", len(a.Signals), len(a.Pushed), writes, p.Writes())
	}
}

func configDocument(c Config) (string, error) {
	b, err := corpus.FormatConfig("reports", "Reports configuration", c)
	return string(b), err
}

// TestAggregateReadsTheLedgerAndOnlyDeliveries shows a Sprint Goal a
// sprint-end merge closed as reported complete in progress though its last
// report said blocked, counts only merges that close an issue as unreported
// work, and words a blocker a component reports on itself.
func TestAggregateReadsTheLedgerAndOnlyDeliveries(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir := t.TempDir()
		for _, week := range []string{"2026-W41", "2026-W42"} {
			start, _ := ParseWeek(week)
			writeReport(t, dir, ReportPath(start, "carol"), Render("carol", week, []Entry{work("g/po-3#1", "platform", "blocked", "no")},
				[]Entry{block("own migration", "platform", "2026-10-05", "platform")}))
		}
		start, _ := ParseWeek("2026-W42")
		writeReport(t, dir, ReportPath(start, "dave"), Render("dave", "2026-W42", []Entry{work("g/po-3#1", "platform", "blocked", "no")},
			[]Entry{block("own migration", "platform", "2026-10-12", "platform")}))
		p.AddMerge("g/po-3", tracker.Merge{IID: 1, Title: "Merge pull request #12 from a fork", Author: "carol", MergedAt: "2026-10-14", Description: "From a pull request"})
		f := p.Tracker()
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "goal"})
		e, text := events.ReportedComplete("01.0", "abc123")
		if _, err := events.Post(f, "g/po-3", 1, e, events.ReportedCompleteRequired, text, false); err != nil {
			t.Fatal(err)
		}
		cfg := Config{Team: []Member{{"carol", "g/po-3"}, {"dave", "g/po-3"}}, System: "g/system"}
		cfg.defaults()

		a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		if len(a.Progress) != 1 || a.Progress[0].State != "reported complete" || !a.Progress[0].Complete {
			t.Errorf("progress = %+v, want g/po-3#1 reported complete", a.Progress)
		}
		shared := false
		for _, s := range a.Signals {
			if s.Kind == SignalUnreportedWork {
				t.Errorf("a merge closing no issue counts as unreported work: %+v", s)
			}
			if s.Kind == SignalSharedBlocker {
				shared = true
				if strings.Contains(s.Detail, "platform blocks platform") {
					t.Errorf("shared blocker on itself reads %q", s.Detail)
				}
			}
		}
		if !shared {
			t.Errorf("no shared-blocker signal: %+v", a.Signals)
		}
	})
}

// TestAggregateSeesATeamWithoutAGoal is srd018 R3.2 for a team whose reports
// name no Sprint Goal of its own: it is reported missing, not left out, and
// its sprint-end merge, which closes only its recurring sprint-end issue,
// is not unreported work, nor is its release merge, which closes the issue
// that cut the release (GH-141), while a merge closing other work still is.
func TestAggregateSeesATeamWithoutAGoal(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir := t.TempDir()
		for _, week := range []string{"2026-W41", "2026-W42"} {
			start, _ := ParseWeek(week)
			writeReport(t, dir, ReportPath(start, "carol"), Render("carol", week, []Entry{work("g/po-3#1", "platform", "on-track", "no")}, nil))
			writeReport(t, dir, ReportPath(start, "erin"), Render("erin", week, nil, nil))
		}
		p.AddMerge("g/po-4", tracker.Merge{IID: 1, Title: "Sprint end 01.0", Author: "erin", MergedAt: "2026-10-14", Description: "Closes g/po-4#1"})
		p.AddMerge("g/po-4", tracker.Merge{IID: 2, Title: "Unplanned work", Author: "erin", MergedAt: "2026-10-14", Description: "Closes g/po-4#2"})
		p.AddMerge("g/po-4", tracker.Merge{IID: 3, Title: "Release corpus 0.47.0", Author: "erin", MergedAt: "2026-10-14", Description: "Closes g/po-4#3"})
		f := p.Tracker()
		defer createdOn(f, "2026-10-12")()
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "goal"})
		f.CreateIssue("g/po-4", tracker.NewIssue{Title: "Recurring: sprint end 01.0", Labels: []string{"plane::data", "type::recurring", "kind::release"}})
		f.CreateIssue("g/po-4", tracker.NewIssue{Title: "Unplanned", Labels: []string{"plane::data", "kind::build"}})
		f.CreateIssue("g/po-4", tracker.NewIssue{Title: "Release corpus 0.47.0", Labels: []string{"plane::data", "kind::release"}})
		cfg := Config{Team: []Member{{"carol", "g/po-3"}, {"erin", "g/po-4"}, {"scrum-master", "g/system"}}, System: "g/system"}
		cfg.defaults()

		a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		var missing, unreported []string
		for _, s := range a.Signals {
			switch s.Kind {
			case SignalMissingReport:
				if strings.Contains(s.Detail, "names a Sprint Goal") {
					missing = append(missing, s.Subject)
				}
			case SignalUnreportedWork:
				unreported = append(unreported, s.Subject)
			}
		}
		if !slices.Equal(missing, []string{"g/po-4"}) {
			t.Errorf("teams reported without a Sprint Goal = %v, want g/po-4 only", missing)
		}
		if !slices.Equal(unreported, []string{mergeRef(t, f, "g/po-4", "Unplanned work")}) {
			t.Errorf("unreported work = %v, want only the merge closing non-recurring work", unreported)
		}
	})
}

// TestAggregateCountsBacklogWorkAsReported shows a merge that closes an
// issue implementing a story on a reported Sprint Goal's Sprint Backlog is
// the goal's work, not unreported, and so is one closing an issue that
// links to such an issue, or one closing only planning on the management
// plane (GH-24), while a merge delivering a story no reported goal lists
// still is.
func TestAggregateCountsBacklogWorkAsReported(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir := t.TempDir()
		for _, week := range []string{"2026-W41", "2026-W42"} {
			start, _ := ParseWeek(week)
			writeReport(t, dir, ReportPath(start, "carol"), Render("carol", week, []Entry{work("g/po-3#1", "platform", "done", "yes")}, nil))
		}
		p.AddMerge("g/po-3", tracker.Merge{IID: 1, Title: "Deliver the story", Author: "carol", MergedAt: "2026-10-14", Description: "Closes #2"})
		p.AddMerge("g/po-3", tracker.Merge{IID: 2, Title: "Deliver another story", Author: "carol", MergedAt: "2026-10-14", Description: "Closes #3"})
		p.AddMerge("g/po-3", tracker.Merge{IID: 3, Title: "Fix a defect in the story", Author: "carol", MergedAt: "2026-10-14", Description: "Closes #4"})
		p.AddMerge("g/po-3", tracker.Merge{IID: 4, Title: "Add the stories", Author: "carol", MergedAt: "2026-10-14", Description: "Closes #5"})
		f := p.Tracker()
		defer createdOn(f, "2026-10-12")()
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "goal", Description: "## Sprint Backlog\n\n- st001-ingest Ingest (3 points)\n"})
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "ingest task", Description: "Implements: st001-ingest\n"})
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "other task", Description: "Implements: st009-other\n"})
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "defect", Description: "Relates-to: g/po-3#2\n", Labels: []string{"plane::data", "kind::defect"}})
		f.CreateIssue("g/po-3", tracker.NewIssue{Title: "add the stories", Labels: []string{"plane::management"}})
		cfg := Config{Team: []Member{{"carol", "g/po-3"}}, System: "g/system"}
		cfg.defaults()

		a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		var unreported []string
		for _, s := range a.Signals {
			if s.Kind == SignalUnreportedWork {
				unreported = append(unreported, s.Subject)
			}
		}
		if !slices.Equal(unreported, []string{mergeRef(t, f, "g/po-3", "Deliver another story")}) {
			t.Errorf("unreported work = %v, want only the merge delivering an unlisted story", unreported)
		}
	})
}

// A week that has not started yet misses no report (GH-66): with the clock
// in 2026-W42, a range to 2026-W45 flags no one for W43 to W45.
func TestNoMissingReportForAWeekNotStarted(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir, f, cfg := aggregateFixture(t, p)
		now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) // Wednesday of 2026-W42
		a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W45", Now: now})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range a.Signals {
			if s.Kind == SignalMissingReport && s.Week > "2026-W42" {
				t.Errorf("a missing report for a week not started: %+v", s)
			}
		}
	})
}

// A week that ended before the reports repository's first commit misses no
// report: reporting had not begun (GH-116). Committed in 2026-W42, the
// repository raises none for W41 and still raises W42's.
func TestNoMissingReportBeforeReportingBegan(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir, f, cfg := aggregateFixture(t, p)
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "reports"}} {
			cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
			cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2026-10-14T12:00:00Z", "GIT_AUTHOR_DATE=2026-10-14T12:00:00Z")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
		now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
		a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W42", Now: now})
		if err != nil {
			t.Fatal(err)
		}
		var weeks []string
		for _, s := range a.Signals {
			if s.Kind == SignalMissingReport && strings.HasPrefix(s.Detail, "no status report") {
				weeks = append(weeks, s.Subject+" "+s.Week)
			}
		}
		if !slices.Equal(weeks, []string{"g/po-3 2026-W42"}) {
			t.Errorf("missing reports %v, want only carol's W42", weeks)
		}
	})
}

// A reports repository first committed on a Monday raises nothing for the
// week that ended as that Monday began, and the day is UTC's, as the weeks
// are: 01:00 at UTC+2 on Monday is still Sunday (GH-141).
func TestReportingBeganOnAMonday(t *testing.T) {
	for _, c := range []struct {
		committed string
		want      []string
	}{
		{"2026-10-19T09:00:00+02:00", []string{"g/po-1 2026-W43", "g/po-2 2026-W43", "g/po-3 2026-W43"}},
		{"2026-10-19T01:00:00+02:00", []string{"g/po-1 2026-W43", "g/po-2 2026-W43", "g/po-3 2026-W42", "g/po-3 2026-W43"}},
	} {
		trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
			dir, f, cfg := aggregateFixture(t, p)
			for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "reports"}} {
				cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
				cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+c.committed, "GIT_AUTHOR_DATE="+c.committed)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
			}
			now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
			a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W42", To: "2026-W43", Now: now})
			if err != nil {
				t.Fatal(err)
			}
			var weeks []string
			for _, s := range a.Signals {
				if s.Kind == SignalMissingReport && strings.HasPrefix(s.Detail, "no status report") {
					weeks = append(weeks, s.Subject+" "+s.Week)
				}
			}
			if !slices.Equal(weeks, c.want) {
				t.Errorf("committed %s: missing reports %v, want %v", c.committed, weeks, c.want)
			}
		})
	}
}

// Narrowed to one project, the aggregate reads only its team's reports,
// raises only its members' missing reports, and states only its Sprint
// Goals' progress (GH-116).
func TestAggregateNarrowedToAProject(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		dir, f, cfg := aggregateFixture(t, p)
		now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
		a, err := Aggregate(f, AggregateInput{Dir: dir, Config: cfg, Policy: program.Defaults(), From: "2026-W41", To: "2026-W42", Now: now, Project: "g/po-1"})
		if err != nil {
			t.Fatal(err)
		}
		if a.Reports != 2 {
			t.Errorf("read %d reports, want alice's 2", a.Reports)
		}
		for _, s := range a.Signals {
			if s.Kind == SignalMissingReport {
				t.Errorf("a missing report outside g/po-1's team: %+v", s)
			}
		}
		for _, pr := range a.Progress {
			if !strings.HasPrefix(pr.SprintGoal, "g/po-1#") {
				t.Errorf("progress outside g/po-1: %+v", pr)
			}
		}
		if len(a.Progress) == 0 {
			t.Error("no progress for g/po-1")
		}
	})
}

// createdOn dates the issues a beads tracker creates until the returned
// function runs, so a fixture's issues exist before the merges that close
// them, as they do in use; other trackers date none (GH-2776).
func createdOn(f tracker.Tracker, day string) func() {
	bt, ok := f.(*beads.Tracker)
	if !ok {
		return func() {}
	}
	at, _ := time.Parse(time.DateOnly, day)
	bt.Store.Now = func() time.Time { return at }
	return func() { bt.Store.Now = time.Now }
}

// TestUnreportedWorkStartsAtTheRepositorysFirstIssue leaves out a merge
// from before the repository's first issue, such as an earlier run's in the
// same history, when no report names a Sprint Goal to start the period
// from, as status does (srd018 R8.1; GH-2776).
func TestUnreportedWorkStartsAtTheRepositorysFirstIssue(t *testing.T) {
	p := trackertest.NewBeads(t)
	p.AddMerge("g/po-3", tracker.Merge{IID: 1, Title: "An earlier run's work", Author: "carol", MergedAt: "2026-10-13", Description: "Closes #1"})
	p.AddMerge("g/po-3", tracker.Merge{IID: 2, Title: "This run's work", Author: "carol", MergedAt: "2026-10-15", Description: "Closes #1"})
	f := p.Tracker()
	restore := createdOn(f, "2026-10-14")
	f.CreateIssue("g/po-3", tracker.NewIssue{Title: "Work", Labels: []string{"plane::data", "kind::build"}})
	restore()
	cfg := Config{Team: []Member{{"carol", "g/po-3"}}, System: "g/system"}
	cfg.defaults()
	a, err := Aggregate(f, AggregateInput{Dir: t.TempDir(), Config: cfg, Policy: program.Defaults(), From: "2026-W42", To: "2026-W42"})
	if err != nil {
		t.Fatal(err)
	}
	var unreported []string
	for _, s := range a.Signals {
		if s.Kind == SignalUnreportedWork {
			unreported = append(unreported, s.Subject)
		}
	}
	if a.Reports != 0 || !slices.Equal(unreported, []string{mergeRef(t, f, "g/po-3", "This run's work")}) {
		t.Fatalf("no report read: unreported work = %v, want only the merge after the repository's first issue", unreported)
	}
}
