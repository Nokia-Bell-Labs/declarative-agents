// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import "testing"

// TestIssueLineNamesAnUnfiledIssueByItsRepository shows a dry run's
// unfiled issue reads by its repository, never as #0.
func TestIssueLineNamesAnUnfiledIssueByItsRepository(t *testing.T) {
	if got := issueLine("sprint-end issue", IssueAction{Project: "g/ops", Action: "filed"}); got != "sprint-end issue in g/ops: filed, not yet numbered" {
		t.Errorf("unfiled = %q", got)
	}
	if got := issueLine("sprint-end issue", IssueAction{Project: "g/ops", IID: 4, Action: "unchanged"}); got != "sprint-end issue g/ops#4: unchanged" {
		t.Errorf("filed = %q", got)
	}
}

// TestBacklogLineSaysWhenThereIsNoVelocity shows the Product Backlog line
// says there is no velocity rather than recommending nothing.
func TestBacklogLineSaysWhenThereIsNoVelocity(t *testing.T) {
	for _, c := range []struct {
		b    Backlog
		want string
	}{
		{Backlog{}, "Product Backlog, no velocity to recommend from: no sprint-end summary records a story done; pass --velocity"},
		{Backlog{Velocity: 1, Measured: true}, "Product Backlog, velocity 1 (measured true), no story fits"},
		{Backlog{Velocity: 5, Recommended: []string{"st001-a", "st002-b"}}, "Product Backlog, velocity 5 (measured false), recommended st001-a, st002-b"},
	} {
		if got := backlogLine(c.b); got != c.want {
			t.Errorf("backlogLine(%+v) = %q, want %q", c.b, got, c.want)
		}
	}
}
