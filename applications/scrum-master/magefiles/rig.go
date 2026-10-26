// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/magefile/mage/mg"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Regression creates, uses, and destroys the regression rig
// (docs/engineering/eng01-regression-rig.md): a program on local disk, a
// beads tracker repository and one code repository per project, that the
// integration targets run scrum-master against with no network.
type Regression mg.Namespace

// Aliases makes the full run `mage regression`.
var Aliases = map[string]interface{}{"regression": Regression.All}

// integrationTargets are the integration targets mage regression runs
// between up and down, in order.
var integrationTargets []func() error

const (
	// rigStateDir holds the runs, untracked; current names the run the
	// integration targets use.
	rigStateDir = ".regression"
	rigCurrent  = "current.json"
	// rigGroup is the program group every rig project sits under.
	rigGroup = "rig"
	// rigUser runs scrum-master and files and works the issues; rigReviewer is the
	// other person a peer read needs.
	rigUser     = "operator"
	rigReviewer = "reviewer"
	// rigWebBase is the host the rig's documents cite repositories under;
	// nothing is ever fetched from it.
	rigWebBase = "https://git.example.invalid"
	// rigSweepAge is how old a run Sweep removes.
	rigSweepAge = 24 * time.Hour
)

// rigProjects are the rig's code repositories, by key under rigGroup.
var rigProjects = []string{"system", "reports", "product-owners/po-1", "product-owners/po-2"}

// rigRun is the current run's record.
type rigRun struct {
	ID  string `json:"run_id"`
	Dir string `json:"dir"`
}

// Up builds a run of the rig under .regression/<run-id> and records it as the current run.
func (Regression) Up() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	run := rigRun{ID: time.Now().UTC().Format("20060102T150405Z")}
	run.Dir = filepath.Join(root, rigStateDir, run.ID)
	if err := buildRig(run.Dir); err != nil {
		os.RemoveAll(run.Dir)
		return err
	}
	b, _ := json.MarshalIndent(run, "", "  ")
	if err := os.WriteFile(filepath.Join(root, rigStateDir, rigCurrent), b, 0o644); err != nil {
		return err
	}
	fmt.Printf("rig %s is up in %s\n", run.ID, run.Dir)
	return nil
}

// Down removes the current run.
func (Regression) Down() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	run, err := currentRun(root)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(run.Dir); err != nil {
		return err
	}
	return os.Remove(filepath.Join(root, rigStateDir, rigCurrent))
}

// Sweep removes runs older than a day that a failed regression left behind.
func (Regression) Sweep() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(root, rigStateDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !e.IsDir() || time.Since(info.ModTime()) < rigSweepAge {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, rigStateDir, e.Name())); err != nil {
			return err
		}
		fmt.Println("deleted", e.Name())
	}
	return nil
}

// All runs up, every integration target, then down; down runs even when a target fails.
func (Regression) All() error {
	if err := (Regression{}).Up(); err != nil {
		return err
	}
	var failed []string
	for _, target := range integrationTargets {
		if err := target(); err != nil {
			failed = append(failed, err.Error())
		}
	}
	if err := (Regression{}).Down(); err != nil {
		failed = append(failed, "down: "+err.Error())
	}
	if len(failed) > 0 {
		return fmt.Errorf("regression failed:\n%s", strings.Join(failed, "\n"))
	}
	return nil
}

func currentRun(root string) (rigRun, error) {
	var run rigRun
	b, err := os.ReadFile(filepath.Join(root, rigStateDir, rigCurrent))
	if errors.Is(err, os.ErrNotExist) {
		return run, errors.New("no rig is up; run mage regression:up")
	}
	if err != nil {
		return run, err
	}
	return run, json.Unmarshal(b, &run)
}

// buildRig lays out a run: the tracker repository, and for each project a
// bare origin and a checkout, the checkouts side by side under checkouts/
// with work/, the checkout scrum-master runs in, so the beads tracker finds each
// project's checkout as a sibling.
func buildRig(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := bareWithFirstCommit(dir, filepath.Join(dir, "tracker.git"), map[string]string{beads.ConfigFile: "layout: " + beads.LayoutFiles + "\n"}); err != nil {
		return err
	}
	for _, name := range append([]string{"work"}, projectNames()...) {
		origin := filepath.Join(dir, "origins", name+".git")
		if err := bareWithFirstCommit(dir, origin, map[string]string{"README.md": "# " + name + "\n"}); err != nil {
			return err
		}
		if _, err := cloneAs(origin, filepath.Join(dir, "checkouts", name), rigUser); err != nil {
			return err
		}
	}
	return nil
}

func projectNames() []string {
	var names []string
	for _, p := range rigProjects {
		names = append(names, path.Base(p))
	}
	return names
}

