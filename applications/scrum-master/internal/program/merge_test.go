// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// TestMergeDeliversOnBeads is the beads delivery (srd042 R2.3): a docs/
// change merges only once the audit passes, the merge is --no-ff on the
// default branch of the main checkout and is pushed, it cites the issue and
// each issue it also closes, and each closes on it. A second merge changes
// nothing.
func TestMergeDeliversOnBeads(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	objective, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Objective R1", Labels: []string{"plane::management"}})
	work, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Change the use case", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::spec", "component::po-1"}})
	p, err := Pop(tr, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p.Worktree, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"docs/use-case.md", "docs/BROKEN"} {
		if err := os.WriteFile(filepath.Join(p.Worktree, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Change the use case")
	gate := MergeGate{Command: "test ! -e docs/BROKEN", Dir: p.Worktree}
	closes := []string{"g/po-1#1"}

	if _, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, closes, false); err == nil || !strings.Contains(err.Error(), "nothing merged") {
		t.Fatalf("merge with a failing audit = %v, want a refusal", err)
	}
	if out := beadstest.Git(t, checkout, "log", "--merges", "--oneline", "main"); out != "" {
		t.Fatalf("a refused merge merged: %s", out)
	}

	beadstest.Git(t, p.Worktree, "rm", "-q", "docs/BROKEN")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Fix the corpus")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	if _, err := Approve(reviewer(t, tr, checkout, "bob"), ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	m, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, closes, false)
	if err != nil {
		t.Fatal(err)
	}
	head := beadstest.Git(t, checkout, "rev-parse", "main")
	remote := beadstest.Git(t, checkout, "ls-remote", "origin", "refs/heads/main")
	message := beadstest.Git(t, checkout, "log", "-1", "--format=%s", "main")
	if !m.Audit || m.Commit != head || !strings.HasPrefix(remote, head) || !strings.Contains(message, "g/po-1#2") || !strings.Contains(message, "closes g/po-1#1") {
		t.Fatalf("merge = %+v, head %s, origin %s, message %q", m, head, remote, message)
	}
	for _, iid := range []int{work.IID, objective.IID} {
		if got, _ := tr.GetIssue("g/po-1", iid); got.State != "closed" {
			t.Fatalf("g/po-1#%d = %s after the merge, want closed", iid, got.State)
		}
	}

	again, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, nil, false)
	if err != nil || again.Merged || again.Commit != head {
		t.Fatalf("second merge = %+v, %v, want nothing new", again, err)
	}

	// A dry run of the delivered issue says nothing would happen and is
	// refused, rather than reporting it would merge (GH-20).
	if _, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, nil, true); err == nil || !strings.Contains(err.Error(), "already delivered") {
		t.Fatalf("dry run of a delivered issue = %v, want it refused as already delivered", err)
	}
}

// TestMergeNeedsTheBeadsTracker: a tracker that does not close issues on a
// local merge is refused.
func TestMergeNeedsTheBeadsTracker(t *testing.T) {
	f := fake.New()
	i := admitted(f, "Score one agent")
	if _, err := Merge(f, ExecGit{}, "g/po-1", "g/po-1", i.IID, "1-score-one-agent", ".", MergeGate{}, nil, false); err == nil || !strings.Contains(err.Error(), "scrum-master issue merge serves the beads tracker") {
		t.Fatalf("merge on a tracker without local merges = %v, want a refusal naming the beads tracker", err)
	}
}

// reviewer is another person on tr's tracker repository, with their own
// clone of it, reading the same code checkout.
func reviewer(t *testing.T, tr *beads.Tracker, checkout, user string) *beads.Tracker {
	t.Helper()
	remote := beadstest.Git(t, tr.Store.Dir, "remote", "get-url", "origin")
	clone := filepath.Join(filepath.Dir(tr.Store.Dir), user+"-tracker")
	beadstest.Clone(t, remote, clone, user)
	return beads.NewTracker(clone, user, checkout)
}

