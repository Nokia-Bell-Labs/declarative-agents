// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Release)
}

// Release runs scrum-master release next, gate, and tag on a clone of po-1 with the policy seeded in the system repository: no tag before a gate, evidence events on the release issue, peer-read pending while only the author could approve, a pushed tag after a passing gate, no second tag at the same version, and the peer read passing on a reviewer's scrum-master release approve (test-rel01.2-sprint-end).
func (Integration) Release() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	system, err := r.project("system")
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	if err := r.seed(system, map[string]string{program.PolicyFile: `---
id: policy
title: Rig policy
---

<!-- field: version -->
## Version

1

<!-- field: releases -->
## Releases

<!-- field: scheme -->
### Scheme

semver

<!-- field: breaking -->
### Breaking

major-and-minor-before-1.0

<!-- field: tag_format -->
### Tag format

{artifact}/v{version}

<!-- field: definition_of_done -->
## Definition of done

<!-- field: grader -->
### Grader

| kind | environment | command |
| --- | --- | --- |
| custom | local | test -d .git |

<!-- field: reviewed -->
### Reviewed

| kind | environment | command |
| --- | --- | --- |
| custom | local | test -d .git |
| peer-read |  |  |
`}); err != nil {
		return err
	}
	clone, err := r.clone(po1, "po-1-release")
	if err != nil {
		return err
	}
	issue, err := r.tr.CreateIssue(po1, tracker.NewIssue{Title: "Release grader 0.1.0"})
	if err != nil {
		return err
	}
	ref := fmt.Sprintf("%s#%d", po1, issue.IID)
	common := []string{"--control", system, "--ref", seedBranch, "--dir", clone}
	c := &check{}

	var next release.NextResult
	if err := r.wordsJSON(&next, append([]string{"release", "next", "--artifact", "grader"}, common...)...); err != nil {
		return err
	}
	c.expect(next.Latest == "" && next.Candidates[1].Version == "0.1.0", "next before any release: %+v", next)

	_, err = r.runWords(append([]string{"release", "tag", "--artifact", "grader", "--version", "0.1.0", "--issue", ref}, common...)...)
	c.expect(err != nil && strings.Contains(err.Error(), "no gate has run"), "tag before a gate: %v", err)

	var gate release.ReleaseGateResult
	if err := r.wordsJSON(&gate, append([]string{"release", "gate", "--artifact", "grader", "--version", "0.1.0", "--issue", ref}, common...)...); err != nil {
		return err
	}
	c.expect(gate.Passed && len(gate.Checks) == 1, "grader gate: %+v", gate)

	_, err = r.runWords("release", "approve", "--project", po1, "--issue", fmt.Sprint(issue.IID))
	c.expect(err != nil && strings.Contains(err.Error(), "author"), "the author approved their own release: %v", err)
	// A pending peer read alone is the state before release approve, not a
	// failure: the gate exits zero, does not pass, and says it waits (#151).
	var awaiting release.ReleaseGateResult
	err = r.wordsJSON(&awaiting, append([]string{"release", "gate", "--artifact", "reviewed", "--version", "0.1.0", "--issue", ref}, common...)...)
	c.expect(err == nil && !awaiting.Passed && awaiting.AwaitingPeerRead, "peer-read with no approval from anyone else: %+v, %v", awaiting, err)

	var tagged release.TagResult
	if err := r.wordsJSON(&tagged, append([]string{"release", "tag", "--artifact", "grader", "--version", "0.1.0", "--issue", ref}, common...)...); err != nil {
		return err
	}
	c.expect(tagged.Pushed && tagged.Tag == "grader/v0.1.0" && tagged.Class == "breaking", "tag: %+v", tagged)
	_, terr := r.remoteTag(po1, "grader/v0.1.0")
	c.expect(terr == nil, "grader/v0.1.0 not at the origin: %v", terr)

	notes, err := r.notes(po1, issue.IID)
	if err != nil {
		return err
	}
	var evidence, pending, taggedNote int
	for _, n := range notes {
		switch {
		case strings.Contains(n.Body, "event=evidence") && strings.Contains(n.Body, "result=pending"):
			pending++
			evidence++
		case strings.Contains(n.Body, "event=evidence"):
			evidence++
		case strings.Contains(n.Body, "Tagged grader/v0.1.0"):
			taggedNote++
		}
	}
	c.expect(evidence == 3 && pending == 1 && taggedNote == 1, "release issue notes: %d evidence, %d pending, %d tagged", evidence, pending, taggedNote)

	_, err = r.runWords(append([]string{"release", "tag", "--artifact", "grader", "--version", "0.1.0", "--issue", ref}, common...)...)
	c.expect(err != nil && strings.Contains(err.Error(), "not greater"), "a second tag at 0.1.0: %v", err)

	// Another person's approval is the peer read (srd015 R1.2), run as the
	// reviewer's peer-read workflow with the agent (GH-37).
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	if out, err := r.runWorkflow(rel, "peer-read", rigReviewer, clone, po1, fmt.Sprint(issue.IID), nil); err != nil || !strings.Contains(out, "final machine state: Done") {
		return fmt.Errorf("the peer-read run: %v: %s", err, tail17(out))
	}
	var reviewed release.ReleaseGateResult
	if err := r.wordsJSON(&reviewed, append([]string{"release", "gate", "--artifact", "reviewed", "--version", "0.1.0", "--issue", ref}, common...)...); err != nil {
		return err
	}
	c.expect(reviewed.Passed, "peer-read after the reviewer's approval: %+v", reviewed)
	// That peer read was for reviewed's 0.1.0: once it is tagged, the next
	// version waits for a new one rather than reuse it (GH-68).
	var reviewedTag release.TagResult
	if err := r.wordsJSON(&reviewedTag, append([]string{"release", "tag", "--artifact", "reviewed", "--version", "0.1.0", "--issue", ref}, common...)...); err != nil {
		return err
	}
	var nextGate release.ReleaseGateResult
	err = r.wordsJSON(&nextGate, append([]string{"release", "gate", "--artifact", "reviewed", "--version", "0.2.0", "--issue", ref}, common...)...)
	c.expect(err == nil && !nextGate.Passed && nextGate.AwaitingPeerRead, "the next version reused the earlier peer read: %+v, %v", nextGate, err)
	// The release workflow (GH-37): a fix since grader/v0.1.0 calls for a
	// patch, which the profile gates and tags with the agent.
	// The run starts in a checkout beside the system repository's, where a
	// word reads the program's control files.
	wf := filepath.Join(r.run.Dir, "checkouts", "po-1-release")
	if err := gitIn(r.run.Dir, "clone", "-q", r.origin(po1), wf); err != nil {
		return err
	}
	writeFileTo(filepath.Join(wf, "FIX.md"), "a fix\n")
	if err := r.commitPush(wf, "Fix the grader's rounding", "FIX.md"); err != nil {
		return err
	}
	out, rerr := r.runWorkflow(rel, "release", rigUser, wf, po1, ref, nil, "SCRUM_MASTER_ARTIFACT=grader", "SCRUM_MASTER_CONTROL="+system, "SCRUM_MASTER_REF="+seedBranch)
	c.expect(rerr == nil && strings.Contains(out, "final machine state: Done"), "the release run: %v: %s", rerr, tail17(out))
	c.expect(strings.Contains(out, "next recommended_version: 0.1.1") && strings.Contains(out, "released tag: grader/v0.1.1"),
		"the release run must print its version and tag as lines (GH-49): %s", tail17(out))
	_, terr = r.remoteTag(po1, "grader/v0.1.1")
	c.expect(terr == nil, "the release run left no grader/v0.1.1 at the origin: %v", terr)

	// A chosen version (GH-47): another fix calls for a patch, and the
	// request chooses the minor instead, which the gate and tag use.
	writeFileTo(filepath.Join(wf, "FIX2.md"), "another fix\n")
	if err := r.commitPush(wf, "Fix the grader's rounding again", "FIX2.md"); err != nil {
		return err
	}
	out, rerr = r.runWorkflow(rel, "release", rigUser, wf, po1, ref, map[string]any{"version": "0.2.0"},
		"SCRUM_MASTER_ARTIFACT=grader", "SCRUM_MASTER_CONTROL="+system, "SCRUM_MASTER_REF="+seedBranch)
	c.expect(rerr == nil && strings.Contains(out, "final machine state: Done"), "the chosen-version release run: %v: %s", rerr, tail17(out))
	_, terr = r.remoteTag(po1, "grader/v0.2.0")
	c.expect(terr == nil, "the chosen-version release left no grader/v0.2.0 at the origin: %v", terr)
	_, terr = r.remoteTag(po1, "grader/v0.1.2")
	c.expect(terr != nil, "the chosen-version release tagged the recommended grader/v0.1.2 too")

	// Nothing to release (GH-64): with no commit since grader/v0.2.0, the
	// run ends in Refused with next's reason, not Failed.
	out, rerr = r.runWorkflow(rel, "release", rigUser, wf, po1, ref, nil,
		"SCRUM_MASTER_ARTIFACT=grader", "SCRUM_MASTER_CONTROL="+system, "SCRUM_MASTER_REF="+seedBranch)
	c.expect(rerr != nil && strings.Contains(out, "final machine state: Refused") && strings.Contains(out, "nothing to release"),
		"a release with nothing to release must end in Refused with the reason: %v: %s", rerr, tail17(out))
	return c.err("release")
}