// bareWithFirstCommit creates a bare repository holding files as the first
// commit on main.
func bareWithFirstCommit(dir, bare string, files map[string]string) error {
	if err := gitIn(dir, "init", "-q", "--bare", "-b", "main", bare); err != nil {
		return err
	}
	seed, err := os.MkdirTemp(dir, "seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(seed)
	if _, err := cloneAs(bare, filepath.Join(seed, "s"), rigUser); err != nil {
		return err
	}
	for p, content := range files {
		writeFileTo(filepath.Join(seed, "s", p), content)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "first commit"}, {"push", "-q", "origin", "HEAD:main"}} {
		if err := gitIn(filepath.Join(seed, "s"), args...); err != nil {
			return err
		}
	}
	return nil
}

// cloneAs clones remote into dir with user's committer identity.
func cloneAs(remote, dir, user string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return dir, nil
	}
	if err := gitIn(filepath.Dir(dir), "clone", "-q", remote, dir); err != nil {
		return "", err
	}
	for _, kv := range [][2]string{{"user.name", user}, {"user.email", user + "@example.invalid"}} {
		if err := gitIn(dir, "config", kv[0], kv[1]); err != nil {
			return "", err
		}
	}
	gitIn(dir, "remote", "set-head", "origin", "main")
	return dir, nil
}

func gitIn(dir string, args ...string) error {
	_, err := gitOut(dir, args...)
	return err
}

func gitOut(dir string, args ...string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// seedBranch holds the fixtures in rig repositories, apart from main, which
// the targets' merges change.
const seedBranch = "seed"

// rigEnv is what an integration target needs from the current run.
type rigEnv struct {
	root    string
	run     rigRun
	group   string
	words   string
	webBase string
	user    string
	// tr is the tracker the target reads and writes beside scrum-master, as rigUser.
	tr *beads.Tracker
}

func newRigEnv() (*rigEnv, error) {
	root, err := repositoryRoot()
	if err != nil {
		return nil, err
	}
	run, err := currentRun(root)
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(run.Dir, "scrum-master")
	if err := runStreaming(root, "go", "build", "-o", bin, "./magefiles/words"); err != nil {
		return nil, fmt.Errorf("build the words: %w", err)
	}
	clone, err := cloneAs(filepath.Join(run.Dir, "tracker.git"), filepath.Join(run.Dir, "tracker-target"), rigUser)
	if err != nil {
		return nil, err
	}
	return &rigEnv{root: root, run: run, group: rigGroup, words: bin, webBase: rigWebBase, user: rigUser,
		tr: beads.NewTracker(clone, rigUser, filepath.Join(run.Dir, "checkouts", "work"))}, nil
}

func (r *rigEnv) project(key string) (string, error) {
	if !slices.Contains(rigProjects, key) {
		return "", fmt.Errorf("rig has no %s repository; the suite needs it", key)
	}
	return rigGroup + "/" + key, nil
}

// checkout is a project's own checkout among the rig's side-by-side ones.
func (r *rigEnv) checkout(project string) string {
	return filepath.Join(r.run.Dir, "checkouts", path.Base(project))
}

func (r *rigEnv) origin(project string) string {
	return filepath.Join(r.run.Dir, "origins", path.Base(project)+".git")
}

// runWords runs the built scrum-master on the rig's tracker as rigUser and returns stdout.
func (r *rigEnv) runWords(args ...string) ([]byte, error) { return r.runWordsAs(rigUser, args...) }

// runWordsAs runs scrum-master as user, from the work checkout, with that person's own
// clone of the tracker repository.
func (r *rigEnv) runWordsAs(user string, args ...string) ([]byte, error) {
	cmd := exec.Command(r.words, args...)
	cmd.Dir = filepath.Join(r.run.Dir, "checkouts", "work")
	cmd.Env = append(os.Environ(), "SCRUM_MASTER_TRACKER="+filepath.Join(r.run.Dir, "tracker.git"),
		"SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(r.run.Dir, "tracker-"+user), "SCRUM_MASTER_USER="+user, "SCRUM_MASTER_WEB_BASE="+r.webBase)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("scrum-master %s: %w: %s", strings.Join(args, " "), err, errb.String())
	}
	return out.Bytes(), nil
}

func (r *rigEnv) wordsJSON(into any, args ...string) error {
	out, err := r.runWords(append(args, "--json")...)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, into)
}

