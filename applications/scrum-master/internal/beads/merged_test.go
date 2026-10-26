// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
)

// deliver makes a branch with one commit in the checkout and merges it into
// main with --no-ff as user on date, the local merge that delivers work on
// beads (srd042 R2.3). A date with a time and zone is used as it is; a bare
// day is 10:00 UTC.
func deliver(t *testing.T, dir, branch, user, date, message string) {
	t.Helper()
	beadstest.Git(t, dir, "checkout", "-q", "-b", branch, "main")
	if err := os.WriteFile(dir+"/"+branch+".txt", []byte(branch), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, dir, "add", "-A")
	beadstest.Git(t, dir, "commit", "-q", "-m", "work on "+branch)
	beadstest.Git(t, dir, "checkout", "-q", "main")
	cmd := exec.Command("git", "-c", "user.name="+user, "-c", "user.email="+user+"@example.com",
		"merge", "-q", "--no-ff", branch, "-m", message)
	cmd.Dir = dir
	if !strings.Contains(date, "T") {
		date += "T10:00:00Z"
	}
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("merge %s: %v: %s", branch, err, out)
	}
}

// TestMergedHistoryFromMergeCommits reads a project's merged history on
// beads from the merge commits on its default branch: each merge is
// numbered by its place in that history, carries its author and merge
// date, and closes the issues its message cites; a plain commit is not a
// merge, and author and date filter it.
func TestMergedHistoryFromMergeCommits(t *testing.T) {
	tr := newTestTracker(t, "alice")
	dir := tr.checkout("g/po-1")
	deliver(t, dir, "1-score", "alice", "2026-10-20", "Merge 1-score (g/po-1#1)\n\nScore one agent.")
	beadstest.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "a plain commit on main")
	deliver(t, dir, "2-rubric", "bob", "2026-10-27", "Merge 2-rubric (g/po-1#2)")

	all, err := tr.Merges("g/po-1", "", "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].IID != 1 || all[1].IID != 2 {
		t.Fatalf("merged = %+v, want merges 1 and 2 only", all)
	}
	first := all[0]
	if first.Author != "alice" || first.MergedAt != "2026-10-20" || first.State != "merged" || first.TargetBranch != "main" ||
		first.Title != "Merge 1-score (g/po-1#1)" || !strings.Contains(first.Description, "Score one agent.") ||
		!strings.Contains(first.Description, "Closes g/po-1#1") {
		t.Fatalf("first merge = %+v", first)
	}

	if recent, _ := tr.Merges("g/po-1", "", "2026-10-26"); len(recent) != 1 || recent[0].IID != 2 || recent[0].Author != "bob" {
		t.Fatalf("since 2026-10-26 = %+v, want bob's merge 2 only", recent)
	}
	if mine, _ := tr.Merges("g/po-1", "alice", "2026-10-01"); len(mine) != 1 || mine[0].IID != 1 {
		t.Fatalf("alice's = %+v, want merge 1 only", mine)
	}
	if _, err := tr.Merges("g/po-1", "", "last week"); err == nil {
		t.Fatal("a since that is not a date was accepted")
	}
}

// TestMergeIsAttributedByItsTrailer reads a merge's author from the
// Merged-by trailer scrum-master issue merge writes, not from the git identity that
// made the commit.
func TestMergeIsAttributedByItsTrailer(t *testing.T) {
	tr := newTestTracker(t, "alice")
	dir := tr.checkout("g/po-1")
	deliver(t, dir, "1-work", "Build Bot", "2026-10-14", "Merge 1-work (g/po-1#1)\n\nMerged-by: alice")
	got, err := tr.Merges("g/po-1", "alice", "2026-10-12")
	if err != nil || len(got) != 1 || got[0].Author != "alice" || strings.Contains(got[0].Description, "Merged-by") {
		t.Fatalf("merges by alice = %+v, %v", got, err)
	}
	if bot, _ := tr.Merges("g/po-1", "Build Bot", "2026-10-12"); len(bot) != 0 {
		t.Fatalf("the git identity still counts: %+v", bot)
	}
}

// TestMergeDatesAreUTC reads a merge committed in a zone behind UTC on its
// UTC day, and compares since as an instant: 03:40 UTC on 2026-10-21 is
// 2026-10-21 whatever zone the committer or the reader is in (#147).
func TestMergeDatesAreUTC(t *testing.T) {
	t.Setenv("TZ", "America/Los_Angeles")
	tr := newTestTracker(t, "alice")
	dir := tr.checkout("g/po-1")
	deliver(t, dir, "1-late", "alice", "2026-10-20T20:40:00-07:00", "Merge 1-late (g/po-1#1)")
	got, err := tr.Merges("g/po-1", "", "2026-10-21")
	if err != nil || len(got) != 1 || got[0].MergedAt != "2026-10-21" {
		t.Fatalf("since 2026-10-21 = %+v, %v; want the merge, on 2026-10-21", got, err)
	}
	if after, _ := tr.Merges("g/po-1", "", "2026-10-21T03:41:00Z"); len(after) != 0 {
		t.Fatalf("since a minute after the merge = %+v, want none", after)
	}
	if at, _ := tr.Merges("g/po-1", "", "2026-10-21T03:40:00Z"); len(at) != 1 {
		t.Fatalf("since the merge's instant = %+v, want it", at)
	}
}
