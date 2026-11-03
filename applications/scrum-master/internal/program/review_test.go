// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/people"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func planDoc(id, parent string, reviewer ...string) []byte {
	line, front := "", ""
	if parent != "" {
		line = "- parent: " + parent + "\n"
	}
	if len(reviewer) > 0 {
		front = "reviewer: " + reviewer[0] + "\n"
	}
	return []byte(fmt.Sprintf("---\nid: %s\ntitle: %s\nstatus: agreed\n%s---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: Product Owner, po\n%s", id, id, front, line))
}

const platformSubsystem = `---
id: srd002-platform
title: "Platform"
---

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, platform
- scrum_repository: https://gitlab.example.com/g/platform

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| api | https://gitlab.example.com/g/api | / |
`

// reviewProgram is a system repository and a team's scrum repository holding
// st001 under ft001 under ep001 under in001, each document naming the reviewer
// reviews gives it, and control/people.md naming the rest, which are scrums.
func reviewProgram(t *testing.T, reviews map[string]string) *fake.Fake {
	t.Helper()
	f := fake.New()
	f.Default = "main"
	doc := func(id, parent string) []byte {
		if who := reviews[id]; who != "" {
			return planDoc(id, parent, who)
		}
		return planDoc(id, parent)
	}
	f.AddFile("g/system", "main", SRDDir+"/srd002-platform.md", []byte(platformSubsystem))
	f.AddFile("g/platform", "main", "docs/specs/stories/st001-a.md", doc("st001-a", "ft001-b"))
	f.AddFile("g/platform", "main", "docs/specs/features/ft001-b.md", doc("ft001-b", "ep001-c"))
	f.AddFile("g/platform", "main", "docs/specs/epics/ep001-c.md", doc("ep001-c", "in001-d"))
	f.AddFile("g/platform", "main", "docs/specs/initiatives/in001-d.md", doc("in001-d", ""))
	f.AddFile("g/platform", "main", "docs/specs/stories/st002-e.md",
		doc("st002-e", "https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/epics/ep009-f.md#R1"))
	f.AddFile("g/system", "main", "docs/specs/epics/ep009-f.md", doc("ep009-f", ""))
	var c people.Config
	if who := reviews["platform"]; who != "" {
		c, _ = people.Apply(c, people.Change{Username: who, Reviews: []string{"platform"}})
	}
	control, err := people.Document(c)
	if err != nil {
		t.Fatal(err)
	}
	f.AddFile("g/system", "main", people.ControlFile, control)
	return f
}

// TestWhoSignsOff walks srd013 R11.1's order: the most specific name wins.
func TestWhoSignsOff(t *testing.T) {
	story := func(extra string) tracker.Issue {
		return tracker.Issue{Description: "Implements: st001-a\n" + extra}
	}
	for _, c := range []struct {
		name    string
		reviews map[string]string
		def     string
		project string
		issue   tracker.Issue
		want    string
		own     bool
	}{
		{name: "nothing named", project: "g/platform", issue: story("")},
		{name: "the program's default", def: "dee", project: "g/platform", issue: story(""), want: "dee"},
		{name: "the scrum over the default", reviews: map[string]string{"platform": "sam"}, def: "dee", project: "g/platform", issue: story(""), want: "sam"},
		{name: "a code repository's scrum", reviews: map[string]string{"platform": "sam"}, project: "g/api", issue: tracker.Issue{}, want: "sam"},
		{name: "the epic over the scrum", reviews: map[string]string{"ep001-c": "eve", "platform": "sam"}, project: "g/platform", issue: story(""), want: "eve"},
		{name: "the feature over the epic", reviews: map[string]string{"ft001-b": "fay", "ep001-c": "eve"}, project: "g/platform", issue: story(""), want: "fay"},
		{name: "the story over the feature", reviews: map[string]string{"st001-a": "sue", "ft001-b": "fay"}, project: "g/platform", issue: story(""), want: "sue"},
		{name: "the line over the story", reviews: map[string]string{"st001-a": "sue"}, project: "g/platform", issue: story("Reviewer: lee\n"), want: "lee", own: true},
		{name: "nothing above the epic", reviews: map[string]string{"in001-d": "ian"}, project: "g/platform", issue: story("")},
		{name: "a parent pinned in the system", reviews: map[string]string{"ep009-f": "eve"}, project: "g/platform", issue: tracker.Issue{Description: "Implements: st002-e\n"}, want: "eve"},
		{name: "an issue naming the epic directly", reviews: map[string]string{"ep001-c": "eve"}, project: "g/platform", issue: tracker.Issue{Description: "Implements: ep001-c\n"}, want: "eve"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := reviewProgram(t, c.reviews)
			r, err := LoadReviews(f, Policy{Control: "g/system", Ref: "main", DefaultReviewer: c.def})
			if err != nil {
				t.Fatal(err)
			}
			s, ok, err := r.ForIssue(c.project, c.issue)
			if err != nil || ok != (c.want != "") || s.Person != c.want || s.Own != c.own {
				t.Fatalf("ForIssue = %+v, %v, %v; want %q, own %v", s, ok, err, c.want, c.own)
			}
		})
	}
}