// TestMergeNeedsAnApprovalOfItsTip holds merge to srd013 R5.5 and R5.6 under
// a policy that requires approval: no merge without an approval, none on the assignee's own, none on an
// approval of an earlier commit; another person's approval of the tip
// merges. For an issue nobody took, the approver must not be the person
// merging.
func TestMergeNeedsAnApprovalOfItsTip(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	bob := reviewer(t, alice, checkout, "bob")
	work, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: "Score one agent", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	p, err := Pop(alice, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	commit := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(p.Worktree, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, p.Worktree, "add", "-A")
		beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "add "+name)
		beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	}
	merge := func() (MergeResult, error) {
		return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, MergeGate{RequireApproval: true}, nil, false)
	}
	commit("score.go")

	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("merge with no approval = %v, want a refusal", err)
	}
	if _, err := Approve(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err == nil || !strings.Contains(err.Error(), "assignee") {
		t.Fatalf("the assignee's approval = %v, want a refusal", err)
	}
	a, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout)
	if err != nil || !a.Posted || a.Commit == "" {
		t.Fatalf("bob's approval = %+v, %v", a, err)
	}
	if again, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err != nil || again.Posted {
		t.Fatalf("bob's second approval = %+v, %v, want nothing posted", again, err)
	}

	commit("later.go")
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), a.Commit[:12]) {
		t.Fatalf("merge after a later commit = %v, want a refusal naming the approved commit", err)
	}
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err != nil {
		t.Fatal(err)
	}
	if m, err := merge(); err != nil || !m.Merged {
		t.Fatalf("merge with bob's approval of the tip = %+v, %v", m, err)
	}

	status, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: "Weekly status", Labels: []string{"plane::management"}})
	beadstest.Git(t, checkout, "checkout", "-q", "-b", "status-week", "main")
	beadstest.Git(t, checkout, "commit", "-q", "--allow-empty", "-m", "status")
	beadstest.Git(t, checkout, "push", "-q", "-u", "origin", "status-week")
	beadstest.Git(t, checkout, "checkout", "-q", "main")
	statusMerge := func() (MergeResult, error) {
		return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", status.IID, "status-week", checkout, MergeGate{RequireApproval: true}, nil, false)
	}
	if _, err := Approve(alice, ExecGit{}, "g/po-1", "g/po-1", status.IID, "status-week", checkout); err != nil {
		t.Fatal(err)
	}
	if _, err := statusMerge(); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("merge on the merger's own approval = %v, want a refusal", err)
	}
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", status.IID, "status-week", checkout); err != nil {
		t.Fatal(err)
	}
	if m, err := statusMerge(); err != nil || !m.Merged {
		t.Fatalf("merge of the unassigned issue with bob's approval = %+v, %v", m, err)
	}
}

// TestMergeWithoutApprovalByDefault is srd013 R5.5 under the default
// policy: an unapproved branch merges and closes its issue, and an
// approval of the tip, when one was given, is still reported.
func TestMergeWithoutApprovalByDefault(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	bob := reviewer(t, alice, checkout, "bob")
	deliver := func(title string, approve bool) MergeResult {
		t.Helper()
		work, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m", Description: goodBody,
			Labels: []string{"plane::data", "kind::build", "component::po-1"}})
		p, err := Pop(alice, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
		if err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, p.Worktree, "commit", "-q", "--allow-empty", "-m", title)
		beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
		if approve {
			if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err != nil {
				t.Fatal(err)
			}
		}
		m, err := Merge(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, MergeGate{}, nil, false)
		if err != nil || !m.Merged {
			t.Fatalf("merge of %q = %+v, %v", title, m, err)
		}
		if got, _ := alice.GetIssue("g/po-1", work.IID); got.State != "closed" {
			t.Fatalf("%q is %s after its merge", title, got.State)
		}
		return m
	}
	if m := deliver("Score one agent", false); m.ApprovedBy != "" {
		t.Fatalf("unapproved merge reports approver %q", m.ApprovedBy)
	}
	if m := deliver("Score a batch", true); m.ApprovedBy != "bob" {
		t.Fatalf("approved merge reports approver %q, want bob", m.ApprovedBy)
	}
}

