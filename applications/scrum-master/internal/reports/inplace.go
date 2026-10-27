// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// ReportDue is the day a report is due in the ISO week starting at
// weekStart: days after the reminder weekday, so the tracker's due-date
// notification arrives on it (srd018 R9.2).
func ReportDue(weekStart time.Time, reminder time.Weekday, days int) time.Time {
	return DueDate(weekStart, reminder).AddDate(0, 0, max(days, 1)-1)
}

// AddWorkingDays is the day n working days after d, Saturday and Sunday
// skipped.
func AddWorkingDays(d time.Time, n int) time.Time {
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n--
		}
	}
	return d
}

// RollupInput is one status-issues run against the roster the system
// repository holds (GH-2725).
type RollupInput struct {
	Config Config
	System string
	Ref    string // ref of the system repository's control files and subsystem SRDs
	Policy program.Policy
	// Lead is who the program rollup is for when the roster names no
	// program_lead: the person running status-issues.
	Lead   string
	Now    time.Time
	DryRun bool
}

// StatusIssuesInPlace leaves, for the coming week, one status issue per
// listed person in the repository they report in, one team rollup issue
// per subsystem in its scrum repository, where its Sprint Goals are, for
// its owner (GH-2738), unless no listed person reports in its repositories
// (GH-2779), and one program rollup issue in
// the system repository, each due after the last; it files nothing twice
// (GH-2725).
func StatusIssuesInPlace(t tracker.Tracker, in RollupInput) ([]StatusResult, error) {
	cfg := in.Config
	reminder, err := Weekday(cfg.ReminderWeekday)
	if err != nil {
		return nil, err
	}
	start := TargetWeek(in.Now, reminder)
	week := ISOWeek(start)
	reportDue := ReportDue(start, reminder, cfg.ReportDueDays)
	teamDue := AddWorkingDays(reportDue, cfg.TeamRollupDueDays)
	programDue := AddWorkingDays(teamDue, cfg.ProgramRollupDueDays)
	base := ensureInput{System: in.System, Ref: in.Ref, Policy: in.Policy, Now: in.Now, DryRun: in.DryRun}
	var out []StatusResult
	people := append([]Member(nil), cfg.Team...)
	sort.SliceStable(people, func(a, b int) bool { return people[a].Username < people[b].Username })
	for _, m := range people {
		project := program.ProjectPath(m.Project)
		r := StatusResult{Kind: "status", User: m.Username, Project: project, Week: week, DueDate: reportDue.Format(time.DateOnly)}
		e := base
		e.Project, e.Title = project, cfg.Title(week)
		e.Body = InPlaceStatusBody(cfg, m.Username, project, start, reportDue)
		e.Mine = func(desc string) (string, bool) {
			mm := statusPattern.FindStringSubmatch(desc)
			if mm == nil || mm[2] != m.Username {
				return "", false
			}
			return mm[1], true
		}
		if err := ensureRecurring(t, &r, e); err != nil {
			return out, err
		}
		out = append(out, r)
	}
	subs, err := program.Teams(t, in.System, in.Ref)
	if err != nil {
		return out, err
	}
	rollup := func(kind, level, owner, project string, due time.Time) error {
		r := StatusResult{Kind: kind, User: owner, Project: project, Week: week, DueDate: due.Format(time.DateOnly)}
		e := base
		subsystem := level
		if level == ProgramLevel {
			subsystem = ""
		}
		e.Project, e.Title = project, cfg.RollupTitle(subsystem, week)
		e.Body = RollupBody(level, owner, project, start, due)
		e.Mine = func(desc string) (string, bool) {
			mm := rollupPattern.FindStringSubmatch(desc)
			if mm == nil || mm[2] != level {
				return "", false
			}
			return mm[1], true
		}
		if err := ensureRecurring(t, &r, e); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	}
	for _, sub := range subs {
		// A sub-program's own program rollup is its team rollup here, so
		// its TPM is asked for no second one (GH-2742).
		if program.IsProgram(t, sub.ScrumRepository()) {
			out = append(out, StatusResult{Kind: "team-rollup", User: sub.Owner, Project: sub.ScrumRepository(), Week: week, Action: "sub-program",
				Reasons: []string{sub.ScrumRepository() + " is a program: its program rollup stands for " + sub.ID + "'s team rollup"}})
			continue
		}
		// A team no person on the roster reports in has nothing to roll up,
		// so it is asked for no rollup (GH-2779).
		if !slices.ContainsFunc(cfg.Team, func(m Member) bool { return slices.Contains(sub.Repositories(), program.ProjectPath(m.Project)) }) {
			out = append(out, StatusResult{Kind: "team-rollup", User: sub.Owner, Project: sub.ScrumRepository(), Week: week, Action: "no-reporters",
				Reasons: []string{"no person on the roster reports in " + strings.Join(sub.Repositories(), ", ") + ", so " + sub.ID + " has nothing to roll up"}})
			continue
		}
		if err := rollup("team-rollup", sub.ID, sub.Owner, sub.ScrumRepository(), teamDue); err != nil {
			return out, err
		}
	}
	lead := cfg.ProgramLead
	if lead == "" {
		lead = in.Lead
	}
	if err := rollup("program-rollup", ProgramLevel, lead, in.System, programDue); err != nil {
		return out, err
	}
	return out, nil
}

