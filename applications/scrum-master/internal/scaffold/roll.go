// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// RollEntry is one registered repository the roll handled: skipped at the
// release it already vendors, opened with its upgrade issue and merge
// request, or failed with the reason (GH-2813).
type RollEntry struct {
	Project string `json:"project"`
	Action  string `json:"action"` // skipped, opened, failed
	// Vendored is the release the repository vendors, for a skipped one.
	Vendored string `json:"vendored,omitempty"`
	Issue    string `json:"issue,omitempty"`
	Branch   string `json:"branch,omitempty"`
	// Review is the merge request, on a tracking system whose host reviews
	// branches.
	Review string `json:"review,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// RollResult is the roll word's report over the registry.
type RollResult struct {
	// Release is the release being rolled, the one an upgrade here vendors.
	Release      string      `json:"release"`
	Repositories []RollEntry `json:"repositories"`
	DryRun       bool        `json:"dry_run"`
}

// Roll rolls the installed scrum-master release across the program from the
// system checkout at dir (GH-2813): it reads the registry, the subsystem
// SRDs, and compares each registered repository's vendored release, its
// .scrum-master manifest, against the release an upgrade here would vendor.
// A repository already at it is skipped, naming the release. For each one
// behind, it files an issue there and opens a branch and merge request that
// takes the release — the upgrade profile's steps, committed on the branch —
// through each repository's normal flow, never a push to a default branch.
// An upgrade branch already at the origin is reported, not duplicated. A
// registered repository with no checkout fails alone, the checkouts word the
// remedy, and the run goes on. A dry run reports what would be opened and
// changes nothing.
func Roll(t tracker.Tracker, dir string, dryRun bool) (RollResult, error) {
	r := RollResult{DryRun: dryRun}
	root, err := toplevel(dir)
	if err != nil {
		return r, fmt.Errorf("roll needs the system checkout: %w", err)
	}
	origin := checkout.Origin(root)
	if !checkout.IsForge(origin) {
		return r, fmt.Errorf("%s has no forge origin to name the registered repositories by", root)
	}
	system := checkout.ProjectPath(origin)
	subs, err := program.SubsystemsIn(root)
	if err != nil {
		return r, err
	}
	if len(subs) == 0 {
		return r, fmt.Errorf("the registry names no repositories: %s holds no subsystem SRD under %s", root, program.SRDDir)
	}
	src, err := install.Source(root)
	if err != nil {
		return r, err
	}
	policy, err := program.LoadPolicy(t, system, "main")
	if err != nil {
		return r, err
	}
	teams := subs.Teams(system)
	r.Release, r.Repositories = src.Release, []RollEntry{}
	for _, repo := range registered(teams, system) {
		r.Repositories = append(r.Repositories, oneRoll(t, root, repo, src, teams, policy, dryRun))
	}
	return r, nil
}

// branchJunk is what a release name contributes nothing to a branch name:
// anything outside git's safe word characters.
var branchJunk = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// rollBranch is the upgrade branch the roll opens for a release, one per
// release, so a rerun finds it rather than opening a second.
func rollBranch(release string) string {
	return "scrum-master-upgrade-" + strings.Trim(branchJunk.ReplaceAllString(release, "-"), "-")
}

// rollMarker names the release an upgrade issue takes, so a rerun finds the
// issue rather than filing a second.
func rollMarker(release string) string {
	return "<!-- scrum-master roll takes " + release + " -->"
}

// oneRoll skips, opens, or fails one registered repository.
func oneRoll(t tracker.Tracker, root string, repo registeredRepo, src install.VendorSource, teams program.Subsystems, policy program.Policy, dryRun bool) RollEntry {
	e := RollEntry{Project: repo.project}
	dir, ok := checkout.Locate(root, repo.project)
	if !ok {
		e.Action, e.Detail = "failed", "no checkout at its group path; scrum-master checkouts lays it down (GH-2812)"
		return e
	}
	vendored, err := install.VendoredRelease(dir)
	if err != nil {
		e.Action, e.Detail = "failed", err.Error()
		return e
	}
	if vendored == src.Release {
		e.Action, e.Vendored, e.Detail = "skipped", vendored, "at "+vendored
		return e
	}
	e.Branch = rollBranch(src.Release)
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	base := "main"
	if head, err := git("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		base = strings.TrimPrefix(head, "origin/")
	}
	// The branch at the origin is the record: a roll already open for this
	// release is reported, never duplicated (one branch per release).
	if heads, err := git("ls-remote", "--heads", "origin", e.Branch); err != nil {
		e.Action, e.Detail = "failed", err.Error()
		return e
	} else if heads != "" {
		e.Action, e.Detail = "opened", "already open, taking "+src.Release
		e.Issue = findRollIssue(t, repo.project, src.Release)
		e.Review = openReview(t, repo.project, e.Branch, base, src.Release, e.Issue, dryRun)
		return e
	}
	if dryRun {
		e.Action, e.Detail = "opened", "would file an issue and open "+e.Branch+" taking "+src.Release
		return e
	}
	if e.Issue = findRollIssue(t, repo.project, src.Release); e.Issue == "" {
		issue, err := fileRollIssue(t, repo.project, vendored, src.Release, e.Branch, teams, policy)
		if err != nil {
			e.Action, e.Detail = "failed", err.Error()
			return e
		}
		e.Issue = issue
	}
	if err := openRollBranch(git, dir, e.Branch, base, src, e.Issue); err != nil {
		e.Action, e.Detail = "failed", err.Error()
		return e
	}
	e.Action = "opened"
	e.Detail = "takes " + src.Release
	e.Review = openReview(t, repo.project, e.Branch, base, src.Release, e.Issue, dryRun)
	return e
}

// findRollIssue is the open issue a roll of this release already filed in
// the repository, by its marker, as project#iid; empty when there is none.
func findRollIssue(t tracker.Tracker, project, release string) string {
	open, err := t.ListIssues(project, tracker.IssueQuery{State: "opened"})
	if err != nil {
		return ""
	}
	for _, i := range open {
		if strings.Contains(i.Description, rollMarker(release)) {
			return fmt.Sprintf("%s#%d", project, i.IID)
		}
	}
	return ""
}

// fileRollIssue files the repository's upgrade issue: work that adapts to a
// released artifact, so plane data and kind consume (srd001), filed by roll
// (op::upgrade).
func fileRollIssue(t tracker.Tracker, project, vendored, release, branch string, teams program.Subsystems, policy program.Policy) (string, error) {
	component, _, _ := teams.ComponentFor(project)
	was := "vendors scrum-master " + vendored
	if vendored == "" {
		was = "vendors no scrum-master release"
	}
	body := fmt.Sprintf(`This repository %s, and the program rolls %s from the system checkout: the upgrade profile's steps on a branch, merged through the repository's normal flow, never a push to a default branch (srd037 R7.4, GH-2813).

## Requirements

1. Review and merge the branch %s, which vendors the release's profiles into .scrum-master, repairs the repository at the release, and refreshes the installed skills.

## Acceptance Criteria

- .scrum-master/manifest.json names %s and the vendored profiles match it (test: scrum-master install --profiles --check; environment: local)

%s
`, was, release, branch, release, rollMarker(release))
	in := tracker.NewIssue{
		Title:       "Take scrum-master release " + release,
		Description: body,
		Labels:      append(program.Labels("data", "", "consume", component.Name), program.Op(program.OpUpgrade)),
	}
	res, err := program.File(t, project, in, policy, false)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s#%d", project, res.IID), nil
}

// openRollBranch takes the release on a new branch of the checkout at dir,
// in a temporary worktree the checkout itself never leaves its branch for:
// the upgrade profile's steps — vendor, doctor, skills — committed once and
// pushed to the origin.
func openRollBranch(git func(...string) (string, error), dir, branch, base string, src install.VendorSource, issue string) error {
	if _, err := git("fetch", "-q", "origin"); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "scrum-master-roll-*")
	if err != nil {
		return err
	}
	wt := filepath.Join(tmp, filepath.Base(branch))
	defer func() {
		git("worktree", "remove", "--force", wt) //nolint:errcheck // Cleanup of a temporary worktree.
		git("branch", "-D", branch)              //nolint:errcheck // The origin's branch is the record.
		os.RemoveAll(tmp)
	}()
	// A local branch a failed run left behind would block the worktree.
	git("branch", "-D", branch) //nolint:errcheck // Absent on a first run.
	if _, err := git("worktree", "add", "-b", branch, wt, "origin/"+base); err != nil {
		return err
	}
	if _, err := install.Vendor(wt, src, false, false); err != nil {
		return err
	}
	if _, err := Doctor(DoctorOptions{Dir: wt, Version: install.Version()}); err != nil {
		return err
	}
	if agents := install.InstalledAgents(wt); len(agents) > 0 {
		skills, err := install.EmbeddedSkills()
		if err != nil {
			return err
		}
		if _, err := install.Install(wt, agents, skills, install.Version(), false, false); err != nil {
			return err
		}
	}
	wgit := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", wt}, args...)...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := wgit("add", "-A"); err != nil {
		return err
	}
	message := "Take scrum-master release " + src.Release
	if issue != "" {
		message += " (" + issue + ")"
	}
	if _, err := wgit("commit", "-q", "-m", message); err != nil {
		return err
	}
	if _, err := wgit("push", "-q", "origin", branch); err != nil {
		return err
	}
	return nil
}

// openReview opens the branch's review, or finds the one already open, on a
// tracking system whose host reviews branches; on any other the branch and
// the issue are the record, as srd013 R5.5 has it.
func openReview(t tracker.Tracker, project, branch, base, release, issue string, dryRun bool) string {
	rv, ok := tracker.For(t, project).(tracker.Reviewer)
	if !ok || dryRun {
		return ""
	}
	body := "Takes scrum-master release " + release + "."
	if issue != "" {
		body += "\n\nCloses " + issue
	}
	review, err := rv.OpenReview(project, branch, base, "Take scrum-master release "+release, body)
	if err != nil {
		return ""
	}
	return review
}

// Human is the roll word's text output: one line per repository and the
// counts.
func (r RollResult) Human() string {
	var b strings.Builder
	counts := map[string]int{}
	for _, e := range r.Repositories {
		counts[e.Action]++
		fmt.Fprintf(&b, "%-7s %s", e.Action, e.Project)
		switch {
		case e.Review != "":
			fmt.Fprintf(&b, "  %s (%s)", e.Review, e.Detail)
		case e.Detail != "":
			fmt.Fprintf(&b, "  (%s)", e.Detail)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "roll %s: %d opened, %d skipped, %d failed", r.Release, counts["opened"], counts["skipped"], counts["failed"])
	if r.DryRun {
		b.WriteString(" (dry run)")
	}
	return b.String()
}

func init() {
	var dir string
	cli.Default.Register(&cli.Command{
		Group:   "roll",
		Summary: "roll the installed scrum-master release across the program: an upgrade issue and merge request in every registered repository behind it (GH-2813)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&dir, "dir", "", "the system repository's checkout (default: the git root)")
		},
		Run: func(ctx *cli.Context) error {
			d := dir
			if d == "" {
				var err error
				if d, err = gitRoot(); err != nil {
					return err
				}
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Roll(t, d, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, r.Human())
		},
	})
}
