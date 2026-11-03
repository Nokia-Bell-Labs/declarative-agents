// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// issue link writes the Implements line, keeps the rest of the description,
// refuses what it cannot name, and needs --force to replace (srd007 R6.6).
func TestLinkIssue(t *testing.T) {
	f := fake.New()
	branch, _ := f.DefaultBranch("g/po-1")
	f.AddFile("g/po-1", branch, "docs/specs/epics/ep001-x.md", []byte("epic"))
	f.AddFile("g/po-1", branch, "docs/specs/epics/ep002-y.md", []byte("epic"))
	f.AddFile("g/po-1", branch, "docs/specs/stories/st001-z.md", []byte("story"))
	f.AddFile("g/system", "v1.0.0", "docs/specs/epics/ep003-z.md", []byte("epic"))
	i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Work", Description: goodBody})
	if err != nil {
		t.Fatal(err)
	}
	body := func() string {
		t.Helper()
		got, err := f.GetIssue("g/po-1", i.IID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Description
	}
	link := func(kind, target string, force, dryRun bool) (LinkResult, error) {
		return LinkIssue(f, "g/po-1", i.IID, kind, target, force, Defaults(), dryRun)
	}

	if r, err := link("epic", "ep001-x", false, true); err != nil || r.Action != "linked" || strings.Contains(body(), "Implements:") {
		t.Fatalf("dry run = %+v, %v\n%s", r, err, body())
	}
	if r, err := link("epic", "ep001-x", false, false); err != nil || r.Action != "linked" {
		t.Fatalf("link = %+v, %v", r, err)
	}
	if b := body(); !strings.HasPrefix(b, goodBody) || !strings.Contains(b, "\nImplements: ep001-x\n") {
		t.Fatalf("body:\n%s", b)
	}
	if r, err := link("epic", "ep001-x", false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("a second link = %+v, %v", r, err)
	}
	for name, try := range map[string]struct{ kind, target, want string }{
		"another without force": {"epic", "ep002-y", "already implements ep001-x"},
		"an unknown epic":       {"epic", "ep009-gone", "holds no epic"},
		"a story as an epic":    {"epic", "st001-z", "names a story, not an epic"},
		"an epic as a story":    {"story", "ep001-x", "names an epic, not a story"},
		"a feature":             {"epic", "ft001-a", "not a story or an epic"},
	} {
		if _, err := link(try.kind, try.target, false, false); err == nil || !strings.Contains(err.Error(), try.want) {
			t.Errorf("%s: err = %v, want %q", name, err, try.want)
		}
	}
	pinned := "https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/epics/ep003-z.md#R1"
	if r, err := link("epic", pinned, true, false); err != nil || r.Action != "linked" {
		t.Fatalf("force = %+v, %v", r, err)
	}
	if b := body(); strings.Contains(b, "ep001-x") || strings.Count(b, "Implements:") != 1 || !strings.Contains(b, "Implements: "+pinned) {
		t.Fatalf("after force:\n%s", b)
	}
	if lint := Lint(mustIssue(t, f, i.IID), Defaults()); slices.ContainsFunc(lint, func(s string) bool { return strings.Contains(s, "Implements") }) {
		t.Fatalf("lint: %v", lint)
	}
}

func mustIssue(t *testing.T, f *fake.Fake, iid int) tracker.Issue {
	t.Helper()
	i, err := f.GetIssue("g/po-1", iid)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// issue list --epic lists the epic's stories' issues in rank order, then the
// issues naming the epic directly; another epic's stay out (srd036 R5.1).
func TestListEpicIncludesItsDirectIssues(t *testing.T) {
	f := fake.New()
	const valid = "../../testdata/hierarchy/valid"
	file := func(implements string) int {
		t.Helper()
		i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: implements, Description: "Implements: " + implements})
		if err != nil {
			t.Fatal(err)
		}
		return i.IID
	}
	direct := file("ep001-agent-evaluation")
	third := file("st003-scorecard-page")
	first := file("st001-score-trajectories")
	pinned := file("https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/epics/ep001-agent-evaluation.md")
	file("ep999-another")

	l, err := ListImplementing(f, "g/po-1", valid, "", "ep001-agent-evaluation")
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, i := range l.Issues {
		got = append(got, i.IID)
	}
	if want := []int{first, third, direct, pinned}; !slices.Equal(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
	if _, err := ListImplementing(f, "g/po-1", valid, "", "ep999-another"); err == nil || !strings.Contains(err.Error(), "holds no epic") {
		t.Fatalf("an epic the hierarchy does not hold: %v", err)
	}
}
