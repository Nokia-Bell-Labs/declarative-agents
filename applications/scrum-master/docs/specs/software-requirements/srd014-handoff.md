---
id: srd014-handoff
title: "Script: handoff (scrum-master handoff ...)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

When one component's work lands, other components often owe something
because of it: an upstream issue's author should hear the downstream work
finished, consumers of a new release must adapt or at least know, older
adaptation work may be overtaken. Done by hand, these transfers are
forgotten or duplicated, and the person who must act finds out late.

Handoff turns two tracker events, a merge and a release tag, into the
comments and issues those events imply. It never assigns anyone; what it
creates enters the receiving repository's backlog. Who consumes an
interface is not a hand-kept map: it is the set of repositories that cite
the interface by pinned URL (srd007 R5.4), which track-goals collects.

<!-- field: goals -->
## Goals

- G1: A merge notifies the issue upstream of what it closed.
- G2: A release creates exactly the work and the notices its consumers are owed.
- G3: Running handoff twice over the same event changes nothing.

<!-- req: R1 -->
## R1 - scrum-master handoff on-merge

<!-- req: R1.1 | status: retired | retired_in: 5 -->
### R1.1

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R1.2 | status: retired | retired_in: 5 -->
### R1.2

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R1.3 | status: retired | retired_in: 5 -->
### R1.3

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R2 -->
## R2 - scrum-master handoff on-release

<!-- req: R2.1 | status: retired | retired_in: 5 -->
### R2.1

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R2.2 | status: retired | retired_in: 5 -->
### R2.2

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R2.3 | status: retired | retired_in: 5 -->
### R2.3

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R2.4 | status: retired | retired_in: 5 -->
### R2.4

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R2.5 | status: retired | retired_in: 5 -->
### R2.5

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R2.6 | status: retired | retired_in: 5 -->
### R2.6

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R3 -->
## R3 - Tracking-issue membership

<!-- req: R3.1 | status: retired | retired_in: 5 -->
### R3.1

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R4 -->
## R4 - scrum-master handoff sweep

<!-- req: R4.1 | status: retired | retired_in: 5 -->
### R4.1

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R5 -->
## R5 - Idempotency and findings

<!-- req: R5.1 -->
### R5.1

Every issue handoff creates carries a handoff marker naming the merge or tag that caused it. Before creating, handoff searches the target project for that marker and consumer and skips creation when it exists.

<!-- req: R5.2 | status: retired | retired_in: 5 -->
### R5.2

Retired in 05.0 (design.md D12): direction in R9.

<!-- req: R5.3 -->
### R5.3

A Downstream section naming an artifact no SRD declares, or a release of an artifact with neither subscriptions nor citing repositories, creates nothing for it, records a finding the dashboard shows (srd019), and makes the subcommand exit non-zero.

<!-- req: R6 -->
## R6 - Never assign, never close

<!-- req: R6.1 -->
### R6.1

Handoff sets no assignee, closes no issue, and changes no label except setting needs-run on recurring issues.

<!-- req: R7 -->
## R7 - Triggers and common behavior

<!-- req: R7.1 -->
### R7.1

The release profile runs scrum-master handoff on-release as its step after the tag word, once the tag is pushed, with the person's own access to each repository's tracking system; no CI job, watcher, or group token is needed. The same word is the standalone handoff profile, idempotent (R5.1), which re-runs a handoff that failed and which a program may also run from a tag pipeline on the release image (eng03), as a service user SCRUM_MASTER_USER names with a credential scoped to the tracking system; the two paths are safe together. A handoff that fails after the push ends the release run in a state naming the handoff profile to re-run, and neither undoes nor fails the tag. Subcommands are in the scrum-master group handoff, honour --json and --dry-run, and never prompt.

<!-- req: R7.2 -->
### R7.2

Handoff reads and writes each repository's issues through the tracking system control/trackers.md routes it to, bd, gh, or glab (srd042 R1.3 to R1.5), with the person's access to it: the tracker clone and git credentials for bd, the gh or glab CLI's existing login for gh and glab. An upstream and a downstream repository may use different systems.

<!-- req: R8 -->
## R8 - Reference update on a downstream release

<!-- req: R8.1 -->
### R8.1

A release tag in a downstream repository files one reference-update issue in each upstream repository whose initiatives list that repository in delivered_by by pinned URL, or whose Sprint Goals the release closed, found from the Cites lines of those Sprint Goals. The issue names the old tag, the tag the upstream pins now, and the new tag.

