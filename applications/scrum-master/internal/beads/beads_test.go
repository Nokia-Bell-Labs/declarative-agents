// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
)

// TestPopAndCloseCitesTheMergeCommit walks one issue's life on beads
// (rel03.0-uc001): pop assigns only the caller, the person starts it, the
// observed merge closes it with the commit cited, a second close changes
// nothing, and every write is in the tracker repository before the command
// returns.
func TestPopAndCloseCitesTheMergeCommit(t *testing.T) {
	other := ready("sm-2")
	other.State, other.Assignee = Taken, "bob"
	f := newFixture(t, ready("sm-1"), other)
	alice := f.clone("alice")

	if _, err := alice.Pop("sm-2"); err == nil || !strings.Contains(err.Error(), "bob") {
		t.Fatalf("pop of bob's issue = %v, want a refusal naming bob", err)
	}

	p, err := alice.Pop("sm-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Provisional || p.Lost || p.Read.Commit == "" {
		t.Fatalf("pop = %+v, want a confirmed pop naming the commit it read", p)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "alice" || got.State != Taken {
		t.Fatalf("tracker after pop = %+v, want taken by alice", got)
	}

	if err := alice.Fire("sm-1", ActorPerson, InProgress); err != nil {
		t.Fatal(err)
	}
	c, err := alice.Close("sm-1", "4f2a9c1")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Changed {
		t.Fatal("first close reported no change")
	}
	got := f.remoteIssue("sm-1")
	last := got.Comments[len(got.Comments)-1]
	if got.Status != StatusClosed || got.State != Closed || got.CloseReason != "4f2a9c1" || !strings.Contains(last.Text, "4f2a9c1") {
		t.Fatalf("tracker after close = %+v, want closed citing 4f2a9c1", got)
	}

	again, err := alice.Close("sm-1", "4f2a9c1")
	if err != nil || again.Changed {
		t.Fatalf("second close = %+v, %v, want no change", again, err)
	}
	if n := len(f.remoteIssue("sm-1").Comments); n != len(got.Comments) {
		t.Fatalf("second close added a comment: %d, want %d", n, len(got.Comments))
	}
}

// TestStateTableReplaysOnBeads fires every row of the srd041 table on beads
// with its actor and checks the assignee effect, then proves transitions
// outside the table are refused with the attempted transition named.
func TestStateTableReplaysOnBeads(t *testing.T) {
	for _, row := range Table {
		for _, from := range row.From {
			name := from + " to " + row.To + " by " + row.Actor
			t.Run(name, func(t *testing.T) {
				i := ready("sm-1")
				i.State = from
				switch from {
				case Taken, InProgress:
					i.Assignee = "alice"
				case Closed:
					i.Status = StatusClosed
				case NeedsTriage:
					i.Labels = append(i.Labels, "needs-triage")
				}
				if from == NeedsTriage && row.To == Taken {
					i.Assignee = "alice" // the author took it as they filed it
				}
				if row.Requires != "" {
					i.Labels = append(i.Labels, row.Requires)
				}
				if row.Kind == KindSprintGoal {
					i.Labels = []string{"plane::management"}
					i.Description = sprintgoal.Marker("04.0", []string{"in001-reliable-agents#R1"})
				}
				if row.To == "-"+Waiting || row.To == "-"+Blocked {
					i.Labels = append(i.Labels, strings.TrimPrefix(row.To, "-"))
				}
				f := newFixture(t, i)
				alice := f.clone("alice")

				if err := alice.Fire("sm-1", row.Actor, row.To); err != nil {
					t.Fatalf("%s refused: %v", name, err)
				}
				got := f.remoteIssue("sm-1")
				switch row.Assignee {
				case EffectSet:
					if got.Assignee != "alice" {
						t.Fatalf("assignee = %q, want alice", got.Assignee)
					}
				case EffectClear:
					if got.Assignee != "" {
						t.Fatalf("assignee = %q, want none", got.Assignee)
					}
				case EffectNone:
					if got.Assignee != i.Assignee {
						t.Fatalf("assignee = %q, want it unchanged as %q", got.Assignee, i.Assignee)
					}
				}
				if overlay, ok := strings.CutPrefix(row.To, "+"); ok && !slices.Contains(got.Labels, overlay) {
					t.Fatalf("labels %v lack %s", got.Labels, overlay)
				}
				if overlay, ok := strings.CutPrefix(row.To, "-"); ok && slices.Contains(got.Labels, overlay) {
					t.Fatalf("labels %v still carry %s", got.Labels, overlay)
				}
				if !strings.HasPrefix(row.To, "+") && !strings.HasPrefix(row.To, "-") && got.State != row.To {
					t.Fatalf("state = %s, want %s", got.State, row.To)
				}
			})
		}
	}

	closed := ready("sm-1")
	closed.State, closed.Status = Closed, StatusClosed
	theirs := ready("sm-2")
	theirs.State, theirs.Assignee = Taken, "bob"
	f := newFixture(t, closed, theirs)
	alice := f.clone("alice")
	for _, c := range []struct{ id, actor, to string }{
		{"sm-1", ActorPerson, InProgress}, // closed to in-progress is not a row
		{"sm-2", ActorPerson, Ready},      // only the assignee releases an issue (R9.2)
		{"sm-2", ActorLint, Closed},       // lint never closes (R9.3)
	} {
		err := alice.Fire(c.id, c.actor, c.to)
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), c.to) {
			t.Fatalf("%s to %s by %s = %v, want a refusal naming the transition", c.id, c.to, c.actor, err)
		}
	}
}

