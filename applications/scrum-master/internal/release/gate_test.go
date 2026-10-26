// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// gateFixture is a release issue on a tracker, a clean checkout at
// revision abc123, and a recorder of the commands local checks run.
type gateFixture struct {
	tr    tracker.Tracker
	issue tracker.Issue
	ran   []string
	fail  map[string]bool
	dirty bool
}

func newGateFixture(t *testing.T, tr tracker.Tracker) *gateFixture {
	t.Helper()
	i, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Release a 0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	return &gateFixture{tr: tr, issue: i, fail: map[string]bool{}}
}

func (g *gateFixture) run(p program.Policy, artifact string, at time.Time) (ReleaseGateResult, error) {
	v, _ := ParseVersion("0.3.0")
	return RunGate(g.tr, ReleaseGateInput{Project: "g/po-1", Issue: g.issue.IID, Artifact: artifact, Version: v, Dir: "/repo",
		Policy: p, Out: io.Discard, Now: at,
		Git: func(args ...string) (string, error) {
			switch args[0] {
			case "status":
				if g.dirty {
					return " M x.go", nil
				}
				return "", nil
			case "rev-parse":
				return "abc123", nil
			}
			return "", nil
		},
		Exec: func(dir, command string, _ io.Writer) error {
			g.ran = append(g.ran, command)
			if g.fail[command] {
				return errors.New("exit status 1")
			}
			return nil
		}})
}

func (g *gateFixture) evidence() []events.Event {
	var out []events.Event
	notes, _ := g.tr.ListNotes("g/po-1", g.issue.IID)
	for _, n := range notes {
		if e, ok := events.Parse(n.Body); ok && e.Name == "evidence" {
			out = append(out, e)
		}
	}
	return out
}

func policyWith(artifact string, checks ...program.Check) program.Policy {
	p := program.Defaults()
	p.DefinitionOfDone = map[string][]program.Check{artifact: checks}
	return p
}

var t0 = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

func TestGateRunsDeclaredChecks(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) { testGateRunsDeclaredChecks(t, tp) })
}

func testGateRunsDeclaredChecks(t *testing.T, tp trackertest.Program) {
	g := newGateFixture(t, tp.Tracker())
	p := policyWith("a",
		program.Check{Kind: "tests", Environment: "local", Suite: "unit", Command: "go test ./..."},
		program.Check{Kind: "custom", Environment: "local", Command: "make lint"})
	g.fail["make lint"] = true
	r, err := g.run(p, "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Passed || len(r.Checks) != 2 || r.Checks[0].Result != ResultPass || r.Checks[1].Result != ResultFail {
		t.Fatalf("result %+v", r)
	}
	if strings.Join(g.ran, "|") != "go test ./...|make lint" {
		t.Fatalf("ran %v", g.ran)
	}
	ev := g.evidence()
	if len(ev) != 2 {
		t.Fatalf("evidence events %d, want one per check", len(ev))
	}
	e := ev[0]
	for k, want := range map[string]string{"requirement": "definition_of_done.a", "test": "tests:unit", "environment": "local",
		"version": "a/v0.3.0", "result": "pass", "artifact": "a", "revision": "abc123", "run": "20261016T090000Z"} {
		if got := e.Values(k); len(got) != 1 || got[0] != want {
			t.Errorf("evidence %s = %v, want %s", k, got, want)
		}
	}

	g.fail["make lint"] = false
	r, err = g.run(p, "a", t0.Add(time.Hour))
	if err != nil || !r.Passed {
		t.Fatalf("second run: %+v %v", r, err)
	}
	if len(g.evidence()) != 4 {
		t.Fatalf("a second run posts its own evidence: %d events", len(g.evidence()))
	}
}

func TestDocumentsOnlyGateIsMageAudit(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) { testDocumentsOnlyGateIsMageAudit(t, tp) })
}

func testDocumentsOnlyGateIsMageAudit(t *testing.T, tp trackertest.Program) {
	g := newGateFixture(t, tp.Tracker())
	r, err := g.run(policyWith("corpus", program.Check{Kind: "custom", Environment: "local", Command: "mage audit"}), "corpus", t0)
	if err != nil || !r.Passed || strings.Join(g.ran, "|") != "mage audit" || r.Checks[0].Test != "custom:mage-audit" {
		t.Fatalf("r=%+v ran=%v err=%v", r, g.ran, err)
	}
}

