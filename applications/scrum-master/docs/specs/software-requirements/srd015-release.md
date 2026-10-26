---
id: srd015-release
title: "Script: release (scrum-master release ...)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

A release is the point where an artifact becomes something other
repositories may depend on, so it is where the checks declared for that
artifact must run (srd008 R2). If tagging and checking are separate manual
steps, tags appear without evidence, and consumers adapt to versions nobody
verified. A program repository has the agent and no build tool of
its own, and creating the tag is a rule rather than a judgement
(design.md D10), so scrum-master does all of it: the gate's evidence, the next
version, the class of the release, and the tag. The person chooses the
version bump; scrum-master refuses a tag the evidence does not support.

<!-- field: goals -->
## Goals

- G1: A version tag exists only for a candidate whose gate produced passing evidence.
- G2: Whether a release is breaking is computed from the version, not argued about.
- G3: The tests that produced a release's evidence ship with it.

<!-- req: R1 -->
## R1 - scrum-master release gate

<!-- req: R1.1 -->
### R1.1

For an artifact and a candidate revision, runs every check the policy declares for the artifact (srd008 R2.1, R2.2), each in its named environment, and posts each result as an evidence event (srd006 R9) on the release issue. A tests or custom check whose environment is local runs the check's command from the repository root.

<!-- req: R1.2 -->
### R1.2

A peer-read check passes when a person other than the release issue's author has approved the release issue, either with a thumbs-up award emoji, on a tracker that has awards, or with an approval comment carrying the marker `<!-- scrum-master release peer-read -->`, which scrum-master release approve posts as the person who runs it and refuses to post for the issue's author. On the beads tracker, which has no awards, the comment is the approval (srd042). The subcommand reports the check pending otherwise and does not wait. scrum-master release tag reads a pending peer read again, so an approval made after the gate ran counts without running the gate again.

<!-- req: R1.3 -->
### R1.3

Exits zero only when every check passed, or when the peer read is the only check pending, the state before scrum-master release approve: it then says the tag waits on that approval.

<!-- req: R2 -->
## R2 - scrum-master release tag

<!-- req: R2.1 -->
### R2.1

scrum-master release tag creates the tag in policy.releases.tag_format for the artifact and version at the candidate revision, and pushes it, only when the most recent gate run for that artifact, version, and revision passed.

<!-- req: R2.2 -->
### R2.2

A version that is not greater than the latest tag of the artifact under semantic versioning is refused.

<!-- req: R2.3 -->
### R2.3

The release is classified as breaking or compatible by policy.releases.breaking, and the class is stated in the tag's message and on the release issue; a minor bump that breaks only because the policy makes minors before 1.0 breaking says so, as breaking (minor before 1.0). A release issue still open when its tag is pushed closes on it; one a merge already closed, such as a sprint-end issue, stays as it is.

<!-- req: R2.4 -->
### R2.4

scrum-master release tag runs no handoff. After the tag is pushed, the release profile runs the post-tag steps: scrum-master handoff on-release (srd014 R7.1) and, in the system repository, the deck render from the merged sprint-end summary (srd018 R10). A post-tag step that fails ends the run in a state naming the profile to re-run, and neither undoes nor fails the release.

<!-- req: R2.5 -->
### R2.5

A declared check this scrum-master cannot run, a contract-tests or scorecard check or any environment other than local, fails the gate with that reason rather than passing, so no tag follows it.

<!-- req: R3 -->
## R3 - scrum-master release next

<!-- req: R3.1 -->
### R3.1

Reports the latest tag of an artifact and the next patch, minor, and major versions, with the class each would have, and recommends one from the commits since the latest tag: minor for a first release, major when a commit says BREAKING CHANGE, patch when every commit fixes something or when the checkout is a corpus whose requirements (its SRDs, initiatives, objectives, and road map) no commit since the tag changed, a republish, minor for any other change, and none when nothing was committed since.

<!-- req: R4 -->
## R4 - Test software ships with the release

<!-- req: R4.1 -->
### R4.1

The release issue records the test-subsystem release whose suites produced the gate's evidence, and the release is published with that pinned reference (srd007 R5), so a consumer can run the same suites.

<!-- req: R5 -->
## R5 - Common behavior

<!-- req: R5.1 -->
### R5.1

Subcommands are in the scrum-master group release, honour --json and --dry-run, and never prompt (srd010 R1).

<!-- field: non_goals -->
## Non-goals

- Choosing the version bump; the person decides it from what scrum-master release next reports.
- Promotion between environments; that is CI/CD in the test subsystem (srd008).
- Publishing packages or images; a program attaches those to the tag through its own CI.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

gate runs exactly the declared checks in their environments, local ones through their command, posts each as an evidence event on the release issue, reports peer-read pending until another person's thumbs-up or approval comment, fails a check it cannot run, and exits zero only when all pass.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R2.5](#r25)

<!-- ac: AC2 -->
### AC2

scrum-master release tag tags and pushes only after a passing gate for the same artifact, version, and revision, refuses a non-increasing version, records the class, and runs no handoff itself; the release profile's post-tag steps run handoff and, in the system repository, the deck, and a post-tag failure names the profile to re-run and leaves the tag.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)

<!-- ac: AC3 -->
### AC3

next reports the latest tag and the three candidate versions with their classes, recommending no republish after a change to the road map or an initiative; the release names the test-subsystem release that produced its evidence; all subcommands honour --json and --dry-run and never prompt.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R4.1](#r41)
- [R5.1](#r51)
