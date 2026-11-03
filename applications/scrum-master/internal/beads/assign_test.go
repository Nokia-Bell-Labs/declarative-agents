// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"strings"
	"testing"
)

// TestAssignThenPop walks an issue a person assigned: it stays ready, can go
// to someone else until work starts, its assignee's pop takes it, and only
// that person pops it (srd041 R9.1).
func TestAssignThenPop(t *testing.T) {
	f := newFixture(t, ready("sm-1"))
	alice, bob := f.clone("alice"), f.clone("bob")

	if err := alice.Assign("sm-1", "carol"); err != nil {
		t.Fatal(err)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "carol" || got.State != Ready {
		t.Fatalf("after assign = %+v, want ready for carol", got)
	}
	if err := bob.Assign("sm-1", "bob"); err != nil {
		t.Fatalf("reassign before work started = %v", err)
	}
	if _, err := alice.Pop("sm-1"); err == nil || !strings.Contains(err.Error(), "bob") {
		t.Fatalf("alice's pop of bob's issue = %v, want a refusal naming bob", err)
	}
	if _, err := bob.Pop("sm-1"); err != nil {
		t.Fatal(err)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "bob" || got.State != Taken {
		t.Fatalf("after pop = %+v, want taken by bob", got)
	}
	if err := alice.Assign("sm-1", "alice"); !errors.Is(err, ErrRefused) {
		t.Fatalf("reassign of a taken issue = %v, want refused", err)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "bob" {
		t.Fatalf("a refused assign changed the assignee: %+v", got)
	}
}

// TestUnassign holds the giving back to the assignee alone (srd041 R9.2), from
// each open state an assigned issue can be in.
func TestUnassign(t *testing.T) {
	f := newFixture(t, ready("sm-1"), ready("sm-2"), ready("sm-3"))
	alice, bob := f.clone("alice"), f.clone("bob")

	// Assigned and not started.
	if err := alice.Assign("sm-1", "bob"); err != nil {
		t.Fatal(err)
	}
	if err := alice.Unassign("sm-1"); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "bob") {
		t.Fatalf("alice giving back bob's = %v, want refused naming bob", err)
	}
	if err := bob.Unassign("sm-1"); err != nil {
		t.Fatal(err)
	}
	if got := f.remoteIssue("sm-1"); got.Assignee != "" || got.State != Ready {
		t.Fatalf("given back = %+v, want ready and unassigned", got)
	}

	// Taken.
	if _, err := bob.Pop("sm-2"); err != nil {
		t.Fatal(err)
	}
	if err := alice.Unassign("sm-2"); !errors.Is(err, ErrRefused) {
		t.Fatalf("alice giving back bob's taken issue = %v, want refused", err)
	}
	if err := bob.Unassign("sm-2"); err != nil {
		t.Fatal(err)
	}
	if got := f.remoteIssue("sm-2"); got.Assignee != "" || got.State != Ready {
		t.Fatalf("taken, given back = %+v, want ready and unassigned", got)
	}

	// In progress.
	if _, err := bob.Pop("sm-3"); err != nil {
		t.Fatal(err)
	}
	if err := bob.Fire("sm-3", ActorPerson, InProgress); err != nil {
		t.Fatal(err)
	}
	if err := bob.Unassign("sm-3"); err != nil {
		t.Fatal(err)
	}
	if got := f.remoteIssue("sm-3"); got.Assignee != "" || got.State != Ready || got.Status != StatusOpen {
		t.Fatalf("in progress, given back = %+v, want ready and unassigned", got)
	}

	// Nobody's: nothing to do.
	if err := bob.Unassign("sm-1"); err != nil {
		t.Fatalf("giving back nobody's = %v", err)
	}
}

// TestAssignFollowsTheProgramsRows: assigning is a row like any other, so a
// program's table written before the rows existed refuses, naming the file
// (srd041 R8.1).
func TestAssignFollowsTheProgramsRows(t *testing.T) {
	f := newFixture(t, ready("sm-1"))
	alice := f.clone("alice")
	alice.LoadStatesFrom(func() ([]Row, error) {
		return []Row{{From: []string{Ready}, To: Taken, Actor: ActorPop, Assignee: EffectSet}}, nil
	})
	if err := alice.Assign("sm-1", "bob"); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), StatesFile) {
		t.Fatalf("assign without the row = %v, want refused naming %s", err, StatesFile)
	}
}

// TestPopOfATakenIssueChangesNothing keeps the second pop's no-op now that an
// assignee's first pop moves the state.
func TestPopOfATakenIssueChangesNothing(t *testing.T) {
	f := newFixture(t, ready("sm-1"))
	alice := f.clone("alice")
	if _, err := alice.Pop("sm-1"); err != nil {
		t.Fatal(err)
	}
	if err := alice.Fire("sm-1", ActorPerson, InProgress); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Pop("sm-1"); err != nil {
		t.Fatal(err)
	}
	if got := f.remoteIssue("sm-1"); got.State != InProgress {
		t.Fatalf("second pop moved the state: %+v", got)
	}
}
