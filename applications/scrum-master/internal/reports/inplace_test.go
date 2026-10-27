// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

const po2 = "g/product-owners/po-2"

// subsystemSRD is a subsystem SRD owned by owner whose one component lives
// in repo.
func subsystemSRD(id, owner, repo string) []byte {
	return []byte(fmt.Sprintf(`---
id: %[1]s
title: "Subsystem: %[1]s"
---

<!-- field: context | form: "tree" -->
## Context

- owner: %[2]s

<!-- req: R1 -->
## R1 - Work

<!-- req: R1.1 -->
### R1.1

Work arrives.

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| %[1]s | https://gitlab.example.com/%[3]s | / |
`, id, owner, repo))
}

// inPlaceProgram is a system repository with its system SRD, two teams'
// subsystem SRDs, and
// a roster of three people, each reporting in their own repository, with
// week 2026-W42's reports merged where they were written: po-1's and the
// program manager's, po-1's team rollup, and a report of an earlier week.
// po-2 has not reported.
func inPlaceProgram(p trackertest.Program) Config {
	// The system SRD declares the system repository as its component; it is
	// the system, never a team (GH-2725).
	p.AddFile("g/system", "main", "docs/specs/software-requirements/srd001-system.md", subsystemSRD("srd001-system", "tpm", "g/system"))
	p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", subsystemSRD("srd010-po-1", "po-1", po1))
	p.AddFile("g/system", "main", "docs/specs/software-requirements/srd011-po-2.md", subsystemSRD("srd011-po-2", "po-2", po2))
	cfg := Config{Team: []Member{{"po-1", po1}, {"po-2", po2}, {"tpm", "g/system"}}}
	doc, _ := configDocument(cfg)
	p.AddFile("g/system", "main", ControlFile, []byte(doc))
	w42, _ := ParseWeek("2026-W42")
	w40, _ := ParseWeek("2026-W40")
	p.AddFile(po1, "main", InPlaceReportPath(w42, "po-1"), []byte(Render("po-1", "2026-W42",
		[]Entry{work(po1+"#1", "po-1", "at-risk", "no")}, []Entry{block("stale scores", "po-1", "2026-10-12", "scoring-api")})))
	p.AddFile(po1, "main", InPlaceReportPath(w40, "po-1"), []byte(Render("po-1", "2026-W40", nil, nil)))
	p.AddFile("g/system", "main", InPlaceReportPath(w42, "tpm"), []byte(Render("tpm", "2026-W42",
		nil, []Entry{block("stale scores", "system", "2026-10-12", "scoring-api")})))
	p.AddFile(po1, "main", RollupPath(w42, "srd010-po-1"), []byte(RenderRollup(RollupFrontMatter{Level: "team", Subsystem: "srd010-po-1", Author: "po-1", Period: "2026-W42"}, nil, nil, nil, nil, nil)))
	cfg.System = "g/system"
	cfg.defaults()
	return cfg
}

