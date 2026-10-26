---
id: srd007-links
title: Dependency links
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Work in one repository often cannot finish until work in another has. The
free tier of the tracker has no blocking links, so a dependency either goes
unrecorded or is recorded as prose nobody's script can read. Then a person
starts work that cannot finish, or waits on something that already landed
without anyone telling them.

Dependencies are two lines in an issue's description: one naming the issue
whose output this issue consumes, one naming a release this issue needs.
track-goals (srd019) reads them, decides whether the issue is waiting, and reports
the moment it is not.

Requirements cross repositories too. A subsystem's requirement satisfies a
system requirement in the TPM repository; a story cites the requirements
its criteria prove, and a Sprint Goal cites the initiative and epic items
it serves. A citation by bare path or by branch drifts
as the cited file changes, so every cross-repository reference is a URL
pinned to a release tag.

Agile tools record more relations than these: the parent each item belongs
to, which item blocks which, which depends on another's output while both
proceed, which relate, which duplicate, and which issue implements which
story. A reader who knows those tools expects every one of them, under the
names the tools use. They are lines in an issue's description and context
items of a hierarchy document, so any tracker holds them and scripts read
them.

<!-- field: goals -->
## Goals

- G1: A dependency on an issue or on a release is recorded in a form scripts read.
- G2: Whether an issue is waiting is computed, never declared by hand.
- G3: The person whose issue becomes unblocked hears about it the next morning without asking.
- G4: Every cross-repository reference names an exact released version of what it cites.
- G5: Every relation an agile tool records (parent, blocks, depends on, relates to, duplicates, implements) is recorded under that name, on issues and on hierarchy documents, in a form scripts read.

<!-- req: R1 -->
## R1 - Upstream line

<!-- req: R1.1 -->
### R1.1

A line of the form Upstream: PROJECT#IID in an issue's description names the issue that produced the artifact this issue consumes or responds to. An issue has at most one Upstream line.

<!-- req: R1.2 -->
### R1.2

Upstream points at the producing issue, not at a tracking issue or a sprint. Handoff writes it on every issue it creates.

<!-- req: R1.3 -->
### R1.3

Every Upstream line is mirrored as a related-issue link between the two issues, so the tracker shows the relation in both directions.

<!-- req: R1.4 -->
### R1.4

An Upstream dependency is satisfied when the named issue is closed. An Upstream line counts as a Blocked-by line naming the producing issue (R6.2); it adds provenance, the issue whose release caused this one.

<!-- req: R2 -->
## R2 - Requires line

<!-- req: R2.1 -->
### R2.1

A line of the form Requires: ARTIFACT >= VERSION names a released artifact and the lowest version this issue can use. An issue may have several Requires lines.

<!-- req: R2.2 -->
### R2.2

A Requires dependency is satisfied when the artifact's producing repository has a version tag at or above VERSION, compared by semantic versioning.

<!-- req: R2.3 -->
### R2.3

A recurring issue's Requires line records the version its consumer is on (srd001 R8), and finishing a consume run updates it.

<!-- req: R3 -->
## R3 - Waiting and unblocked

<!-- req: R3.1 -->
### R3.1

Track-goals sets waiting (srd041 R10) on an open issue while any of its Blocked-by, Upstream, or Requires dependencies is unsatisfied, and clears it when all are satisfied. Depends-on and Relates-to never set waiting (R6.3).

<!-- req: R3.2 -->
### R3.2

An issue whose waiting flag track-goals cleared since the person's previous morning run is reported to its assignee as unblocked since yesterday.

<!-- req: R3.3 -->
### R3.3

A dependency naming an issue or project track-goals cannot read is reported as unresolvable and leaves the issue waiting. Track-goals never guesses that a dependency is satisfied.

<!-- req: R4 -->
## R4 - Cycles

<!-- req: R4.1 -->
### R4.1

Track-goals builds the graph of open issues and their Blocked-by and Upstream links, and reports every cycle it finds as a finding listing the issues in the cycle.

<!-- req: R4.2 -->
### R4.2

A cycle is a finding for the program lead, not an error. Track-goals keeps evaluating every other issue, and issues in the cycle stay waiting.

<!-- req: R4.3 -->
### R4.3

Cycle findings appear in the program's weekly roll-up and dashboard until the cycle is broken.

<!-- req: R5 -->
## R5 - Pinned references

<!-- req: R5.1 -->
### R5.1

A cross-repository reference is a URL of the form https://HOST/GROUP/REPO/-/blob/TAG/PATH#ANCHOR, where TAG is a release tag of REPO, PATH is a file in it, and ANCHOR is a requirement id (Rn or Rn.m) or an interface name in that file. In an issue description it is a line Cites: URL; in a corpus document it is a field value.

<!-- req: R5.2 | derived_from: [{"repo": "github.com/Nokia-Bell-Labs/declarative-agents", "release": "v0.20261018.1", "srd": "srd014-spec-graph", "item": "R3.16", "reason": "The parent resolves a cross-repository citation only at its validated release through a local clone cache; a citation here resolves the same way, at its tag, so the parent's audit can check it."}] -->
### R5.2

A reference resolves when the tag exists, the path exists at that tag, and the anchor names an entry of the file at that tag.

<!-- req: R5.3 | derived_from: [{"repo": "github.com/Nokia-Bell-Labs/declarative-agents", "release": "v0.20261018.1", "srd": "srd014-spec-graph", "item": "R3.18", "reason": "The parent reports a citation without a release as unpinned; this corpus inherits that rule and makes it fail the audit."}] -->
### R5.3

