// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/standup"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// StatusLabels are the status issue's labels besides its component: a
// recurring data issue whose work is a review of the week (srd018 R6.1).
var StatusLabels = []string{"plane::data", "type::recurring", "kind::review"}

var statusPattern = regexp.MustCompile(`<!-- scrum-master reports status week=(\S+) user=(\S+) -->`)

func statusMarker(week, user string) string {
	return fmt.Sprintf("<!-- scrum-master reports status week=%s user=%s -->", week, user)
}

// DueDate is the day after the reminder weekday in the ISO week starting at
// weekStart, so the tracker's due-date notification arrives on the
// reminder weekday (srd018 R9.2).
func DueDate(weekStart time.Time, reminder time.Weekday) time.Time {
	day := int(reminder)
	if day == 0 {
		day = 7
	}
	return weekStart.AddDate(0, 0, day)
}

// TargetWeek is the start of the week a status issue filed now is for: this
// week until its due date has passed, then the next.
func TargetWeek(now time.Time, reminder time.Weekday) time.Time {
	start := WeekStart(now)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if day.After(DueDate(start, reminder)) {
		return start.AddDate(0, 0, 7)
	}
	return start
}

// StatusBody is the status issue's description: the mention, the guided
// script as Requirements, the report as Acceptance Criteria, the
// Recurrence section, and an empty Previous Runs (srd018 R9.2, R4.3).
func StatusBody(cfg Config, m Member, reportsProject string, weekStart time.Time) string {
	reminder, _ := Weekday(cfg.ReminderWeekday)
	week := ISOWeek(weekStart)
	path := ReportPath(weekStart, m.Username)
	branch := week + "-" + m.Username
	// The system repository holds no Sprint Goal of its own; its week takes
	// the period from the teams' (srd018 R8.1).
	control := ""
	if cfg.System != "" && m.Project == cfg.System {
		control = " --control " + cfg.System
	}
	return fmt.Sprintf(`@%[1]s: your weekly status for %[2]s, due %[3]s.

%[9]s

## Requirements

1. Run the status skill for %[4]s as %[1]s for this repository's open Sprint Goals and your activity this week.
2. Model step (structure): for each open Sprint Goal, ask and record its status (on-track, at-risk, blocked, or done), its problems, and whether it is complete; record what blocks the work.
3. Take this issue with the pop skill from the reports checkout, for project %[5]s with issue_project %[4]s and branch %[6]s, and in the worktree it opens write %[7]s in the report format (schema/report.md).
4. Check it with the reports-validate skill and fix what it reports.
5. Commit and push the branch; when the program's policy requires approval, ask a reviewer to approve it with the approve skill. Then merge it with the merge skill, with issue_project %[4]s and branch %[6]s; the merge closes this issue.

## Acceptance Criteria

- %[7]s in %[5]s covers every open Sprint Goal of %[4]s and passes the reports-validate skill's check (test: reports_validate; environment: local)

## Recurrence

Weekly, due the day after %[8]s. After its merge closes it, the reports repository's scheduled run of the status-issues skill opens the next week's instance.

## Previous Runs

- none
`, m.Username, week, DueDate(weekStart, reminder).Format("2006-01-02"), m.Project, reportsProject, branch, path, reminder, statusMarker(week, m.Username), control)
}

// StatusResult is what status-issues did for one person.
type StatusResult struct {
	User     string   `json:"user"`
	Project  string   `json:"project"`
	Week     string   `json:"week"`
	Action   string   `json:"action"` // exists, recurred, filed
	IID      int      `json:"iid,omitempty"`
	WebURL   string   `json:"web_url,omitempty"`
	DueDate  string   `json:"due_date"`
	Admitted bool     `json:"admitted"`
	Reasons  []string `json:"reasons,omitempty"`
}

// StatusInput is one status-issues run.
type StatusInput struct {
	Config  Config
	Reports string // the reports repository's project path
	Policy  program.Policy
	Ref     string // ref of the system repository's subsystem SRDs
	Now     time.Time
	DryRun  bool
}

// StatusIssues leaves one status issue per listed person for the target
// week: an existing one is kept, the person's latest status issue is
// recurred, and with none a fresh one is filed (srd018 R7).
func StatusIssues(t tracker.Tracker, in StatusInput) ([]StatusResult, error) {
	reminder, err := Weekday(in.Config.ReminderWeekday)
	if err != nil {
		return nil, err
	}
	start := TargetWeek(in.Now, reminder)
	week, due := ISOWeek(start), DueDate(start, reminder).Format("2006-01-02")
	var out []StatusResult
	for _, m := range in.Config.Team {
		r := StatusResult{User: m.Username, Project: m.Project, Week: week, DueDate: due}
		issues, err := t.ListIssues(m.Project, tracker.IssueQuery{State: "all", Labels: []string{"type::recurring"}})
		if err != nil {
			return out, err
		}
		var latest *tracker.Issue
		latestWeek := ""
		for k := range issues {
			mm := statusPattern.FindStringSubmatch(issues[k].Description)
			if mm == nil || mm[2] != m.Username {
				continue
			}
			if mm[1] == week {
				r.Action, r.IID, r.WebURL, r.Admitted = "exists", issues[k].IID, issues[k].WebURL, true
				break
			}
			if mm[1] > latestWeek {
				latest, latestWeek = &issues[k], mm[1]
			}
		}
		if r.Action == "" {
			body := StatusBody(in.Config, m, in.Reports, start)
			if latest != nil {
				rr, err := standup.Recur(t, m.Project, latest.IID, standup.RecurOptions{
					Title: in.Config.Title(week), DueDate: due, Body: body, Policy: in.Policy, Now: in.Now, DryRun: in.DryRun})
				if err != nil {
					return out, err
				}
				r.Action, r.IID, r.WebURL, r.Admitted = "recurred", rr.Next, rr.NextURL, rr.Admitted || in.DryRun
			} else {
				l, err := program.Derive(t, m.Project, in.Config.System, in.Ref, nil, program.Labelling{Plane: "data", Kind: "review"})
				if err != nil {
					return out, err
				}
				labels := append([]string{"type::recurring"}, program.Labels(l.Plane, "", l.Kind, l.Component)...)
				labels = append(labels, program.Op(program.OpStatus))
				fr, err := program.File(t, m.Project, tracker.NewIssue{Title: in.Config.Title(week), Description: body, Labels: labels, DueDate: due}, in.Policy, in.DryRun)
				if err != nil {
					return out, err
				}
				r.Action, r.IID, r.WebURL, r.Admitted, r.Reasons = "filed", fr.IID, fr.WebURL, fr.Lint.Admitted, fr.Lint.Reasons
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].User < out[b].User })
	return out, nil
}

func itoa(n int) string { return strconv.Itoa(n) }
