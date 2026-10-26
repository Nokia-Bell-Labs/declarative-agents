// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TestMilestonesOnBeads holds the milestone half of the tracker's
// semantics (srd042 R2.2): ensuring a group milestone twice leaves one and
// reports what changed, a lookup walks from the project up its groups, and
// the active milestones of a project are its own and its groups', by due
// date.
func TestMilestonesOnBeads(t *testing.T) {
	tr := newTestTracker(t, "alice")
	sprint := tracker.Milestone{Title: "01.1", Description: "sprint 01.1", StartDate: "2026-11-02", DueDate: "2026-11-13"}

	for _, want := range []string{tracker.MilestoneCreated, tracker.MilestoneUnchanged} {
		if got, err := tr.EnsureMilestone("g", sprint); err != nil || got != want {
			t.Fatalf("ensure = %q, %v, want %q", got, err, want)
		}
	}
	moved := sprint
	moved.DueDate = "2026-11-20"
	if got, err := tr.EnsureMilestone("g", moved); err != nil || got != tracker.MilestoneUpdated {
		t.Fatalf("ensure moved = %q, %v, want updated", got, err)
	}

	m, err := tr.FindMilestone("g/po-1", "01.1")
	if err != nil || m.DueDate != "2026-11-20" {
		t.Fatalf("find from the project = %+v, %v, want the group's milestone", m, err)
	}
	if _, err := tr.FindMilestone("g/po-1", "09.9"); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("find missing = %v, want ErrNotFound", err)
	}

	if _, err := tr.EnsureMilestone("g/po-1", tracker.Milestone{Title: "spike", DueDate: "2026-11-06"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.EnsureMilestone("other", tracker.Milestone{Title: "elsewhere", DueDate: "2026-11-01"}); err != nil {
		t.Fatal(err)
	}
	active, err := tr.ActiveMilestones("g/po-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0].Title != "spike" || active[1].Title != "01.1" {
		t.Fatalf("active = %+v, want spike then 01.1, nothing from another group", active)
	}
}

// TestMilestonesMergeAcrossClones: two clones ensure different milestones
// between each other's pulls, and both land in the tracker repository.
func TestMilestonesMergeAcrossClones(t *testing.T) {
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	open := func(user string) *Tracker {
		clone := filepath.Join(root, user+"-tracker")
		beadstest.Clone(t, remote, clone, user)
		return NewTracker(clone, user, root)
	}
	alice, bob := open("alice"), open("bob")

	bob.Store.beforeWrite = func() {
		bob.Store.beforeWrite = nil
		if _, err := alice.EnsureMilestone("g", tracker.Milestone{Title: "01.1", DueDate: "2026-11-13"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := bob.EnsureMilestone("g", tracker.Milestone{Title: "01.2", DueDate: "2026-11-27"}); err != nil {
		t.Fatal(err)
	}
	carol := open("carol")
	active, err := carol.ActiveMilestones("g/po-1")
	if err != nil || len(active) != 2 {
		t.Fatalf("milestones a fresh clone reads = %+v, %v, want both", active, err)
	}
}
