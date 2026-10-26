// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package tracker_test

import (
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// An issue closing records when it closed, and reopening clears it, so the
// timeline reads an issue's actual end from the tracker (GH-82).
func TestClosingRecordsClosedAt(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		tr := p.Tracker()
		i, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "work"})
		if err != nil {
			t.Fatal(err)
		}
		if i.ClosedAt != "" {
			t.Fatalf("an open issue has closed_at %q", i.ClosedAt)
		}
		closeState, reopen := "close", "reopen"
		if _, err := tr.UpdateIssue("g/po-1", i.IID, tracker.IssueUpdate{State: &closeState}); err != nil {
			t.Fatal(err)
		}
		if got, _ := tr.GetIssue("g/po-1", i.IID); got.State != "closed" || got.ClosedAt == "" {
			t.Fatalf("a closed issue %+v", got)
		}
		if _, err := tr.UpdateIssue("g/po-1", i.IID, tracker.IssueUpdate{State: &reopen}); err != nil {
			t.Fatal(err)
		}
		if got, _ := tr.GetIssue("g/po-1", i.IID); got.ClosedAt != "" {
			t.Fatalf("a reopened issue keeps closed_at %q", got.ClosedAt)
		}
	})
}