func TestGatePeerReadNeedsAnotherPersonsThumbsUp(t *testing.T) {
	f := fake.New()
	g := newGateFixture(t, f)
	p := policyWith("a", program.Check{Kind: "peer-read"})
	key := "g/po-1#1"
	f.Awards[key] = []tracker.Award{{Name: "thumbsup", User: g.issue.Author}, {Name: "heart", User: "reviewer"}}
	r, err := g.run(p, "a", t0)
	if err != nil || r.Passed || r.Checks[0].Result != ResultPending || r.Checks[0].Environment != "none" {
		t.Fatalf("own thumbs-up: %+v %v", r, err)
	}
	// The evidence sentence names no "none" environment (GH-196, GH-20).
	posted := false
	for _, n := range f.Notes[key] {
		if strings.Contains(n.Body, "Release gate for") {
			posted = true
			if strings.Contains(n.Body, " in none") {
				t.Fatalf("the evidence sentence names in none: %q", n.Body)
			}
		}
	}
	if !posted {
		t.Fatalf("the gate posted no evidence on %s: %+v", key, f.Notes[key])
	}
	f.Awards[key] = append(f.Awards[key], tracker.Award{Name: "thumbsup", User: "reviewer"})
	r, err = g.run(p, "a", t0.Add(time.Minute))
	if err != nil || !r.Passed || !strings.Contains(r.Checks[0].Reason, "reviewer") {
		t.Fatalf("reviewer thumbs-up: %+v %v", r, err)
	}
}

func TestGateFailsChecksItCannotRun(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) { testGateFailsChecksItCannotRun(t, tp) })
}

func testGateFailsChecksItCannotRun(t *testing.T, tp trackertest.Program) {
	g := newGateFixture(t, tp.Tracker())
	p := policyWith("a",
		program.Check{Kind: "scorecard", Environment: "dev", Against: "current", Threshold: "no-regression"},
		program.Check{Kind: "contract-tests", Environment: "dev", Consumers: "all"},
		program.Check{Kind: "tests", Environment: "dev", Suite: "subsystem-install", Command: "go test ./..."},
		program.Check{Kind: "tests", Environment: "local", Suite: "unit"})
	r, err := g.run(p, "a", t0)
	if err != nil || r.Passed || len(g.ran) != 0 {
		t.Fatalf("r=%+v ran=%v err=%v", r, g.ran, err)
	}
	for _, c := range r.Checks {
		if c.Result != ResultFail || c.Reason == "" {
			t.Errorf("%s: %s %q", c.Test, c.Result, c.Reason)
		}
	}
	if !strings.Contains(r.Checks[0].Reason, "not runnable") || !strings.Contains(r.Checks[2].Reason, "not runnable") || !strings.Contains(r.Checks[3].Reason, "no command") {
		t.Fatalf("reasons %+v", r.Checks)
	}
}

func TestGateRefusesNoChecksOrADirtyCheckout(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) { testGateRefusesNoChecksOrADirtyCheckout(t, tp) })
}

func testGateRefusesNoChecksOrADirtyCheckout(t *testing.T, tp trackertest.Program) {
	g := newGateFixture(t, tp.Tracker())
	p := program.Defaults()
	p.DefinitionOfDone = map[string][]program.Check{"other": {{Kind: "custom", Environment: "local", Command: "true"}}}
	if _, err := g.run(p, "a", t0); err == nil || !strings.Contains(err.Error(), "no checks") {
		t.Fatalf("no checks: %v", err)
	}
	g.dirty = true
	if _, err := g.run(program.Defaults(), "a", t0); err == nil || !strings.Contains(err.Error(), "clean checkout") {
		t.Fatalf("dirty: %v", err)
	}
	if len(g.evidence()) != 0 {
		t.Fatal("a refused gate posted evidence")
	}
}

// TestGatePeerReadAcceptsAnotherPersonsApproval: on every tracker, a peer
// read is also an approval comment that scrum-master release approve posts as the
// person running it (srd015 R1.2). The author cannot approve their own
// release, the gate stays pending until someone else does, and a second
// approval by the same person posts nothing.
func TestGatePeerReadAcceptsAnotherPersonsApproval(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, tp trackertest.Program) {
		g := newGateFixture(t, tp.Tracker())
		p := policyWith("a", program.Check{Kind: "peer-read"})

		if _, err := Approve(g.tr, "g/po-1", g.issue.IID); err == nil || !strings.Contains(err.Error(), "author") {
			t.Fatalf("author's approval = %v, want a refusal naming the author", err)
		}
		r, err := g.run(p, "a", t0)
		if err != nil || r.Passed || r.Checks[0].Result != ResultPending || !r.AwaitingPeerRead {
			t.Fatalf("before approval: %+v %v, want awaiting the peer read", r, err)
		}

		reviewer := tp.As("reviewer")
		a, err := Approve(reviewer, "g/po-1", g.issue.IID)
		if err != nil || !a.Posted {
			t.Fatalf("reviewer's approval = %+v, %v", a, err)
		}
		if again, err := Approve(reviewer, "g/po-1", g.issue.IID); err != nil || again.Posted {
			t.Fatalf("second approval = %+v, %v, want nothing posted", again, err)
		}
		r, err = g.run(p, "a", t0.Add(time.Minute))
		if err != nil || !r.Passed || r.AwaitingPeerRead || !strings.Contains(r.Checks[0].Reason, "reviewer") {
			t.Fatalf("after approval: %+v %v", r, err)
		}
	})
}