func TestLoadReviewsRefusesAnUnreadableDocument(t *testing.T) {
	f := fake.New()
	f.AddFile("g/system", "main", people.ControlFile, []byte("---\nid: people\ntitle: People\n---\n\n<!-- field: people -->\n## People\n\n| username | reviews |\n| --- | --- |\n| a | ep1 |\n| b | ep1 |\n"))
	if _, err := LoadReviews(f, Policy{Control: "g/system", Ref: "main"}); err == nil || !strings.Contains(err.Error(), "two reviewers") {
		t.Fatalf("LoadReviews = %v, want it refused", err)
	}
}

func TestSignedDocument(t *testing.T) {
	for p, want := range map[string]bool{
		"docs/VISION.md": true, "docs/ARCHITECTURE.md": true, "docs/road-map.md": true, SRDDir + "/srd1-a.md": true,
		"docs/specs/use-cases/rel1-uc1-a.md": true, "docs/specs/stories/st1-a.md": true, "docs/specs/epics/ep1-a.md": true,
		"docs/specs/objectives/obj1-a.md": true, "docs/guides/process.md": false, "docs/specs/stories/notes.txt": false, "control/people.md": false,
	} {
		if SignedDocument(p) != want {
			t.Errorf("SignedDocument(%s) = %v", p, !want)
		}
	}
}

// TestRequiredAddsADocumentsReviewer: an issue's line replaces the default but
// not the reviewer of a document the branch changes.
func TestRequiredAddsADocumentsReviewer(t *testing.T) {
	f := reviewProgram(t, map[string]string{"st001-a": "sue"})
	r, err := LoadReviews(f, Policy{Control: "g/system", Ref: "main", DefaultReviewer: "dee"})
	if err != nil {
		t.Fatal(err)
	}
	people := func(ss []Signoff) (out []string) {
		for _, s := range ss {
			out = append(out, s.Person)
		}
		return out
	}
	i := tracker.Issue{Description: "Reviewer: lee\n"}
	got, err := r.Required("g/platform", "g/platform", i, nil)
	if err != nil || !reflect.DeepEqual(people(got), []string{"lee"}) {
		t.Fatalf("the line alone = %v, %v", people(got), err)
	}
	story, feature := ChangedDocument{Path: "docs/specs/stories/st001-a.md"}, ChangedDocument{Path: "docs/specs/features/ft002-unnamed.md"}
	got, err = r.Required("g/platform", "g/platform", i, []ChangedDocument{story, feature})
	if err != nil || !reflect.DeepEqual(people(got), []string{"lee", "sue", "dee"}) {
		t.Fatalf("with documents = %v, %v", people(got), err)
	}
	got, err = r.Required("g/platform", "g/platform", tracker.Issue{Description: "Reviewer: sue\n"}, []ChangedDocument{story})
	if err != nil || !reflect.DeepEqual(people(got), []string{"sue"}) {
		t.Fatalf("one person twice = %v, %v", people(got), err)
	}
	added := ChangedDocument{Path: "docs/specs/stories/st009-new.md", Added: planDoc("st009-new", "ep001-c", "nan")}
	got, err = r.Required("g/platform", "g/platform", tracker.Issue{Description: "Reviewer: lee\n"}, []ChangedDocument{added})
	if err != nil || !reflect.DeepEqual(people(got), []string{"lee", "nan"}) {
		t.Fatalf("a document the branch adds = %v, %v", people(got), err)
	}
}

