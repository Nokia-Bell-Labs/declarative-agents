// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"flag"
	"fmt"
	"path"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// LinkResult is what scrum-master issue link did.
type LinkResult struct {
	Ref        string     `json:"ref"`
	Implements string     `json:"implements"`
	Action     string     `json:"action"`
	Lint       LintResult `json:"lint"`
	DryRun     bool       `json:"dry_run,omitempty"`
}

// LinkIssue writes the Implements line naming target, a story or an epic of
// kind, on a filed issue and lints it again (srd007 R6.6). It refuses a target
// of another kind or one its repository does not hold, and an issue already
// naming something else unless force says to replace it.
func LinkIssue(t tracker.Tracker, project string, iid int, kind, target string, force bool, p Policy, dryRun bool) (LinkResult, error) {
	r := LinkResult{Ref: fmt.Sprintf("%s#%d", project, iid), Implements: target, Action: "unchanged", DryRun: dryRun}
	if err := links.CheckTarget(links.Link{Kind: links.Implements, Target: target}); err != nil {
		return r, err
	}
	if got := links.ImplementsKind(target); got != kind {
		return r, fmt.Errorf("%s names %s, not %s", target, article(got), article(kind))
	}
	if err := documentHeld(t, project, kind, target); err != nil {
		return r, err
	}
	i, err := t.GetIssue(project, iid)
	if err != nil {
		return r, err
	}
	for _, l := range links.ParseLinks(i.Description) {
		if l.Kind == links.Implements && l.Target != target && !force {
			return r, fmt.Errorf("%s already implements %s; --force replaces it", r.Ref, l.Target)
		}
	}
	if body := withLine(i.Description, links.Implements, target); body != i.Description {
		r.Action = "linked"
		i.Description = body
		if !dryRun {
			if i, err = t.UpdateIssue(project, iid, tracker.IssueUpdate{Description: &body}); err != nil {
				return r, err
			}
		}
	}
	r.Lint, err = Apply(t, i, Lint(i, p), p, dryRun)
	return r, err
}

func article(kind string) string {
	if kind == "epic" {
		return "an epic"
	}
	return "a story"
}

// documentHeld refuses a target its repository does not hold: a pinned URL's
// at its tag, a bare id's in the issue's own repository's default branch.
func documentHeld(t tracker.Tracker, project, kind, target string) error {
	holder, file, ref := project, "", ""
	if strings.Contains(target, "://") {
		r, err := links.Parse(target)
		if err != nil {
			return err
		}
		holder, file, ref = r.Project, r.Path, r.Tag
	} else {
		branch, err := t.DefaultBranch(project)
		if err != nil {
			return err
		}
		folder := "stories"
		if kind == "epic" {
			folder = "epics"
		}
		file, ref = path.Join("docs/specs", folder, target+".md"), branch
	}
	if _, err := t.GetFile(holder, file, ref); errors.Is(err, tracker.ErrNotFound) {
		return fmt.Errorf("%s holds no %s %s at %s", holder, kind, file, ref)
	} else if err != nil {
		return err
	}
	return nil
}

// withLine is the description with one line of key, naming value: the first
// one rewritten in place and any other dropped, else a new line at its end;
// an empty value drops them all.
func withLine(description, key, value string) string {
	line := key + ": " + value
	var out []string
	found := false
	for _, l := range strings.Split(description, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), key+":") {
			out = append(out, l)
		} else if !found && value != "" {
			out, found = append(out, line), true
		}
	}
	if found || value == "" {
		return strings.Join(out, "\n")
	}
	if strings.TrimSpace(description) == "" {
		return line + "\n"
	}
	return strings.TrimRight(description, "\n") + "\n\n" + line + "\n"
}

func init() {
	var a struct {
		project, story, epic, control, ref string
		iid                                int
		force                              bool
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "link", FromCheckout: true, Summary: "name the story or epic a filed issue implements, and lint it again",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.project, "project", "", "project path (required)")
			fs.IntVar(&a.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&a.epic, "epic", "", "the epic, by same-repository id or pinned URL")
			fs.StringVar(&a.story, "story", "", "the story, by same-repository id or pinned URL")
			fs.BoolVar(&a.force, "force", false, "replace the story or epic the issue already names")
			fs.StringVar(&a.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout")
			fs.StringVar(&a.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if a.project == "" || a.iid == 0 || (a.epic == "") == (a.story == "") {
				return fmt.Errorf("%w: link needs --project, --issue, and one of --epic or --story", cli.ErrUsage)
			}
			kind, target := "epic", a.epic
			if a.story != "" {
				kind, target = "story", a.story
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p, err := LoadPolicy(t, ControlFor(a.control, ".", ""), a.ref)
			if err != nil {
				return err
			}
			r, err := LinkIssue(t, a.project, a.iid, kind, target, a.force, p, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, fmt.Sprintf("%s implements %s: %s; %s", r.Ref, target, r.Action, verdict(r.Lint)))
		},
	})
}
