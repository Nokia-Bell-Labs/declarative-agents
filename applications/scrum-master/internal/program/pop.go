// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Git runs git in a directory and returns its standard output. Tests use a
// recording implementation.
type Git interface {
	Run(dir string, args ...string) (string, error)
}

// ExecGit runs the git on PATH.
type ExecGit struct{}

func (ExecGit) Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

var slugJunk = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns an issue title into a branch slug: lowercase words joined by
// hyphens, at most 40 characters.
func Slug(title string) string {
	s := strings.Trim(slugJunk.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	if s == "" {
		s = "issue"
	}
	return s
}

// PopResult is what pop reports.
type PopResult struct {
	Project string `json:"project"`
	// IssueProject holds the issue when it is not Project.
	IssueProject string `json:"issue_project,omitempty"`
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	// Body is the issue body: its Requirements are the work's steps and its
	// Acceptance Criteria what must hold, so the worker needs no tracker
	// round-trip after the pop (srd036 R6.3).
	Body     string `json:"body,omitempty"`
	WebURL   string `json:"web_url"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree,omitempty"`
	Checkout string `json:"checkout,omitempty"`
	Assigned bool   `json:"assigned"` // pop assigned the caller now
	// Provisional is an assignment taken while the tracker repository was
	// unreachable; scrum-master tracker reconcile confirms it or reports it lost.
	Provisional bool `json:"provisional,omitempty"`
	Created     bool `json:"branch_created"` // pop created the branch now
	Added       bool `json:"worktree_added"` // pop added the worktree now
	// Review is the URL of the pull or merge request pop opened, or found,
	// on a tracking system whose host reviews the branch.
	Review string `json:"review,omitempty"`
	DryRun bool   `json:"dry_run"`
	// Installed are the agents whose skills were installed into the worktree
	// (srd036 R2.3); InstallError says why that failed, without undoing pop.
	Installed    []string `json:"installed,omitempty"`
	InstallError string   `json:"install_error,omitempty"`
	// NotAdmitted are the reasons a management or control issue fails
	// admission. They do not block pop, which refuses only data issues
	// admission has not admitted (srd013 R7.1).
	NotAdmitted []string `json:"not_admitted,omitempty"`
}

// Pop claims an issue for the caller and opens its own branch and worktree
// (srd013 R7). It refuses a closed issue, an issue assigned to anyone else,
// and a data issue admission does not admit. Everything it finds already in
// place is reused, so a second pop changes nothing. dir is any directory in
// the local checkout of the project.
func Pop(t tracker.Tracker, git Git, project string, iid int, dir string, p Policy, dryRun bool) (PopResult, error) {
	return PopIn(t, git, project, project, iid, "", dir, p, dryRun)
}

// PopIn is Pop for an issue whose work lands in another repository, as a
// weekly status issue's report lands in the reports repository: the issue
// is claimed in issueProject, and the branch, named branch when given, and
// its worktree are opened in project.
func PopIn(t tracker.Tracker, git Git, project, issueProject string, iid int, branch, dir string, p Policy, dryRun bool) (PopResult, error) {
	r, i, err := Claim(t, project, issueProject, iid, p, dryRun)
	if err != nil {
		return r, err
	}
	if branch != "" {
		r.Branch = branch
	}
	if r.Created, r.Review, err = OpenIssueBranch(t, project, issueProject, iid, r.Branch, i.Title, dryRun); err != nil {
		return r, err
	}
	r.Checkout, r.Worktree, r.Added, err = AddIssueWorktree(git, dir, r.Branch, dryRun)
	if err != nil {
		return r, fmt.Errorf("pop needs a local checkout of %s: %w", project, err)
	}
	return r, nil
}

// OpenIssueBranch is the branch word (GH-204, srd013 R7.5): create or reuse
// the issue's branch from the default branch and, on a tracking system
// whose host reviews branches, open or find its review. It assigns nothing
// and adds no worktree.
func OpenIssueBranch(t tracker.Tracker, project, issueProject string, iid int, branch, title string, dryRun bool) (created bool, review string, err error) {
	base, err := t.DefaultBranch(project)
	if err != nil {
		return false, "", err
	}
	exists, err := t.BranchExists(project, branch)
	if err != nil {
		return false, "", err
	}
	if !exists {
		created = true
		if !dryRun {
			if err := t.CreateBranch(project, branch, base); err != nil {
				return created, "", err
			}
		}
	}
	if rv, ok := tracker.For(t, project).(tracker.Reviewer); ok && !dryRun {
		ref := fmt.Sprintf("%s#%d", issueProject, iid)
		if review, err = rv.OpenReview(project, branch, base, title, "Delivers "+ref+".\n\nCloses "+ref); err != nil {
			return created, "", fmt.Errorf("opened %s, but not its review: %w", branch, err)
		}
	}
	return created, review, nil
}

// AddIssueWorktree is the worktree word (GH-204, srd013 R7.5): the sibling
// worktree for branch beside the checkout around dir, created or reused,
// with the branch checked out and a git-crypt key linked where the
// repository has one.
func AddIssueWorktree(git Git, dir, branch string, dryRun bool) (checkout, worktree string, added bool, err error) {
	top, err := git.Run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", false, err
	}
	root := strings.TrimSpace(top)
	worktree = filepath.Join(filepath.Dir(root), filepath.Base(root)+"-"+branch)
	if _, err := os.Stat(worktree); err == nil {
		return root, worktree, false, nil
	}
	if dryRun {
		return root, worktree, true, nil
	}
	if _, err := git.Run(root, "fetch", "origin", branch); err != nil {
		return root, worktree, true, err
	}
	if _, err := git.Run(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return root, worktree, true, addWorktree(git, root, worktree, worktree, branch)
	}
	return root, worktree, true, addWorktree(git, root, worktree, "--track", "-b", branch, worktree, "origin/"+branch)
}