type approvalsStub struct {
	localDelivery
	at    []string
	stale map[string]string
}

func (a approvalsStub) approvals(string, int, string, string) ([]string, map[string]string, error) {
	return a.at, a.stale, nil
}

func TestSignedOff(t *testing.T) {
	const tip = "aaaaaaaaaaaaaaaaaaaa"
	own := Signoff{Person: "lee", Why: "the issue's Reviewer line", Own: true}
	def := Signoff{Person: "dee", Why: "the program's default reviewer"}
	for _, c := range []struct {
		name     string
		required []Signoff
		at       []string
		stale    map[string]string
		excluded []string
		merger   string
		want     []string
		refused  []string
	}{
		{name: "their approval", required: []Signoff{own}, at: []string{"lee"}, excluded: []string{"ann"}, merger: "ann", want: []string{"lee"}},
		{name: "only theirs counts", required: []Signoff{own}, at: []string{"bob"}, excluded: []string{"ann"}, merger: "ann", refused: []string{"lee", "has not approved"}},
		{name: "an earlier commit", required: []Signoff{own}, stale: map[string]string{"lee": "bbbbbbbbbbbbbbbbb"}, excluded: []string{"ann"}, merger: "ann", refused: []string{"lee", "bbbbbbbbbbbb"}},
		{name: "everyone needed is named", required: []Signoff{own, def}, excluded: []string{"ann"}, merger: "ann", refused: []string{"lee", "dee"}},
		{name: "one of two", required: []Signoff{own, def}, at: []string{"lee"}, excluded: []string{"ann"}, merger: "ann", refused: []string{"dee"}},
		{name: "a repository of one: the default wrote it and merges it", required: []Signoff{def}, excluded: []string{"dee"}, merger: "dee", want: []string{"dee"}},
		{name: "a repository of one: the line names the author, who merges", required: []Signoff{own}, excluded: []string{"lee"}, merger: "lee", want: []string{"lee"}},
		{name: "a reviewer who did not write it merges with no approval", required: []Signoff{own}, excluded: []string{"ann"}, merger: "lee", want: []string{"lee"}},
		{name: "running the merge signs off only for yourself", required: []Signoff{own, def}, excluded: []string{"lee"}, merger: "lee", refused: []string{"dee"}},
		{name: "someone else merging a reviewer's own work needs another's approval", required: []Signoff{def}, at: []string{"bob"}, excluded: []string{"dee"}, merger: "cal", want: []string{"bob"}},
		{name: "and is refused without it", required: []Signoff{def}, at: []string{"dee"}, excluded: []string{"dee"}, merger: "cal", refused: []string{"someone other than dee"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := signedOff(approvalsStub{at: c.at, stale: c.stale}, c.required, c.excluded, c.merger, "g/p", 1, "1-x", tip)
			if len(c.refused) > 0 {
				if err == nil {
					t.Fatalf("signedOff = %v, want a refusal", got)
				}
				for _, s := range c.refused {
					if !strings.Contains(err.Error(), s) {
						t.Fatalf("refusal %q does not say %q", err, s)
					}
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("signedOff = %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

// TestARepositoryOfOneIsNotLockedOut: one person, named every way, delivers their
// own work with no other account, and the merge commit says they signed off. With
// require_approval on, naming yourself does not get round it.
func TestARepositoryOfOneIsNotLockedOut(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	deliver := func(title, extra string, gate MergeGate) (MergeResult, error) {
		t.Helper()
		work, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m", Description: goodBody + extra,
			Labels: []string{"plane::data", "kind::build", "component::po-1"}})
		p, err := Pop(alice, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
		if err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, p.Worktree, "commit", "-q", "--allow-empty", "-m", title)
		beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
		return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, nil, false)
	}
	for name, c := range map[string]struct {
		extra string
		gate  MergeGate
	}{
		"their own line": {extra: "\nReviewer: alice\n"},
		"the default":    {gate: MergeGate{Policy: Policy{DefaultReviewer: "alice"}}},
	} {
		m, err := deliver(name, c.extra, c.gate)
		if err != nil || !m.Merged || m.ApprovedBy != "alice" {
			t.Fatalf("%s: merge = %+v, %v", name, m, err)
		}
		if body := beadstest.Git(t, checkout, "log", "-1", "--format=%B", "main"); !strings.Contains(body, "Approved-by: alice") {
			t.Fatalf("%s: merge commit = %q", name, body)
		}
	}
	gate := MergeGate{RequireApproval: true}
	if _, err := deliver("Round the policy", "\nReviewer: alice\n", gate); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("naming yourself under require_approval = %v, want a refusal", err)
	}
}

// TestMergeHoldsTheNamedReviewer is the gate on a real merge, with the default
// policy's require_approval off: another's approval does not stand in, and the
// merge commit records who signed off.
func TestMergeHoldsTheNamedReviewer(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	bob, carol := reviewer(t, alice, checkout, "bob"), reviewer(t, alice, checkout, "carol")
	work, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: "Named", Milestone: "m", Description: goodBody + "\nReviewer: carol\n",
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	p, err := Pop(alice, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "commit", "-q", "--allow-empty", "-m", "Named")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	merge := func() (MergeResult, error) {
		return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, MergeGate{}, nil, false)
	}
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "carol") {
		t.Fatalf("merge with no approval = %v, want a refusal naming carol", err)
	}
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err != nil {
		t.Fatal(err)
	}
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "carol") {
		t.Fatalf("merge on bob's approval = %v, want a refusal naming carol", err)
	}
	if _, err := Approve(carol, ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err != nil {
		t.Fatal(err)
	}
	m, err := merge()
	if err != nil || !m.Merged || m.ApprovedBy != "carol" {
		t.Fatalf("merge on carol's approval = %+v, %v", m, err)
	}
	body := beadstest.Git(t, checkout, "log", "-1", "--format=%B", "main")
	if !strings.Contains(body, "Approved-by: carol") || !strings.Contains(body, "Merged-by: alice") {
		t.Fatalf("merge commit = %q, want both trailers", body)
	}
}

// TestMergeHoldsADefault: a default holds a merge someone else runs until they
// approve it.
func TestMergeHoldsADefault(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	work, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: "Held by the default", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	p, err := Pop(alice, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "commit", "-q", "--allow-empty", "-m", "Held")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	merge := func() (MergeResult, error) {
		gate := MergeGate{Policy: Policy{DefaultReviewer: "dan"}}
		return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, nil, false)
	}
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "dan") {
		t.Fatalf("merge under a default = %v, want a refusal naming dan", err)
	}
	if _, err := Approve(reviewer(t, alice, checkout, "dan"), ExecGit{}, "g/po-1", "g/po-1", work.IID, "", checkout); err != nil {
		t.Fatal(err)
	}
	if m, err := merge(); err != nil || m.ApprovedBy != "dan" {
		t.Fatalf("merge on dan's approval = %+v, %v", m, err)
	}
}

