// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package reports keeps weekly status in a fixed, checkable shape and reads
// it back: the reports repository's scaffold, the report format, the weekly
// status issue, and aggregation by component (srd018).
package reports

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FrontMatter is a report's index, derived from its body (config format
// report).
type FrontMatter struct {
	Author     string   `yaml:"author" json:"author"`
	Sprint     string   `yaml:"sprint" json:"sprint"`
	Components []string `yaml:"components" json:"components"`
	BlockedOn  []string `yaml:"blocked-on" json:"blocked_on"`
}

// Entry is one list item of Work or Blockers: key: value pairs.
type Entry map[string]string

// Report is one parsed report file. Impact and Next, the plain list items
// of their optional sections, say why the week's work mattered and what
// the author commits to next; validate and aggregate read a report without
// them as before (GH-2725).
type Report struct {
	Path        string      `json:"path"`
	FrontMatter FrontMatter `json:"front_matter"`
	Work        []Entry     `json:"work"`
	Blockers    []Entry     `json:"blockers"`
	Impact      []string    `json:"impact,omitempty"`
	Next        []string    `json:"next,omitempty"`
	hasWork     bool
	hasBlockers bool
}

// Keys each entry must carry, and the values some keys allow.
var (
	WorkKeys    = []string{"sprint_goal", "component", "status", "complete", "problems"}
	BlockerKeys = []string{"blocker", "component", "since", "waiting-on"}
	Statuses    = []string{"on-track", "at-risk", "blocked", "done"}
)

// ISOWeek names t's ISO week, YYYY-Www.
func ISOWeek(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// Quarter names the quarter of t's ISO week, taken from its Thursday.
func Quarter(t time.Time) string {
	thursday := t.AddDate(0, 0, 4-isoWeekday(t))
	return fmt.Sprintf("%d-Q%d", thursday.Year(), (int(thursday.Month())-1)/3+1)
}

// WeekStart is the Monday 00:00 UTC of t's ISO week.
func WeekStart(t time.Time) time.Time {
	t = t.UTC()
	d := t.AddDate(0, 0, 1-isoWeekday(t))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func isoWeekday(t time.Time) int {
	if wd := int(t.Weekday()); wd != 0 {
		return wd
	}
	return 7
}

// ReportPath is where a person's report for t's week lives in a central
// reports repository.
func ReportPath(t time.Time, username string) string {
	return path.Join("reports", Quarter(t), ISOWeek(t)+"-"+username+".md")
}

// StatusDir holds a repository's status: the reports of the people who
// worked in it and its rollups, outside docs/ so the corpus gate reads
// none of it as intent (GH-2725).
const StatusDir = "status"

// InPlaceReportPath is where a person's report for t's week lives in the
// repository where they worked.
func InPlaceReportPath(t time.Time, username string) string {
	return path.Join(StatusDir, ReportPath(t, username))
}

// ProgramLevel names the program's rollup; a team's is named for its
// subsystem.
const ProgramLevel = "program"

// RollupPath is where the rollup for t's week lives: a team's under its
// subsystem's name, the program's as program.
func RollupPath(t time.Time, name string) string {
	return path.Join(StatusDir, "rollups", Quarter(t), ISOWeek(t)+"-"+name+".md")
}

var entryPattern = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)

// ParseReport reads a report's front matter and its Work and Blockers
// entries. It fails only when the text cannot be read as a report at all;
// Check says whether a readable report is valid.
func ParseReport(p string, src []byte) (Report, error) {
	r := Report{Path: p}
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return r, fmt.Errorf("%s: no front matter", p)
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return r, fmt.Errorf("%s: front matter is not closed", p)
	}
	if err := yaml.Unmarshal([]byte(head), &r.FrontMatter); err != nil {
		return r, fmt.Errorf("%s: front matter: %w", p, err)
	}
	var section *[]Entry
	var plain *[]string
	for _, line := range strings.Split(body, "\n") {
		switch strings.TrimSpace(line) {
		case "## Work":
			section, plain, r.hasWork = &r.Work, nil, true
			continue
		case "## Blockers":
			section, plain, r.hasBlockers = &r.Blockers, nil, true
			continue
		case "## Impact":
			section, plain = nil, &r.Impact
			continue
		case "## Next":
			section, plain = nil, &r.Next
			continue
		}
		if strings.HasPrefix(line, "## ") {
			section, plain = nil, nil
			continue
		}
		m := entryPattern.FindStringSubmatch(line)
		if m == nil || strings.TrimSpace(m[1]) == "none" {
			continue
		}
		if plain != nil {
			*plain = append(*plain, strings.TrimSpace(m[1]))
			continue
		}
		if section == nil {
			continue
		}
		e := parseEntry(m[1])
		if section == &r.Work {
			e = sprintGoalKey(e)
		}
		*section = append(*section, e)
	}
	return r, nil
}

// sprintGoalKey reads a work entry's objective key, its name in reports
// written before release 04.0, as sprint_goal (config format report).
func sprintGoalKey(e Entry) Entry {
	if v, ok := e["objective"]; ok {
		if _, has := e["sprint_goal"]; !has {
			e["sprint_goal"] = v
		}
		delete(e, "objective")
	}
	return e
}

