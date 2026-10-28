// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// centralCheckout is a central reports repository's checkout naming
// g/system, with po-1's 2026-W42 report.
func centralCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{Team: []Member{{"po-1", po1}}, System: "g/system"}
	doc, err := configDocument(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	w42, _ := ParseWeek("2026-W42")
	writeReport(t, dir, ReportPath(w42, "po-1"), Render("po-1", "2026-W42", []Entry{work(po1+"#1", "po-1", "at-risk", "no")}, nil))
	return dir
}

// Every word that reads reports chooses them by one rule: in place when
// the roster in the system repository names a reporter, else the central
// checkout it was given, and it says which and why (GH-2775).
func TestChooseSourceIsOneRule(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		central := centralCheckout(t)
		inPlaceProgram(p)
		// init writes the roster into every system repository: one that
		// names nobody leaves the central checkout the source.
		empty, _ := configDocument(Config{})
		p.AddFile("g/empty", "main", ControlFile, []byte(empty))
		f := p.Tracker()
		s, err := ChooseSource(f, "g/system", "main", central)
		if err != nil || !s.InPlace || len(s.Config.Team) != 3 || !strings.Contains(s.Reason, "names 3 reporter(s)") || !strings.Contains(s.Reason, "are not read") {
			t.Fatalf("a roster naming reporters, beside a central checkout = %+v, %v; want in place, saying the central reports are not read", s, err)
		}
		s, err = ChooseSource(f, "g/empty", "main", central)
		if err != nil || s.InPlace || s.Central != central || !strings.Contains(s.Reason, "names no reporter") {
			t.Fatalf("an empty roster beside a central checkout = %+v, %v; want central", s, err)
		}
		if s, err = ChooseSource(f, "g/none", "main", central); err != nil || s.InPlace {
			t.Fatalf("no roster beside a central checkout = %+v, %v; want central", s, err)
		}
		if s, err = ChooseSource(f, "g/empty", "main", t.TempDir()); err != nil || !s.InPlace || !strings.Contains(s.Reason, "no central reports checkout") {
			t.Fatalf("an empty roster and a directory that is no reports checkout = %+v, %v; want in place, saying so", s, err)
		}
		if _, err = ChooseSource(f, "", "main", ""); err == nil {
			t.Fatal("no system and no central checkout chose a source")
		}
	})
}

// A source in place that holds no report for the period, while the central
// checkout the word was given holds some, says so rather than leaving
// every reporter missing in silence (GH-2775).
func TestExplainNamesReportsTheOtherSourceHolds(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		central := centralCheckout(t)
		inPlaceProgram(p)
		lone, _ := configDocument(Config{Team: []Member{{"po-2", po2}}})
		p.AddFile("g/lone", "main", ControlFile, []byte(lone))
		f := p.Tracker()
		s, err := ChooseSource(f, "g/system", "main", central)
		if err != nil {
			t.Fatal(err)
		}
		col, err := Collect(f, CollectInput{Config: s.Config, System: "g/system", Ref: "main", Level: ProgramLevel, From: "2026-W43", To: "2026-W43"})
		if err != nil {
			t.Fatal(err)
		}
		// 2026-W43: nothing in place; the central checkout holds W42 only.
		in := AggregateInput{Config: s.Config, From: "2026-W43", To: "2026-W43", Collection: &col}
		a, err := Aggregate(f, in)
		if err != nil {
			t.Fatal(err)
		}
		in.Collection = nil
		s.Explain(f, &a, in)
		if a.Source != s.Reason {
			t.Fatalf("a week neither source holds: source %q, want only the reason", a.Source)
		}
		col, err = Collect(f, CollectInput{Config: s.Config, System: "g/system", Ref: "main", Level: ProgramLevel, From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		in = AggregateInput{Config: s.Config, From: "2026-W42", To: "2026-W42", Collection: &col}
		if a, err = Aggregate(f, in); err != nil || a.Reports == 0 {
			t.Fatalf("2026-W42 in place = %+v, %v", a, err)
		}
		in.Collection = nil
		s.Explain(f, &a, in)
		if strings.Contains(a.Source, "move them into place") {
			t.Fatalf("reports in place: source %q names the central ones", a.Source)
		}
		// A roster naming only someone who wrote nothing in place: the
		// central checkout's W42 report is named.
		if s, err = ChooseSource(f, "g/lone", "main", central); err != nil || !s.InPlace {
			t.Fatalf("a roster naming po-2 = %+v, %v", s, err)
		}
		col, err = Collect(f, CollectInput{Config: s.Config, System: "g/lone", Ref: "main", Level: ProgramLevel, From: "2026-W42", To: "2026-W42"})
		if err != nil {
			t.Fatal(err)
		}
		in = AggregateInput{Config: s.Config, From: "2026-W42", To: "2026-W42", Collection: &col}
		if a, err = Aggregate(f, in); err != nil || a.Reports != 0 {
			t.Fatalf("po-2's 2026-W42 in place = %+v, %v; want none", a, err)
		}
		in.Collection = nil
		s.Explain(f, &a, in)
		if !strings.Contains(a.Source, "hold 1: move them into place") {
			t.Fatalf("nothing in place, one report central: source %q, want it named", a.Source)
		}
	})
}

