// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TestLinkLinesBecomeDependencies files a defect whose body names a
// blocker, a related issue, and an issue it blocks: each line becomes bd's
// dependency on the dependent issue, the export carries them, a document
// target names none, and an update adds a new line's edge (#152).
func TestLinkLinesBecomeDependencies(t *testing.T) {
	tr := newTestTracker(t, "alice")
	blocker, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "blocker"})
	related, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "related"})
	blocked, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "blocked"})
	body := fmt.Sprintf("A defect.\n\nBlocked-by: g/po-1#%d\nRelates-to: g/po-1#%d\nBlocks: g/po-1#%d\nRelates-to: st001-score\n",
		blocker.IID, related.IID, blocked.IID)
	defect, err := tr.CreateAdmitted("g/po-1", tracker.NewIssue{Title: "defect", Description: body, Labels: []string{"plane::data"}},
		func(tracker.Issue) ([]string, string) { return nil, "" })
	if err != nil {
		t.Fatal(err)
	}
	id := func(i tracker.Issue) string { return strings.TrimPrefix(i.WebURL, "beads:") }
	deps := func(i tracker.Issue) []string {
		got, _, err := tr.Store.Find("g/po-1", i.IID)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range got.Dependencies {
			if d.IssueID != got.ID || d.CreatedBy != "alice" || d.CreatedAt == "" {
				t.Fatalf("dependency %+v is not %s's", d, got.ID)
			}
			out = append(out, d.Type+" "+d.DependsOnID)
		}
		return out
	}
	if got, want := deps(defect), []string{"blocks " + id(blocker), "related " + id(related)}; !slices.Equal(got, want) {
		t.Fatalf("defect's dependencies = %v, want %v", got, want)
	}
	if got, want := deps(blocked), []string{"blocks " + id(defect)}; !slices.Equal(got, want) {
		t.Fatalf("the blocked issue's dependencies = %v, want %v", got, want)
	}

	b, err := tr.Store.Export()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var i struct {
			ID           string `json:"id"`
			Dependencies []struct {
				IssueID     string `json:"issue_id"`
				DependsOnID string `json:"depends_on_id"`
				Type        string `json:"type"`
			} `json:"dependencies"`
		}
		if err := json.Unmarshal([]byte(line), &i); err != nil {
			t.Fatal(err)
		}
		if i.ID == id(defect) && len(i.Dependencies) == 2 && i.Dependencies[0].DependsOnID == id(blocker) && i.Dependencies[0].Type == "blocks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the export lacks the defect's edges in bd's shape:\n%s", b)
	}

	more := body + fmt.Sprintf("Depends-on: g/po-1#%d\n", blocked.IID)
	if _, err := tr.UpdateIssue("g/po-1", defect.IID, tracker.IssueUpdate{Description: &more}); err != nil {
		t.Fatal(err)
	}
	// Depends-on does not make an issue wait (srd007 R6.3): a related edge.
	if got := deps(defect); len(got) != 3 || got[2] != "related "+id(blocked) {
		t.Fatalf("after the update: %v, want the new related edge added", got)
	}
}

// TestDependenciesMergeAsASet keeps each side's new edge and drops one a
// side removed.
func TestDependenciesMergeAsASet(t *testing.T) {
	edge := func(to string) Dependency {
		return Dependency{IssueID: "sm-1", DependsOnID: to, Type: DependencyBlocks}
	}
	base := Issue{ID: "sm-1", Dependencies: []Dependency{edge("sm-2"), edge("sm-3")}}
	ours := Issue{ID: "sm-1", Dependencies: []Dependency{edge("sm-2"), edge("sm-3"), edge("sm-4")}}
	theirs := Issue{ID: "sm-1", Dependencies: []Dependency{edge("sm-2"), edge("sm-5")}}
	var got []string
	for _, d := range mergeIssue(Table, base, ours, theirs, "2026-11-02T09:00:00Z").Dependencies {
		got = append(got, d.DependsOnID)
	}
	if !slices.Equal(got, []string{"sm-2", "sm-5", "sm-4"}) {
		t.Fatalf("merged edges = %v, want sm-2, theirs' sm-5, ours' sm-4, without the removed sm-3", got)
	}
}
