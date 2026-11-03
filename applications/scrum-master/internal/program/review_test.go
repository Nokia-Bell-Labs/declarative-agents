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

func planDoc(id, parent string) []byte {
	line := ""
	if parent != "" {
		line = "- parent: " + parent + "\n"
	}
	return []byte(fmt.Sprintf("---\nid: %s\ntitle: %s\nstatus: agreed\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: Product Owner, po\n%s", id, id, line))
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
// st001 under ft001 under ep001 under in001, naming reviewers as given.
func reviewProgram(t *testing.T, reviews map[string]string) *fake.Fake {
	t.Helper()
	f := fake.New()
	f.Default = "main"
	f.AddFile("g/system", "main", SRDDir+"/srd002-platform.md", []byte(platformSubsystem))
	f.AddFile("g/platform", "main", "docs/specs/stories/st001-a.md", planDoc("st001-a", "ft001-b"))
	f.AddFile("g/platform", "main", "docs/specs/features/ft001-b.md", planDoc("ft001-b", "ep001-c"))
	f.AddFile("g/platform", "main", "docs/specs/epics/ep001-c.md", planDoc("ep001-c", "in001-d"))
	f.AddFile("g/platform", "main", "docs/specs/initiatives/in001-d.md", planDoc("in001-d", ""))
	f.AddFile("g/platform", "main", "docs/specs/stories/st002-e.md",
		planDoc("st002-e", "https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/epics/ep009-f.md#R1"))
	f.AddFile("g/system", "main", "docs/specs/epics/ep009-f.md", planDoc("ep009-f", ""))
	var c people.Config
	for name, who := range reviews {
		c, _ = people.Apply(c, people.Change{Username: who, Reviews: []string{name}})
	}
	doc, err := people.Document(c)
	if err != nil {
		t.Fatal(err)
	}
	f.AddFile("g/system", "main", people.ControlFile, doc)
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
		{name: "the epic over the scrum", reviews: map[string]string{"platform/ep001-c": "eve", "platform": "sam"}, project: "g/platform", issue: story(""), want: "eve"},
		{name: "the feature over the epic", reviews: map[string]string{"platform/ft001-b": "fay", "platform/ep001-c": "eve"}, project: "g/platform", issue: story(""), want: "fay"},
		{name: "the story over the feature", reviews: map[string]string{"platform/st001-a": "sue", "platform/ft001-b": "fay"}, project: "g/platform", issue: story(""), want: "sue"},
		{name: "the line over the story", reviews: map[string]string{"platform/st001-a": "sue"}, project: "g/platform", issue: story("Reviewer: lee\n"), want: "lee", own: true},
		{name: "nothing above the epic", reviews: map[string]string{"platform/in001-d": "ian"}, project: "g/platform", issue: story("")},
		{name: "a parent pinned in the system", reviews: map[string]string{"ep009-f": "eve"}, project: "g/platform", issue: tracker.Issue{Description: "Implements: st002-e\n"}, want: "eve"},
		{name: "an issue naming the epic directly", reviews: map[string]string{"platform/ep001-c": "eve"}, project: "g/platform", issue: tracker.Issue{Description: "Implements: ep001-c\n"}, want: "eve"},
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

// TestRequiredAddsADocumentsReviewer: an issue's line replaces the default but
// not the reviewer of a document the branch changes.
func TestRequiredAddsADocumentsReviewer(t *testing.T) {
	f := reviewProgram(t, map[string]string{"platform/st001-a": "sue"})
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
	got, err = r.Required("g/platform", "g/platform", i, []string{"st001-a", "ft002-unnamed"})
	if err != nil || !reflect.DeepEqual(people(got), []string{"lee", "sue", "dee"}) {
		t.Fatalf("with documents = %v, %v", people(got), err)
	}
	got, err = r.Required("g/platform", "g/platform", tracker.Issue{Description: "Reviewer: sue\n"}, []string{"st001-a"})
	if err != nil || !reflect.DeepEqual(people(got), []string{"sue"}) {
		t.Fatalf("one person twice = %v, %v", people(got), err)
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
		want     []string
		refused  []string
	}{
		{name: "their approval", required: []Signoff{own}, at: []string{"lee"}, excluded: []string{"ann"}, want: []string{"lee"}},
		{name: "only theirs counts", required: []Signoff{own}, at: []string{"bob"}, excluded: []string{"ann"}, refused: []string{"lee", "has not approved"}},
		{name: "an earlier commit", required: []Signoff{own}, stale: map[string]string{"lee": "bbbbbbbbbbbbbbbbb"}, excluded: []string{"ann"}, refused: []string{"lee", "bbbbbbbbbbbb"}},
		{name: "everyone needed is named", required: []Signoff{own, def}, excluded: []string{"ann"}, refused: []string{"lee", "dee"}},
		{name: "one of two", required: []Signoff{own, def}, at: []string{"lee"}, excluded: []string{"ann"}, refused: []string{"dee"}},
		{name: "an inherited reviewer who wrote it falls back", required: []Signoff{def}, at: []string{"bob"}, excluded: []string{"dee"}, want: []string{"bob"}},
		{name: "the fallback still needs someone", required: []Signoff{def}, at: []string{"dee"}, excluded: []string{"dee"}, refused: []string{"someone other than dee"}},
		{name: "an issue's own line naming the author is refused", required: []Signoff{own}, at: []string{"bob"}, excluded: []string{"lee"}, refused: []string{"wrote the work"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := signedOff(approvalsStub{at: c.at, stale: c.stale}, c.required, c.excluded, "g/p", 1, "1-x", tip)
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

// TestMergeHoldsADefaultAndFallsBack: a default holds every merge, and a default
// reviewer who wrote the work falls back to anyone else.
func TestMergeHoldsADefaultAndFallsBack(t *testing.T) {
	alice, checkout := newBeadsTracker(t, "alice")
	bob := reviewer(t, alice, checkout, "bob")
	deliver := func(title, def string) (func() (MergeResult, error), int) {
		t.Helper()
		work, _ := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m", Description: goodBody,
			Labels: []string{"plane::data", "kind::build", "component::po-1"}})
		p, err := Pop(alice, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
		if err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, p.Worktree, "commit", "-q", "--allow-empty", "-m", title)
		beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
		return func() (MergeResult, error) {
			gate := MergeGate{Policy: Policy{DefaultReviewer: def}}
			return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, gate, nil, false)
		}, work.IID
	}
	merge, iid := deliver("Held by the default", "dan")
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "dan") {
		t.Fatalf("merge under a default = %v, want a refusal naming dan", err)
	}
	if _, err := Approve(reviewer(t, alice, checkout, "dan"), ExecGit{}, "g/po-1", "g/po-1", iid, "", checkout); err != nil {
		t.Fatal(err)
	}
	if m, err := merge(); err != nil || m.ApprovedBy != "dan" {
		t.Fatalf("merge on dan's approval = %+v, %v", m, err)
	}

	merge, iid = deliver("The default wrote it", "alice")
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "someone other than alice") {
		t.Fatalf("merge of the default's own work = %v, want it to need anyone else", err)
	}
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", iid, "", checkout); err != nil {
		t.Fatal(err)
	}
	if m, err := merge(); err != nil || m.ApprovedBy != "bob" {
		t.Fatalf("merge on bob's approval = %+v, %v", m, err)
	}
}