// A team collects its subsystem's reports where they were written, and the
// program every repository's reports and the teams' rollups, each with its
// repository and the commit that last changed it; an earlier week stays out,
// and nothing is cloned or written (GH-2725).
func TestCollectReadsReportsWhereTheyWereWritten(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		cfg := inPlaceProgram(p)
		f := p.Tracker()
		writes := p.Writes()
		team, err := Collect(f, CollectInput{Config: cfg, System: "g/system", Ref: "main", Level: "team", Project: po1, From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		if team.Subsystem != "srd010-po-1" || !slices.Equal(team.Repositories, []string{po1}) || len(team.Reports) != 1 || len(team.Rollups) != 0 {
			t.Fatalf("team collection = %+v", team)
		}
		r := team.Reports[0]
		if r.Repository != po1 || r.Name != "po-1" || r.Week != "2026-W42" || r.Commit == "" || !strings.Contains(r.Content, "stale scores") || len(team.Unread) != 0 {
			t.Fatalf("team report = %+v, unread %v", r, team.Unread)
		}
		prog, err := Collect(f, CollectInput{Config: cfg, System: "g/system", Ref: "main", Level: ProgramLevel, From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, r := range prog.Reports {
			names = append(names, r.Name)
		}
		slices.Sort(names)
		if !slices.Equal(prog.Repositories, []string{"g/system", po1, po2}) || !slices.Equal(names, []string{"po-1", "tpm"}) ||
			len(prog.Rollups) != 1 || prog.Rollups[0].Name != "srd010-po-1" || prog.Rollups[0].Commit == "" {
			t.Fatalf("program collection = %+v", prog)
		}
		if p.Writes() != writes {
			t.Errorf("collect wrote %d times", p.Writes()-writes)
		}
	})
}

// Aggregated across the repositories it was collected from, a blocker two
// reports in two repositories share is one signal, a person who has not
// reported is missing with their open status issue named, and merged work
// in a repository no report mentions is unreported; no signal compares
// people (GH-2725, srd018 R3.4).
func TestAggregateReadsACollectionAcrossRepositories(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		cfg := inPlaceProgram(p)
		p.AddMerge(po2, tracker.Merge{IID: 1, Title: "Unplanned work", Author: "po-2", MergedAt: "2026-10-14", Description: "Closes " + po2 + "#1"})
		f := p.Tracker()
		defer createdOn(f, "2026-10-12")()
		f.CreateIssue(po1, tracker.NewIssue{Title: "goal"})
		f.CreateIssue(po2, tracker.NewIssue{Title: "Unplanned", Labels: []string{"plane::data", "kind::build"}})
		status, _ := f.CreateIssue(po2, tracker.NewIssue{Title: "Recurring: weekly status 2026-W42", Labels: []string{"type::recurring"},
			Description: InPlaceStatusBody(cfg, "po-2", po2, mustWeek("2026-W42"), mustWeek("2026-W42").AddDate(0, 0, 3))})
		col, err := Collect(f, CollectInput{Config: cfg, System: "g/system", Ref: "main", Level: ProgramLevel, From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		if len(col.Open) != 1 || col.Open[0].Kind != "status" || col.Open[0].Name != "po-2" || col.Open[0].IID != status.IID {
			t.Fatalf("open = %+v", col.Open)
		}
		a, err := Aggregate(f, AggregateInput{Config: cfg, Policy: program.Defaults(), From: "2026-W42", To: "2026-W42", Collection: &col, Now: mustWeek("2026-W43")})
		if err != nil {
			t.Fatal(err)
		}
		var shared, missing, unreported []Signal
		for _, s := range a.Signals {
			switch s.Kind {
			case SignalSharedBlocker:
				shared = append(shared, s)
			case SignalMissingReport:
				missing = append(missing, s)
			case SignalUnreportedWork:
				unreported = append(unreported, s)
			}
			for _, word := range []string{" than ", " fewer ", " better ", " worse "} {
				if strings.Contains(s.Detail, word) {
					t.Errorf("a signal compares people: %q", s.Detail)
				}
			}
		}
		if len(shared) != 1 || shared[0].Subject != "scoring-api" || !strings.Contains(shared[0].Detail, "in 2 reports") {
			t.Errorf("shared blockers = %+v", shared)
		}
		ref := fmt.Sprintf("%s#%d", po2, status.IID)
		if len(missing) != 1 || missing[0].Subject != po2 || !strings.Contains(missing[0].Detail, "its status issue "+ref+" is open") {
			t.Errorf("missing reports = %+v", missing)
		}
		if len(unreported) != 1 || unreported[0].Component != po2 {
			t.Errorf("unreported work = %+v", unreported)
		}
	})
}

// With the roster in the system repository, status-issues leaves one
// status issue per person where they report, one team rollup issue per
// subsystem for its owner, and one program rollup issue for the lead, each
// due after the last, and files nothing on a second run (GH-2725).
func TestStatusIssuesInPlaceFileReportsAndRollups(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		cfg := inPlaceProgram(p)
		f := p.Tracker()
		in := RollupInput{Config: cfg, System: "g/system", Ref: "main", Policy: program.Defaults(), Lead: "tpm", Now: monday}
		res, err := StatusIssuesInPlace(f, in)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range res {
			got = append(got, fmt.Sprintf("%s %s %s %s %s", r.Action, r.Kind, r.User, r.Project, r.DueDate))
		}
		want := []string{
			"filed status po-1 " + po1 + " 2026-10-15",
			"filed status po-2 " + po2 + " 2026-10-15",
			"filed status tpm g/system 2026-10-15",
			"filed team-rollup po-1 " + po1 + " 2026-10-16",
			"filed team-rollup po-2 " + po2 + " 2026-10-16",
			"filed program-rollup tpm g/system 2026-10-19",
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("status-issues =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		i, _ := f.GetIssue(po1, res[0].IID)
		if !strings.Contains(i.Description, "status/reports/2026-Q4/2026-W42-po-1.md in "+po1) || strings.Contains(i.Description, "g/reports") {
			t.Errorf("the status issue does not ask for the report in place:\n%s", i.Description)
		}
		team, _ := f.GetIssue(po1, res[3].IID)
		if team.Title != "Recurring: team rollup srd010-po-1 2026-W42" || !strings.HasPrefix(team.Description, "@po-1: the srd010-po-1 team rollup") ||
			!strings.Contains(team.Description, "status/rollups/2026-Q4/2026-W42-srd010-po-1.md") {
			t.Errorf("team rollup issue = %+v", team)
		}
		writes := p.Writes()
		again, err := StatusIssuesInPlace(f, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range again {
			if r.Action != "exists" {
				t.Errorf("a second run %s %s %s", r.Action, r.Kind, r.User)
			}
		}
		if p.Writes() != writes {
			t.Errorf("a second run wrote %d times", p.Writes()-writes)
		}
	})
}

// previous finds a person's last report before this week, its blockers and
// what they committed to next; path names where this week's goes (GH-2725).
func TestPreviousAndPath(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		w41, _ := ParseWeek("2026-W41")
		p.AddFile(po1, "main", InPlaceReportPath(w41, "po-1"), []byte(RenderWith("po-1", "2026-W41", nil,
			[]Entry{block("stale scores", "po-1", "2026-10-05", "scoring-api")}, []string{"the grader scores one task"}, []string{"score a second task"})))
		p.AddFile(po1, "main", InPlaceReportPath(mustWeek("2026-W42"), "po-1"), []byte(Render("po-1", "2026-W42", nil, nil)))
		f := p.Tracker()
		prev, err := PreviousReport(f, Config{}, po1, "po-1", monday)
		if err != nil {
			t.Fatal(err)
		}
		if !prev.Found || prev.Week != "2026-W41" || prev.Commit == "" || !slices.Equal(prev.Next, []string{"score a second task"}) ||
			len(prev.Blockers) != 1 || prev.Blockers[0]["waiting-on"] != "scoring-api" || !slices.Equal(prev.Impact, []string{"the grader scores one task"}) {
			t.Fatalf("previous = %+v", prev)
		}
		if none, err := PreviousReport(f, Config{}, po1, "po-9", monday); err != nil || none.Found {
			t.Fatalf("previous of someone who never reported = %+v, %v", none, err)
		}
	})
	// The path is the one the person's status issue names; in place the
	// issue names no branch, pop makes it, and centrally the issue's branch
	// (GH-2777).
	cfg := Config{}
	cfg.defaults()
	sp := PathOf("po-1", "", monday, false)
	body := InPlaceStatusBody(cfg, "po-1", po1, monday, monday.AddDate(0, 0, 3))
	if sp.Path != "status/reports/2026-Q4/2026-W42-po-1.md" || !strings.Contains(body, "write "+sp.Path+" ") || sp.Branch != "" {
		t.Errorf("in-place report path = %+v; want the issue's path and no branch", sp)
	}
	sp = PathOf("po-1", "", monday, true)
	body = StatusBody(cfg, Member{"po-1", po1}, "g/reports", monday)
	if sp.Path != "reports/2026-Q4/2026-W42-po-1.md" || !strings.Contains(body, "write "+sp.Path+" ") || !strings.Contains(body, "branch "+sp.Branch+",") {
		t.Errorf("central report path = %+v; want the issue's path and branch", sp)
	}
	if rp := PathOf("", ProgramLevel, monday, false); rp.Path != "status/rollups/2026-Q4/2026-W42-program.md" || rp.Branch != "" {
		t.Errorf("rollup path = %+v", rp)
	}
}

// A rollup validates when its front matter, file name, and sections agree
// and every claim cites a source it lists by repository and commit; a claim
// citing an unlisted source, or none, fails (GH-2725).
func TestRollupFormat(t *testing.T) {
	sources := []Entry{{"repository": po1, "path": "status/reports/2026-Q4/2026-W42-po-1.md", "commit": "abc123"}}
	cite := Cite(po1, "abc123")
	good := RenderRollup(RollupFrontMatter{Level: ProgramLevel, Author: "tpm", Period: "2026-W41..2026-W42"},
		[]Entry{{"headline": "the grader scores one task", "cites": cite}},
		[]Entry{{"sprint_goal": po1 + "#1", "component": "po-1", "state": "at-risk", "evidence": "stale scores", "cites": cite}},
		[]Entry{{"kind": "ask", "text": "a fix to the scoring API", "owner": "platform", "cites": cite}},
		[]string{"score a second task"}, sources)
	if problems := CheckFile("status/rollups/2026-Q4/2026-W42-program.md", []byte(good)); len(problems) != 0 {
		t.Fatalf("a good rollup: %v\n%s", problems, good)
	}
	bad := strings.Replace(good, "cites: "+cite+"\n", "cites: "+po2+"@def456\n", 1)
	bad = strings.Replace(bad, "evidence: stale scores; cites: "+cite, "evidence: stale scores; cites: ", 1)
	problems := CheckFile("status/rollups/2026-Q4/2026-W41-program.md", []byte(bad))
	for _, want := range []string{"file name is 2026-W41-program.md", "progress 1: no cites", "headline 1 cites " + po2 + "@def456, which Sources does not list"} {
		if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, want) }) {
			t.Errorf("a bad rollup's problems %v lack %q", problems, want)
		}
	}
	if problems := CheckFile("status/rollups/2026-Q4/2026-W42-x.md", []byte("---\nlevel: team\nauthor: a\nperiod: 2026-W42\n---\n")); !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, "names its subsystem") }) {
		t.Errorf("a team rollup without its subsystem: %v", problems)
	}
}