A reference to a branch, to a commit not reachable from a tag, or with no anchor is unpinned. An unpinned or unresolvable reference is a finding that fails the audit; it is never treated as satisfied.

<!-- req: R5.4 -->
### R5.4

The set of repositories that cite an interface by pinned URL is that interface's list of consumers. Handoff reads consumers from it.

<!-- req: R5.5 -->
### R5.5

Moving a reference to a newer tag is an edit made through the inner loop, with the old and new tags in the merged change, so the change of what is cited is reviewed.

<!-- req: R6 -->
## R6 - Agile link lines on issues

<!-- req: R6.1 -->
### R6.1

An issue's description may carry the lines Blocked-by: TARGET, Blocks: TARGET, Depends-on: TARGET, Relates-to: TARGET, Duplicates: TARGET, and Implements: TARGET. TARGET is PROJECT#IID for an issue, a same-repository document id for a hierarchy document, or a pinned URL (R5.1) for a document in another repository. An issue carries at most one Duplicates line and at most one Implements line; the others may repeat.

<!-- req: R6.2 -->
### R6.2

Blocks and Blocked-by are one link seen from its two ends: a Blocks line on A naming B is the same link as a Blocked-by line on B naming A, and scripts read either end. A Blocked-by dependency is satisfied when the named issue is closed, or when the named story is done (srd045).

<!-- req: R6.3 -->
### R6.3

Depends-on names an item whose output this one needs while both proceed, and Relates-to an item that shares context. scrum-master issue show (srd013) reports both; neither makes an issue waiting.

<!-- req: R6.4 -->
### R6.4

Duplicates names the item this one repeats. A duplicate issue closes as a duplicate, which srd041 records apart from a decline; a duplicate document is retired with replaced_by naming the original.

<!-- req: R6.5 -->
### R6.5

Implements names the story an issue executes. tasks writes it on every issue it creates from a story; a story may be implemented by several issues, and an issue implements at most one story. An issue has no Parent line: its story is its parent.

<!-- req: R7 -->
## R7 - Links on hierarchy documents

<!-- req: R7.1 -->
### R7.1

A theme, initiative, epic, feature, or story carries its links as context items (design.md hierarchy_links): parent, blocked_by, depends_on, relates_to, and duplicates, with the meanings R6 gives the issue lines and the same targets.

<!-- req: R7.2 -->
### R7.2

A document has at most one parent, of the kind above it: an initiative's parent is a theme, an epic's an initiative, a feature's an epic, and a story's a feature or an epic. Only a feature must have a parent.

<!-- req: R8 -->
## R8 - Lint

<!-- req: R8.1 -->
### R8.1

scrum-master issue lint (srd013 R1) reports a link line whose target is malformed, a cross-repository target that is not pinned (R5.3), and a second Duplicates or Implements line.

<!-- req: R8.2 -->
### R8.2

The corpus gate reports a hierarchy document whose parent or link target does not resolve, whose parent is of the wrong kind, or whose parent or blocked_by links form a cycle.

<!-- field: non_goals -->
## Non-goals

- Tracker-native link types; every link is a line or a context item any tracker holds.
- Dependencies on tasks; dependencies are between issues or on releases, because a task is a step, not a commitment.
- Ordering work by its dependencies; scheduling is a policy that may read waiting, not part of this SRD.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Every issue handoff creates carries one Upstream line naming the producing issue and a matching related link; an issue with an Upstream line is waiting until that issue closes.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

An issue with Requires ARTIFACT >= 1.2 is waiting while the latest tag is 1.1 and stops waiting when 1.2 or 2.0 is tagged; a finished consume run updates its recurring issue's Requires version.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC3 -->
### AC3

The waiting flag tracks the Blocked-by, Upstream, and Requires dependencies exactly, and never Depends-on or Relates-to; an issue that stopped waiting since the last morning run is listed as unblocked for its assignee; an unreadable dependency keeps the issue waiting and is reported.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC4 -->
### AC4

Two issues whose Blocked-by or Upstream links name each other produce one cycle finding listing both, both stay waiting, every other issue is still resolved, and the finding appears in the roll-up until the cycle is broken.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)

<!-- ac: AC5 -->
### AC5

A Cites URL with an existing tag, path, and anchor resolves; one naming a branch, a missing tag, a missing path, or a missing anchor is reported as a finding and fails the audit; the citing repositories of an interface are its consumers; retargeting a citation is a reviewed merge.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)
- [R5.4](#r54)
- [R5.5](#r55)

<!-- ac: AC6 -->
### AC6

An issue's Blocked-by, Blocks, Depends-on, Relates-to, Duplicates, and Implements lines are read with the targets R6.1 allows; a Blocks line on one issue and a Blocked-by line on the other are one link; Depends-on and Relates-to are shown and never set waiting; a duplicate closes as a duplicate; the issues tasks files for a story carry Implements naming it.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)
- [R6.4](#r64)
- [R6.5](#r65)

<!-- ac: AC7 -->
### AC7

Each hierarchy document's links are context items with the meanings of the issue lines; a story's parent is a feature or an epic, a feature's parent is required and is an epic, and no document has two parents.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)

<!-- ac: AC8 -->
### AC8

lint flags a malformed or unpinned link target and a second Duplicates or Implements line; the corpus gate fails on an unresolved parent or link target, a parent of the wrong kind, and a parent or blocked_by cycle.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