// TestADocumentsReviewerHoldsItsChange: changing a document needs its reviewer,
// read from main so the branch cannot rename them; a change to its issues record
// or people block alone does not, so someone away can be replaced.
func TestADocumentsReviewerHoldsItsChange(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	write := func(dir, rel, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srd, story := SRDDir+"/srd045-x.md", "docs/specs/stories/st001-a.md"
	srdDoc := func(reviewer, body string) string {
		return "---\nid: srd045-x\ntitle: X\nreviewer: " + reviewer + "\n---\n\n" + body + "\n"
	}
	write(checkout, srd, srdDoc("carol", "one"))
	write(checkout, story, string(planDoc("st001-a", "", "carol")))
	beadstest.Git(t, checkout, "add", "-A")
	beadstest.Git(t, checkout, "commit", "-q", "-m", "Name carol")
	beadstest.Git(t, checkout, "push", "-q", "origin", "main")
	deliver := func(title string, change func(worktree string)) func() (MergeResult, error) {
		t.Helper()
		i, err := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m", Description: goodBody,
			Labels: []string{"plane::data", "kind::spec", "component::po-1"}})
		if err != nil {
			t.Fatal(err)
		}
		p, err := Pop(alice, ExecGit{}, "g/po-1", i.IID, checkout, Defaults(), false)
		if err != nil {
			t.Fatal(err)
		}
		change(p.Worktree)
		beadstest.Git(t, p.Worktree, "add", "-A")
		beadstest.Git(t, p.Worktree, "commit", "-q", "-m", title)
		beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
		return func() (MergeResult, error) {
			gate := MergeGate{Command: "true", Dir: p.Worktree, Policy: Policy{Control: "g/po-1", Ref: "main"}}
			return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", i.IID, p.Branch, p.Worktree, gate, nil, false)
		}
	}
	approve := func(user string, m func() (MergeResult, error)) {
		t.Helper()
		out := beadstest.Git(t, checkout, "branch", "--list", "--sort=-committerdate", "--format=%(refname:short)")
		branch := strings.Fields(out)[0]
		iid, _, _ := strings.Cut(branch, "-")
		var n int
		fmt.Sscan(iid, &n)
		if _, err := Approve(reviewer(t, alice, checkout, user), ExecGit{}, "g/po-1", "g/po-1", n, "", checkout); err != nil {
			t.Fatal(err)
		}
	}

	record := deliver("Record the issues", func(w string) {
		write(w, story, string(planDoc("st001-a", "", "carol"))+"\n<!-- field: issues | form: \"tree\" -->\n## Issues\n\n- \"g/po-1#1\":\n  - state: open\n")
	})
	if m, err := record(); err != nil || !m.Merged {
		t.Fatalf("merge of the issues record alone = %+v, %v", m, err)
	}

	change := deliver("Change the SRD and rename its reviewer", func(w string) { write(w, srd, srdDoc("bob", "two")) })
	if _, err := change(); err == nil || !strings.Contains(err.Error(), "carol") {
		t.Fatalf("merge changing the SRD = %v, want a refusal naming carol", err)
	}
	approve("bob", change)
	if _, err := change(); err == nil || !strings.Contains(err.Error(), "carol") {
		t.Fatalf("merge on the branch's own reviewer = %v, want a refusal naming carol", err)
	}
	approve("carol", change)
	if m, err := change(); err != nil || m.ApprovedBy != "carol" {
		t.Fatalf("merge on carol's approval = %+v, %v", m, err)
	}

	// Main now names bob. Replacing him touches only the people block.
	replace := deliver("Replace the reviewer", func(w string) { write(w, srd, srdDoc("dan", "two")) })
	if m, err := replace(); err != nil || !m.Merged {
		t.Fatalf("merge replacing the reviewer alone = %+v, %v", m, err)
	}
	addition := deliver("Add a document", func(w string) {
		write(w, "docs/specs/software-requirements/srd046-y.md", "---\nid: srd046-y\ntitle: Y\nreviewer: erin\n---\n\nBody\n")
	})
	if _, err := addition(); err == nil || !strings.Contains(err.Error(), "erin") {
		t.Fatalf("merge adding a document = %v, want a refusal naming erin", err)
	}
}

