// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.WeeklyStatus, Integration{}.Reports)
}

// weeklyEnv is what both release 01.1 targets share: the system, reports,
// and po-1 repositories, the rig user, and a reports clone scaffolded by
// scrum-master reports init.
type weeklyEnv struct {
	*rigEnv
	system, reports, po1, user, clone string
}

func newWeeklyEnv(cloneName string) (*weeklyEnv, error) {
	r, err := newRigEnv()
	if err != nil {
		return nil, err
	}
	e := &weeklyEnv{rigEnv: r, user: r.user}
	for key, into := range map[string]*string{"system": &e.system, "reports": &e.reports, "product-owners/po-1": &e.po1} {
		if *into, err = r.project(key); err != nil {
			return nil, err
		}
	}
	po2, err := r.project("product-owners/po-2")
	if err != nil {
		return nil, err
	}
	if err := r.seedSystem(e.system, e.po1, po2); err != nil {
		return nil, err
	}
	if _, err := r.runWords("issue", "labels", "--group", r.group); err != nil {
		return nil, err
	}
	if e.clone, err = r.clone(e.reports, cloneName); err != nil {
		return nil, err
	}
	// The reports repository is laid down by its workflow, with the agent
	// (GH-37).
	rel, err := repoRuntime()
	if err != nil {
		return nil, err
	}
	out, err := r.runWorkflow(rel, "reports-init", e.user, e.clone, e.reports, "", map[string]any{"members": e.user + "=" + e.po1}, "SCRUM_MASTER_SYSTEM="+e.system)
	if err != nil || !strings.Contains(out, "final machine state: Done") {
		return e, fmt.Errorf("the reports-init run: %v: %s", err, tail17(out))
	}
	// The skills reports-init installs run vendored profiles (GH-68).
	if _, err := os.Stat(filepath.Join(e.clone, ".scrum-master", "manifest.json")); err != nil {
		return e, fmt.Errorf("the reports-init run vendored no profiles: %v", err)
	}
	return e, nil
}

// objectiveIssue files an open objective issue in project, as tasks
// would, and returns its reference.
func (e *weeklyEnv) objectiveIssue(project, title string) (string, int, error) {
	i, err := e.tr.CreateIssue(project, tracker.NewIssue{Title: title,
		Description: "An objective.\n\n<!-- scrum-master goals objective=obj001-reliable-agents:R1 release=" + rigRelease + " -->",
		DueDate:     time.Now().AddDate(0, 0, 21).Format(time.DateOnly)})
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%s#%d", project, i.IID), i.IID, nil
}

