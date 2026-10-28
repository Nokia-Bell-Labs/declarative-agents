// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var monday = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) // ISO week 2026-W42

func TestWeekHelpers(t *testing.T) {
	if ISOWeek(monday) != "2026-W42" || Quarter(monday) != "2026-Q4" || ReportPath(monday, "po-1") != "reports/2026-Q4/2026-W42-po-1.md" {
		t.Fatalf("%s %s %s", ISOWeek(monday), Quarter(monday), ReportPath(monday, "po-1"))
	}
	if !WeekStart(monday.AddDate(0, 0, 6)).Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("week start of Sunday: %v", WeekStart(monday.AddDate(0, 0, 6)))
	}
	if q := Quarter(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)); q != "2026-Q4" {
		t.Fatalf("2026-12-31 is in 2026-W53: %s", q)
	}
}

func TestInitScaffoldsOnceAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Team: []Member{{"po-1", "g/po-1"}}, System: "g/system"}
	res, err := Init(dir, cfg, monday, false, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"reports.md", "reports/2026-Q4/.gitkeep", "schema/report.md", "summaries/.gitkeep", "deck/template.md", "README.md", "site/index.md"}
	for i, r := range res {
		if filepath.ToSlash(r.Path) != want[i] || r.Action != "created" {
			t.Fatalf("result %d: %+v", i, r)
		}
	}
	got, err := LoadConfig(dir)
	if err != nil || got.ReminderWeekday != "wednesday" || got.Team[0].Project != "g/po-1" || got.Title("2026-W42") != "Recurring: weekly status 2026-W42" {
		t.Fatalf("config %+v %v", got, err)
	}
	if _, findings, _ := Validate(dir, []string{"schema/report.md"}); len(findings) != 1 || !strings.Contains(findings[0].Problems[0], "file name") {
		t.Fatalf("the schema example validates except for its name: %+v", findings)
	}

	os.WriteFile(filepath.Join(dir, "reports.md"), []byte("---\nid: reports\ntitle: Reports\n---\n\n<!-- field: reminder_weekday -->\n## Reminder weekday\n\nfriday\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "summaries", "2026-Q4.md"), []byte("kept\n"), 0o644)
	res, err = Init(dir, Config{}, monday.AddDate(0, 3, 0), false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Action != "kept" && !strings.HasPrefix(filepath.ToSlash(r.Path), "reports/2027-Q1") {
			t.Fatalf("second init: %+v", r)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "reports.md")); !strings.Contains(string(b), "friday") {
		t.Fatal("init overwrote reports.md")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "summaries", "2026-Q4.md")); string(b) != "kept\n" {
		t.Fatal("init overwrote a summary")
	}
}

func writeReport(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(content), 0o644)
}

func TestValidatePassesPerObjectiveReport(t *testing.T) {
	dir := t.TempDir()
	report := Render("po-1", "2026-W42", []Entry{
		{"objective": "g/po-1#4", "component": "grader", "status": "at-risk", "complete": "no", "problems": "stale scoring results"},
		{"objective": "g/po-1#5", "component": "grader", "status": "done", "complete": "yes", "problems": "none"},
	}, []Entry{{"blocker": "stale scoring results", "component": "grader", "since": "2026-10-08", "waiting-on": "scoring-api"}})
	writeReport(t, dir, "reports/2026-Q4/2026-W42-po-1.md", report)
	writeReport(t, dir, "reports/2026-Q4/2026-W42-po-2.md", Render("po-2", "2026-W42", []Entry{
		{"objective": "g/po-2#1", "component": "fleet", "status": "on-track", "complete": "no", "problems": "none"}}, nil))
	checked, findings, err := Validate(dir, nil)
	if err != nil || checked != 2 || findings == nil || len(findings) != 0 {
		t.Fatalf("checked %d findings %+v err %v", checked, findings, err)
	}
	r, _ := ParseReport("x", []byte(report))
	if len(r.Work) != 2 || r.Work[0]["problems"] != "stale scoring results" || len(r.Blockers) != 1 {
		t.Fatalf("parsed %+v", r)
	}
}

