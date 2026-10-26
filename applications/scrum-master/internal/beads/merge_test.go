// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"strings"
	"testing"
)

// TestMergeRefusesAReplayOutsideTheTable merges a clone that started an
// issue over a tracker that closed it meanwhile: closed to in-progress is not
// a row, so the merge keeps closed and records the refusal as a comment,
// while a plain field and a label both sides changed merge by their rules.
func TestMergeRefusesAReplayOutsideTheTable(t *testing.T) {
	base := ready("sm-1")
	base.State, base.Assignee, base.UpdatedAt = Taken, "alice", "2026-11-02T09:00:00Z"

	ours := base
	ours.State, ours.StateActor, ours.Status = InProgress, ActorPerson, StatusInProgress
	ours.Title, ours.UpdatedAt = "renamed by ours", "2026-11-02T09:05:00Z"
	ours.Labels = []string{"plane::data", "kind::build"}

	theirs := base
	theirs.State, theirs.StateActor, theirs.Status = Closed, ActorPerson, StatusClosed
	theirs.Title, theirs.UpdatedAt = "renamed by theirs", "2026-11-02T09:03:00Z"
	theirs.Labels = nil

	m := mergeIssue(Table, base, ours, theirs, "2026-11-02T09:10:00Z")
	if m.State != Closed || m.Status != StatusClosed {
		t.Fatalf("state = %s/%s, want closed kept", m.State, m.Status)
	}
	if len(m.Comments) != 1 || !strings.Contains(m.Comments[0].Text, "closed to in-progress by person") {
		t.Fatalf("comments = %+v, want the refusal recorded", m.Comments)
	}
	if m.Title != "renamed by ours" {
		t.Fatalf("title = %q, want the later writer's", m.Title)
	}
	if strings.Join(m.Labels, ",") != "kind::build" {
		t.Fatalf("labels = %v, want theirs' removal and ours' addition", m.Labels)
	}
}
