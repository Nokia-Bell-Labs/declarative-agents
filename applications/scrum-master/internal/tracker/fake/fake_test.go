// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package fake

import (
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func TestFakeEnsureMilestoneCountsOnlyChanges(t *testing.T) {
	f := New()
	m := tracker.Milestone{Title: "01.2", DueDate: "2026-11-06"}
	for i, want := range []string{tracker.MilestoneCreated, tracker.MilestoneUnchanged} {
		if got, _ := f.EnsureMilestone("program", m); got != want {
			t.Fatalf("run %d: %s, want %s", i, got, want)
		}
	}
	m.DueDate = "2026-11-13"
	if got, _ := f.EnsureMilestone("program", m); got != tracker.MilestoneUpdated || f.Writes != 2 || len(f.Milestones) != 1 {
		t.Fatalf("moved: %s writes=%d milestones=%d", got, f.Writes, len(f.Milestones))
	}
}