// InPlaceStatusBody is a person's status issue: the report is written and
// merged in this repository, the one they worked in.
func InPlaceStatusBody(cfg Config, user, project string, weekStart, due time.Time) string {
	reminder, _ := Weekday(cfg.ReminderWeekday)
	week := ISOWeek(weekStart)
	p := InPlaceReportPath(weekStart, user)
	return fmt.Sprintf(`@%[1]s: your weekly status for %[2]s, due %[3]s.

%[4]s

## Requirements

1. Work this issue with the status-report skill in %[5]s: it gathers your week (reports week), what you committed to last week (reports previous), and where the report goes (reports path), asks you only what the tracker cannot know, and writes the report once you approve it.
2. Without the skill: take this issue with the pop skill, write %[6]s in its worktree in the report format: front matter, then the sections Work, Blockers, Impact, and Next, each as the reports-validate skill checks it, and fix what that skill names.
3. Commit and push; when the program's policy requires approval, ask a reviewer to approve it with the approve skill. Merge it with the merge skill; the merge closes this issue.

## Acceptance Criteria

- %[6]s in %[5]s covers every open Sprint Goal of %[5]s and passes the reports-validate skill's check (test: reports_validate; environment: local)

## Recurrence

Weekly, due %[7]s. After its merge closes it, the scheduled run of the status-issues skill opens the next week's instance.

## Previous Runs

- none
`, user, week, due.Format(time.DateOnly), statusMarker(week, user), project, p, dueRule(cfg.ReportDueDays, reminder))
}

func dueRule(days int, reminder time.Weekday) string {
	if days <= 1 {
		return fmt.Sprintf("the day after %s", reminder)
	}
	return fmt.Sprintf("%d days after %s", days, reminder)
}

// RollupBody is a rollup issue: a team's, level its subsystem, or the
// program's, written and merged in project.
func RollupBody(level, owner, project string, weekStart, due time.Time) string {
	week := ISOWeek(weekStart)
	p := RollupPath(weekStart, level)
	what, reads := "the "+level+" team rollup", "the week's merged reports of the subsystem's repositories (reports collect --level team)"
	if level == ProgramLevel {
		what, reads = "the program rollup", "the teams' rollups and the system repository's own reports (reports collect --level program), opening a raw report only to cite a signal"
	}
	mention := ""
	if owner != "" {
		mention = "@" + owner + ": "
	}
	return fmt.Sprintf(`%[1]s%[2]s for %[3]s, due %[4]s.

%[5]s

## Requirements

1. Work this issue with the status-rollup skill in %[6]s: it collects %[7]s, aggregates them into signals (reports aggregate), proposes a disposition for each signal and asks you only for judgment, and writes the rollup once you approve it.
2. Without the skill: take this issue with the pop skill, write %[8]s in its worktree in the rollup format (config format rollup), every claim citing the report or rollup it rests on by repository and commit, and check it with the reports-validate skill.
3. Commit and push; when the program's policy requires approval, ask a reviewer to approve it with the approve skill. Merge it with the merge skill; the merge closes this issue.

## Acceptance Criteria

- %[8]s in %[6]s passes the reports-validate skill's check, every claim citing its source by repository and commit (test: reports_validate; environment: local)

## Recurrence

Weekly, due the working day after the rollups it reads. After its merge closes it, the scheduled run of the status-issues skill opens the next week's instance.

## Previous Runs

- none
`, mention, what, week, due.Format(time.DateOnly), rollupMarker(week, level), project, reads, p)
}