// TestADocumentsReviewerHoldsItsChange: changing an SRD needs its reviewer, read
// from main so the branch cannot rename them; a change to a document's issues
// record alone does not.
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
	reviews := func(who string) string {
		c, _ := people.Apply(people.Config{}, people.Change{Username: who, Reviews: []string{"srd045-x", "st001-a"}})
		doc, err := people.Document(c)
		if err != nil {
			t.Fatal(err)
		}
		return string(doc)
	}
	srd, story := SRDDir+"/srd045-x.md", "docs/specs/stories/st001-a.md"
	write(checkout, srd, "# one\n")
	write(checkout, story, string(planDoc("st001-a", "")))
	write(checkout, people.ControlFile, reviews("carol"))
	beadstest.Git(t, checkout, "add", "-A")
	beadstest.Git(t, checkout, "commit", "-q", "-m", "Name carol")
	beadstest.Git(t, checkout, "push", "-q", "origin", "main")
	gate := func(dir string) MergeGate {
		return MergeGate{Command: "true", Dir: dir, Policy: Policy{Control: "g/po-1", Ref: "main"}}
	}
	file := func(title string) tracker.Issue {
		i, err := alice.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m", Description: goodBody,
			Labels: []string{"plane::data", "kind::spec", "component::po-1"}})
		if err != nil {
			t.Fatal(err)
		}
		return i
	}

	ledger := file("Record the issues")
	p, err := Pop(alice, ExecGit{}, "g/po-1", ledger.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	write(p.Worktree, story, string(planDoc("st001-a", ""))+"\n<!-- field: issues | form: \"tree\" -->\n## Issues\n\n- \"g/po-1#1\":\n  - state: open\n")
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Record")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	if m, err := Merge(alice, ExecGit{}, "g/po-1", "g/po-1", ledger.IID, p.Branch, p.Worktree, gate(p.Worktree), nil, false); err != nil || !m.Merged {
		t.Fatalf("merge of the issues record alone = %+v, %v", m, err)
	}

	change := file("Change the SRD")
	p, err = Pop(alice, ExecGit{}, "g/po-1", change.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	write(p.Worktree, srd, "# two\n")
	write(p.Worktree, people.ControlFile, reviews("bob"))
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Change, and name bob")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	merge := func() (MergeResult, error) {
		return Merge(alice, ExecGit{}, "g/po-1", "g/po-1", change.IID, p.Branch, p.Worktree, gate(p.Worktree), nil, false)
	}
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "carol") {
		t.Fatalf("merge changing the SRD = %v, want a refusal naming carol", err)
	}
	if _, err := Approve(reviewer(t, alice, checkout, "bob"), ExecGit{}, "g/po-1", "g/po-1", change.IID, "", checkout); err != nil {
		t.Fatal(err)
	}
	if _, err := merge(); err == nil || !strings.Contains(err.Error(), "carol") {
		t.Fatalf("merge on the branch's own reviewer = %v, want a refusal naming carol", err)
	}
	if _, err := Approve(reviewer(t, alice, checkout, "carol"), ExecGit{}, "g/po-1", "g/po-1", change.IID, "", checkout); err != nil {
		t.Fatal(err)
	}
	if m, err := merge(); err != nil || m.ApprovedBy != "carol" {
		t.Fatalf("merge on carol's approval = %+v, %v", m, err)
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
	if _, err = NameReviewer(f, p, p, i.IID, "erin", true, Policy{}, false); err == nil || !strings.Contains(err.Error(), "erin") {
		t.Fatalf("naming the assignee = %v, want a refusal", err)
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
	f := reviewProgram(t, map[string]string{"platform/st001-a": "me"})
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
