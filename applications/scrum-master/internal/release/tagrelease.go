// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TagInput is one scrum-master release tag: the release issue holding the gate's
// evidence, the candidate, and the checkout to tag.
type TagInput struct {
	Project  string
	Issue    int
	Artifact string
	Version  Version
	Policy   program.Policy
	Git      Git
	DryRun   bool
}

// TagResult is what scrum-master release tag reports (srd015 R2).
type TagResult struct {
	Project  string `json:"project"`
	Issue    int    `json:"issue"`
	Tag      string `json:"tag"`
	Revision string `json:"revision"`
	Previous string `json:"previous,omitempty"`
	Class    string `json:"class"`
	// Label is Class as the tag message and note say it.
	Label   string `json:"-"`
	GateRun string `json:"gate_run"`
	Pushed  bool   `json:"pushed"`
	// Closed is the release issue the tag closed, when it was still open.
	Closed bool `json:"closed,omitempty"`
	DryRun bool `json:"dry_run"`
}

// gateRun is the evidence one gate run posted for a candidate.
type gateRun struct {
	run, revision string
	results       map[string]string // test -> result
}

// latestGateRun reads the release issue's evidence events for the tag and
// returns the most recent run.
func latestGateRun(notes []tracker.Note, artifact, tag string) (gateRun, bool) {
	runs := map[string]*gateRun{}
	for _, n := range notes {
		e, ok := events.Parse(n.Body)
		if !ok || e.Name != "evidence" || first(e, "artifact") != artifact || first(e, "version") != tag {
			continue
		}
		id := first(e, "run")
		r := runs[id]
		if r == nil {
			r = &gateRun{run: id, revision: first(e, "revision"), results: map[string]string{}}
			runs[id] = r
		}
		r.results[first(e, "test")] = first(e, "result")
	}
	if len(runs) == 0 {
		return gateRun{}, false
	}
	ids := make([]string, 0, len(runs))
	for id := range runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return *runs[ids[len(ids)-1]], true
}

func first(e events.Event, key string) string {
	if v := e.Values(key); len(v) > 0 {
		return v[0]
	}
	return ""
}

// ReleaseTag creates and pushes the candidate's tag at HEAD, only when the
// most recent gate run for that artifact and version passed every declared
// check at that revision (srd015 R2.1, srd008 R2.4) and the version is
// greater than the artifact's latest tag (R2.2). The tag message and a
// comment on the release issue state the class (R2.3). It runs no handoff
// (R2.4).
func ReleaseTag(t tracker.Tracker, in TagInput) (TagResult, error) {
	tag := TagName(in.Policy.Releases.TagFormat, in.Artifact, in.Version)
	r := TagResult{Project: in.Project, Issue: in.Issue, Tag: tag, DryRun: in.DryRun}
	if status, err := in.Git("status", "--porcelain"); err != nil {
		return r, err
	} else if status != "" {
		return r, errors.New("scrum-master release tag runs on a clean checkout; commit or stash the changes first")
	}
	rev, err := in.Git("rev-parse", "HEAD")
	if err != nil {
		return r, err
	}
	r.Revision = rev

	local, err := in.Git("tag", "--list")
	if err != nil {
		return r, err
	}
	remote, err := RemoteTags(in.Git, "*")
	if err != nil {
		return r, err
	}
	tags := strings.Fields(local + "\n" + remote)
	latest, found := Latest(in.Policy.Releases.TagFormat, in.Artifact, tags)
	if found {
		r.Previous = latest.String()
		if !latest.Less(in.Version) {
			return r, fmt.Errorf("%s is not greater than the latest release %s; scrum-master release next lists the candidates", in.Version, latest)
		}
	}

	notes, err := t.ListNotes(in.Project, in.Issue)
	if err != nil {
		return r, err
	}
	run, ok := latestGateRun(notes, in.Artifact, tag)
	if !ok {
		return r, fmt.Errorf("no gate has run for %s on %s#%d; run the release skill, whose gate comes first", tag, in.Project, in.Issue)
	}
	r.GateRun = run.run
	if run.revision != rev {
		return r, fmt.Errorf("the latest gate for %s ran at %s, not at HEAD %s; run the release skill again, which gates HEAD", tag, short(run.revision), short(rev))
	}
	issue, err := t.GetIssue(in.Project, in.Issue)
	if err != nil {
		return r, err
	}
	var failing []string
	for _, c := range in.Policy.Gate(in.Artifact) {
		res := run.results[testName(c)]
		if c.Kind == "peer-read" && res == ResultPending {
			// An approval made after the gate ran counts: the peer read is
			// read again now, not only at gate time.
			if by, err := peerRead(t, in.Project, in.Issue, issue.Author); err != nil {
				return r, err
			} else if by != "" {
				res = ResultPass
			}
		}
		if res != ResultPass {
			if res == "" {
				res = "missing"
			}
			failing = append(failing, testName(c)+" "+res)
		}
	}
	if len(failing) > 0 {
		return r, fmt.Errorf("the latest gate for %s (run %s) did not pass: %s", tag, run.run, strings.Join(failing, ", "))
	}

	r.Class = Class(in.Policy.Releases.Breaking, latest, in.Version)
	r.Label = ClassLabel(in.Policy.Releases.Breaking, latest, in.Version)
	if in.DryRun {
		return r, nil
	}
	message := fmt.Sprintf("Release %s, %s\n\nGate run %s passed at %s on %s#%d.", tag, r.Label, run.run, rev, in.Project, in.Issue)
	if r.Previous != "" {
		message = fmt.Sprintf("Release %s, %s (previous %s)\n\nGate run %s passed at %s on %s#%d.", tag, r.Label, r.Previous, run.run, rev, in.Project, in.Issue)
	}
	if _, err := in.Git("tag", "-a", tag, rev, "-m", message); err != nil {
		return r, err
	}
	if _, err := in.Git("push", "origin", "refs/tags/"+tag); err != nil {
		return r, fmt.Errorf("created %s but could not push it: %w", tag, err)
	}
	r.Pushed = true
	note := fmt.Sprintf("Tagged %s at %s: a %s release.\n\n%s", tag, short(rev), r.Label, TaggedMarker)
	if err := t.CreateNote(in.Project, in.Issue, note); err != nil {
		return r, err
	}
	// A release issue filed for the release closes on its tag; a sprint-end
	// or planning issue its merge closed stays as it is.
	if issue.State != "closed" {
		closed := "close"
		if _, err := t.UpdateIssue(in.Project, in.Issue, tracker.IssueUpdate{State: &closed}); err != nil {
			return r, fmt.Errorf("tagged %s, but %s#%d did not close: %w", tag, in.Project, in.Issue, err)
		}
		r.Closed = true
	}
	return r, nil
}