// WeeklyStatus files a status issue for the rig user, works it into a validated report merged in the reports repository with scrum-master issue merge, and proves the merge closes it and the next week's run recurs it with the next due date (test-rel01.1-weekly-status).
func (Integration) WeeklyStatus() error {
	e, err := newWeeklyEnv("reports-weekly")
	if err != nil {
		return err
	}
	c := &check{}
	objective, _, err := e.objectiveIssue(e.po1, "Score one agent's trajectories")
	if err != nil {
		return err
	}

	var filed []reports.StatusResult
	if err := e.wordsJSON(&filed, "reports", "status-issues", "--dir", e.clone, "--reports", e.reports, "--ref", seedBranch); err != nil {
		return err
	}
	c.expect(len(filed) == 1 && filed[0].Action == "filed" && filed[0].Admitted, "status-issues: %+v", filed)
	if len(filed) != 1 {
		return c.err("weeklyStatus")
	}
	status := filed[0]
	issue, err := e.issue(e.po1, status.IID)
	if err != nil {
		return err
	}
	c.expect(issue.DueDate == status.DueDate, "due date %s, want %s", issue.DueDate, status.DueDate)
	c.expect(len(issue.Assignees) == 0 && strings.Contains(issue.Description, "@"+e.user), "assignees %d; mention present %v", len(issue.Assignees), strings.Contains(issue.Description, "@"+e.user))
	c.expect(strings.Contains(issue.Description, "## Recurrence") && slices.Contains(issue.Labels, "type::recurring"), "recurrence section or label missing")
	var again []reports.StatusResult
	if err := e.wordsJSON(&again, "reports", "status-issues", "--dir", e.clone, "--reports", e.reports, "--ref", seedBranch); err != nil {
		return err
	}
	c.expect(len(again) == 1 && again[0].Action == "exists", "second run: %+v", again)

	var week reports.WeekResult
	if err := e.wordsJSON(&week, "reports", "week", "--project", e.po1, "--user", e.user); err != nil {
		return err
	}
	found := false
	for _, o := range week.SprintGoals {
		found = found || o.Ref == objective
	}
	c.expect(found, "week objectives %+v lack %s", week.SprintGoals, objective)

	start, err := reports.ParseWeek(status.Week)
	if err != nil {
		return err
	}
	path := reports.ReportPath(start, e.user)
	branch := status.Week + "-" + e.user
	report := reports.Render(e.user, status.Week, []reports.Entry{{"objective": objective, "component": "po-1", "status": "at-risk", "complete": "no", "problems": "stale scoring results"}}, nil)
	if err := e.git(e.clone, "switch", "--quiet", "-c", branch); err != nil {
		return err
	}
	writeFileTo(filepath.Join(e.clone, path), strings.Replace(report, "- po-1", "- wrong", 1))
	_, err = e.runWords("reports", "validate", "--dir", e.clone, path)
	c.expect(err != nil && strings.Contains(err.Error(), "break the report format"), "validate of a mismatched report: %v", err)
	writeFileTo(filepath.Join(e.clone, path), report)
	if _, err := e.runWords("reports", "validate", "--dir", e.clone, path); err != nil {
		return err
	}
	if err := e.commitPush(e.clone, "Weekly status "+status.Week, path); err != nil {
		return err
	}
	if err := e.approve(e.reports, e.po1, status.IID, branch); err != nil {
		return err
	}
	var merged program.MergeResult
	if err := e.wordsJSON(&merged, "issue", "merge", "--project", e.reports, "--issue-project", e.po1,
		"--issue", fmt.Sprint(status.IID), "--branch", branch, "--dir", e.clone); err != nil {
		return err
	}
	c.expect(merged.Merged, "merge: %+v", merged)
	c.expect(e.closed(e.po1, status.IID), "status issue #%d still open after the report merged", status.IID)

	var next []reports.StatusResult
	nextDay := start.AddDate(0, 0, 7).Format("2006-01-02")
	if err := e.wordsJSON(&next, "reports", "status-issues", "--dir", e.clone, "--reports", e.reports, "--ref", seedBranch, "--date", nextDay); err != nil {
		return err
	}
	c.expect(len(next) == 1 && next[0].Action == "recurred", "next week: %+v", next)
	if len(next) == 1 {
		n, err := e.issue(e.po1, next[0].IID)
		if err != nil {
			return err
		}
		want := start.AddDate(0, 0, 7)
		c.expect(n.DueDate == next[0].DueDate && next[0].Week == reports.ISOWeek(want), "next instance due %s week %s", n.DueDate, next[0].Week)
		c.expect(strings.Contains(n.Description, fmt.Sprintf("%s#%d (due %s): closed by its merge.", e.po1, status.IID, status.DueDate)), "next instance lacks the run in Previous Runs")
	}
	// The program manager's status issues as a workflow, with the agent
	// (GH-37): a third week's, filed once.
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	thirdDay := start.AddDate(0, 0, 14).Format("2006-01-02")
	out, serr := e.runWorkflow(rel, "status-issues", e.user, e.clone, e.reports, "", map[string]any{"ref": seedBranch, "date": thirdDay},
		"SCRUM_MASTER_REPORTS="+e.reports)
	c.expect(serr == nil && strings.Contains(out, "final machine state: Done"), "the status-issues run: %v: %s", serr, tail17(out))
	return c.err("weeklyStatus")
}