// TestGateLine prints each check as scrum-master release gate does: no "in none" for
// a check with no environment, such as the peer read, and no trailing space
// on a line with no reason (#188).
func TestGateLine(t *testing.T) {
	for _, c := range []struct {
		check CheckResult
		want  string
	}{
		{CheckResult{Result: "pending", Test: "peer-read", Environment: "none", Reason: "awaiting scrum-master release approve"}, "  pending  peer-read awaiting scrum-master release approve"},
		{CheckResult{Result: "pass", Test: "unit", Environment: "local"}, "  pass     unit in local"},
		{CheckResult{Result: "fail", Test: "unit", Environment: "local", Reason: "exit status 1"}, "  fail     unit in local exit status 1"},
	} {
		if got := gateLine(c.check); got != c.want {
			t.Errorf("gateLine(%+v) = %q, want %q", c.check, got, c.want)
		}
	}
}

// A gate on a closed release issue — the documented order for a plan or a
// sprint end, gated after the merge that closed it — says so rather than
// running silently (GH-20).
func TestGateReportsAClosedReleaseIssue(t *testing.T) {
	f := fake.New()
	g := newGateFixture(t, f)
	p := policyWith("a", program.Check{Kind: "peer-read"})
	r, err := g.run(p, "a", t0)
	if err != nil || r.IssueClosed {
		t.Fatalf("open issue: closed=%v %v", r.IssueClosed, err)
	}
	closed := "close"
	if _, err := f.UpdateIssue("g/po-1", g.issue.IID, tracker.IssueUpdate{State: &closed}); err != nil {
		t.Fatal(err)
	}
	if r, err = g.run(p, "a", t0.Add(time.Minute)); err != nil || !r.IssueClosed {
		t.Fatalf("closed issue: closed=%v %v", r.IssueClosed, err)
	}
}

// A peer read counts only after the release issue's last tag: one given
// for the release that tag cut is not reused for the next (GH-68).
func TestPeerReadAfterTheLastTag(t *testing.T) {
	f := fake.New()
	f.Me = tracker.User{Username: "author"}
	i, _ := f.CreateIssue("g/p", tracker.NewIssue{Title: "Release"})
	f.Me = tracker.User{Username: "reviewer"}
	f.CreateNote("g/p", i.IID, "Read it.\n\n"+PeerReadMarker)
	if by, err := peerRead(f, "g/p", i.IID, "author"); err != nil || by != "reviewer" {
		t.Fatalf("a peer read before any tag = %q, %v", by, err)
	}
	f.Me = tracker.User{Username: "author"}
	f.CreateNote("g/p", i.IID, "Tagged grader/v0.1.0 at abc: a compatible release.\n\n"+TaggedMarker)
	if by, err := peerRead(f, "g/p", i.IID, "author"); err != nil || by != "" {
		t.Fatalf("the earlier release's peer read was reused: %q, %v", by, err)
	}
	f.Me = tracker.User{Username: "reviewer"}
	f.CreateNote("g/p", i.IID, "Read the next.\n\n"+PeerReadMarker)
	if by, err := peerRead(f, "g/p", i.IID, "author"); err != nil || by != "reviewer" {
		t.Fatalf("a peer read after the tag = %q, %v", by, err)
	}
}

// The release issue is project#iid, or a bare iid in --project as every
// other word takes SCRUM_MASTER_ISSUE; a bare iid with no project is a
// usage error naming both forms (GH-109).
func TestIssueRefTakesBothForms(t *testing.T) {
	for _, c := range []struct{ ref, project string }{{"acme/system#5", ""}, {"5", "acme/system"}, {"acme/system#5", "acme/other"}} {
		project, iid, err := IssueRef(c.ref, c.project)
		if err != nil || project != "acme/system" || iid != 5 {
			t.Errorf("IssueRef(%q, %q) = %q, %d, %v", c.ref, c.project, project, iid, err)
		}
	}
	if _, _, err := IssueRef("5", ""); err == nil || !strings.Contains(err.Error(), "project#iid or pass --project") {
		t.Errorf("bare iid with no project: %v", err)
	}
}
