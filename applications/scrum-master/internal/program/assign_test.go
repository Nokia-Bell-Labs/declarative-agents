// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// stateFake keeps whether work started, as beads does.
type stateFake struct {
	*fake.Fake
	started bool
}

func (s *stateFake) Started(string, int) (bool, error) { return s.started, nil }

// TestAssignAndPopReadTheTrackersWorkState: where the tracking system records
// that work started, its record decides, not a branch, and the assignee's pop
// of an issue not yet started still takes it.
func TestAssignAndPopReadTheTrackersWorkState(t *testing.T) {
	const p = "g/po-1"
	f := &stateFake{Fake: fake.New()}
	i := admitted(f.Fake, "Score one agent")
	if err := f.SetAssignee(p, i.IID, f.Me.Username); err != nil {
		t.Fatal(err)
	}

	r, _, err := Claim(f, p, p, i.IID, Defaults(), false)
	if err != nil || !r.Assigned {
		t.Fatalf("pop of an assigned issue not started = %+v, %v, want it taken", r, err)
	}
	if _, err := AssignIssue(f, p, p, i.IID, "carol", true); err != nil {
		t.Fatalf("assign before work started = %v", err)
	}

	f.started = true
	writes := f.Writes
	if r, _, err = Claim(f, p, p, i.IID, Defaults(), false); err != nil || r.Assigned || f.Writes != writes {
		t.Fatalf("second pop = %+v, %v, writes %d, want nothing changed", r, err, f.Writes-writes)
	}
	if _, err := AssignIssue(f, p, p, i.IID, "carol", false); err == nil || !strings.Contains(err.Error(), "started by "+f.Me.Username) {
		t.Fatalf("assign after work started = %v", err)
	}
}

func TestAssignIssue(t *testing.T) {
	const p = "g/po-1"
	f := fake.New()
	i := admitted(f, "Score one agent")

	writes := f.Writes
	r, err := AssignIssue(f, p, p, i.IID, "alice", true)
	if err != nil || r.Action != "assigned" || !r.DryRun || f.Writes != writes {
		t.Fatalf("dry run = %+v, %v, writes %d", r, err, f.Writes-writes)
	}
	if got, _ := f.GetIssue(p, i.IID); len(got.Assignees) != 0 {
		t.Fatalf("a dry run assigned %v", got.Assignees)
	}

	if r, err = AssignIssue(f, p, p, i.IID, "alice", false); err != nil || r.Action != "assigned" {
		t.Fatalf("assign = %+v, %v", r, err)
	}
	if got, _ := f.GetIssue(p, i.IID); !reflect.DeepEqual(got.Assignees, []string{"alice"}) {
		t.Fatalf("assignees = %v", got.Assignees)
	}

	writes = f.Writes
	if r, err = AssignIssue(f, p, p, i.IID, "alice", false); err != nil || r.Action != "unchanged" || f.Writes != writes {
		t.Fatalf("second assign = %+v, %v, writes %d", r, err, f.Writes-writes)
	}

	// Nothing has started: it can go to someone else.
	if r, err = AssignIssue(f, p, p, i.IID, "bob", false); err != nil || !reflect.DeepEqual(r.Previous, []string{"alice"}) {
		t.Fatalf("reassign = %+v, %v", r, err)
	}

	// The pop branch shows work started, and only the assignee is named.
	if err := f.CreateBranch(p, branchOf(i), "main"); err != nil {
		t.Fatal(err)
	}
	if _, err = AssignIssue(f, p, p, i.IID, "carol", false); err == nil || !strings.Contains(err.Error(), "started by bob") {
		t.Fatalf("reassign after start = %v, want a refusal naming bob", err)
	}
	if got, _ := f.GetIssue(p, i.IID); !reflect.DeepEqual(got.Assignees, []string{"bob"}) {
		t.Fatalf("a refused assign changed the assignees: %v", got.Assignees)
	}

	closeState := "close"
	if _, err := f.UpdateIssue(p, i.IID, tracker.IssueUpdate{State: &closeState}); err != nil {
		t.Fatal(err)
	}
	if _, err = AssignIssue(f, p, p, i.IID, "carol", false); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("assign of a closed issue = %v", err)
	}
}

