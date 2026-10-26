// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// RollupFrontMatter is a rollup's index (config format rollup): its level,
// a team's subsystem, its author, and the ISO weeks it covers, one week or
// first..last.
type RollupFrontMatter struct {
	Level     string `yaml:"level" json:"level"`
	Subsystem string `yaml:"subsystem,omitempty" json:"subsystem,omitempty"`
	Author    string `yaml:"author" json:"author"`
	Period    string `yaml:"period" json:"period"`
}

// Rollup is one parsed rollup: the lead's account of a team's or the
// program's period, every claim citing the report or rollup it rests on by
// repository and commit (GH-2725).
type Rollup struct {
	Path        string            `json:"path"`
	FrontMatter RollupFrontMatter `json:"front_matter"`
	Headlines   []Entry           `json:"headlines"`
	Progress    []Entry           `json:"progress"`
	Risks       []Entry           `json:"risks_and_asks"`
	Next        []string          `json:"next_period"`
	Sources     []Entry           `json:"sources"`
	sections    []string
}

// The rollup's sections, in order, and the keys each kind of entry carries.
var (
	RollupSections = []string{"Headlines", "Progress", "Risks and asks", "Next period", "Sources"}
	HeadlineKeys   = []string{"headline", "cites"}
	ProgressKeys   = []string{"sprint_goal", "component", "state", "evidence", "cites"}
	RiskKeys       = []string{"kind", "text", "owner", "cites"}
	SourceKeys     = []string{"repository", "path", "commit"}
	// ProgressStates are a Sprint Goal's ledger state (srd006) or, short
	// of one, its latest reported status; never a count of issues.
	ProgressStates = []string{"met", "reported complete", "on-track", "at-risk", "blocked", "done"}
	RiskKinds      = []string{"risk", "ask"}
	RollupLevels   = []string{"team", ProgramLevel}
)

// Cite names a source as a rollup cites it: repository@commit.
func Cite(repository, commit string) string { return repository + "@" + commit }

// ParseRollup reads a rollup's front matter and sections. It fails only
// when the text cannot be read as a rollup at all; Check says whether a
// readable rollup is valid.
func ParseRollup(p string, src []byte) (Rollup, error) {
	r := Rollup{Path: p}
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
	var entries *[]Entry
	var plain *[]string
	for _, line := range strings.Split(body, "\n") {
		if title, ok := strings.CutPrefix(strings.TrimSpace(line), "## "); ok {
			entries, plain = nil, nil
			r.sections = append(r.sections, title)
			switch title {
			case "Headlines":
				entries = &r.Headlines
			case "Progress":
				entries = &r.Progress
			case "Risks and asks":
				entries = &r.Risks
			case "Next period":
				plain = &r.Next
			case "Sources":
				entries = &r.Sources
			}
			continue
		}
		m := entryPattern.FindStringSubmatch(line)
		if m == nil || strings.TrimSpace(m[1]) == "none" {
			continue
		}
		switch {
		case plain != nil:
			*plain = append(*plain, strings.TrimSpace(m[1]))
		case entries != nil:
			*entries = append(*entries, parseEntry(m[1]))
		}
	}
	return r, nil
}

// Name is the rollup's name in its file name: the team's subsystem, or
// program.
func (r Rollup) Name() string {
	if r.FrontMatter.Level == ProgramLevel {
		return ProgramLevel
	}
	return r.FrontMatter.Subsystem
}

// LastWeek is the last ISO week the rollup covers.
func (r Rollup) LastWeek() string {
	_, last, ok := strings.Cut(r.FrontMatter.Period, "..")
	if !ok {
		return r.FrontMatter.Period
	}
	return last
}

