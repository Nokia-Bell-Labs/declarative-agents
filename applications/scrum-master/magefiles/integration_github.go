// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/github"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// githubRigRepo is the private repository the opt-in GitHub rig works in,
// created on first use and reused by every run.
const githubRigRepo = "petar-djukic/tpm-gh-rig"

// Github runs the opt-in GitHub rig: with gh logged in (GH_TOKEN), it
// builds scrum-master, and with SCRUM_MASTER_TRACKER=gh in a checkout of the rig repository
// proves scrum-master issue labels writes the schema into the repository, scrum-master issue
// file files an admitted issue on a dated milestone, scrum-master issue pop opens
// the branch with a draft pull request, GitHub itself refuses the author's
// own approval, and under the default policy scrum-master issue merge merges the
// author's own pull request and closes the issue. A second person's
// approval, and the refusals of a policy that requires one, are proven
// offline (internal/github), since the rig has one identity. It never joins
// mage regression (srd042, #133).
func (Integration) Github() error {
	repo := githubRigRepo
	if r := strings.TrimSpace(os.Getenv("GITHUB_RIG_REPO")); r != "" { //nolint:forbidigo // The opt-in rig reads its tokens and endpoints from the environment (eng01).
		repo = r
	}
	if out, err := exec.Command("gh", "api", "user", "-q", ".login").Output(); err != nil {
		return fmt.Errorf("gh is not logged in: %w", err)
	} else {
		fmt.Printf("github rig as %s in %s\n", strings.TrimSpace(string(out)), repo)
	}
	if exec.Command("gh", "repo", "view", repo).Run() != nil {
		if out, err := exec.Command("gh", "repo", "create", repo, "--private", "--add-readme", "--description", "scrum-master's GitHub rig: disposable issues and pull requests").CombinedOutput(); err != nil {
			return fmt.Errorf("create %s: %v: %s", repo, err, out)
		}
	}
	root, err := os.MkdirTemp("", "scrum-master-github-rig-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return err
	}
	bin := filepath.Join(root, "scrum-master")
	if err := runStreaming(".", "go", "build", "-o", bin, "./magefiles/words"); err != nil {
		return err
	}
	clone := filepath.Join(root, filepath.Base(repo))
	if out, err := exec.Command("gh", "repo", "clone", repo, clone, "--", "-q").CombinedOutput(); err != nil {
		return fmt.Errorf("clone %s: %v: %s", repo, err, out)
	}
	// git authenticates through gh, as the same login, rather than through
	// whatever credential the machine's keychain holds for github.com.
	for _, args := range [][]string{
		{"config", "credential.helper", ""},
		{"config", "--add", "credential.helper", "!gh auth git-credential"},
	} {
		if err := gitIn(clone, args...); err != nil {
			return err
		}
	}
	words := func(dir string, args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "SCRUM_MASTER_TRACKER=gh")
		var errb bytes.Buffer
		cmd.Stderr = &errb
		out, err := cmd.Output()
		if err != nil {
			return out, fmt.Errorf("scrum-master %s: %w: %s", strings.Join(args, " "), err, errb.String())
		}
		return out, nil
	}
	c := &check{}
	stamp := time.Now().UTC().Format("20060102T150405Z")

	if _, err := words(clone, "issue", "labels", "--group", repo); err != nil {
		return err
	}
	gh := github.New(clone, github.CLI{}, nil)
	ls, err := gh.GroupLabels(repo)
	if err != nil {
		return err
	}
	c.expect(len(ls) >= len(program.DefaultLabels()), "labels in %s: %d, want the %d of the schema", repo, len(ls), len(program.DefaultLabels()))

	milestone := "rig-" + stamp
	today := time.Now().UTC()
	if _, err := gh.EnsureMilestone(repo, tracker.Milestone{Title: milestone, Description: "GitHub rig run " + stamp,
		StartDate: today.AddDate(0, 0, -1).Format(time.DateOnly), DueDate: today.AddDate(0, 0, 30).Format(time.DateOnly)}); err != nil {
		return err
	}
	body := filepath.Join(root, "issue.md")
	writeFileTo(body, "## Requirements\n\n1. Add the rig note.\n\n## Acceptance Criteria\n\n- The note exists (test: note_exists; environment: local)\n")
	var filed program.FileResult
	out, err := words(clone, "issue", "file", "--project", repo, "--title", "Rig run "+stamp, "--body-file", body,
		"--plane", "data", "--kind", "build", "--component", "rig", "--milestone", milestone, "--json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, &filed); err != nil {
		return err
	}
	c.expect(filed.IID > 0 && filed.Lint.Admitted, "file: %+v", filed)

	var popped program.PopResult
	if out, err = words(clone, "issue", "pop", "--project", repo, "--issue", fmt.Sprint(filed.IID), "--dir", clone, "--json"); err != nil {
		return err
	}
	if err := json.Unmarshal(out, &popped); err != nil {
		return err
	}
	c.expect(strings.Contains(popped.Review, "/pull/"), "pop opened no pull request: %+v", popped)
	draft, _ := exec.Command("gh", "pr", "view", popped.Branch, "--repo", repo, "--json", "isDraft", "-q", ".isDraft").Output()
	c.expect(strings.TrimSpace(string(draft)) == "true", "the pull request for %s is not a draft: %q", popped.Branch, draft)

	writeFileTo(filepath.Join(popped.Worktree, "RIG.md"), "# Rig run "+stamp+"\n")
	for _, args := range [][]string{{"add", "RIG.md"}, {"commit", "-q", "-m", "Add the rig note"}, {"push", "-q", "-u", "origin", "HEAD"}} {
		if err := gitIn(popped.Worktree, args...); err != nil {
			return err
		}
	}
	tip, _ := exec.Command("git", "-C", popped.Worktree, "rev-parse", "HEAD").Output()
	_, err = gh.Approve(repo, popped.Branch, strings.TrimSpace(string(tip)))
	// gh api reports GitHub's refusal of an author's own review as 422.
	c.expect(err != nil && (strings.Contains(err.Error(), "422") || strings.Contains(strings.ToLower(err.Error()), "own pull request")), "GitHub let the author approve: %v", err)

	// The default policy requires no approval, so the one identity merges
	// its own pull request (srd013 R5.5).
	var merged program.MergeResult
	if out, err = words(popped.Worktree, "issue", "merge", "--project", repo, "--issue", fmt.Sprint(filed.IID), "--dir", popped.Worktree, "--json"); err != nil {
		c.expect(false, "merge of the author's own pull request under the default policy: %v", err)
		exec.Command("gh", "pr", "close", popped.Branch, "--repo", repo, "--delete-branch").Run()
		closed := "close"
		gh.UpdateIssue(repo, filed.IID, tracker.IssueUpdate{State: &closed})
		return c.err("github")
	}
	if err := json.Unmarshal(out, &merged); err != nil {
		return err
	}
	c.expect(merged.Merged && merged.Commit != "" && merged.ApprovedBy == "", "merge: %+v", merged)
	state, _ := exec.Command("gh", "pr", "view", popped.Branch, "--repo", repo, "--json", "state", "-q", ".state").Output()
	c.expect(strings.TrimSpace(string(state)) == "MERGED", "the pull request for %s is %q, want MERGED", popped.Branch, state)
	issue, err := gh.GetIssue(repo, filed.IID)
	c.expect(err == nil && issue.State == "closed", "issue #%d after its merge: %+v, %v", filed.IID, issue.State, err)
	exec.Command("git", "-C", clone, "push", "-q", "origin", "--delete", popped.Branch).Run()
	return c.err("github")
}