// reviewingFake is a tracker whose host reviews branches, recording who it asked.
type reviewingFake struct {
	*fake.Fake
	asked []string
	err   error
}

func (r *reviewingFake) OpenReview(string, string, string, string, string) (string, error) {
	return "https://example.com/review", nil
}
func (r *reviewingFake) Approve(string, string, string) (bool, error) { return true, nil }
func (r *reviewingFake) Approval(string, string, string, []string) (string, string, error) {
	return "", "", nil
}
func (r *reviewingFake) Approvals(string, string, string) ([]string, map[string]string, error) {
	return nil, nil, nil
}
func (r *reviewingFake) Merge(string, string, string, string) (string, error) { return "", nil }
func (r *reviewingFake) RequestReview(_, _ string, usernames []string) error {
	if r.err != nil {
		return r.err
	}
	r.asked = append(r.asked, usernames...)
	return nil
}

func TestNameReviewer(t *testing.T) {
	const p = "g/po-1"
	f := fake.New()
	i := admitted(f, "Score one agent")
	writes := f.Writes
	r, err := NameReviewer(f, p, p, i.IID, "carol", false, Policy{}, true)
	if err != nil || r.Action != "named" || f.Writes != writes {
		t.Fatalf("dry run = %+v, %v, writes %d", r, err, f.Writes-writes)
	}
	if r, err = NameReviewer(f, p, p, i.IID, "carol", false, Policy{}, false); err != nil || r.Action != "named" {
		t.Fatalf("name = %+v, %v", r, err)
	}
	got, _ := f.GetIssue(p, i.IID)
	if ReviewerLine(got.Description) != "carol" {
		t.Fatalf("body = %q", got.Description)
	}
	writes = f.Writes
	if r, err = NameReviewer(f, p, p, i.IID, "carol", false, Policy{}, false); err != nil || r.Action != "unchanged" || f.Writes != writes {
		t.Fatalf("second run = %+v, %v", r, err)
	}
	if _, err = NameReviewer(f, p, p, i.IID, "dan", false, Policy{}, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("replacing without force = %v", err)
	}
	if r, err = NameReviewer(f, p, p, i.IID, "dan", true, Policy{}, false); err != nil || r.Previous != "carol" {
		t.Fatalf("replacing with force = %+v, %v", r, err)
	}
	got, _ = f.GetIssue(p, i.IID)
	if strings.Count(got.Description, "Reviewer:") != 1 {
		t.Fatalf("body has more than one line: %q", got.Description)
	}
	if err := f.SetAssignee(p, i.IID, "erin"); err != nil {
		t.Fatal(err)
	}
	if _, err = NameReviewer(f, p, p, i.IID, "erin", true, Policy{}, false); err != nil {
		t.Fatalf("naming the assignee, who signs off by merging = %v", err)
	}
	if r, err = NameReviewer(f, p, p, i.IID, "", false, Policy{}, false); err != nil || r.Action != "dropped" {
		t.Fatalf("clear = %+v, %v", r, err)
	}
	got, _ = f.GetIssue(p, i.IID)
	if strings.Contains(got.Description, "Reviewer:") {
		t.Fatalf("cleared body = %q", got.Description)
	}
}

