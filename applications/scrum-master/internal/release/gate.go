// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Check results an evidence event records.
const (
	ResultPass    = "pass"
	ResultFail    = "fail"
	ResultPending = "pending"
)

// CheckResult is how one declared check ended.
type CheckResult struct {
	Test        string `json:"test"`
	Environment string `json:"environment"`
	Result      string `json:"result"`
	Reason      string `json:"reason,omitempty"`
}

// ReleaseGateResult is what scrum-master release gate reports (srd015 R1).
type ReleaseGateResult struct {
	Project  string `json:"project"`
	Issue    int    `json:"issue"`
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
	Tag      string `json:"tag"`
	Revision string `json:"revision"`
	Run      string `json:"run"`
	Passed   bool   `json:"passed"`
	// AwaitingPeerRead is a gate whose every other check passed and whose
	// peer read is pending: the state before scrum-master release approve, not a
	// failure (srd015 R1.3).
	AwaitingPeerRead bool          `json:"awaiting_peer_read"`
	Checks           []CheckResult `json:"checks"`
	// IssueClosed says the release issue is closed — as the planning or
	// sprint-end issue is once the merge that delivered it has landed, the
	// documented order — so the evidence lands on a closed issue (GH-20).
	IssueClosed bool `json:"issue_closed,omitempty"`
	DryRun      bool `json:"dry_run"`
}

// ReleaseGateInput is one gate run: the release issue, the candidate, and the
// checkout the local checks run in.
type ReleaseGateInput struct {
	Project  string
	Issue    int
	Artifact string
	Version  Version
	Dir      string
	Policy   program.Policy
	Git      Git
	Exec     Exec
	Out      io.Writer // the output of local checks
	Now      time.Time
	DryRun   bool
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9._/-]+`)

// testName identifies a check in its evidence event: kind, then suite or
// command, without spaces.
func testName(c program.Check) string {
	detail := c.Suite
	if detail == "" {
		detail = c.Command
	}
	if detail == "" {
		return c.Kind
	}
	return c.Kind + ":" + strings.Trim(nonWord.ReplaceAllString(detail, "-"), "-")
}

// RunGate runs the checks the policy declares for the artifact at the
// checkout's HEAD and posts one evidence event per check on the release
// issue (srd015 R1, srd008 R2). Local tests and custom checks run their
// command; peer-read passes on another person's thumbs-up or approval
// comment; any other
// check fails as not runnable (R2.5). A dry run runs nothing and posts
// nothing.
func RunGate(t tracker.Tracker, in ReleaseGateInput) (ReleaseGateResult, error) {
	r := ReleaseGateResult{Project: in.Project, Issue: in.Issue, Artifact: in.Artifact, Version: in.Version.String(),
		Tag: TagName(in.Policy.Releases.TagFormat, in.Artifact, in.Version), Run: in.Now.UTC().Format("20060102T150405Z"), DryRun: in.DryRun}
	checks := in.Policy.Gate(in.Artifact)
	if len(checks) == 0 {
		return r, fmt.Errorf("the policy declares no checks for %s and no default; a release needs a gate (srd008 R2.1)", in.Artifact)
	}
	if status, err := in.Git("status", "--porcelain"); err != nil {
		return r, err
	} else if status != "" {
		return r, errors.New("the gate runs on a clean checkout; commit or stash the changes first")
	}
	rev, err := in.Git("rev-parse", "HEAD")
	if err != nil {
		return r, err
	}
	r.Revision = rev
	issue, err := t.GetIssue(in.Project, in.Issue)
	if err != nil {
		return r, err
	}
	r.IssueClosed = issue.State == "closed"

	r.Passed = true
	awaiting, others := false, true
	for _, c := range checks {
		res := CheckResult{Test: testName(c), Environment: c.Environment}
		if !in.DryRun {
			res.Result, res.Reason = runCheck(t, in, issue, c)
		}
		if res.Environment == "" {
			res.Environment = "none"
		}
		r.Passed = r.Passed && res.Result == ResultPass
		switch {
		case c.Kind == "peer-read" && res.Result == ResultPending:
			awaiting = true
		case res.Result != ResultPass:
			others = false
		}
		r.Checks = append(r.Checks, res)
	}
	r.AwaitingPeerRead = awaiting && others && !r.Passed
	if in.DryRun {
		r.Passed = false
		return r, nil
	}
	for _, res := range r.Checks {
		e := events.Evidence("definition_of_done."+in.Artifact, res.Test, res.Environment, r.Tag, res.Result,
			events.KV{Key: "artifact", Value: in.Artifact}, events.KV{Key: "revision", Value: r.Revision}, events.KV{Key: "run", Value: r.Run})
		text := fmt.Sprintf("Release gate for %s at %s: %s %s.", r.Tag, short(r.Revision), res.Test, res.Result)
		if res.Environment != "" && res.Environment != "none" {
			// A peer read has no environment; "none" stays in the event's
			// fields, not its sentence (GH-196, GH-20).
			text = fmt.Sprintf("Release gate for %s at %s: %s %s in %s.", r.Tag, short(r.Revision), res.Test, res.Result, res.Environment)
		}
		if res.Reason != "" {
			text += " " + res.Reason
		}
		if _, err := events.Post(t, in.Project, in.Issue, e, events.EvidenceRequired, text, false); err != nil {
			return r, err
		}
	}
	return r, nil
}

func runCheck(t tracker.Tracker, in ReleaseGateInput, issue tracker.Issue, c program.Check) (string, string) {
	switch {
	case c.Kind == "peer-read":
		by, err := peerRead(t, in.Project, in.Issue, issue.Author)
		if err != nil {
			return ResultFail, err.Error()
		}
		if by != "" {
			return ResultPass, "approved by " + by + "."
		}
		return ResultPending, "waiting for a thumbs-up or a peer read, the peer-read skill, from someone other than " + issue.Author + "."
	case c.Kind != "tests" && c.Kind != "custom":
		return ResultFail, "a " + c.Kind + " check is not runnable by this scrum-master (srd015 R2.5)."
	case c.Environment != "local":
		return ResultFail, "environment " + strconv.Quote(c.Environment) + " is not runnable by this scrum-master; only local is (srd015 R2.5)."
	case strings.TrimSpace(c.Command) == "":
		return ResultFail, "the check declares no command to run."
	}
	if err := in.Exec(in.Dir, c.Command, in.Out); err != nil {
		return ResultFail, c.Command + " failed: " + err.Error() + "."
	}
	return ResultPass, ""
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

// IssueRef reads the release issue as project#iid, or as a bare iid in
// project, so SCRUM_MASTER_ISSUE means what it means to every other word
// (GH-109).
func IssueRef(ref, project string) (string, int, error) {
	if iid, err := strconv.Atoi(ref); err == nil && iid > 0 {
		if project == "" {
			return "", 0, fmt.Errorf("issue %q names no project: write project#iid or pass --project", ref)
		}
		return project, iid, nil
	}
	return ParseIssueRef(ref)
}

// ParseIssueRef reads project#iid.
func ParseIssueRef(ref string) (string, int, error) {
	project, n, ok := strings.Cut(ref, "#")
	iid, err := strconv.Atoi(n)
	if !ok || project == "" || err != nil || iid <= 0 {
		return "", 0, fmt.Errorf("issue %q is not project#iid", ref)
	}
	return project, iid, nil
}