// Reports aggregates two weeks of fixture reports against the rig's tracker and proves every signal, progress per objective, and that --push files an actionable signal once (test-rel01.1-weekly-status).
func (Integration) Reports() error {
	e, err := newWeeklyEnv("reports-aggregate")
	if err != nil {
		return err
	}
	c := &check{}
	po2, _ := e.project("product-owners/po-2")
	blocked, _, err := e.objectiveIssue(e.po1, "Grader for one agent")
	if err != nil {
		return err
	}
	claimed, _, err := e.objectiveIssue(e.po1, "Grader notes")
	if err != nil {
		return err
	}
	met, metIID, err := e.objectiveIssue(po2, "Fleet rollout")
	if err != nil {
		return err
	}
	if err := e.tr.CreateNote(po2, metIID, "Met.\n\n<!-- scrum-master program event=met objective=obj001 -->"); err != nil {
		return err
	}
	if err := e.mergedWork(e.po1, "Unreported cache fix", "Closes #999"); err != nil {
		return err
	}

	thisWeek := reports.WeekStart(time.Now())
	lastWeek := thisWeek.AddDate(0, 0, -7)
	since := lastWeek.AddDate(0, 0, -30).Format("2006-01-02")
	put := func(user string, week time.Time, work, blockers []reports.Entry) {
		writeFileTo(filepath.Join(e.clone, reports.ReportPath(week, user)), reports.Render(user, reports.ISOWeek(week), work, blockers))
	}
	w := func(obj, comp, status, complete string) reports.Entry {
		return reports.Entry{"objective": obj, "component": comp, "status": status, "complete": complete, "problems": "none"}
	}
	b := func(what, comp, waiting string) reports.Entry {
		return reports.Entry{"blocker": what, "component": comp, "since": since, "waiting-on": waiting}
	}
	put(e.user, lastWeek, []reports.Entry{w(blocked, "po-1", "at-risk", "no")}, []reports.Entry{b("stale scores", "po-1", "scoring-api")})
	put(e.user, thisWeek, []reports.Entry{w(blocked, "po-1", "blocked", "no"), w(claimed, "po-1", "done", "yes")}, []reports.Entry{b("stale scores", "po-1", "scoring-api")})
	put("fixture-two", thisWeek, []reports.Entry{w(met, "po-2", "done", "yes")}, []reports.Entry{b("no quota", "po-2", "scoring-api")})

	from, to := reports.ISOWeek(lastWeek), reports.ISOWeek(thisWeek)
	var a reports.Aggregation
	if err := e.wordsJSON(&a, "reports", "aggregate", "--dir", e.clone, "--from", from, "--to", to, "--ref", seedBranch); err != nil {
		return err
	}
	kinds := map[string]int{}
	for _, s := range a.Signals {
		kinds[s.Kind]++
		c.expect(!strings.Contains(s.Detail, e.user) && !strings.Contains(s.Detail, "fixture-two"), "signal names a person: %s", s.Detail)
	}
	for _, k := range []string{reports.SignalSharedBlocker, reports.SignalAgingBlocker, reports.SignalSlipped, reports.SignalContradiction, reports.SignalUnreportedWork} {
		c.expect(kinds[k] > 0, "no %s signal: %v", k, kinds)
	}
	progress := map[string]string{}
	for _, p := range a.Progress {
		progress[p.SprintGoal] = p.State
	}
	c.expect(progress[met] == "met" && progress[claimed] == "reported complete", "progress %v", progress)

	var pushed reports.Aggregation
	if err := e.wordsJSON(&pushed, "reports", "aggregate", "--dir", e.clone, "--from", from, "--to", to, "--ref", seedBranch, "--push"); err != nil {
		return err
	}
	filed := 0
	for _, p := range pushed.Pushed {
		if p.Action == "filed" {
			filed++
		}
	}
	c.expect(filed > 0, "--push filed nothing: %+v", pushed.Pushed)
	var repeat reports.Aggregation
	if err := e.wordsJSON(&repeat, "reports", "aggregate", "--dir", e.clone, "--from", from, "--to", to, "--ref", seedBranch, "--push"); err != nil {
		return err
	}
	for _, p := range repeat.Pushed {
		c.expect(p.Action == "exists", "a second --push filed %s again", p.Key)
	}
	// The program manager's aggregate as a workflow, with the agent (GH-37).
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	out, aerr := e.runWorkflow(rel, "aggregate", e.user, e.clone, e.reports, "", map[string]any{"from": from, "to": to, "ref": seedBranch})
	c.expect(aerr == nil && strings.Contains(out, "final machine state: Done"), "the aggregate run: %v: %s", aerr, tail17(out))
	return c.err("reports")
}

// mergedWork merges a one-file change into project's main with a merge
// commit whose message carries closes, as work merged without a status
// report would land.
func (e *weeklyEnv) mergedWork(project, title, closes string) error {
	clone, err := e.rigEnv.clone(project, path.Base(project)+"-work")
	if err != nil {
		return err
	}
	branch := "work-" + strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	writeFileTo(filepath.Join(clone, branch+".md"), title+"\n")
	for _, args := range [][]string{{"fetch", "-q", "origin"}, {"checkout", "-q", "-B", branch, "origin/main"}, {"add", "-A"}, {"commit", "-q", "-m", title},
		{"checkout", "-q", "-B", "main", "origin/main"}, {"merge", "-q", "--no-ff", branch, "-m", title + "\n\n" + closes}, {"push", "-q", "origin", "main"}} {
		if err := e.git(clone, args...); err != nil {
			return err
		}
	}
	return nil
}