// aggregate without --control finds the system repository from a reports
// checkout that is the site's home only, or from the system checkout, as
// the other words do, and a central checkout's reports.md still names it
// first (GH-2791).
func TestAggregateFindsTheSystemWithoutControl(t *testing.T) {
	root := t.TempDir()
	system, site := filepath.Join(root, "system"), filepath.Join(root, "reports")
	os.MkdirAll(filepath.Join(system, "control"), 0o755)
	os.WriteFile(filepath.Join(system, "control", "trackers.md"), []byte("---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n"), 0o644)
	os.MkdirAll(filepath.Join(site, "site"), 0o755)
	for dir, origin := range map[string]string{system: "https://gitlab.example.com/g/system.git", site: "https://gitlab.example.com/g/reports.git"} {
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", origin}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
	}
	for _, dir := range []string{site, system} {
		if got, err := aggregateSystem("", dir); err != nil || got != "g/system" {
			t.Errorf("aggregate in %s without --control = %q, %v; want g/system", filepath.Base(dir), got, err)
		}
	}
	if got, _ := aggregateSystem("g/other", site); got != "g/other" {
		t.Errorf("--control g/other = %q", got)
	}
	central := centralCheckout(t)
	if got, _ := aggregateSystem("", central); got != "g/system" {
		t.Errorf("a central checkout naming g/system = %q", got)
	}
}

// A reports checkout without reports.md, given while the roster names
// reporters, is named as not read (GH-2791).
func TestChooseSourceNamesACheckoutItDoesNotRead(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		inPlaceProgram(p)
		f := p.Tracker()
		site := t.TempDir()
		s, err := ChooseSource(f, "g/system", "main", site)
		if err != nil || !s.InPlace || !strings.Contains(s.Reason, site+" holds no reports.md, so it is no central reports checkout and is not read") {
			t.Fatalf("a site-only checkout beside a roster = %+v, %v", s, err)
		}
		if s, _ = ChooseSource(f, "g/system", "main", ""); strings.Contains(s.Reason, "not read") {
			t.Errorf("no checkout given: %q names one", s.Reason)
		}
	})
}

// A reports checkout given as "." is named by its absolute path (GH-2799).
func TestChooseSourceNamesACheckoutByItsPath(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		inPlaceProgram(p)
		f := p.Tracker()
		site, _ := filepath.EvalSymlinks(t.TempDir())
		t.Chdir(site)
		s, err := ChooseSource(f, "g/system", "main", ".")
		if err != nil || !strings.Contains(s.Reason, "; "+site+" holds no reports.md") || strings.Contains(s.Reason, "; . holds") {
			t.Fatalf(`"." beside a roster = %q, %v; want it named %s`, s.Reason, err, site)
		}
	})
}
