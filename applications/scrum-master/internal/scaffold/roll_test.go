// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
)

// rollProgram is a program for the roll word: the checkouts program with its
// checkouts laid down, and a release v9.9.9 installed at SCRUM_MASTER_HOME
// for the roll to vendor from.
func rollProgram(t *testing.T, registered ...string) (root, system string) {
	t.Helper()
	root, _, system = checkoutsProgram(t, registered...)
	if r, err := Checkouts(system, false); err != nil {
		t.Fatalf("Checkouts: %v, %+v", err, r)
	}
	// A registered repository went through onboard: it holds the subsystem
	// corpus skeleton on its default branch.
	for _, p := range registered {
		dir := filepath.Join(root, filepath.FromSlash(p))
		files, err := corpusFiles("subsystem", dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := write(dir, files, false, false); err != nil {
			t.Fatal(err)
		}
		checkoutsGit(t, dir, "add", "-A")
		checkoutsGit(t, dir, "commit", "-q", "-m", "onboarded")
		checkoutsGit(t, dir, "push", "-q", "origin", "main")
	}
	home := t.TempDir()
	os.WriteFile(filepath.Join(home, "release"), []byte("v9.9.9\n"), 0o644)
	os.MkdirAll(filepath.Join(home, "agents", "check"), 0o755)
	os.WriteFile(filepath.Join(home, "agents", "check", "profile.yaml"), []byte("name: check\n"), 0o644)
	os.MkdirAll(filepath.Join(home, "agent-core", "tools"), 0o755)
	os.WriteFile(filepath.Join(home, "agent-core", "tools", "exec.yaml"), []byte("unit: exec\n"), 0o644)
	t.Setenv(install.SourceEnv, "")
	t.Setenv(install.HomeEnv, home)
	return root, system
}

// atRelease marks a checkout as vendoring the installed release, as the
// upgrade profile leaves it.
func atRelease(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, install.VendorDir), 0o755)
	b, _ := json.Marshal(install.Manifest{Release: "v9.9.9", Files: map[string]string{}})
	if err := os.WriteFile(filepath.Join(dir, install.ManifestFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// byRollProject is the entry for one project in a roll result.
func byRollProject(t *testing.T, r RollResult, p string) RollEntry {
	t.Helper()
	for _, e := range r.Repositories {
		if e.Project == p {
			return e
		}
	}
	t.Fatalf("%s is not in the result: %+v", p, r.Repositories)
	return RollEntry{}
}

// The roll skips the repository already at the release, naming it, and opens
// an issue, branch, and upgrade in the one behind: the upgrade's steps
// committed on the branch at the origin, the checkout's own branch untouched
// (GH-2813).
func TestRollSkipsTheCurrentAndOpensAnUpgradeInTheRest(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm", "g/analytics/model-evaluation")
	atRelease(t, filepath.Join(root, "g", "pods", "p1", "tpm"))
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	if r.Release != "v9.9.9" {
		t.Fatalf("release = %q, want v9.9.9", r.Release)
	}
	if e := byRollProject(t, r, "g/pods/p1/tpm"); e.Action != "skipped" || e.Detail != "at v9.9.9" {
		t.Errorf("the current repository = %s (%s), want skipped at v9.9.9", e.Action, e.Detail)
	}
	e := byRollProject(t, r, "g/analytics/model-evaluation")
	if e.Action != "opened" || e.Branch != "scrum-master-upgrade-v9.9.9" || e.Issue == "" {
		t.Fatalf("the repository behind = %+v, want opened with its issue and branch", e)
	}
	issues, _ := f.ListIssues("g/analytics/model-evaluation", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 || !strings.Contains(issues[0].Title, "v9.9.9") {
		t.Fatalf("issues in the repository behind: %+v", issues)
	}
	for _, label := range []string{"plane::data", "kind::consume", "op::upgrade"} {
		if !issues[0].HasLabel(label) {
			t.Errorf("the issue lacks %s: %v", label, issues[0].Labels)
		}
	}
	// The branch is at the origin with the upgrade committed; the checkout's
	// own branch never left main and holds nothing new.
	dir := filepath.Join(root, "g", "analytics", "model-evaluation")
	checkoutsGit(t, dir, "fetch", "-q", "origin", e.Branch)
	manifest := checkoutsGit(t, dir, "show", "origin/"+e.Branch+":"+install.ManifestFile)
	if !strings.Contains(manifest, "v9.9.9") {
		t.Errorf("the branch's manifest does not take v9.9.9:\n%s", manifest)
	}
	if cur := checkoutsGit(t, dir, "branch", "--show-current"); cur != "main" {
		t.Errorf("the checkout left main for %s", cur)
	}
	if status := checkoutsGit(t, dir, "status", "--porcelain"); status != "" {
		t.Errorf("the checkout holds changes:\n%s", status)
	}
	msg := checkoutsGit(t, dir, "log", "-1", "--format=%s", "origin/"+e.Branch)
	if !strings.Contains(msg, e.Issue) {
		t.Errorf("the commit %q does not cite the issue %s", msg, e.Issue)
	}
	if h := r.Human(); !strings.Contains(h, "1 opened, 1 skipped, 0 failed") {
		t.Errorf("Human = %q", h)
	}
}

// reviewerFake is a tracking system whose host reviews branches: OpenReview
// returns the merge request, the same one when it is already open.
type reviewerFake struct {
	*fake.Fake
	opened []string
}

func (r *reviewerFake) OpenReview(project, branch, base, title, body string) (string, error) {
	r.opened = append(r.opened, project+"@"+branch)
	return "https://forge.example.com/" + project + "/-/merge_requests/1", nil
}

func (r *reviewerFake) Approve(string, string, string) (bool, error) { return false, nil }
func (r *reviewerFake) Approvals(string, string, string) ([]string, map[string]string, error) {
	return nil, nil, nil
}

func (r *reviewerFake) RequestReview(string, string, []string) error { return nil }

func (r *reviewerFake) Approval(string, string, string, []string) (string, string, error) {
	return "", "", nil
}
func (r *reviewerFake) Merge(string, string, string, string) (string, error) { return "", nil }

// A second roll finds the upgrade already open: it reports the merge request
// and opens nothing — no second issue, no second commit (GH-2813).
func TestRollReportsAnOpenUpgradeAndDuplicatesNothing(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	f := &reviewerFake{Fake: fake.New()}
	first, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("first roll: %v", err)
	}
	e := byRollProject(t, first, "g/pods/p1/tpm")
	if e.Action != "opened" || e.Review == "" {
		t.Fatalf("first roll = %+v, want opened with its merge request", e)
	}
	dir := filepath.Join(root, "g", "pods", "p1", "tpm")
	checkoutsGit(t, dir, "fetch", "-q", "origin", e.Branch)
	tip := checkoutsGit(t, dir, "rev-parse", "origin/"+e.Branch)

	again, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("second roll: %v", err)
	}
	e = byRollProject(t, again, "g/pods/p1/tpm")
	if e.Action != "opened" || !strings.Contains(e.Detail, "already open") || e.Review == "" || e.Issue == "" {
		t.Fatalf("second roll = %+v, want the open merge request reported", e)
	}
	if issues, _ := f.ListIssues("g/pods/p1/tpm", tracker.IssueQuery{State: "opened"}); len(issues) != 1 {
		t.Errorf("%d issues after the second roll, want 1", len(issues))
	}
	checkoutsGit(t, dir, "fetch", "-q", "origin", e.Branch)
	if now := checkoutsGit(t, dir, "rev-parse", "origin/"+e.Branch); now != tip {
		t.Errorf("the second roll moved the branch: %s -> %s", tip, now)
	}
}

// A registered repository with no checkout fails alone, the checkouts word
// the remedy, and the rest are still processed (GH-2813).
func TestRollFailsARepositoryWithNoCheckoutAlone(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm", "g/pods/p2/tpm")
	os.RemoveAll(filepath.Join(root, "g", "pods", "p2", "tpm"))
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	if e := byRollProject(t, r, "g/pods/p2/tpm"); e.Action != "failed" || !strings.Contains(e.Detail, "scrum-master checkouts") {
		t.Errorf("the missing checkout = %s (%s), want failed with the checkouts word as the remedy", e.Action, e.Detail)
	}
	if e := byRollProject(t, r, "g/pods/p1/tpm"); e.Action != "opened" {
		t.Errorf("the other repository = %s (%s), want opened", e.Action, e.Detail)
	}
}

// A dry run reports what would be opened and changes nothing: no issue, no
// branch (GH-2813).
func TestRollDryRunOpensNothing(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	f := fake.New()
	r, err := Roll(f, system, true)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	e := byRollProject(t, r, "g/pods/p1/tpm")
	if !r.DryRun || e.Action != "opened" || !strings.Contains(e.Detail, "would file an issue") {
		t.Fatalf("dry run = %+v", r)
	}
	if f.Writes != 0 {
		t.Errorf("a dry run wrote %d times to the tracker", f.Writes)
	}
	dir := filepath.Join(root, "g", "pods", "p1", "tpm")
	if heads := checkoutsGit(t, dir, "ls-remote", "--heads", "origin", e.Branch); heads != "" {
		t.Errorf("a dry run pushed the branch:\n%s", heads)
	}
}

// A repository ahead of the release this machine would roll is skipped, saying
// so, with no issue and no branch: an upgrade from here would take it back
// (GH-2899).
func TestRollLeavesARepositoryAheadOfTheMachine(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	dir := filepath.Join(root, "g", "pods", "p1", "tpm")
	os.MkdirAll(filepath.Join(dir, install.VendorDir), 0o755)
	b, _ := json.Marshal(install.Manifest{Release: "v0.20261103.4", Files: map[string]string{}})
	if err := os.WriteFile(filepath.Join(dir, install.ManifestFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(install.HomeEnv, releaseHome(t, "v0.20261103.2"))
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	e := byRollProject(t, r, "g/pods/p1/tpm")
	if e.Action != "skipped" || e.Vendored != "v0.20261103.4" || !strings.Contains(e.Detail, "ahead") || !strings.Contains(e.Detail, "install.sh v0.20261103.4") {
		t.Fatalf("the repository ahead = %+v", e)
	}
	if issues, _ := f.ListIssues("g/pods/p1/tpm", tracker.IssueQuery{State: "opened"}); len(issues) != 0 {
		t.Errorf("issues filed: %+v", issues)
	}
	if out := checkoutsGit(t, dir, "ls-remote", "--heads", "origin", "scrum-master-upgrade-*"); out != "" {
		t.Errorf("a branch opened: %s", out)
	}
}

// An agent older than the release it would roll refuses, touching nothing
// (GH-2899).
func TestRollRefusesAnAgentOlderThanTheRelease(t *testing.T) {
	_, system := rollProgram(t, "g/pods/p1/tpm")
	t.Setenv(install.HomeEnv, releaseHome(t, "v0.20261103.4"))
	saved := version.Release
	version.Release = "v0.20261103.2"
	defer func() { version.Release = saved }()
	f := fake.New()
	if _, err := Roll(f, system, false); err == nil || !strings.Contains(err.Error(), "nothing was rolled") {
		t.Fatalf("Roll = %v, want a refusal", err)
	}
	if issues, _ := f.ListIssues("g/pods/p1/tpm", tracker.IssueQuery{State: "opened"}); len(issues) != 0 {
		t.Errorf("issues filed: %+v", issues)
	}
}

// releaseHome is where install.sh would have unpacked one release.
func releaseHome(t *testing.T, release string) string {
	t.Helper()
	home := t.TempDir()
	os.WriteFile(filepath.Join(home, "release"), []byte(release+"\n"), 0o644)
	os.MkdirAll(filepath.Join(home, "agents"), 0o755)
	os.MkdirAll(filepath.Join(home, "agent-core", "tools"), 0o755)
	return home
}

// The upgrade issue goes on the repository's active sprint milestone, as file
// puts a data issue, so it is not needs-triage for the want of one; with no
// sprint covering today the issue is filed without one and the entry says
// why (GH-2917).
func TestRollFilesTheUpgradeOnTheActiveSprint(t *testing.T) {
	_, system := rollProgram(t, "g/analytics/model-evaluation")
	f := fake.New()
	today := time.Now()
	f.Milestones["Sprint 7"] = tracker.Milestone{Title: "Sprint 7",
		StartDate: today.AddDate(0, 0, -3).Format("2006-01-02"), DueDate: today.AddDate(0, 0, 10).Format("2006-01-02")}
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	e := byRollProject(t, r, "g/analytics/model-evaluation")
	if e.Action != "opened" || e.Detail != "takes v9.9.9" {
		t.Fatalf("entry = %+v, want opened taking v9.9.9", e)
	}
	issues, _ := f.ListIssues("g/analytics/model-evaluation", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 || issues[0].Milestone != "Sprint 7" {
		t.Fatalf("the upgrade issue = %+v, want it on Sprint 7", issues)
	}
	if issues[0].HasLabel("needs-triage") {
		t.Errorf("the upgrade issue is needs-triage: %v", issues[0].Labels)
	}
}

func TestRollNamesWhyAnUpgradeHasNoSprint(t *testing.T) {
	_, system := rollProgram(t, "g/analytics/model-evaluation")
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	e := byRollProject(t, r, "g/analytics/model-evaluation")
	if e.Action != "opened" || !strings.Contains(e.Detail, "milestone: no active milestone") {
		t.Fatalf("entry = %+v, want opened naming why it has no milestone", e)
	}
	if h := r.Human(); !strings.Contains(h, "milestone: no active milestone") {
		t.Errorf("Human does not say why:\n%s", h)
	}
}