// TestObjectiveIssueIsSprintGoal lets track-goals close an objective issue
// filed before release 04.0 as it closes a Sprint Goal, and refuses that
// close on an issue with neither marker (srd041 R8, srd034 R3.7).
func TestObjectiveIssueIsSprintGoal(t *testing.T) {
	old := ready("sm-1")
	old.Labels = []string{"plane::management"}
	old.Description = "body\n<!-- scrum-master goals objective=obj001-reliable-agents#R1 release=01.0 -->\n"
	plain := ready("sm-2")
	plain.Labels = []string{"plane::management"}
	f := newFixture(t, old, plain)
	alice := f.clone("alice")
	if err := alice.Fire("sm-1", ActorTrackGoals, Closed); err != nil {
		t.Fatalf("track-goals close of an objective issue refused: %v", err)
	}
	if got := f.remoteIssue("sm-1"); got.State != Closed {
		t.Fatalf("state = %s, want closed", got.State)
	}
	if err := alice.Fire("sm-2", ActorTrackGoals, Closed); !errors.Is(err, ErrRefused) {
		t.Fatalf("track-goals closed an issue that is no Sprint Goal: %v", err)
	}
}

// TestContendedPopKeepsTheEarlierTaker runs two clones against one tracker
// repository (rel03.0-uc002): disjoint writes made in each other's window
// both land, and when both pop one issue the earlier push keeps it and the
// later pop is told it lost.
func TestContendedPopKeepsTheEarlierTaker(t *testing.T) {
	f := newFixture(t, ready("sm-1"), ready("sm-2"), ready("sm-3"))
	alice, bob := f.clone("alice"), f.clone("bob")

	bob.beforeWrite = func() {
		bob.beforeWrite = nil
		if err := alice.Comment("sm-2", "alice was here"); err != nil {
			t.Fatal(err)
		}
	}
	if err := bob.Comment("sm-3", "bob was here"); err != nil {
		t.Fatal(err)
	}
	if c := f.remoteIssue("sm-2").Comments; len(c) != 1 || c[0].Text != "alice was here" {
		t.Fatalf("sm-2 comments = %+v, want alice's", c)
	}
	if c := f.remoteIssue("sm-3").Comments; len(c) != 1 || c[0].Text != "bob was here" {
		t.Fatalf("sm-3 comments = %+v, want bob's", c)
	}

	bob.beforeWrite = func() {
		bob.beforeWrite = nil
		if p, err := alice.Pop("sm-1"); err != nil || p.Lost {
			t.Fatalf("alice's pop = %+v, %v", p, err)
		}
	}
	p, err := bob.Pop("sm-1")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Lost || p.LostTo != "alice" {
		t.Fatalf("bob's pop = %+v, want lost to alice", p)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "alice" {
		t.Fatalf("tracker assignee = %q, want alice, the earlier push", got.Assignee)
	}
}

// TestOfflineReadNamesTheLastPulledCommit takes one clone offline
// (rel03.0-uc002): its reads serve the cache and name the last pulled
// commit and time, its pops are provisional, and on reconnecting one
// confirms while the other reports lost to the taker who pushed first.
func TestOfflineReadNamesTheLastPulledCommit(t *testing.T) {
	f := newFixture(t, ready("sm-1"), ready("sm-2"))
	alice, bob := f.clone("alice"), f.clone("bob")

	_, online, err := bob.Issues()
	if err != nil || online.Offline || online.Commit == "" {
		t.Fatalf("online read = %+v, %v", online, err)
	}

	f.offline(bob)
	if _, err := alice.Pop("sm-1"); err != nil {
		t.Fatal(err)
	}
	_, cached, err := bob.Issues()
	if err != nil {
		t.Fatal(err)
	}
	if !cached.Offline || cached.Commit != online.Commit || !cached.At.Equal(online.At) {
		t.Fatalf("offline read = %+v, want the cache named by %s at %s", cached, online.Commit, online.At)
	}

	stale, err := bob.Pop("sm-1")
	if err != nil || !stale.Provisional {
		t.Fatalf("offline pop of sm-1 = %+v, %v, want provisional", stale, err)
	}
	fresh, err := bob.Pop("sm-2")
	if err != nil || !fresh.Provisional {
		t.Fatalf("offline pop of sm-2 = %+v, %v, want provisional", fresh, err)
	}

	f.online(bob)
	r, err := bob.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"sm-1": "lost to alice", "sm-2": "confirmed"}
	if len(r.Reconciled) != len(want) {
		t.Fatalf("reconcile = %+v, want %v", r, want)
	}
	for _, x := range r.Reconciled {
		if x.Outcome != want[x.ID] {
			t.Fatalf("reconcile %s = %q, want %q", x.ID, x.Outcome, want[x.ID])
		}
	}
	if got := f.remoteIssue("sm-2"); got.Assignee != "bob" {
		t.Fatalf("sm-2 assignee = %q, want bob's queued pop pushed", got.Assignee)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "alice" {
		t.Fatalf("sm-1 assignee = %q, want alice kept", got.Assignee)
	}
}

// TestOfflineReadAfterAWrite is srd042 R3.3 after a write: the commit a
// clone pushed is what it and its origin last agreed on, so a later read
// that cannot reach the origin names that commit, not the pull before it.
func TestOfflineReadAfterAWrite(t *testing.T) {
	f := newFixture(t, ready("sm-1"))
	bob := f.clone("bob")
	if _, _, err := bob.Issues(); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Pop("sm-1"); err != nil {
		t.Fatal(err)
	}
	pushed, err := bob.git("rev-parse", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	f.offline(bob)
	_, cached, err := bob.Issues()
	if err != nil || !cached.Offline || cached.Commit != pushed {
		t.Fatalf("offline read after a write = %+v, %v; want the pushed commit %s", cached, err, pushed)
	}
}