func parseEntry(s string) Entry {
	e := Entry{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			e[strings.TrimSpace(part)] = ""
			continue
		}
		e[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return e
}

// Derive computes the front matter the body implies.
func (r Report) Derive() FrontMatter {
	fm := FrontMatter{Author: r.FrontMatter.Author, Sprint: r.FrontMatter.Sprint, Components: []string{}, BlockedOn: []string{}}
	add := func(list *[]string, v string) {
		if v != "" && !slices.Contains(*list, v) {
			*list = append(*list, v)
		}
	}
	for _, e := range r.Work {
		add(&fm.Components, e["component"])
	}
	for _, e := range r.Blockers {
		add(&fm.Components, e["component"])
		add(&fm.BlockedOn, e["waiting-on"])
	}
	slices.Sort(fm.Components)
	slices.Sort(fm.BlockedOn)
	return fm
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Check lists every way the report breaks the format (srd018 R2.1, R2.2).
// The file name must be <sprint>-<author>.md.
func (r Report) Check() []string {
	var problems []string
	fm := r.FrontMatter
	if want := fm.Sprint + "-" + fm.Author + ".md"; path.Base(r.Path) != want {
		problems = append(problems, fmt.Sprintf("file name is %s; the front matter says %s", path.Base(r.Path), want))
	}
	if !r.hasWork {
		problems = append(problems, "no ## Work section: every report has one, one entry per open Sprint Goal")
	}
	if !r.hasBlockers {
		problems = append(problems, "no ## Blockers section: every report has one, holding the single entry - none when nothing blocks")
	}
	for i, e := range r.Work {
		problems = append(problems, missing(fmt.Sprintf("work entry %d", i+1), e, WorkKeys)...)
		if v, ok := e["status"]; ok && !slices.Contains(Statuses, v) {
			problems = append(problems, fmt.Sprintf("work entry %d: status %q is not one of %s", i+1, v, strings.Join(Statuses, ", ")))
		}
		if v, ok := e["complete"]; ok && v != "yes" && v != "no" {
			problems = append(problems, fmt.Sprintf("work entry %d: complete %q is not yes or no", i+1, v))
		}
	}
	for i, e := range r.Blockers {
		problems = append(problems, missing(fmt.Sprintf("blocker %d", i+1), e, BlockerKeys)...)
		if v, ok := e["since"]; ok && !datePattern.MatchString(v) {
			problems = append(problems, fmt.Sprintf("blocker %d: since %q is not YYYY-MM-DD", i+1, v))
		}
	}
	want := r.Derive()
	if !slices.Equal(norm(fm.Components), want.Components) {
		problems = append(problems, fmt.Sprintf("front matter components %v disagree with the body %v", fm.Components, want.Components))
	}
	if !slices.Equal(norm(fm.BlockedOn), want.BlockedOn) {
		problems = append(problems, fmt.Sprintf("front matter blocked-on %v disagree with the body %v", fm.BlockedOn, want.BlockedOn))
	}
	return problems
}

func norm(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func missing(where string, e Entry, keys []string) []string {
	var out []string
	for _, k := range keys {
		if v, ok := e[k]; !ok || v == "" {
			out = append(out, fmt.Sprintf("%s: no %s", where, k))
		}
	}
	return out
}

// Render writes a report in the format, its front matter derived from the
// body, so what scrum-master writes always validates.
func Render(author, sprint string, work, blockers []Entry) string {
	return RenderWith(author, sprint, work, blockers, nil, nil)
}

// RenderWith is Render with the report's Impact and Next, each section
// written only when it holds an item.
func RenderWith(author, sprint string, work, blockers []Entry, impact, next []string) string {
	named := make([]Entry, len(work))
	for i, e := range work {
		named[i] = sprintGoalKey(maps.Clone(e))
	}
	work = named
	r := Report{FrontMatter: FrontMatter{Author: author, Sprint: sprint}, Work: work, Blockers: blockers}
	fm := r.Derive()
	head, _ := yaml.Marshal(fm)
	var b strings.Builder
	b.WriteString("---\n" + string(head) + "---\n## Work\n\n")
	for _, e := range work {
		b.WriteString("- " + entryText(e, WorkKeys) + "\n")
	}
	b.WriteString("\n## Blockers\n\n")
	if len(blockers) == 0 {
		b.WriteString("- none\n")
	}
	for _, e := range blockers {
		b.WriteString("- " + entryText(e, BlockerKeys) + "\n")
	}
	for _, s := range []struct {
		title string
		items []string
	}{{"Impact", impact}, {"Next", next}} {
		if len(s.items) == 0 {
			continue
		}
		b.WriteString("\n## " + s.title + "\n\n")
		for _, it := range s.items {
			b.WriteString("- " + it + "\n")
		}
	}
	return b.String()
}

func entryText(e Entry, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+e[k])
	}
	return strings.Join(parts, "; ")
}