// seed commits files to the project's seed branch, created from main once.
func (r *rigEnv) seed(project string, files map[string]string) error {
	dir := r.checkout(project)
	if err := gitIn(dir, "fetch", "-q", "origin"); err != nil {
		return err
	}
	start := "origin/main"
	if _, err := gitOut(dir, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+seedBranch); err == nil {
		start = "origin/" + seedBranch
	}
	if err := gitIn(dir, "checkout", "-q", "-B", seedBranch, start); err != nil {
		return err
	}
	for p, content := range files {
		writeFileTo(filepath.Join(dir, p), content)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "--allow-empty", "-m", "Seed regression fixtures"}, {"push", "-q", "origin", seedBranch}, {"checkout", "-q", "main"}} {
		if err := gitIn(dir, args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *rigEnv) tag(project, tag string) error {
	if _, err := r.remoteTag(project, tag); err == nil {
		return nil
	}
	dir := r.checkout(project)
	if err := gitIn(dir, "fetch", "-q", "origin"); err != nil {
		return err
	}
	if err := gitIn(dir, "tag", tag, "origin/"+seedBranch); err != nil {
		return err
	}
	return gitIn(dir, "push", "-q", "origin", "refs/tags/"+tag)
}

// remoteTag is the commit a tag names at the project's origin.
func (r *rigEnv) remoteTag(project, tag string) (string, error) {
	return gitOut(r.run.Dir, "--git-dir", r.origin(project), "rev-parse", "--verify", "-q", "refs/tags/"+tag+"^{commit}")
}

func (r *rigEnv) remoteBranch(project, branch string) bool {
	_, err := gitOut(r.run.Dir, "--git-dir", r.origin(project), "rev-parse", "--verify", "-q", "refs/heads/"+branch)
	return err == nil
}

// milestone ensures a group milestone on the rig group, started yesterday,
// so it is the active sprint scrum-master issue file defaults to.
func (r *rigEnv) milestone(title, due string) error {
	_, err := r.tr.EnsureMilestone(r.group, tracker.Milestone{Title: title,
		StartDate: time.Now().AddDate(0, 0, -1).Format(time.DateOnly), DueDate: due})
	return err
}

func (r *rigEnv) issues(project string) ([]tracker.Issue, error) {
	return r.tr.ListIssues(project, tracker.IssueQuery{State: "opened"})
}

func (r *rigEnv) issue(project string, iid int) (tracker.Issue, error) {
	return r.tr.GetIssue(project, iid)
}

func (r *rigEnv) notes(project string, iid int) ([]tracker.Note, error) {
	ns, err := r.tr.ListNotes(project, iid)
	var out []tracker.Note
	for _, n := range ns {
		if !n.System {
			out = append(out, n)
		}
	}
	return out, err
}

// clone clones a rig repository into the run's clones/ directory, with the
// rig user's identity, and returns its path; pop puts worktrees beside it.
func (r *rigEnv) clone(project, name string) (string, error) {
	return cloneAs(r.origin(project), filepath.Join(r.run.Dir, "clones", name), rigUser)
}

func (r *rigEnv) git(dir string, args ...string) error { return gitIn(dir, args...) }

// commitPush stages files (all changes when none are named), commits, and
// pushes the worktree's branch.
func (r *rigEnv) commitPush(wt, message string, files ...string) error {
	add := append([]string{"add"}, files...)
	if len(files) == 0 {
		add = []string{"add", "-A"}
	}
	for _, args := range [][]string{add, {"commit", "--quiet", "-m", message}, {"push", "--quiet", "-u", "origin", "HEAD"}} {
		if err := r.git(wt, args...); err != nil {
			return err
		}
	}
	return nil
}

// approve has the reviewer approve a branch for merge from a clone of
// their own (srd013 R5.6); branch empty is the issue's pop branch.
func (r *rigEnv) approve(project, issueProject string, iid int, branch string) error {
	clone, err := r.clone(project, "reviewer-"+path.Base(project))
	if err != nil {
		return err
	}
	args := []string{"issue", "approve", "--project", project, "--issue", fmt.Sprint(iid), "--dir", clone}
	if issueProject != project {
		args = append(args, "--issue-project", issueProject)
	}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	_, err = r.runWordsAs(rigReviewer, args...)
	return err
}

// closed reports whether the tracker holds the issue closed.
func (r *rigEnv) closed(project string, iid int) bool {
	i, err := r.issue(project, iid)
	return err == nil && i.State == "closed"
}

// check collects failed expectations so a target reports all of them.
type check struct{ failures []string }

func (c *check) expect(ok bool, format string, args ...any) {
	if !ok {
		c.failures = append(c.failures, fmt.Sprintf(format, args...))
	}
}

func (c *check) err(name string) error {
	if len(c.failures) == 0 {
		fmt.Printf("integration:%s passed\n", name)
		return nil
	}
	return fmt.Errorf("integration:%s failed:\n  %s", name, strings.Join(c.failures, "\n  "))
}
