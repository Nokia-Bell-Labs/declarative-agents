// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"os"
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
