// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package standup holds the person's daily and recurring tracker steps
// (srd010). It implements scrum-master issue recur (srd013 R8), formerly scrum-master
// standup recur.
package standup

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// RecurOptions shape the next instance; zero values keep the issue's own.
type RecurOptions struct {
	Title   string // the next instance's title
	DueDate string // YYYY-MM-DD; empty is the issue's due date plus seven days
	// Body replaces the issue's body above its Previous Runs section.
	Body string
	// Summary is the run's result: it closes an open instance as its run
	// summary and follows the outcome on the Previous Runs line.
	Summary string
	// Policy admits the next instance, as scrum-master issue file admits an issue.
	Policy program.Policy
	Now    time.Time
	DryRun bool
}

// RecurResult is what recur reports.
type RecurResult struct {
	Project  string `json:"project"`
	Previous int    `json:"previous"`
	Closed   bool   `json:"closed"`   // recur closed the previous instance itself
	Existing bool   `json:"existing"` // the next instance already existed
	Next     int    `json:"next,omitempty"`
	NextURL  string `json:"next_url,omitempty"`
	Title    string `json:"title"`
	DueDate  string `json:"due_date"`
	// Admitted is whether the next instance passed admission.
	Admitted bool `json:"admitted"`
	DryRun   bool `json:"dry_run"`
}

// The marker keeps the text scrum-master standup recur wrote: it is tracker data that
// finds the next instance, including one an earlier release opened (srd013 R8.2).
var recursPattern = regexp.MustCompile(`\n*<!-- scrum-master standup recurs \S+ -->\n?`)

func recursMarker(project string, iid int) string {
	return fmt.Sprintf("<!-- scrum-master standup recurs %s#%d -->", project, iid)
}

// Recur opens the next instance of a recurring issue (srd013 R8.1, srd001
// R8.3): same labels and milestone, the next due date, and the run
// appended to Previous Runs, admitted under the policy as a filed issue is. An open issue is closed first with a run
// summary; one a merge closed is left as it is. When the next
// instance already exists, nothing is written.
func Recur(t tracker.Tracker, project string, iid int, o RecurOptions) (RecurResult, error) {
	r := RecurResult{Project: project, Previous: iid, DryRun: o.DryRun}
	i, err := t.GetIssue(project, iid)
	if err != nil {
		return r, err
	}
	r.Title = o.Title
	if r.Title == "" {
		r.Title = i.Title
	}
	r.DueDate = o.DueDate
	if r.DueDate == "" {
		if i.DueDate == "" {
			return r, fmt.Errorf("%s#%d has no due date; name the next one", project, iid)
		}
		d, err := time.Parse("2006-01-02", i.DueDate)
		if err != nil {
			return r, err
		}
		r.DueDate = d.AddDate(0, 0, 7).Format("2006-01-02")
	}

	marker := recursMarker(project, iid)
	all, err := t.ListIssues(project, tracker.IssueQuery{State: "all"})
	if err != nil {
		return r, err
	}
	for _, other := range all {
		if strings.Contains(other.Description, marker) {
			r.Existing, r.Next, r.NextURL = true, other.IID, other.WebURL
			return r, nil
		}
	}

	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	outcome := "closed by its merge"
	if i.State != "closed" {
		r.Closed = true
		outcome = "closed by the recur skill without its work"
	}
	summary := strings.Join(strings.Fields(o.Summary), " ")
	run := fmt.Sprintf("- %s#%d (due %s): %s.", project, iid, orNone(i.DueDate), outcome)
	if summary != "" {
		// The run's result is what the line records, the summary's own
		// sentence kept whole; closed by recur, the instance merged no work
		// of its own (GH-20).
		how := "Closed by its merge."
		if r.Closed {
			how = "Closed by the recur skill."
		}
		if !strings.HasSuffix(summary, ".") && !strings.HasSuffix(summary, "!") && !strings.HasSuffix(summary, "?") {
			summary += "."
		}
		run = fmt.Sprintf("- %s#%d (due %s): %s %s", project, iid, orNone(i.DueDate), summary, how)
	}
	body := nextBody(i.Description, o.Body, run) + "\n\n" + marker + "\n"
	if o.DryRun {
		return r, nil
	}
	if r.Closed {
		note := fmt.Sprintf("Run summary: closed on %s without a merge; the next instance is due %s.", now.Format("2006-01-02"), r.DueDate)
		if o.Summary != "" {
			note = "Run summary: " + strings.TrimSpace(o.Summary) + "\n\n" + fmt.Sprintf("Closed on %s without a merge; the next instance is due %s.", now.Format("2006-01-02"), r.DueDate)
		}
		if err := t.CreateNote(project, iid, note); err != nil {
			return r, err
		}
		// The reason says what the close was, without contradicting the
		// run's summary: recur closed it, and the summary says what ran
		// (GH-22).
		reason := "closed by the recur skill, without a merge; the next instance is due " + r.DueDate
		if o.Summary != "" {
			reason = "closed by the recur skill after the run its summary records, without a merge; the next instance is due " + r.DueDate
		}
		if _, err := t.UpdateIssue(project, iid, tracker.IssueUpdate{State: ptr("close"), CloseReason: &reason}); err != nil {
			return r, err
		}
	}
	// The next instance keeps the labels but one: it was filed by recur.
	var labels []string
	for _, l := range i.Labels {
		if !strings.HasPrefix(l, "op::") {
			labels = append(labels, l)
		}
	}
	labels = append(labels, program.Op(program.OpRecur))
	next, err := program.CreateLinted(t, project, tracker.NewIssue{Title: r.Title, Description: body, Labels: labels, DueDate: r.DueDate, Milestone: i.Milestone}, o.Policy)
	if err != nil {
		return r, err
	}
	r.Next, r.NextURL, r.Admitted = next.IID, next.WebURL, next.Lint.Admitted
	return r, nil
}

func ptr[T any](v T) *T { return &v }

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// nextBody is the base body (the replacement, or the old body above its
// Previous Runs) with the old runs and the new one under Previous Runs.
func nextBody(old, replacement, run string) string {
	old = recursPattern.ReplaceAllString(old, "\n")
	base, runs := splitRuns(old)
	if replacement != "" {
		base, _ = splitRuns(recursPattern.ReplaceAllString(replacement, "\n"))
	}
	var kept []string
	for _, l := range runs {
		if t := strings.TrimSpace(l); t != "" && t != "- none" && !strings.HasPrefix(t, "<!--") {
			kept = append(kept, t)
		}
	}
	kept = append(kept, run)
	return strings.TrimRight(base, "\n") + "\n\n## Previous Runs\n\n" + strings.Join(kept, "\n")
}

// splitRuns cuts a body at its Previous Runs heading: the text above and
// the lines of the section, which ends at the next heading.
func splitRuns(body string) (string, []string) {
	lines := strings.Split(body, "\n")
	for n, l := range lines {
		if strings.TrimSpace(l) != "## Previous Runs" {
			continue
		}
		var runs []string
		rest := n + 1
		for ; rest < len(lines) && !strings.HasPrefix(lines[rest], "## "); rest++ {
			runs = append(runs, lines[rest])
		}
		above := append(lines[:n:n], lines[rest:]...)
		return strings.Join(above, "\n"), runs
	}
	return body, nil
}
