// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package program is the scrum-master issue group: the issue operations every
// repository shares, admission and filing (srd013). Its former name,
// program, runs nothing and names the group (srd013 R2.2).
package program

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	cli.Default.Retire("program", "issue")
	var labelArgs struct {
		group, control, ref string
		update, migrate     bool
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "labels", Summary: "create the program's labels from control/labels.md or the defaults",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&labelArgs.group, "group", "", "program group path; defaults to the program group control/trackers.md records")
			fs.StringVar(&labelArgs.control, "control", "", "program repository holding control/labels.md; empty uses the defaults")
			fs.StringVar(&labelArgs.ref, "ref", "main", "ref of control/labels.md")
			fs.BoolVar(&labelArgs.update, "update", false, "also rewrite the color and description of labels that differ")
			fs.BoolVar(&labelArgs.migrate, "migrate", false, "also relabel issues carrying a renamed type value (type::epic to type::tracking) in --control and its subsystems' repositories")
		},
		Run: func(ctx *cli.Context) error {
			if labelArgs.group == "" {
				labelArgs.group = checkout.RecordedGroup(".")
			}
			if labelArgs.group == "" {
				return fmt.Errorf("%w: labels needs --group, as control/trackers.md records no program group", cli.ErrUsage)
			}
			if labelArgs.migrate && labelArgs.control == "" {
				return fmt.Errorf("%w: labels --migrate needs --control, the system repository", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			want, err := LoadLabels(t, labelArgs.control, labelArgs.ref)
			if err != nil {
				return err
			}
			if labelArgs.control != "" {
				components, err := ComponentLabels(t, labelArgs.control, labelArgs.ref)
				if err != nil {
					return err
				}
				want = append(want, components...)
			}
			res, err := SyncLabels(t, labelArgs.group, want, labelArgs.update, ctx.DryRun)
			if err != nil {
				return err
			}
			counts := map[string]int{}
			for _, r := range res {
				counts[r.Action]++
			}
			human := fmt.Sprintf("%s: %d created, %d updated, %d unchanged, %d differ", labelArgs.group,
				counts["created"], counts["updated"], counts["unchanged"], counts["differs"])
			if !labelArgs.migrate {
				return ctx.Emit(res, human)
			}
			m, err := migrate(t, labelArgs.control, labelArgs.ref, ctx.DryRun)
			if err != nil {
				return err
			}
			m.Labels = res
			human += fmt.Sprintf("\n%d issues relabelled", len(m.Relabelled))
			for _, r := range m.Relabelled {
				human += fmt.Sprintf("\n  %s#%d %s -> %s", r.Project, r.IID, r.From, r.To)
			}
			if len(m.StaleTypes) > 0 {
				human += fmt.Sprintf("\n%s %s still lists %s; change it with a plane::control issue", labelArgs.control, LabelsFile, strings.Join(m.StaleTypes, ", "))
			}
			return ctx.Emit(m, human)
		},
	})

	var listArgs struct{ project, dir, story, epic string }
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "list", Summary: "the open issues implementing a story, or every story under an epic, in the order implement works them",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&listArgs.project, "project", "", "project path (required)")
			fs.StringVar(&listArgs.story, "implements", "", "the story the issues implement, by id")
			fs.StringVar(&listArgs.epic, "epic", "", "every story under this epic, by id, read from --dir's hierarchy")
			fs.StringVar(&listArgs.dir, "dir", ".", "the checkout holding the stories, for --epic")
		},
		Run: func(ctx *cli.Context) error {
			if listArgs.project == "" || (listArgs.story == "") == (listArgs.epic == "") {
				return fmt.Errorf("%w: list needs --project and one of --implements or --epic", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := ListImplementing(t, listArgs.project, listArgs.dir, listArgs.story, listArgs.epic)
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, i := range r.Issues {
				fmt.Fprintf(&b, "%s  %-12s %s  %s\n", i.Ref, i.Story, orNone(strings.Join(i.Assignees, ",")), i.Title)
			}
			for _, i := range r.Blocked {
				fmt.Fprintf(&b, "%s  %-12s blocked by %s  %s\n", i.Ref, i.Story, strings.Join(i.BlockedBy, ", "), i.Title)
			}
			if len(r.Issues)+len(r.Blocked) == 0 {
				fmt.Fprintf(&b, "no open issue implements %s\n", strings.Join(r.Stories, ", "))
			}
			return ctx.Emit(r, b.String())
		},
	})

	var popArgs struct {
		project, issueProject, branch, dir, control, ref, worktree string
		iid                                                        int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "pop", Summary: "claim an issue and open its own branch and worktree",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&popArgs.project, "project", "", "project path (required)")
			fs.IntVar(&popArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&popArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&popArgs.branch, "branch", "", "the branch to open; empty names it from the issue")
			fs.StringVar(&popArgs.dir, "dir", ".", "a directory in the local checkout of the project")
			fs.StringVar(&popArgs.worktree, "worktree", "", "claim the issue into this worktree another pop opened, on its branch, so a story's issues share one branch (srd036)")
			fs.StringVar(&popArgs.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout, else the defaults")
			fs.StringVar(&popArgs.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if popArgs.project == "" || popArgs.iid == 0 {
				return fmt.Errorf("%w: pop needs --project and --issue", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p, err := LoadPolicy(t, ControlFor(popArgs.control, popArgs.dir, ""), popArgs.ref)
			if err != nil {
				return err
			}
			issueProject := popArgs.issueProject
			if issueProject == "" {
				issueProject = popArgs.project
			}
			if popArgs.worktree != "" && popArgs.branch != "" {
				return fmt.Errorf("%w: --worktree takes the worktree's branch; give --branch or --worktree, not both", cli.ErrUsage)
			}
			var r PopResult
			if popArgs.worktree != "" {
				r, err = PopInto(t, ExecGit{}, popArgs.project, issueProject, popArgs.iid, popArgs.worktree, p, ctx.DryRun)
			} else {
				r, err = PopIn(t, ExecGit{}, popArgs.project, issueProject, popArgs.iid, popArgs.branch, popArgs.dir, p, ctx.DryRun)
			}
			if err != nil {
				return err
			}
			human := fmt.Sprintf("%s#%d on %s in %s", issueProject, r.IID, r.Branch, r.Worktree)
			if issueProject != r.Project {
				human = fmt.Sprintf("%s#%d on %s of %s in %s", issueProject, r.IID, r.Branch, r.Project, r.Worktree)
			}
			if len(r.NotAdmitted) > 0 {
				human += "\nnot admitted, which does not block a non-data issue: " + strings.Join(r.NotAdmitted, "; ")
			}
			if r.Provisional {
				// Taken offline: the assignment is the clone's until the
				// tracker repository confirms it (GH-20).
				human += "\nprovisional: taken while the tracker repository was unreachable; run scrum-master tracker reconcile once it is reachable"
			}
			if !ctx.DryRun {
				if r.Installed, err = InstallSkills(r.Checkout, r.Worktree); err != nil {
					r.InstallError = err.Error()
					human += "\nskills not installed in the worktree: " + r.InstallError
				} else if len(r.Installed) > 0 {
					human += "\nskills installed in the worktree for " + strings.Join(r.Installed, ", ")
				}
			}
			return ctx.Emit(r, human)
		},
	})

	var claimArgs struct {
		project, issueProject, branch, control, ref string
		iid                                         int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "claim", Summary: "the claim word: assign the issue to the caller with pop's refusals, naming the branch to open (GH-204)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&claimArgs.project, "project", "", "project path (required)")
			fs.IntVar(&claimArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&claimArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&claimArgs.branch, "branch", "", "the branch to open in place of the one named for the issue (GH-47)")
			fs.StringVar(&claimArgs.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout, else the defaults")
			fs.StringVar(&claimArgs.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if claimArgs.project == "" || claimArgs.iid == 0 {
				return fmt.Errorf("%w: claim needs --project and --issue", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p, err := LoadPolicy(t, ControlFor(claimArgs.control, ".", ""), claimArgs.ref)
			if err != nil {
				return err
			}
			issueProject := claimArgs.issueProject
			if issueProject == "" {
				issueProject = claimArgs.project
			}
			r, _, err := Claim(t, claimArgs.project, issueProject, claimArgs.iid, p, ctx.DryRun)
			if err != nil {
				return err
			}
			if claimArgs.branch != "" {
				r.Branch = claimArgs.branch
			}
			// issue_project is always in the output, empty for --project,
			// so the pop profile's later words can read it (GH-47).
			out := struct {
				PopResult
				IssueProject string `json:"issue_project"`
			}{r, r.IssueProject}
			return ctx.Emit(out, fmt.Sprintf("%s#%d claimed; open its branch %s with scrum-master issue open-branch", issueProject, r.IID, r.Branch))
		},
	})

	var obArgs struct {
		project, issueProject, branch string
		iid                           int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "open-branch", Summary: "the branch word: create or reuse the issue's branch and, on a hosted tracker, its review (GH-204)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&obArgs.project, "project", "", "project path (required)")
			fs.IntVar(&obArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&obArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&obArgs.branch, "branch", "", "the branch to open (required; scrum-master issue claim names it)")
		},
		Run: func(ctx *cli.Context) error {
			if obArgs.project == "" || obArgs.iid == 0 || obArgs.branch == "" {
				return fmt.Errorf("%w: open-branch needs --project, --issue, and --branch", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			issueProject := obArgs.issueProject
			if issueProject == "" {
				issueProject = obArgs.project
			}
			i, err := t.GetIssue(issueProject, obArgs.iid)
			if err != nil {
				return err
			}
			created, review, err := OpenIssueBranch(t, obArgs.project, issueProject, obArgs.iid, obArgs.branch, i.Title, ctx.DryRun)
			if err != nil {
				return err
			}
			r := struct {
				Project string `json:"project"`
				IID     int    `json:"iid"`
				Branch  string `json:"branch"`
				Created bool   `json:"branch_created"`
				Review  string `json:"review,omitempty"`
				DryRun  bool   `json:"dry_run"`
			}{obArgs.project, obArgs.iid, obArgs.branch, created, review, ctx.DryRun}
			human := fmt.Sprintf("branch %s opened on %s", r.Branch, r.Project)
			if !created {
				human = fmt.Sprintf("branch %s already on %s", r.Branch, r.Project)
			}
			return ctx.Emit(r, human)
		},
	})

	var awArgs struct {
		branch, dir string
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "add-worktree", Summary: "the worktree word: the sibling worktree for the issue's branch, created or reused (GH-204)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&awArgs.branch, "branch", "", "the branch the worktree checks out (required)")
			fs.StringVar(&awArgs.dir, "dir", ".", "a directory in the local checkout of the project")
		},
		Run: func(ctx *cli.Context) error {
			if awArgs.branch == "" {
				return fmt.Errorf("%w: add-worktree needs --branch", cli.ErrUsage)
			}
			checkout, worktree, added, err := AddIssueWorktree(ExecGit{}, awArgs.dir, awArgs.branch, ctx.DryRun)
			if err != nil {
				return err
			}
			r := struct {
				Branch   string `json:"branch"`
				Checkout string `json:"checkout"`
				Worktree string `json:"worktree"`
				Added    bool   `json:"worktree_added"`
				DryRun   bool   `json:"dry_run"`
			}{awArgs.branch, checkout, worktree, added, ctx.DryRun}
			human := fmt.Sprintf("worktree %s on %s", worktree, awArgs.branch)
			if !added {
				human = fmt.Sprintf("worktree %s already there", worktree)
			}
			return ctx.Emit(r, human)
		},
	})

	var isArgs struct {
		checkout, worktree string
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "install-skills", Summary: "the install word: the checkout's agents' skills into the worktree (GH-204, srd036 R2.3)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&isArgs.checkout, "checkout", "", "the checkout whose installed agents decide what installs (required)")
			fs.StringVar(&isArgs.worktree, "worktree", "", "the worktree the skills install into (required)")
		},
		Run: func(ctx *cli.Context) error {
			if isArgs.checkout == "" || isArgs.worktree == "" {
				return fmt.Errorf("%w: install-skills needs --checkout and --worktree", cli.ErrUsage)
			}
			if ctx.DryRun {
				return fmt.Errorf("install-skills has no dry run; scrum-master issue pop --dry-run shows the whole pop")
			}
			installed, err := InstallSkills(isArgs.checkout, isArgs.worktree)
			if err != nil {
				return err
			}
			r := struct {
				Worktree  string   `json:"worktree"`
				Installed []string `json:"installed"`
			}{isArgs.worktree, installed}
			human := "skills installed for " + strings.Join(installed, ", ")
			if len(installed) == 0 {
				human = "nothing to install; the checkout has no agents or the worktree already has their skills"
			}
			return ctx.Emit(r, human)
		},
	})

	var closeArgs struct {
		project, dir string
		iid          int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "close", Summary: "close an issue on the local merge that delivered it (beads tracker)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&closeArgs.project, "project", "", "project path (required)")
			fs.IntVar(&closeArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&closeArgs.dir, "dir", ".", "a directory in the local checkout holding the merge")
		},
		Run: func(ctx *cli.Context) error {
			if closeArgs.project == "" || closeArgs.iid == 0 {
				return fmt.Errorf("%w: close needs --project and --issue", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Close(t, ExecGit{}, closeArgs.project, closeArgs.iid, closeArgs.dir, "", ctx.DryRun)
			if err != nil {
				return err
			}
			human := fmt.Sprintf("%s#%d closed on %s", r.Project, r.IID, r.Commit)
			if !r.Closed && !r.DryRun {
				human = fmt.Sprintf("%s#%d was already closed on %s", r.Project, r.IID, r.Commit)
			}
			return ctx.Emit(r, human)
		},
	})

	var commentArgs struct {
		project, body, bodyFile string
		iid                     int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "comment", Summary: "post a comment on an issue, such as a completed issue's Actual LOC line",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&commentArgs.project, "project", "", "project holding the issue (required)")
			fs.IntVar(&commentArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&commentArgs.body, "body", "", "the comment's text")
			fs.StringVar(&commentArgs.bodyFile, "body-file", "", "a file holding the comment's text, in place of --body")
		},
		Run: func(ctx *cli.Context) error {
			if commentArgs.project == "" || commentArgs.iid == 0 {
				return fmt.Errorf("%w: comment needs --project and --issue", cli.ErrUsage)
			}
			body := commentArgs.body
			if commentArgs.bodyFile != "" {
				if body != "" {
					return fmt.Errorf("%w: give --body or --body-file, not both", cli.ErrUsage)
				}
				b, err := os.ReadFile(commentArgs.bodyFile)
				if err != nil {
					return err
				}
				body = string(b)
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("%w: comment needs a non-empty --body or --body-file", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Comment(t, commentArgs.project, commentArgs.iid, body, ctx.DryRun)
			if err != nil {
				return err
			}
			human := fmt.Sprintf("commented on %s#%d", r.Project, r.IID)
			if r.DryRun {
				human = fmt.Sprintf("dry run: would comment on %s#%d", r.Project, r.IID)
			}
			return ctx.Emit(r, human)
		},
	})

	var approveArgs struct {
		project, issueProject, branch, dir string
		iid                                int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "approve", Summary: "approve an issue's branch for merge, as a reviewer other than its assignee",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&approveArgs.project, "project", "", "project holding the branch (required)")
			fs.IntVar(&approveArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&approveArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&approveArgs.branch, "branch", "", "branch to approve; empty uses the issue's pop branch")
			fs.StringVar(&approveArgs.dir, "dir", ".", "a checkout of the project")
		},
		Run: func(ctx *cli.Context) error {
			if approveArgs.project == "" || approveArgs.iid == 0 {
				return fmt.Errorf("%w: approve needs --project and --issue", cli.ErrUsage)
			}
			if ctx.DryRun {
				return fmt.Errorf("%w: approve is one comment and has no dry run", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			issueProject := approveArgs.issueProject
			if issueProject == "" {
				issueProject = approveArgs.project
			}
			r, err := Approve(t, ExecGit{}, approveArgs.project, issueProject, approveArgs.iid, approveArgs.branch, approveArgs.dir)
			if err != nil {
				return err
			}
			human := fmt.Sprintf("%s approved at %s by %s", r.Branch, r.Commit, r.By)
			if !r.Posted {
				human = fmt.Sprintf("%s was already approved at %s by %s", r.Branch, r.Commit, r.By)
			}
			return ctx.Emit(r, human)
		},
	})

	var mergeArgs struct {
		project, issueProject, branch, audit, dir, control, ref string
		iid                                                     int
		closes                                                  cli.Strings
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "merge", Summary: "deliver an issue by a local merge that closes it (beads tracker)",
		Flags: func(fs *flag.FlagSet) {
			// A repeatable flag appends, so each run starts it empty: the
			// words run many times in one agent process (GH-28).
			mergeArgs.closes = nil
			fs.StringVar(&mergeArgs.project, "project", "", "project path (required)")
			fs.IntVar(&mergeArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&mergeArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&mergeArgs.branch, "branch", "", "branch to merge; empty uses the current branch of the working directory")
			fs.StringVar(&mergeArgs.audit, "audit", "", "corpus gate command for a docs/ change; empty uses the policy's audit_command, scrum-master backlog check in a repository without magefiles")
			fs.StringVar(&mergeArgs.dir, "dir", ".", "the worktree holding the branch, where the audit runs")
			fs.StringVar(&mergeArgs.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout, else --project, else the defaults")
			fs.StringVar(&mergeArgs.ref, "ref", "main", "ref of the control files")
			fs.Var(&mergeArgs.closes, "closes", "another issue the merge closes, project#iid; repeat for each (srd038 R2.1)")
		},
		Run: func(ctx *cli.Context) error {
			if mergeArgs.project == "" || mergeArgs.iid == 0 {
				return fmt.Errorf("%w: merge needs --project and --issue", cli.ErrUsage)
			}
			branch := mergeArgs.branch
			refuse := func(err error) error {
				return refusedJSON(ctx, MergeResult{Project: mergeArgs.project, IID: mergeArgs.iid, Branch: branch, DryRun: ctx.DryRun}, err)
			}
			if branch == "" {
				var err error
				if branch, err = inferBranch(mergeArgs.dir, mergeArgs.iid); err != nil {
					return refuse(err)
				}
			}
			t, err := ctx.Tracker()
			if err != nil {
				return refuse(err)
			}
			p, err := LoadPolicy(t, ControlFor(mergeArgs.control, mergeArgs.dir, mergeArgs.project), mergeArgs.ref)
			if err != nil {
				return refuse(err)
			}
			gate := MergeGate{Command: AuditFor(p, mergeArgs.dir), Dir: mergeArgs.dir, RequireApproval: p.RequireApproval}
			if mergeArgs.audit != "" {
				gate.Command = mergeArgs.audit
			}
			issueProject := mergeArgs.issueProject
			if issueProject == "" {
				issueProject = mergeArgs.project
			}
			r, err := Merge(t, ExecGit{}, mergeArgs.project, issueProject, mergeArgs.iid, branch, mergeArgs.dir, gate, mergeArgs.closes, ctx.DryRun)
			if err != nil && r.Project == "" {
				return refuse(err)
			} else if err != nil {
				return refusedJSON(ctx, r, err)
			}
			audit := "no docs/ change, so no audit ran"
			if r.Audit {
				audit = "audit " + r.AuditCommand + " passed"
			}
			human := fmt.Sprintf("%s merged into %s on %s; %s; closed %s", r.Branch, r.Project, r.Commit, audit, strings.Join(r.Closed, ", "))
			if len(r.Reported) > 0 {
				human += "; reported complete " + strings.Join(r.Reported, ", ")
			}
			if len(r.AlreadyReported) > 0 {
				human += "; already reported complete " + strings.Join(r.AlreadyReported, ", ")
			}
			if r.DryRun {
				approved := "with no approval"
				if r.ApprovedBy != "" {
					approved = "approved by " + r.ApprovedBy
				}
				human = fmt.Sprintf("dry run: %s would merge into %s, %s, closing %s#%d", r.Branch, r.Project, approved, issueProject, r.IID)
				if r.Commit != "" && !r.Merged {
					// Already merged: nothing merges, approval is moot, and
					// the run would only close what is open (GH-20).
					human = fmt.Sprintf("dry run: %s is already merged on %s; a run would close %s#%d", r.Branch, short12(r.Commit), issueProject, r.IID)
				}
				// Only what is open would close: a --closes issue another
				// merge closed first is reported, not closed (GH-22).
				open := openRefs(mergeArgs.closes, r.AlreadyClosed)
				if len(open) > 0 {
					human += " and " + strings.Join(open, ", ")
				}
				if len(r.AlreadyClosed) > 0 {
					human += "; already closed: " + strings.Join(r.AlreadyClosed, ", ")
				}
			} else if !r.Merged {
				human = fmt.Sprintf("%s was already merged on %s; closed %s", r.Branch, r.Commit, strings.Join(r.Closed, ", "))
			}
			// closes and issue_project are always in the output, so the
			// merge profile's later words can read them from the gate (GH-47).
			out := struct {
				MergeResult
				IssueProject string   `json:"issue_project"`
				Closes       []string `json:"closes"`
			}{r, r.IssueProject, append([]string{}, mergeArgs.closes...)}
			return ctx.Emit(out, human)
		},
	})

	var mbArgs struct {
		project, issueProject, branch, audit, dir, control, ref string
		iid                                                     int
		closes                                                  cli.Strings
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "merge-branch", Summary: "the merge word: gates, the --no-ff merge, and the push, closing nothing (GH-201)",
		Flags: func(fs *flag.FlagSet) {
			// A repeatable flag appends, so each run starts it empty: the
			// words run many times in one agent process (GH-28).
			mbArgs.closes = nil
			fs.StringVar(&mbArgs.project, "project", "", "project path (required)")
			fs.IntVar(&mbArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&mbArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&mbArgs.branch, "branch", "", "branch to merge; empty uses the current branch of the working directory")
			fs.StringVar(&mbArgs.audit, "audit", "", "corpus gate command for a docs/ change; empty uses the policy's audit_command")
			fs.StringVar(&mbArgs.dir, "dir", ".", "the worktree holding the branch, where the audit runs")
			fs.StringVar(&mbArgs.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout, else --project, else the defaults")
			fs.StringVar(&mbArgs.ref, "ref", "main", "ref of the control files")
			fs.Var(&mbArgs.closes, "closes", "another issue the merge cites and close-delivered closes, project#iid; repeat for each")
		},
		Run: func(ctx *cli.Context) error {
			if mbArgs.project == "" || mbArgs.iid == 0 {
				return fmt.Errorf("%w: merge-branch needs --project and --issue", cli.ErrUsage)
			}
			branch := mbArgs.branch
			refuse := func(err error) error {
				return refusedJSON(ctx, MergeResult{Project: mbArgs.project, IID: mbArgs.iid, Branch: branch, DryRun: ctx.DryRun}, err)
			}
			if branch == "" {
				var err error
				if branch, err = inferBranch(mbArgs.dir, mbArgs.iid); err != nil {
					return refuse(err)
				}
			}
			t, err := ctx.Tracker()
			if err != nil {
				return refuse(err)
			}
			p, err := LoadPolicy(t, ControlFor(mbArgs.control, mbArgs.dir, mbArgs.project), mbArgs.ref)
			if err != nil {
				return refuse(err)
			}
			gate := MergeGate{Command: AuditFor(p, mbArgs.dir), Dir: mbArgs.dir, RequireApproval: p.RequireApproval}
			if mbArgs.audit != "" {
				gate.Command = mbArgs.audit
			}
			issueProject := mbArgs.issueProject
			if issueProject == "" {
				issueProject = mbArgs.project
			}
			r, err := MergeBranch(t, ExecGit{}, mbArgs.project, issueProject, mbArgs.iid, branch, mbArgs.dir, gate, mbArgs.closes, ctx.DryRun)
			if err != nil && r.Project == "" {
				return refuse(err)
			} else if err != nil {
				return refusedJSON(ctx, r, err)
			}
			human := fmt.Sprintf("%s merged into %s on %s; nothing closed: run scrum-master issue close-delivered --commit %s", r.Branch, r.Project, r.Commit, r.Commit)
			if r.DryRun && r.Commit != "" && !r.Merged {
				human = fmt.Sprintf("dry run: %s is already merged on %s; nothing would merge", r.Branch, r.Commit)
			} else if r.DryRun {
				human = fmt.Sprintf("dry run: %s would merge into %s", r.Branch, r.Project)
			} else if !r.Merged {
				human = fmt.Sprintf("%s was already merged on %s; nothing closed", r.Branch, r.Commit)
			}
			// The merge word's output always carries closes, what
			// close-delivered takes next (GH-201, GH-20).
			out := struct {
				MergeResult
				Closes []string `json:"closes"`
			}{r, append([]string{}, r.ToClose...)}
			return ctx.Emit(out, human)
		},
	})

	var cdArgs struct {
		project, issueProject, commit, dir string
		closes                             cli.Strings
		iid                                int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "close-delivered", Summary: "the close word: close the issue and each --closes on the merge commit, and report complete each closed Sprint Goal (GH-201)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&cdArgs.project, "project", "", "project path (required)")
			fs.IntVar(&cdArgs.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&cdArgs.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&cdArgs.commit, "commit", "", "the merge commit merge-branch reported (required)")
			cdArgs.closes = nil
			fs.Var(&cdArgs.closes, "closes", "another issue the merge closes, project#iid; repeat for each, or comma-separate them")
			fs.StringVar(&cdArgs.dir, "dir", ".", "a directory in the local checkout")
		},
		Run: func(ctx *cli.Context) error {
			if cdArgs.project == "" || cdArgs.iid == 0 || cdArgs.commit == "" {
				return fmt.Errorf("%w: close-delivered needs --project, --issue, and --commit", cli.ErrUsage)
			}
			if ctx.DryRun {
				return fmt.Errorf("close-delivered has no dry run; the merge skill's gate shows the whole delivery")
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			issueProject := cdArgs.issueProject
			if issueProject == "" {
				issueProject = cdArgs.project
			}
			var closes []string
			for _, value := range cdArgs.closes {
				for _, ref := range strings.Split(value, ",") {
					if ref = strings.TrimSpace(ref); ref != "" {
						closes = append(closes, ref)
					}
				}
			}
			closed, reported, wasReported, already, err := CloseDelivered(t, ExecGit{}, cdArgs.dir, cdArgs.commit, issueProject, cdArgs.iid, closes)
			r := DeliveredResult{Project: cdArgs.project, IID: cdArgs.iid, Commit: cdArgs.commit, Closed: closed, Reported: reported, AlreadyReported: wasReported, AlreadyClosed: already}
			if r.Reported == nil {
				r.Reported = []string{}
			}
			if issueProject != cdArgs.project {
				r.IssueProject = issueProject
			}
			if err != nil {
				return err
			}
			human := fmt.Sprintf("closed %s on %s", strings.Join(closed, ", "), cdArgs.commit)
			if len(reported) > 0 {
				human += "; reported complete " + strings.Join(reported, ", ")
			}
			if len(wasReported) > 0 {
				human += "; already reported complete " + strings.Join(wasReported, ", ")
			}
			return ctx.Emit(r, human)
		},
	})

	var lintArgs struct {
		project, control, ref, since string
		iid                          int
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "lint", Summary: "check issues against the label schema and admission policy",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&lintArgs.project, "project", "", "project path (required)")
			fs.IntVar(&lintArgs.iid, "issue", 0, "issue IID; or use --since")
			fs.StringVar(&lintArgs.since, "since", "", "lint every issue updated since YYYY-MM-DD")
			fs.StringVar(&lintArgs.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout, else the defaults")
			fs.StringVar(&lintArgs.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if lintArgs.project == "" || (lintArgs.iid == 0) == (lintArgs.since == "") {
				return fmt.Errorf("%w: lint needs --project and exactly one of --issue or --since", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			control := ControlFor(lintArgs.control, ".", "")
			p, err := LoadPolicy(t, control, lintArgs.ref)
			if err != nil {
				return err
			}
			var subs Subsystems
			if control != "" {
				if subs, err = LoadSubsystems(t, control, lintArgs.ref); err != nil && !errors.Is(err, tracker.ErrNotFound) {
					return err
				}
			}
			var issues []tracker.Issue
			if lintArgs.iid != 0 {
				i, err := t.GetIssue(lintArgs.project, lintArgs.iid)
				if err != nil {
					return err
				}
				issues = []tracker.Issue{i}
			} else if issues, err = t.ListIssues(lintArgs.project, tracker.IssueQuery{State: "opened", UpdatedAfter: lintArgs.since}); err != nil {
				return err
			}
			var results []LintResult
			var human strings.Builder
			for _, i := range issues {
				// An issue filed without its component gets the one its
				// project's repository is declared as, which admission
				// needs (GH-67).
				var labeled []string
				if !slices.ContainsFunc(i.Labels, func(l string) bool { return strings.HasPrefix(l, "component::") }) {
					if c, _, ok := subs.ComponentFor(i.Project); ok {
						label := "component::" + c.Name
						if !ctx.DryRun {
							if _, err := t.UpdateIssue(i.Project, i.IID, tracker.IssueUpdate{AddLabels: []string{label}}); err != nil {
								return err
							}
						}
						i.Labels, labeled = append(i.Labels, label), []string{label}
					}
				}
				r, err := Apply(t, i, Lint(i, p), p, ctx.DryRun)
				if err != nil {
					return err
				}
				if len(labeled) > 0 {
					r.Labeled, r.Changed = labeled, true
				}
				results = append(results, r)
				fmt.Fprintf(&human, "%s#%d: %s; milestone %s\n", r.Project, r.IID, verdict(r), orNone(r.Milestone))
			}
			if lintArgs.iid != 0 && len(results) == 1 {
				// One issue linted is one object, not a list of one (GH-64).
				return ctx.Emit(results[0], human.String())
			}
			return ctx.Emit(results, human.String())
		},
	})

	var fileArgs struct {
		project, title, bodyFile, plane, kind, typ, component, milestone, due, control, system, ref string
		paths                                                                                       []string
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "file", Summary: "file an unassigned issue and lint it",
		Flags: func(fs *flag.FlagSet) {
			fileArgs.paths = nil
			fs.StringVar(&fileArgs.project, "project", "", "project path (required)")
			fs.StringVar(&fileArgs.title, "title", "", "issue title (required)")
			fs.StringVar(&fileArgs.bodyFile, "body-file", "", "file holding the description (required)")
			fs.StringVar(&fileArgs.plane, "plane", "", "plane: management, control, or data; derived from --path when empty")
			fs.Func("path", "a path the request changes (repeatable); plane and kind follow from it", func(v string) error {
				fileArgs.paths = append(fileArgs.paths, v)
				return nil
			})
			fs.StringVar(&fileArgs.system, "system", "", "system repository whose subsystem SRDs give the component; also the default --control")
			fs.StringVar(&fileArgs.kind, "kind", "", "kind, on data issues")
			fs.StringVar(&fileArgs.typ, "type", "", "type: tracking, sprint, or recurring")
			fs.StringVar(&fileArgs.component, "component", "", "component")
			fs.StringVar(&fileArgs.milestone, "milestone", "", "milestone title")
			fs.StringVar(&fileArgs.due, "due", "", "due date YYYY-MM-DD")
			fs.StringVar(&fileArgs.control, "control", "", "program repository holding control/ files; empty is --system, else the system repository beside the checkout")
			fs.StringVar(&fileArgs.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if fileArgs.project == "" || fileArgs.title == "" || fileArgs.bodyFile == "" {
				return fmt.Errorf("%w: file needs --project, --title, and --body-file", cli.ErrUsage)
			}
			body, err := os.ReadFile(fileArgs.bodyFile)
			if err != nil {
				return err
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			control := fileArgs.control
			if control == "" {
				control = fileArgs.system
			}
			p, err := LoadPolicy(t, ControlFor(control, ".", ""), fileArgs.ref)
			if err != nil {
				return err
			}
			if err := CheckProject("system", fileArgs.system); err != nil {
				return fmt.Errorf("%w: %v", cli.ErrUsage, err)
			}
			// The component derives from the system repository's subsystem
			// SRDs, the one beside the checkout when --system names none
			// (GH-67).
			system := ControlFor(fileArgs.system, ".", "")
			l, err := Derive(t, fileArgs.project, system, fileArgs.ref, fileArgs.paths,
				Labelling{Plane: fileArgs.plane, Kind: fileArgs.kind, Component: fileArgs.component})
			if err != nil {
				return err
			}
			milestone, reason := fileArgs.milestone, ""
			if milestone == "" && l.Plane == "data" && fileArgs.typ == "" {
				if milestone, reason, err = SprintMilestone(t, fileArgs.project, system, time.Now(), l.Kind == "release"); err != nil {
					return err
				}
			}
			in := tracker.NewIssue{Title: fileArgs.title, Description: string(body), DueDate: fileArgs.due, Milestone: milestone,
				Labels: append(Labels(l.Plane, fileArgs.typ, l.Kind, l.Component), Op(OpFile))}
			// The milestone must exist, as admit requires: filing into one
			// that does not would admit the issue to nothing (GH-67).
			if in.Milestone != "" {
				if _, err := t.FindMilestone(fileArgs.project, in.Milestone); err != nil {
					return fmt.Errorf("%s finds no milestone %s in its group or an ancestor; the plan-push skill creates it, or file without one: %w", fileArgs.project, in.Milestone, err)
				}
			}
			r, err := File(t, fileArgs.project, in, p, ctx.DryRun)
			if err != nil {
				return err
			}
			r.Plane, r.Kind, r.Component, r.Derived = l.Plane, l.Kind, l.Component, l.Derived
			r.Milestone, r.MilestoneReason = milestone, reason
			human := fmt.Sprintf("%s#%d %s: %s\nplane %s, kind %s, component %s, milestone %s",
				r.Project, r.IID, r.WebURL, verdict(r.Lint), r.Plane, orNone(r.Kind), orNone(r.Component), orNone(r.Milestone))
			if r.MilestoneReason != "" {
				// Why the milestone is what it is — or absent — is the fix a
				// needs-triage verdict asks for (GH-20).
				human += "\nmilestone: " + r.MilestoneReason
			}
			return ctx.Emit(r, human)
		},
	})
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func verdict(r LintResult) string {
	notes := ""
	if len(r.Notes) > 0 {
		notes = " (" + strings.Join(r.Notes, "; ") + ")"
	}
	if r.Admitted {
		return "admitted" + notes
	}
	return "needs-triage: " + strings.Join(r.Reasons, "; ") + notes
}

// inferBranch is the issue's branch from the checkout at dir, when no
// --branch names one: the branch checked out there, which must be the
// issue's own <iid>-<slug> branch. A checkout on the default branch, or on
// another issue's branch, would merge the wrong thing, so it is refused
// with the fix (GH-22).
func inferBranch(dir string, iid int) (string, error) {
	out, err := ExecGit{}.Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(out)
	if !strings.HasPrefix(branch, fmt.Sprintf("%d-", iid)) {
		return "", fmt.Errorf("%s is checked out on %s, not the issue's %d-<slug> branch; run from the issue's worktree, or name its branch with --branch", dir, branch, iid)
	}
	return branch, nil
}

// refusedJSON emits r with the refusal under --json, so a caller reading
// JSON gets JSON for every refusal, the checkout's branch among them, and
// returns err, so the exit status still says it was refused (GH-22,
// GH-24). r holds at least the project and issue the command was given.
func refusedJSON(ctx *cli.Context, r MergeResult, err error) error {
	if ctx.JSON {
		r.Refused = err.Error()
		_ = ctx.Emit(r, "")
	}
	return err
}

// openRefs are the --closes references not already closed: what a merge
// would close (GH-22).
func openRefs(closes, already []string) []string {
	var open []string
	for _, ref := range closes {
		if !slices.Contains(already, ref) {
			open = append(open, ref)
		}
	}
	return open
}