// PopInto claims an issue into a worktree another pop opened, so a
// story's issues are built on one branch and close in one merge (srd036):
// the same refusals as Pop, and no branch or worktree of its own.
func PopInto(t tracker.Tracker, git Git, project, issueProject string, iid int, worktree string, p Policy, dryRun bool) (PopResult, error) {
	r, _, err := Claim(t, project, issueProject, iid, p, dryRun)
	if err != nil {
		return r, err
	}
	branch, err := git.Run(worktree, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branch) == "" {
		return r, fmt.Errorf("%s is not a worktree on a branch: %v", worktree, err)
	}
	top, err := git.Run(worktree, "rev-parse", "--show-toplevel")
	if err != nil {
		return r, err
	}
	r.Branch, r.Worktree = strings.TrimSpace(branch), strings.TrimSpace(top)
	if common, err := git.Run(worktree, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		r.Checkout = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(common), "/"), "/.git")
	}
	return r, nil
}

// Claim is the claim word (GH-204, srd013 R7.1, srd041 R9.1): it refuses a
// closed issue, an issue assigned to anyone else, and a data issue
// admission does not admit, assigns the caller, and names the branch the
// branch word opens. The assignment is the atomic transaction the word
// boundary protects (GH-201).
func Claim(t tracker.Tracker, project, issueProject string, iid int, p Policy, dryRun bool) (PopResult, tracker.Issue, error) {
	i, err := t.GetIssue(issueProject, iid)
	if err != nil {
		return PopResult{}, i, err
	}
	r := PopResult{Project: project, IID: iid, Title: i.Title, Body: i.Description, WebURL: i.WebURL, DryRun: dryRun}
	r.Branch = fmt.Sprintf("%d-%s", iid, Slug(i.Title))
	if issueProject != project {
		r.IssueProject = issueProject
	}
	if i.State == "closed" {
		return r, i, fmt.Errorf("%s#%d is closed", issueProject, iid)
	}
	me, err := tracker.UserIn(t, issueProject)
	if err != nil {
		return r, i, err
	}
	for _, a := range i.Assignees {
		if a != me.Username {
			return r, i, fmt.Errorf("%s#%d is assigned to %s; only that person pops it", issueProject, iid, a)
		}
	}
	if reasons := Lint(i, p); len(reasons) > 0 {
		if i.HasLabel("plane::data") {
			return r, i, fmt.Errorf("%s#%d is not admitted: %s", issueProject, iid, strings.Join(reasons, "; "))
		}
		r.NotAdmitted = reasons
	}
	if !slices.Contains(i.Assignees, me.Username) {
		r.Assigned = true
		if !dryRun {
			if err := t.AssignIssue(issueProject, iid, me); err != nil {
				return r, i, err
			}
			if reader, ok := tracker.For(t, issueProject).(tracker.Reader); ok {
				read, _ := reader.LastRead()
				r.Provisional = read.Offline
			}
		}
	}
	return r, i, nil
}

// addWorktree adds worktree with the worktree add arguments args, then
// checks it out. The checkout runs the repository's smudge filters, and
// git-crypt reads its key from the worktree's own git dir, so the key
// directory of the shared git dir is linked there first. Without git-crypt
// this is a plain worktree add.
func addWorktree(git Git, root, worktree string, args ...string) error {
	if _, err := git.Run(root, append([]string{"worktree", "add", "--no-checkout"}, args...)...); err != nil {
		return err
	}
	common, err := git.Run(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	if key := filepath.Join(strings.TrimSpace(common), "git-crypt"); isDir(key) {
		own, err := git.Run(worktree, "rev-parse", "--path-format=absolute", "--git-dir")
		if err != nil {
			return err
		}
		if err := os.Symlink(key, filepath.Join(strings.TrimSpace(own), "git-crypt")); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	_, err = git.Run(worktree, "checkout")
	return err
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// InstallSkills installs into worktree the skills of every agent installed
// in checkout (srd036 R2.3). The installed files are untracked, so a new
// worktree has none, and an agent started there could not finish the issue.
// A checkout with no installed agent installs nothing. It returns the agents
// whose skills it wrote, an empty list when the worktree already had them
// (GH-2792).
func InstallSkills(checkout, worktree string) ([]string, error) {
	agents := install.InstalledAgents(checkout)
	if len(agents) == 0 {
		return []string{}, nil
	}
	skills, err := install.EmbeddedSkills()
	if err != nil {
		return nil, err
	}
	results, err := install.Install(worktree, agents, skills, install.Version(), false, false)
	if err != nil {
		return nil, err
	}
	// Only the agents whose files this install wrote are reported: a pop
	// into a worktree that already has them changes nothing.
	changed := []string{}
	for _, r := range results {
		switch r.Action {
		case "created", "updated", "replaced", "removed":
			if r.Agent != "" && !slices.Contains(changed, r.Agent) {
				changed = append(changed, r.Agent)
			}
		}
	}
	return changed, nil
}
