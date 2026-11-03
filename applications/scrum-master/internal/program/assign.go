// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// AssignResult is what scrum-master issue assign or unassign did.
type AssignResult struct {
	Ref      string   `json:"ref"`
	Assignee string   `json:"assignee,omitempty"`
	Previous []string `json:"previous,omitempty"`
	Action   string   `json:"action"`
	DryRun   bool     `json:"dry_run,omitempty"`
}

// workStarted reports whether an assigned issue's work has started: the
// tracking system's own record where it keeps one, else the issue's pop
// branch in project. A branch named for an earlier title is not found.
func workStarted(t tracker.Tracker, project, issueProject string, i tracker.Issue) (bool, error) {
	if ws, ok := tracker.For(t, issueProject).(tracker.WorkState); ok {
		return ws.Started(issueProject, i.IID)
	}
	return t.BranchExists(project, fmt.Sprintf("%d-%s", i.IID, Slug(i.Title)))
}

// AssignIssue makes person the issue's assignee and leaves it for them to
// pop (srd041 R9.1). Any person may, until work has started: then it refuses
// and names the assignee, who gives it back first, so nobody loses work in
// progress unknowingly. An issue given back may have a branch; the next
// assignee's pop reuses it. project holds the branch and issueProject the
// issue. A tracker that accepted the write without assigning them, as GitHub
// does for someone it cannot assign, is an error.
func AssignIssue(t tracker.Tracker, project, issueProject string, iid int, person string, dryRun bool) (AssignResult, error) {
	r := AssignResult{Ref: fmt.Sprintf("%s#%d", issueProject, iid), Assignee: person, Action: "unchanged", DryRun: dryRun}
	i, err := t.GetIssue(issueProject, iid)
	if err != nil {
		return r, err
	}
	if i.State == "closed" {
		return r, fmt.Errorf("%s is closed", r.Ref)
	}
	r.Previous = i.Assignees
	if slices.Equal(i.Assignees, []string{person}) {
		return r, nil
	}
	if len(i.Assignees) > 0 {
		started, err := workStarted(t, project, issueProject, i)
		if err != nil {
			return r, err
		}
		if started {
			return r, fmt.Errorf("%s has work started by %s; they give it back with scrum-master issue unassign first", r.Ref, strings.Join(i.Assignees, ", "))
		}
	}
	r.Action = "assigned"
	if dryRun {
		return r, nil
	}
	if err := t.SetAssignee(issueProject, iid, person); err != nil {
		return r, err
	}
	got, err := t.GetIssue(issueProject, iid)
	if err != nil {
		return r, err
	}
	if !slices.Contains(got.Assignees, person) {
		return r, fmt.Errorf("the tracker did not assign %s to %s; check that they can be assigned in %s", person, r.Ref, issueProject)
	}
	return r, nil
}

// UnassignIssue gives the issue back: the person running it, its assignee,
// leaves it, and an issue they had started is ready again (srd041 R9.2).
func UnassignIssue(t tracker.Tracker, project string, iid int, dryRun bool) (AssignResult, error) {
	r := AssignResult{Ref: fmt.Sprintf("%s#%d", project, iid), Action: "unchanged", DryRun: dryRun}
	i, err := t.GetIssue(project, iid)
	if err != nil {
		return r, err
	}
	r.Previous = i.Assignees
	if len(i.Assignees) == 0 {
		return r, nil
	}
	me, err := tracker.UserIn(t, project)
	if err != nil {
		return r, err
	}
	if !slices.Equal(i.Assignees, []string{me.Username}) {
		return r, fmt.Errorf("%s is assigned to %s; only that person gives it back", r.Ref, strings.Join(i.Assignees, ", "))
	}
	r.Action = "unassigned"
	if dryRun {
		return r, nil
	}
	return r, t.SetAssignee(project, iid, "")
}

func init() {
	var a struct {
		project, issueProject, person string
		iid                           int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "assign", Summary: "assign an issue to someone, for them to pop; agents never run it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.project, "project", "", "project holding the issue's branch (required)")
			fs.IntVar(&a.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&a.person, "person", "", "the tracker username to assign (required)")
			fs.StringVar(&a.issueProject, "issue-project", "", "project holding the issue, when not --project")
		},
		Run: func(ctx *cli.Context) error {
			if a.project == "" || a.iid == 0 || a.person == "" {
				return fmt.Errorf("%w: assign needs --project, --issue, and --person", cli.ErrUsage)
			}
			if strings.ContainsAny(a.person, " \t/\\") {
				return fmt.Errorf("%w: %q is not a username", cli.ErrUsage, a.person)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			issueProject := a.issueProject
			if issueProject == "" {
				issueProject = a.project
			}
			r, err := AssignIssue(t, a.project, issueProject, a.iid, a.person, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, assignLine(r, a.person))
		},
	})

	var u struct {
		project string
		iid     int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "unassign", Summary: "give back an issue assigned to you, so it can be popped or assigned again",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&u.project, "project", "", "project holding the issue (required)")
			fs.IntVar(&u.iid, "issue", 0, "issue IID (required)")
		},
		Run: func(ctx *cli.Context) error {
			if u.project == "" || u.iid == 0 {
				return fmt.Errorf("%w: unassign needs --project and --issue", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := UnassignIssue(t, u.project, u.iid, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, assignLine(r, "no one"))
		},
	})
}

func assignLine(r AssignResult, who string) string {
	action := r.Action
	if r.DryRun && action != "unchanged" {
		action = "would be " + action
	}
	return fmt.Sprintf("%s assigned to %s: %s\n", r.Ref, who, action)
}