// Previous is a person's last report before a week: what they said was
// blocking them and what they committed to next, which this week's report
// answers (GH-2725).
type Previous struct {
	Found      bool     `json:"found"`
	Repository string   `json:"repository,omitempty"`
	Path       string   `json:"path,omitempty"`
	Commit     string   `json:"commit,omitempty"`
	Week       string   `json:"week,omitempty"`
	Work       []Entry  `json:"work,omitempty"`
	Blockers   []Entry  `json:"blockers,omitempty"`
	Impact     []string `json:"impact,omitempty"`
	Next       []string `json:"next,omitempty"`
}

// PreviousReport finds user's latest report in project before day's week,
// looking back through this quarter and the last, and in the central
// reports repository the roster still names.
func PreviousReport(t tracker.Tracker, cfg Config, project, user string, day time.Time) (Previous, error) {
	week := ISOWeek(day)
	start := WeekStart(day)
	quarters := []string{Quarter(start.AddDate(0, -3, 0)), Quarter(start)}
	var candidates []Collected
	for _, q := range quarters {
		got, reason := readStatus(t, project, path.Join(StatusDir, "reports", q))
		if reason != "" {
			return Previous{}, fmt.Errorf("%s: %s", project, reason)
		}
		candidates = append(candidates, got...)
		if cfg.Reports != "" {
			got, _ := readStatus(t, cfg.Reports, path.Join("reports", q))
			for i := range got {
				got[i].Source = SourceCentral
			}
			candidates = append(candidates, got...)
		}
	}
	var best *Collected
	for i := range candidates {
		name, w, ok := statusName(candidates[i].Path)
		if !ok || name != user || w >= week || (best != nil && w <= best.Week) {
			continue
		}
		candidates[i].Name, candidates[i].Week = name, w
		best = &candidates[i]
	}
	if best == nil {
		return Previous{Found: false}, nil
	}
	r, err := ParseReport(best.Path, []byte(best.Content))
	if err != nil {
		return Previous{}, err
	}
	return Previous{Found: true, Repository: best.Repository, Path: best.Path, Commit: best.Commit, Week: best.Week,
		Work: r.Work, Blockers: r.Blockers, Impact: r.Impact, Next: r.Next}, nil
}

// StatusPath is where a status file for a week goes, and, for a central
// reports repository, the branch its issue names. In place the issue names
// no branch: pop makes it, <iid>-<slug> (srd013 R7; GH-2777).
type StatusPath struct {
	Path    string `json:"path"`
	Week    string `json:"week"`
	Quarter string `json:"quarter"`
	Branch  string `json:"branch,omitempty"`
}

// PathOf is the path a person's status issue names for day's week: in
// place, or, central, in a central reports repository on the branch its
// issue names (StatusBody); with level set, the rollup's, a team's named
// for its subsystem, the program's as program.
func PathOf(user, level string, day time.Time, central bool) StatusPath {
	start := WeekStart(day)
	week := ISOWeek(start)
	switch {
	case level != "":
		return StatusPath{Path: RollupPath(start, level), Week: week, Quarter: Quarter(start)}
	case central:
		return StatusPath{Path: ReportPath(start, user), Week: week, Quarter: Quarter(start), Branch: week + "-" + user}
	}
	return StatusPath{Path: InPlaceReportPath(start, user), Week: week, Quarter: Quarter(start)}
}
