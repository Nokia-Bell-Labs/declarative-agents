// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package tracker_test

import (
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// TestRouterServesEachProjectFromItsSystem routes by the longest
// configured path: a project's own entry, then its closest group, then the
// default; a call lands in exactly one system (#158).
func TestRouterServesEachProjectFromItsSystem(t *testing.T) {
	def, team, ops := fake.New(), fake.New(), fake.New()
	r := &tracker.Router{Default: def, Systems: map[string]tracker.Tracker{"acme/delivery": team, "acme/delivery/ops": ops}}
	for path, want := range map[string]tracker.Tracker{
		"acme/system":             def,
		"acme/delivery/fleet":     team,
		"acme/delivery":           team,
		"acme/delivery/ops":       ops,
		"acme/delivery/ops/infra": ops,
		"acme/deliveryx/fleet":    def, // a prefix that is not a group boundary
	} {
		if got := r.For(path); got != want {
			t.Errorf("For(%s) chose the wrong system", path)
		}
	}
	if _, err := r.CreateIssue("acme/delivery/fleet", tracker.NewIssue{Title: "team work"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := team.ListIssues("acme/delivery/fleet", tracker.IssueQuery{}); len(got) != 1 {
		t.Fatalf("the team system holds %d issues, want 1", len(got))
	}
	if got, _ := def.ListIssues("acme/delivery/fleet", tracker.IssueQuery{}); len(got) != 0 {
		t.Fatalf("the default system holds %d of the team's issues, want 0", len(got))
	}
	if tracker.For(def, "acme/delivery/fleet") != def {
		t.Fatal("For on a tracker that does not route is not the tracker itself")
	}
}