// The roster's due offsets and rollup titles default, and a roster read
// from the system repository names it (GH-2725).
func TestControlRoster(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		inPlaceProgram(p)
		cfg, err := LoadControl(p.Tracker(), "g/system", "main")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.System != "g/system" || len(cfg.Team) != 3 || cfg.ReportDueDays != 1 || cfg.TeamRollupDueDays != 1 || cfg.ProgramRollupDueDays != 1 ||
			cfg.RollupTitle("srd010-po-1", "2026-W42") != "Recurring: team rollup srd010-po-1 2026-W42" || cfg.RollupTitle("", "2026-W42") != "Recurring: program rollup 2026-W42" {
			t.Fatalf("roster = %+v", cfg)
		}
	})
	if d := AddWorkingDays(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), 1); d.Format(time.DateOnly) != "2026-10-19" {
		t.Errorf("a working day after a Friday = %s, want the Monday", d.Format(time.DateOnly))
	}
}

func mustWeek(w string) time.Time {
	d, err := ParseWeek(w)
	if err != nil {
		panic(err)
	}
	return d
}

// The rollup format's example is a valid rollup (GH-2725).
func TestRollupFormatExampleValidates(t *testing.T) {
	b, err := os.ReadFile("../../docs/specs/config-formats/rollup.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(b), "<!-- field: example -->")
	_, rest, _ = strings.Cut(rest, "```yaml\n|\n")
	block, _, _ := strings.Cut(rest, "\n```")
	var lines []string
	for _, l := range strings.Split(block, "\n") {
		lines = append(lines, strings.TrimPrefix(l, "    "))
	}
	if problems := CheckFile("status/rollups/2026-Q4/2026-W42-program.md", []byte(strings.Join(lines, "\n")+"\n")); len(problems) != 0 {
		t.Fatalf("the example breaks the format: %v", problems)
	}
}

// A program whose roster is in the system repository gets a site home with
// no reports in it: init lays down the README, the site's front page, and
// the deck template, and no reports.md (GH-2725).
func TestInitWithoutARosterLaysDownTheSiteOnly(t *testing.T) {
	dir := t.TempDir()
	res, err := Init(dir, Config{}, monday, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range res {
		got = append(got, filepath.ToSlash(r.Path)+" "+r.Action)
	}
	if strings.Join(got, ", ") != "deck/template.md created, README.md created, site/index.md created" {
		t.Fatalf("init = %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err == nil {
		t.Error("init wrote reports.md for a program whose roster is in the system repository")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); !strings.Contains(string(b), "control/reports.md") {
		t.Errorf("the README does not say where reports live:\n%s", b)
	}
}

// The newest program rollup in a checkout reads as the aggregate states its
// progress, so plan's candidates read the lead's rollup as they read the
// aggregate: a Sprint Goal at risk is neither met nor complete (GH-2725).
func TestLatestProgramRollupReadsAsAnAggregate(t *testing.T) {
	dir := t.TempDir()
	if LatestProgramRollup(dir) != "" {
		t.Fatal("a checkout with no rollup has one")
	}
	src := []Entry{{"repository": po1, "path": "p", "commit": "c"}}
	for week, state := range map[string]string{"2026-W41": "met", "2026-W42": "at-risk"} {
		p := filepath.Join(dir, filepath.FromSlash(RollupPath(mustWeek(week), ProgramLevel)))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(RenderRollup(RollupFrontMatter{Level: ProgramLevel, Author: "tpm", Period: week}, nil,
			[]Entry{{"sprint_goal": po1 + "#1", "component": "po-1", "state": state, "evidence": "e", "cites": Cite(po1, "c")}}, nil, nil, src)), 0o644)
	}
	latest := LatestProgramRollup(dir)
	if !strings.HasSuffix(latest, "2026-W42-program.md") {
		t.Fatalf("latest = %s", latest)
	}
	b, _ := os.ReadFile(latest)
	r, err := ParseRollup(latest, b)
	if err != nil {
		t.Fatal(err)
	}
	a := r.Aggregation()
	if len(a.Progress) != 1 || a.Progress[0].SprintGoal != po1+"#1" || a.Progress[0].State != "at-risk" || a.Progress[0].Met || a.Progress[0].Complete {
		t.Fatalf("progress = %+v", a.Progress)
	}
}

// A team that names a scrum repository apart from its code journals there:
// the roster's member listed in its code repository reports in the scrum
// repository, its team rollup is filed there, and a team collection reads
// the scrum repository with the code (srd018; GH-2738).
func TestATeamJournalsInItsScrumRepository(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		const scrum = "g/po-2-scrum"
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd001-system.md", subsystemSRD("srd001-system", "tpm", "g/system"))
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", subsystemSRD("srd010-po-1", "po-1", po1))
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd011-po-2.md",
			[]byte(strings.Replace(string(subsystemSRD("srd011-po-2", "po-2", po2)), "- owner: po-2", "- owner: po-2\n- scrum_repository: https://gitlab.example.com/"+scrum, 1)))
		doc, _ := configDocument(Config{Team: []Member{{"po-1", po1}, {"po-2", po2}}})
		p.AddFile("g/system", "main", ControlFile, []byte(doc))
		w42, _ := ParseWeek("2026-W42")
		p.AddFile(scrum, "main", InPlaceReportPath(w42, "po-2"), []byte(Render("po-2", "2026-W42", nil, nil)))
		f := p.Tracker()
		cfg, err := LoadControl(f, "g/system", "main")
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.Team[1]; got.Project != scrum {
			t.Fatalf("po-2 reports in %s, want %s", got.Project, scrum)
		}
		if got := cfg.Team[0]; got.Project != po1 {
			t.Errorf("po-1, whose team names no scrum repository, reports in %s", got.Project)
		}
		res, err := StatusIssuesInPlace(f, RollupInput{Config: cfg, System: "g/system", Ref: "main", Policy: program.Defaults(), Lead: "tpm", Now: monday})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range res {
			got = append(got, r.Kind+" "+r.User+" "+r.Project)
		}
		for _, want := range []string{"status po-2 " + scrum, "team-rollup po-2 " + scrum, "team-rollup po-1 " + po1} {
			if !slices.Contains(got, want) {
				t.Errorf("status-issues filed %v, lacking %q", got, want)
			}
		}
		col, err := Collect(f, CollectInput{Config: cfg, System: "g/system", Ref: "main", Level: "team", Subsystem: "srd011-po-2", From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(col.Repositories, []string{scrum, po2}) || len(col.Reports) != 1 || col.Reports[0].Repository != scrum {
			t.Errorf("the team collection read %v and %+v", col.Repositories, col.Reports)
		}
	})
}

// An upper program reads a sub-program at its interface: its program
// rollup is its team rollup, named for its subsystem, none of its reports
// are read, nobody in it is missing a report here, and status-issues asks
// its TPM for no second rollup (srd018; GH-2742).
func TestASubProgramsRollupIsItsTeamRollup(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		const sub = "g/eval-program"
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd001-system.md", subsystemSRD("srd001-system", "tpm", "g/system"))
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", subsystemSRD("srd010-po-1", "po-1", po1))
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd012-eval.md",
			[]byte(strings.Replace(string(subsystemSRD("srd012-eval", "tpm-eval", "g/eval-code")), "- owner: tpm-eval", "- owner: tpm-eval\n- scrum_repository: https://gitlab.example.com/"+sub, 1)))
		doc, _ := configDocument(Config{Team: []Member{{"po-1", po1}, {"tpm-eval", sub}}})
		p.AddFile("g/system", "main", ControlFile, []byte(doc))
		p.AddFile(sub, "main", "control/trackers.md", []byte("---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n"))
		w42, _ := ParseWeek("2026-W42")
		p.AddFile(po1, "main", RollupPath(w42, "srd010-po-1"), []byte(RenderRollup(RollupFrontMatter{Level: "team", Subsystem: "srd010-po-1", Author: "po-1", Period: "2026-W42"}, nil, nil, nil, nil, nil)))
		p.AddFile(sub, "main", RollupPath(w42, ProgramLevel), []byte(RenderRollup(RollupFrontMatter{Level: "program", Author: "tpm-eval", Period: "2026-W42"}, nil, nil, nil, nil, nil)))
		p.AddFile(sub, "main", InPlaceReportPath(w42, "tpm-eval"), []byte(Render("tpm-eval", "2026-W42", nil, nil)))
		f := p.Tracker()
		cfg, err := LoadControl(f, "g/system", "main")
		if err != nil {
			t.Fatal(err)
		}
		col, err := Collect(f, CollectInput{Config: cfg, System: "g/system", Ref: "main", Level: ProgramLevel, From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		var rollups []string
		for _, r := range col.Rollups {
			rollups = append(rollups, r.Repository+" "+r.Name)
		}
		if !slices.Equal(rollups, []string{sub + " srd012-eval", po1 + " srd010-po-1"}) {
			t.Errorf("rollups = %v, want po-1's team rollup and the sub-program's program rollup as srd012-eval's", rollups)
		}
		for _, r := range col.Reports {
			if r.Repository == sub {
				t.Errorf("the upper program read the sub-program's report %s", r.Path)
			}
		}
		if !slices.Equal(col.SubPrograms, []string{sub}) {
			t.Errorf("sub-programs = %v", col.SubPrograms)
		}
		agg, err := Aggregate(f, AggregateInput{Config: cfg, Policy: program.Defaults(), From: "2026-W42", To: "2026-W42", Collection: &col, Now: monday})
		if err != nil {
			t.Fatal(err)
		}
		missing := map[string]bool{}
		for _, s := range agg.Signals {
			if s.Kind == SignalMissingReport {
				missing[s.Subject] = true
			}
		}
		if missing[sub] || !missing[po1] {
			t.Errorf("missing reports = %v, want po-1's and not the sub-program's", missing)
		}
		res, err := StatusIssuesInPlace(f, RollupInput{Config: cfg, System: "g/system", Ref: "main", Policy: program.Defaults(), Lead: "tpm", Now: monday})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range res {
			if r.Kind == "team-rollup" {
				got = append(got, r.Action+" "+r.Project)
			}
		}
		if !slices.Equal(got, []string{"filed " + po1, "sub-program " + sub}) {
			t.Errorf("team rollups = %v, want one filed for po-1 and none for the sub-program", got)
		}
	})
}