// TestAssignChecksTheBranchWhereItLives: an issue in one project may hold its
// branch in another.
func TestAssignChecksTheBranchWhereItLives(t *testing.T) {
	f := fake.New()
	i := admitted(f, "Score one agent")
	if err := f.SetAssignee("g/po-1", i.IID, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := f.CreateBranch("g/code", branchOf(i), "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := AssignIssue(f, "g/code", "g/po-1", i.IID, "carol", false); err == nil {
		t.Fatal("assigned over a branch in the code project")
	}
}

// droppingFake accepts an assignment and makes none, as GitHub does for a
// login it cannot assign.
type droppingFake struct{ *fake.Fake }

func (droppingFake) SetAssignee(string, int, string) error { return nil }

func TestAssignRefusesATrackerThatDidNotAssign(t *testing.T) {
	f := fake.New()
	i := admitted(f, "Score one agent")
	if _, err := AssignIssue(droppingFake{f}, "g/po-1", "g/po-1", i.IID, "stranger", false); err == nil || !strings.Contains(err.Error(), "did not assign stranger") {
		t.Fatalf("assign = %v, want the tracker's silence reported", err)
	}
}

// TestPopOfAnAssignedIssue: the assignee pops it without a second assignment,
// and anyone else is refused.
func TestPopOfAnAssignedIssue(t *testing.T) {
	const p = "g/po-1"
	f := fake.New()
	i := admitted(f, "Score one agent")
	if _, err := AssignIssue(f, p, p, i.IID, "alice", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Claim(f, p, p, i.IID, Defaults(), false); err == nil || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("another person's pop = %v, want a refusal naming alice", err)
	}
	if _, err := AssignIssue(f, p, p, i.IID, f.Me.Username, false); err != nil {
		t.Fatal(err)
	}
	writes := f.Writes
	r, _, err := Claim(f, p, p, i.IID, Defaults(), false)
	if err != nil || r.Assigned || f.Writes != writes {
		t.Fatalf("assignee's pop = %+v, %v, writes %d", r, err, f.Writes-writes)
	}
}

func TestUnassignIssue(t *testing.T) {
	const p = "g/po-1"
	f := fake.New()
	i := admitted(f, "Score one agent")

	if r, err := UnassignIssue(f, p, i.IID, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("unassign of nobody's = %+v, %v", r, err)
	}
	if err := f.SetAssignee(p, i.IID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := UnassignIssue(f, p, i.IID, false); err == nil || !strings.Contains(err.Error(), "alice") {
		t.Fatalf("unassign of alice's = %v, want a refusal naming alice", err)
	}
	if err := f.SetAssignee(p, i.IID, f.Me.Username); err != nil {
		t.Fatal(err)
	}
	if r, err := UnassignIssue(f, p, i.IID, true); err != nil || r.Action != "unassigned" {
		t.Fatalf("dry run = %+v, %v", r, err)
	}
	if got, _ := f.GetIssue(p, i.IID); len(got.Assignees) != 1 {
		t.Fatalf("a dry run unassigned: %v", got.Assignees)
	}
	if _, err := UnassignIssue(f, p, i.IID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.GetIssue(p, i.IID); len(got.Assignees) != 0 {
		t.Fatalf("assignees = %v", got.Assignees)
	}

	// Given back, it can go to someone else even with its branch still there.
	if err := f.CreateBranch(p, branchOf(i), "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := AssignIssue(f, p, p, i.IID, "carol", false); err != nil {
		t.Fatalf("assign after unassign = %v", err)
	}
}

func TestListAssigned(t *testing.T) {
	const a, b = "g/po-1", "g/po-2"
	f := fake.New()
	mine, err := f.CreateIssue(a, tracker.NewIssue{Title: "mine", Description: "Implements: st001-x\n"})
	if err != nil {
		t.Fatal(err)
	}
	theirs, _ := f.CreateIssue(a, tracker.NewIssue{Title: "theirs"})
	there, _ := f.CreateIssue(b, tracker.NewIssue{Title: "there"})
	held, _ := f.CreateIssue(b, tracker.NewIssue{Title: "held", Description: "Blocked-by: #1\n"})
	for _, c := range []struct {
		project string
		iid     int
		who     string
	}{{a, mine.IID, f.Me.Username}, {a, theirs.IID, "alice"}, {b, there.IID, f.Me.Username}, {b, held.IID, f.Me.Username}} {
		if err := f.SetAssignee(c.project, c.iid, c.who); err != nil {
			t.Fatal(err)
		}
	}
	r, err := ListAssigned(f, []string{a, b, a})
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, i := range r.Issues {
		refs = append(refs, i.Ref)
	}
	if want := []string{"g/po-1#1", "g/po-2#1"}; !reflect.DeepEqual(refs, want) {
		t.Fatalf("issues = %v, want %v", refs, want)
	}
	if r.Issues[0].Story != "st001-x" {
		t.Fatalf("story = %q", r.Issues[0].Story)
	}
	if len(r.Blocked) != 1 || r.Blocked[0].Title != "held" {
		t.Fatalf("blocked = %+v", r.Blocked)
	}
}

func TestAssignCommandsRefuseBadUsage(t *testing.T) {
	t.Chdir(t.TempDir()) // outside any checkout, so no project is read from one
	for _, args := range [][]string{
		{"issue", "assign", "--project", "g/p", "--issue", "1"},
		{"issue", "assign", "--project", "g/p", "--person", "alice"},
		{"issue", "assign", "--issue", "1", "--person", "alice"},
		{"issue", "assign", "--project", "g/p", "--issue", "1", "--person", "../a"},
		{"issue", "unassign", "--project", "g/p"},
		{"issue", "list", "--mine", "--epic", "ep1", "--project", "g/p"},
	} {
		var out bytes.Buffer
		if err := cli.Default.Run(args, &out, nil); !errors.Is(err, cli.ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}

func branchOf(i tracker.Issue) string { return fmt.Sprintf("%d-%s", i.IID, Slug(i.Title)) }

func TestClaimRefusesATrackingIssue(t *testing.T) {
	f := fake.New()
	i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Epic: x", Labels: Labels("management", "tracking", "", "")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Claim(f, "g/po-1", "g/po-1", i.IID, Defaults(), false); err == nil || !strings.Contains(err.Error(), "tracking issue") {
		t.Fatalf("claim = %v, want a refusal naming why", err)
	}
	if got, _ := f.GetIssue("g/po-1", i.IID); len(got.Assignees) != 0 {
		t.Fatalf("a refused claim assigned %v", got.Assignees)
	}
}

func TestListsLeaveOutTrackingIssues(t *testing.T) {
	const p = "g/po-1"
	f := fake.New()
	mirror, _ := f.CreateIssue(p, tracker.NewIssue{Title: "Epic: x", Description: "Reviewer: " + f.Me.Username + "\n", Labels: Labels("management", "tracking", "", "")})
	if err := f.SetAssignees(p, mirror.IID, []string{f.Me.Username}); err != nil {
		t.Fatal(err)
	}
	work, _ := f.CreateIssue(p, tracker.NewIssue{Title: "Work"})
	if err := f.SetAssignee(p, work.IID, f.Me.Username); err != nil {
		t.Fatal(err)
	}
	r, err := ListAssigned(f, []string{p})
	if err != nil || len(r.Issues) != 1 || r.Issues[0].IID != work.IID {
		t.Fatalf("mine = %+v, %v", r.Issues, err)
	}
	reviews, err := LoadReviews(f, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetAssignees(p, mirror.IID, nil); err != nil {
		t.Fatal(err)
	}
	if r, err = ListReviewing(f, reviews, []string{p}); err != nil || len(r.Issues) != 0 {
		t.Fatalf("reviewing = %+v, %v", r.Issues, err)
	}
}

func TestTakeable(t *testing.T) {
	const p = "g/po-1"
	f := fake.New()
	free := admitted(f, "Free and admitted")
	mine := admitted(f, "Already mine")
	if err := f.SetAssignee(p, mine.IID, f.Me.Username); err != nil {
		t.Fatal(err)
	}
	theirs := admitted(f, "Someone else's")
	if err := f.SetAssignee(p, theirs.IID, "alice"); err != nil {
		t.Fatal(err)
	}
	_, _ = f.CreateIssue(p, tracker.NewIssue{Title: "A data issue not admitted", Labels: []string{"plane::data"}})
	_, _ = f.CreateIssue(p, tracker.NewIssue{Title: "Epic: x", Labels: Labels("management", "tracking", "", "")})
	closed := admitted(f, "Closed")
	state := "close"
	if _, err := f.UpdateIssue(p, closed.IID, tracker.IssueUpdate{State: &state}); err != nil {
		t.Fatal(err)
	}
	got, err := Takeable(f, p, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, i := range got {
		titles = append(titles, i.Title)
	}
	if !reflect.DeepEqual(titles, []string{free.Title, mine.Title}) {
		t.Fatalf("takeable = %q", titles)
	}
	writes := f.Writes
	err = noIssueNamed(f, p, Defaults())
	if err == nil || !strings.Contains(err.Error(), "Free and admitted") || !strings.Contains(err.Error(), "SCRUM_MASTER_ISSUE") || strings.Contains(err.Error(), "alice") {
		t.Fatalf("no issue named = %v", err)
	}
	if f.Writes != writes {
		t.Fatal("listing wrote")
	}
	if err := noIssueNamed(fake.New(), p, Defaults()); err == nil || !strings.Contains(err.Error(), "none can be taken") {
		t.Fatalf("nothing to take = %v", err)
	}
}