<!-- req: R8.2 -->
### R8.2

The issue asks for the pinned references to move to the new tag through the inner loop (srd007 R5.5); no script edits a reference in place. Its Requirements name the files and fields to change, computed from the initiative file and its delivered_by entries for the downstream repository.

<!-- req: R8.3 -->
### R8.3

It carries the handoff marker <!-- handoff reference-update from=<repo> tag=<tag> -->, the neutral form of the managed markers. Before filing, handoff searches the upstream repository's issues in every state for the marker, so a closed issue also suppresses a second; a second run files nothing (R5.1). This is the slice of srd014 on-release 05.0 builds.

<!-- req: R8.4 -->
### R8.4

One reference-update issue stays open per upstream–downstream pair: when one is open for an earlier tag, handoff adds the new tag and a comment to it instead of filing a sibling. The marker search and this rule behave the same on bd, gh, and glab, through the tracker interface rather than one system's search.

<!-- req: R8.5 -->
### R8.5

Handoff refuses, exits non-zero, and files nothing when a repository's tracking system is unreachable, the bd tracker repository or the gh or glab CLI logged out, or when the upstream repository's sibling checkout is missing.

<!-- req: R9 -->
## R9 - Direction

<!-- req: R9.1 -->
### R9.1

Release 06.0 builds the rest of handoff: on-merge notices to the upstream issue and review issues for a Downstream section; the release fan-out, which marks subscriptions needs-run, files consume issues, and posts release and superseded events, with review and handoff issues for Cites lines ending in | review or | handoff; appending created issues to a tracking issue's Members; a sweep over a date range as the fallback when handoff did not run; and bd's dedup of issues created concurrently, by derived issue ids.

<!-- req: R9.2 -->
### R9.2

Until then, handoff files only the reference-update issue of R8, and a merge or a release posts no event and files no other issue.

<!-- field: non_goals -->
## Non-goals

- Deciding what a consumer must change; the consume issue's assignee decides.
- Notifying outside the tracker.
- Assigning or prioritising created issues.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 | status: retired | retired_in: 5 -->
### AC1

Retired in 05.0 (design.md D12): direction in R9.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)

<!-- ac: AC2 | status: retired | retired_in: 5 -->
### AC2

Retired in 05.0 (design.md D12): direction in R9.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)
- [R2.5](#r25)
- [R2.6](#r26)

<!-- ac: AC3 | status: retired | retired_in: 5 -->
### AC3

Retired in 05.0 (design.md D12): direction in R9.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)

<!-- ac: AC4 | status: retired | retired_in: 5 -->
### AC4

Retired in 05.0 (design.md D12): direction in R9.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R5.1](#r51)
- [R5.2](#r52)

<!-- ac: AC5 -->
### AC5

An undeclared or uncited artifact creates nothing, records a dashboard finding, and exits non-zero; no handoff run sets an assignee, closes an issue, or changes a label other than needs-run.

<!-- field: traces -->
#### Traces

- [R5.3](#r53)
- [R6.1](#r61)

<!-- ac: AC6 -->
### AC6

The release profile runs handoff on-release after its tag word with the person's access, a handoff failure names the handoff profile and leaves the tag, the handoff profile re-runs it idempotently, and each repository's issues go through its routed tracking system, an upstream and a downstream on different systems included; subcommands honour --json and --dry-run and never prompt.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)

<!-- ac: AC7 -->
### AC7

A downstream release tag files one reference-update issue in each upstream repository that lists or is closed by it, naming the pinned old tag, the new tag, and the fields to change; a second run files nothing, a closed issue included; an open issue for an earlier tag gets the new tag and a comment instead of a sibling, on bd, gh, and glab; an unreachable tracking system or a missing upstream checkout files nothing and exits non-zero; no reference is edited outside the inner loop.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
- [R8.3](#r83)
- [R8.4](#r84)
- [R8.5](#r85)

<!-- ac: AC8 -->
### AC8

Release 06.0's planning writes on-merge, the release fan-out, tracking-issue membership, sweep, and bd's dedup of concurrent creates from R9.1; until then handoff files only the reference-update issue (R9.2).

<!-- field: traces -->
#### Traces

- [R9.1](#r91)
- [R9.2](#r92)