// TestSprintEndMergeRecordsReportedComplete is a sprint-end merge (srd038
// R2): its summary must be in format, a Sprint Goal it closes gets a
// reported_complete event on its ledger, a --closes issue another merge
// already closed is reported, not refused, and each issue it closes
// records this merge, though an earlier merge cites it too (srd013 R5.1).
func TestSprintEndMergeRecordsReportedComplete(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	goal, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal: scoring", Labels: []string{"plane::management"},
		Description: "Goal\n\n" + sprintgoal.Marker("01.0", []string{"in001-scoring#R1"}) + "\n"})
	done, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Closed earlier", Labels: []string{"plane::management"}})
	closed := "close"
	if _, err := tr.UpdateIssue("g/po-1", done.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	// A team closed its own Sprint Goal before the program's merge names it.
	teamGoal, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal: grading", Labels: []string{"plane::management"},
		Description: "Goal\n\n" + sprintgoal.Marker("01.0", []string{"in001-scoring#R2"}) + "\n"})
	if _, err := tr.UpdateIssue("g/po-1", teamGoal.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	end, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: sprint end 01.0", Labels: []string{"plane::management"}})
	beadstest.Git(t, checkout, "checkout", "-q", "-b", "earlier", "main")
	beadstest.Git(t, checkout, "commit", "-q", "--allow-empty", "-m", "earlier work")
	beadstest.Git(t, checkout, "checkout", "-q", "main")
	beadstest.Git(t, checkout, "merge", "-q", "--no-ff", "earlier", "-m", fmt.Sprintf("Merge earlier (g/po-1#%d)", goal.IID))
	beadstest.Git(t, checkout, "push", "-q", "origin", "main")
	earlier := beadstest.Git(t, checkout, "rev-parse", "HEAD")
	p, err := Pop(tr, ExecGit{}, "g/po-1", end.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(p.Worktree, sprintgoal.SummaryDir, "01.0.md")
	if err := os.MkdirAll(filepath.Dir(summary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(summary, []byte("# Sprint end 01.0\n\nScoring shipped.\n\n## Stories done\n\n- st001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Summary")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	bob := reviewer(t, tr, checkout, "bob")
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", end.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	gate := MergeGate{Command: "true", Dir: p.Worktree}
	closes := []string{fmt.Sprintf("g/po-1#%d", goal.IID), fmt.Sprintf("g/po-1#%d", done.IID), fmt.Sprintf("g/po-1#%d", teamGoal.IID)}
	if _, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", end.IID, p.Branch, p.Worktree, gate, closes, false); err == nil || !strings.Contains(err.Error(), "is not project#story") {
		t.Fatalf("merge of a summary with a bare story id = %v, want a refusal", err)
	}

	if err := os.WriteFile(summary, []byte("# Sprint end 01.0\n\nScoring shipped.\n\n## Stories done\n\n- g/po-1#st001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "commit", "-q", "-am", "Name the story's project")
	beadstest.Git(t, p.Worktree, "push", "-q")
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", end.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	m, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", end.IID, p.Branch, p.Worktree, gate, closes, false)
	if err != nil {
		t.Fatal(err)
	}
	// The team-closed Sprint Goal is reported complete too; the closed
	// issue that is no Sprint Goal is not (#150).
	// The subject cites only what the merge closes: the issues another
	// merge closed are reported, not cited (#188).
	subject := beadstest.Git(t, checkout, "log", "-1", "--format=%s", m.Commit)
	if !strings.Contains(subject, "closes "+closes[0]) || strings.Contains(subject, closes[1]+")") || strings.Contains(subject, closes[1]+",") || strings.Contains(subject, closes[2]) {
		t.Fatalf("merge subject %q: want %s cited and the already-closed %s and %s left out", subject, closes[0], closes[1], closes[2])
	}
	if !slices.Equal(m.Reported, []string{closes[0], closes[2]}) || !slices.Equal(m.AlreadyClosed, []string{closes[1], closes[2]}) {
		t.Fatalf("merge = %+v", m)
	}
	if !m.Audit || m.AuditCommand != "true" || m.AuditReason != "docs/ changed" {
		t.Fatalf("merge audit = %v %q %q, want the gate's command and reason named", m.Audit, m.AuditCommand, m.AuditReason)
	}
	teamNotes, _ := tr.ListNotes("g/po-1", teamGoal.IID)
	if n := countEvents(teamNotes, "reported_complete"); n != 1 {
		t.Fatalf("the team-closed Sprint Goal has %d reported_complete events, want 1", n)
	}
	if posted, already, err := reportClosed(tr, closes[2], "another-commit"); err != nil || posted || !already {
		t.Fatalf("reporting it again = posted %v, already %v, %v; want already reported, nothing posted (GH-24)", posted, already, err)
	}
	if teamNotes, _ = tr.ListNotes("g/po-1", teamGoal.IID); countEvents(teamNotes, "reported_complete") != 1 {
		t.Fatal("a second report posted a second event")
	}
	notes, _ := tr.ListNotes("g/po-1", goal.IID)
	if !slices.ContainsFunc(notes, func(n tracker.Note) bool { return strings.Contains(n.Body, "Closed by merge commit "+m.Commit) }) ||
		slices.ContainsFunc(notes, func(n tracker.Note) bool { return strings.Contains(n.Body, earlier) }) {
		t.Fatalf("the Sprint Goal does not record the merge %s that closed it (earlier merge %s)", m.Commit, earlier)
	}
	found := false
	for _, n := range notes {
		if e, ok := events.Parse(n.Body); ok && e.Name == "reported_complete" && e.Values("commit")[0] == m.Commit {
			found = true
		}
	}
	if got, _ := tr.GetIssue("g/po-1", goal.IID); got.State != "closed" || !found {
		t.Fatalf("Sprint Goal %s, notes %+v; want closed with a reported_complete event", got.State, notes)
	}
}

// TestSprintEndMergeReadsTheSummaryThroughTextconv reads the summary on the
// branch as its textconv shows it, as git-crypt decrypts: stored reversed,
// line by line, it is only in format once read through rev.
func TestSprintEndMergeReadsTheSummaryThroughTextconv(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	// As in a git-crypt repository, the attributes are on main already.
	beadstest.Git(t, checkout, "config", "diff.rev.textconv", "rev <")
	os.WriteFile(filepath.Join(checkout, ".gitattributes"), []byte("docs/sprint-ends/*.md diff=rev\n"), 0o644)
	beadstest.Git(t, checkout, "add", ".gitattributes")
	beadstest.Git(t, checkout, "commit", "-q", "-m", "attributes")
	beadstest.Git(t, checkout, "push", "-q", "origin", "main")
	end, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Recurring: sprint end 01.0", Labels: []string{"plane::management"}})
	p, err := Pop(tr, ExecGit{}, "g/po-1", end.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(p.Worktree, sprintgoal.SummaryDir, "01.0.md")
	os.MkdirAll(filepath.Dir(summary), 0o755)
	plain := []string{"# Sprint end 01.0", "", "## Stories done", "", "- g/po-1#st001"}
	var stored []string
	for _, l := range plain {
		r := []rune(l)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		stored = append(stored, string(r))
	}
	os.WriteFile(summary, []byte(strings.Join(stored, "\n")+"\n"), 0o644)
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Summary")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	if _, err := Approve(reviewer(t, tr, checkout, "bob"), ExecGit{}, "g/po-1", "g/po-1", end.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	m, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", end.IID, p.Branch, p.Worktree, MergeGate{Command: "true", Dir: p.Worktree}, nil, false)
	if err != nil || !m.Merged {
		t.Fatalf("merge of a summary read through textconv = %+v, %v", m, err)
	}
}

func countEvents(notes []tracker.Note, name string) int {
	n := 0
	for _, note := range notes {
		if e, ok := events.Parse(note.Body); ok && e.Name == name {
			n++
		}
	}
	return n
}

// TestMergeReadsTheBranchAgainstOrigin merges a branch that changes no
// docs/, popped from origin's main after another checkout pushed a docs/
// change there, while the main checkout's own main is still behind: the
// branch's changes are read against origin, so no audit runs for the other
// change (#156).
func TestMergeReadsTheBranchAgainstOrigin(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	work, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Build the reader", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	other := filepath.Join(filepath.Dir(checkout), "elsewhere")
	beadstest.Git(t, filepath.Dir(checkout), "clone", "-q", filepath.Join(filepath.Dir(checkout), "po-1.git"), other)
	beadstest.Identity(t, other, "carol")
	if err := os.MkdirAll(filepath.Join(other, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "docs", "VISION.md"), []byte("# Vision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, other, "add", "-A")
	beadstest.Git(t, other, "commit", "-q", "-m", "Someone else's docs change")
	beadstest.Git(t, other, "push", "-q", "origin", "HEAD:main")

	// pop branches from origin's main, which now has the docs change, while
	// the main checkout's own main stays behind it.
	p, err := Pop(tr, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(p.Worktree, "reader.go"), []byte("package reader\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Build the reader")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	if _, err := Approve(reviewer(t, tr, checkout, "bob"), ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	m, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, MergeGate{Command: "false", Dir: p.Worktree}, nil, false)
	if err != nil || m.Audit || !m.Merged || m.AuditCommand != "false" || m.AuditReason != "no docs/ change" {
		t.Fatalf("merge = %+v, %v; want merged with no audit, the command and the reason named, as the branch changes no docs/", m, err)
	}
}