// Cites lists every source an entry cites.
func Cites(e Entry) []string {
	var out []string
	for _, c := range strings.Split(e["cites"], ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// Check lists every way the rollup breaks the format: its front matter and
// file name agree, its sections are all there in order, every entry carries
// its keys, and every claim cites a source its Sources section lists by
// repository and commit (GH-2725).
func (r Rollup) Check() []string {
	var problems []string
	fm := r.FrontMatter
	if !slices.Contains(RollupLevels, fm.Level) {
		problems = append(problems, fmt.Sprintf("level %q is not team or program", fm.Level))
	}
	if fm.Level == "team" && fm.Subsystem == "" {
		problems = append(problems, "a team rollup names its subsystem")
	}
	if fm.Author == "" {
		problems = append(problems, "no author")
	}
	if fm.Period == "" {
		problems = append(problems, "no period")
	} else {
		for _, w := range strings.Split(fm.Period, "..") {
			if _, err := ParseWeek(w); err != nil {
				problems = append(problems, fmt.Sprintf("period %q: %v", fm.Period, err))
			}
		}
	}
	if want := r.LastWeek() + "-" + r.Name() + ".md"; path.Base(r.Path) != want {
		problems = append(problems, fmt.Sprintf("file name is %s; the front matter says %s", path.Base(r.Path), want))
	}
	var seen []string
	for _, s := range r.sections {
		if slices.Contains(RollupSections, s) {
			seen = append(seen, s)
		}
	}
	if !slices.Equal(seen, RollupSections) {
		problems = append(problems, fmt.Sprintf("sections are %s; a rollup has %s, in that order", strings.Join(seen, ", "), strings.Join(RollupSections, ", ")))
	}
	sources := map[string]bool{}
	for i, e := range r.Sources {
		problems = append(problems, missing(fmt.Sprintf("source %d", i+1), e, SourceKeys)...)
		sources[Cite(e["repository"], e["commit"])] = true
	}
	cited := func(where string, e Entry) {
		for _, c := range Cites(e) {
			if !sources[c] {
				problems = append(problems, fmt.Sprintf("%s cites %s, which Sources does not list", where, c))
			}
		}
	}
	for i, e := range r.Headlines {
		where := fmt.Sprintf("headline %d", i+1)
		problems = append(problems, missing(where, e, HeadlineKeys)...)
		cited(where, e)
	}
	for i, e := range r.Progress {
		where := fmt.Sprintf("progress %d", i+1)
		problems = append(problems, missing(where, e, ProgressKeys)...)
		if v, ok := e["state"]; ok && v != "" && !slices.Contains(ProgressStates, v) {
			problems = append(problems, fmt.Sprintf("%s: state %q is not one of %s", where, v, strings.Join(ProgressStates, ", ")))
		}
		cited(where, e)
	}
	for i, e := range r.Risks {
		where := fmt.Sprintf("risk or ask %d", i+1)
		problems = append(problems, missing(where, e, RiskKeys)...)
		if v, ok := e["kind"]; ok && v != "" && !slices.Contains(RiskKinds, v) {
			problems = append(problems, fmt.Sprintf("%s: kind %q is not risk or ask", where, v))
		}
		cited(where, e)
	}
	return problems
}

// RenderRollup writes a rollup in the format, so what scrum-master writes
// always validates when every claim cites a listed source.
func RenderRollup(fm RollupFrontMatter, headlines, progress, risks []Entry, next []string, sources []Entry) string {
	head, _ := yaml.Marshal(fm)
	var b strings.Builder
	b.WriteString("---\n" + string(head) + "---\n")
	section := func(title string, entries []Entry, keys []string) {
		b.WriteString("## " + title + "\n\n")
		if len(entries) == 0 {
			b.WriteString("- none\n")
		}
		for _, e := range entries {
			b.WriteString("- " + entryText(e, keys) + "\n")
		}
		b.WriteString("\n")
	}
	section("Headlines", headlines, HeadlineKeys)
	section("Progress", progress, ProgressKeys)
	section("Risks and asks", risks, RiskKeys)
	b.WriteString("## Next period\n\n")
	if len(next) == 0 {
		b.WriteString("- none\n")
	}
	for _, n := range next {
		b.WriteString("- " + n + "\n")
	}
	b.WriteString("\n")
	section("Sources", sources, SourceKeys)
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// IsRollupPath reports whether a status file is a rollup rather than a
// report.
func IsRollupPath(p string) bool {
	return strings.Contains("/"+p, "/"+StatusDir+"/rollups/")
}