// TestRequestReview: the host's review asks the reviewer once a branch has one;
// a host that refuses is a note, and neither the author nor the assignee is asked.
func TestRequestReview(t *testing.T) {
	const p = "g/po-1"
	f := &reviewingFake{Fake: fake.New()}
	i := admitted(f.Fake, "Score one agent")
	if r, err := NameReviewer(f, p, p, i.IID, "carol", false, Policy{}, false); err != nil || r.Requested {
		t.Fatalf("with no branch yet = %+v, %v", r, err)
	}
	if len(f.asked) != 0 {
		t.Fatalf("asked %v before a branch existed", f.asked)
	}
	if err := f.CreateBranch(p, branchOf(i), "main"); err != nil {
		t.Fatal(err)
	}
	if r, err := NameReviewer(f, p, p, i.IID, "dan", true, Policy{}, false); err != nil || !r.Requested {
		t.Fatalf("with a branch = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(f.asked, []string{"dan"}) {
		t.Fatalf("asked %v", f.asked)
	}
	f.err = errors.New("no such collaborator")
	got, _ := f.GetIssue(p, i.IID)
	if who, note := requestReview(f, p, p, got, branchOf(i), Policy{}, false); who != "" || !strings.Contains(note, "no such collaborator") {
		t.Fatalf("a refusing host = %q, %q", who, note)
	}
	f.err, f.asked = nil, nil
	got.Assignees = []string{"dan"}
	if who, _ := requestReview(f, p, p, got, branchOf(i), Policy{}, false); who != "" || len(f.asked) != 0 {
		t.Fatalf("asked the assignee: %q, %v", who, f.asked)
	}
	got.Assignees, got.Description = nil, "Reviewer: "+f.Me.Username+"\n"
	if who, _ := requestReview(f, p, p, got, branchOf(i), Policy{}, false); who != "" || len(f.asked) != 0 {
		t.Fatalf("asked the caller: %q, %v", who, f.asked)
	}
}

func TestListReviewing(t *testing.T) {
	const a = "g/platform"
	f := reviewProgram(t, map[string]string{"st001-a": "me"})
	mine, _ := f.CreateIssue(a, tracker.NewIssue{Title: "By the story", Description: "Implements: st001-a\n"})
	line, _ := f.CreateIssue(a, tracker.NewIssue{Title: "By the line", Description: "Reviewer: me\n"})
	other, _ := f.CreateIssue(a, tracker.NewIssue{Title: "Someone else", Description: "Reviewer: bob\n"})
	own, _ := f.CreateIssue(a, tracker.NewIssue{Title: "My own work", Description: "Reviewer: me\n"})
	if err := f.SetAssignee(a, own.IID, f.Me.Username); err != nil {
		t.Fatal(err)
	}
	reviews, err := LoadReviews(f, Policy{Control: "g/system", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := ListReviewing(f, reviews, []string{a})
	if err != nil {
		t.Fatal(err)
	}
	var iids []int
	for _, i := range r.Issues {
		iids = append(iids, i.IID)
	}
	if !reflect.DeepEqual(iids, []int{mine.IID, line.IID}) {
		t.Fatalf("issues = %v; want %d and %d, not %d or %d", iids, mine.IID, line.IID, other.IID, own.IID)
	}
}

func TestReviewerCommandRefusesBadUsage(t *testing.T) {
	t.Chdir(t.TempDir()) // outside any checkout, so no project is read from one
	for _, args := range [][]string{
		{"issue", "reviewer", "--project", "g/p", "--issue", "1"},
		{"issue", "reviewer", "--project", "g/p", "--issue", "1", "--person", "a", "--clear"},
		{"issue", "reviewer", "--project", "g/p", "--person", "a"},
		{"issue", "reviewer", "--project", "g/p", "--issue", "1", "--person", "../a"},
		{"issue", "list", "--mine", "--reviewing", "--project", "g/p"},
		{"issue", "list", "--reviewing", "--epic", "e", "--project", "g/p"},
	} {
		var out strings.Builder
		if err := cli.Default.Run(args, &out, nil); !errors.Is(err, cli.ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}