func TestValidateFailsFrontMatterMismatch(t *testing.T) {
	dir := t.TempDir()
	good := Render("po-1", "2026-W42", []Entry{
		{"objective": "g/po-1#4", "component": "grader", "status": "blocked", "complete": "no", "problems": "waiting on scoring"}},
		[]Entry{{"blocker": "scoring", "component": "grader", "since": "2026-10-08", "waiting-on": "scoring-api"}})
	writeReport(t, dir, "reports/2026-Q4/2026-W42-po-1.md", strings.Replace(good, "- scoring-api", "- fleet", 1))
	writeReport(t, dir, "reports/2026-Q4/2026-W42-po-2.md", strings.Replace(Render("po-2", "2026-W42", []Entry{
		{"objective": "g/po-2#1", "component": "fleet", "status": "late", "complete": "maybe", "problems": "none"}}, nil), "## Blockers", "## Notes", 1))
	_, findings, err := Validate(dir, nil)
	if err != nil || len(findings) != 2 {
		t.Fatalf("findings %+v err %v", findings, err)
	}
	all := strings.Join(append(findings[0].Problems, findings[1].Problems...), "\n")
	for _, want := range []string{"blocked-on [fleet] disagree with the body [scoring-api]", "no ## Blockers section: every report has one, holding the single entry - none when nothing blocks", `status "late"`, `complete "maybe"`} {
		if !strings.Contains(all, want) {
			t.Errorf("no finding %q in:\n%s", want, all)
		}
	}
	if !strings.HasSuffix(findings[0].Path, "2026-W42-po-1.md") {
		t.Fatalf("finding names %s", findings[0].Path)
	}
}

// TestReportsConfigExampleDecodes holds the reports-config format to its
// example, and init's written configuration to the same reader.
func TestReportsConfigExampleDecodes(t *testing.T) {
	b, err := os.ReadFile("../../docs/specs/config-formats/reports-config.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(b), "<!-- field: config_file_layout -->")
	_, rest, _ = strings.Cut(rest, "```markdown\n")
	doc, _, _ := strings.Cut(rest, "\n```")
	c, err := ParseConfig([]byte(doc + "\n"))
	if err != nil || len(c.Team) == 0 || c.Team[0].Username == "" || c.ReminderWeekday == "" {
		t.Fatalf("example = %+v, %v", c, err)
	}
	written, err := configDocument(c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseConfig([]byte(written))
	if err != nil || !reflect.DeepEqual(again, c) {
		t.Fatalf("round trip = %+v, %v; want %+v", again, err, c)
	}
}

// TestInPlaceReportsHaveImpactAndNext validates a report written in place
// only with the sections its status issue asks for, Work, Blockers, Impact,
// and Next, naming each one missing, while a central repository's report
// may still omit Impact and Next (GH-2778).
func TestInPlaceReportsHaveImpactAndNext(t *testing.T) {
	w42, _ := ParseWeek("2026-W42")
	work := []Entry{work("g/po-1#4", "grader", "on-track", "no")}
	full := RenderWith("po-1", "2026-W42", work, nil, []string{"the grader scores a task"}, []string{"score a second task"})
	inPlace := InPlaceReportPath(w42, "po-1")
	if problems := CheckFile(inPlace, []byte(full)); len(problems) != 0 {
		t.Fatalf("an in-place report with every section = %v", problems)
	}
	for _, c := range []struct {
		name         string
		impact, next []string
	}{{"Impact", nil, []string{"x"}}, {"Next", []string{"x"}, nil}} {
		problems := CheckFile(inPlace, []byte(RenderWith("po-1", "2026-W42", work, nil, c.impact, c.next)))
		if len(problems) != 1 || !strings.Contains(problems[0], "no ## "+c.name+" section") {
			t.Errorf("an in-place report without %s = %v; want that section named", c.name, problems)
		}
	}
	// Named absolutely, as a profile's file parameter is, or through
	// --dir, the same report is held to the same sections (GH-2790).
	dir := t.TempDir()
	writeReport(t, dir, inPlace, RenderWith("po-1", "2026-W42", work, nil, []string{"x"}, nil))
	for _, paths := range [][]string{{filepath.Join(dir, inPlace)}, {inPlace}, nil} {
		_, findings, err := Validate(dir, paths)
		if err != nil || len(findings) != 1 || !strings.Contains(strings.Join(findings[0].Problems, " "), "no ## Next section") {
			t.Errorf("validate %v without Next = %+v, %v; want it refused", paths, findings, err)
		}
	}
	if problems := CheckFile(ReportPath(w42, "po-1"), []byte(Render("po-1", "2026-W42", work, nil))); len(problems) != 0 {
		t.Errorf("a central report without Impact and Next = %v; want valid, as before", problems)
	}
}
