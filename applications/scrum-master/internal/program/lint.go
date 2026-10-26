// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bufio"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// lintMarker heads every admission comment, so a repeated failure with the
// same reasons posts nothing new.
const lintMarker = "<!-- scrum-master program lint -->"

var (
	evidencePattern = regexp.MustCompile(`\(test:\s*[^;)]+;\s*environment:\s*[^)]+\)`)
	numberedEntry   = regexp.MustCompile(`^\d+\. `)
)

// Lint checks one issue against the label schema, admission policy, and its
// link lines (srd013 R1.1, srd007 R8.1) and returns every reason it fails;
// none means admitted. The named rules are the ones the policy declares
// (config format policy, admission.rules; all of them when it declares
// none), so a program disables one by removing its name; the link-line
// checks always apply. A program whose subsystem SRDs declare no component
// yet needs none on its issues (srd001 R21.2).
func Lint(i tracker.Issue, p Policy) []string {
	var reasons []string
	names := p.AdmissionRules
	if names == nil {
		names = RuleNames()
	}
	for _, name := range names {
		n := slices.IndexFunc(rules, func(r rule) bool { return r.name == name })
		if n < 0 {
			reasons = append(reasons, fmt.Sprintf("admission rule %q is not one scrum-master implements, so it cannot apply (config format policy)", name))
			continue
		}
		reasons = append(reasons, rules[n].check(i, p)...)
	}

	for _, c := range links.ParseCites(i.Description) {
		if err := links.CheckPinned(c.URL); err != nil {
			reasons = append(reasons, "Cites line is not pinned: "+err.Error())
		}
	}
	count := map[string]int{}
	for _, l := range links.ParseLinks(i.Description) {
		count[l.Kind]++
		if err := links.CheckTarget(l); err != nil {
			reasons = append(reasons, l.Kind+" line: "+err.Error())
		}
	}
	for _, k := range []string{links.Duplicates, links.Implements} {
		if count[k] > 1 {
			reasons = append(reasons, fmt.Sprintf("has %d %s lines, at most one", count[k], k))
		}
	}
	return reasons
}

// section returns the list entries under a "## name" heading and whether
// the heading exists.
func section(body, name string) ([]string, bool) {
	var entries []string
	in, found := false, false
	s := bufio.NewScanner(strings.NewReader(body))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(strings.TrimPrefix(line, "## ")) == name
			found = found || in
			continue
		}
		if in && (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || numberedEntry.MatchString(line)) {
			entries = append(entries, line)
		}
	}
	return entries, found
}

func shorten(s string) string {
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

// LintResult is what lint reports for one issue.
type LintResult struct {
	Project  string   `json:"project"`
	IID      int      `json:"iid"`
	Admitted bool     `json:"admitted"`
	Reasons  []string `json:"reasons,omitempty"`
	Changed  bool     `json:"changed"`
	// Labeled is the labels lint added: the component the issue's project
	// is declared as, when it had none (GH-67).
	Labeled []string `json:"labeled,omitempty"`
	// Milestone is the milestone the issue carries, which admission reads
	// for a data issue (GH-20).
	Milestone string `json:"milestone,omitempty"`
}

// lintNote is the comment lint leaves on a failing issue.
func lintNote(reasons []string) string {
	body := lintMarker + "\nAdmission failed:\n"
	for _, reason := range reasons {
		body += "- " + reason + "\n"
	}
	return body
}

// admittedNote is the comment lint leaves when an issue that failed
// admission passes, so the failure above it reads as resolved.
const admittedNote = lintMarker + "\nAdmitted: the issue now passes every admission check.\n"

// lastLint is the body of the latest admission comment in notes, or "".
func lastLint(notes []tracker.Note) string {
	last := ""
	for _, n := range notes {
		if strings.HasPrefix(strings.TrimSpace(n.Body), lintMarker) {
			last = strings.TrimSpace(n.Body)
		}
	}
	return last
}

// Apply sets needs-triage with a comment listing the reasons on a failing
// issue and clears it on a passing one, recording the admission under a
// failure comment. It changes no other label and never the assignee
// (srd013 R1.2, srd041 R9.4). A dry run writes nothing.
func Apply(t tracker.Tracker, i tracker.Issue, reasons []string, p Policy, dryRun bool) (LintResult, error) {
	r := LintResult{Project: i.Project, IID: i.IID, Admitted: len(reasons) == 0, Reasons: reasons, Milestone: i.Milestone}
	triaged := i.HasLabel(p.TriageLabel)
	triager, byState := tracker.For(t, i.Project).(tracker.Triager)
	if r.Admitted {
		var err error
		switch {
		case dryRun:
			r.Changed = triaged
			return r, nil
		case byState:
			r.Changed, err = triager.Triage(i.Project, i.IID, true)
		case triaged:
			r.Changed = true
			_, err = t.UpdateIssue(i.Project, i.IID, tracker.IssueUpdate{RemoveLabels: []string{p.TriageLabel}})
		}
		if err != nil || !r.Changed {
			return r, err
		}
		notes, err := t.ListNotes(i.Project, i.IID)
		if err != nil {
			return r, err
		}
		if strings.Contains(lastLint(notes), "Admission failed") {
			err = t.CreateNote(i.Project, i.IID, admittedNote)
		}
		return r, err
	}
	body := lintNote(reasons)
	notes, err := t.ListNotes(i.Project, i.IID)
	if err != nil {
		return r, err
	}
	posted := lastLint(notes) == strings.TrimSpace(body)
	r.Changed = !triaged || !posted
	if dryRun {
		return r, nil
	}
	if !triaged && byState {
		if _, err := triager.Triage(i.Project, i.IID, false); err != nil {
			return r, err
		}
	} else if !triaged {
		if _, err := t.UpdateIssue(i.Project, i.IID, tracker.IssueUpdate{AddLabels: []string{p.TriageLabel}}); err != nil {
			return r, err
		}
	}
	if !posted {
		return r, t.CreateNote(i.Project, i.IID, body)
	}
	return r, nil
}
