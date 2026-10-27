---
id: srd045-backlog
title: "The backlog hierarchy: themes, initiatives, epics, features, and stories"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

An agile reader expects a backlog shaped the way their tools shape it: a
theme names a strategic area, an initiative a multi-team outcome, an epic
a large body of work, an optional feature a capability inside it, and a
user story the unit of value a team finishes in one sprint, with tasks
beneath. Each item has one parent, progress rolls up from the stories,
story points and velocity live on stories, and the Product Owner orders
the backlog by rank.

Before release 04.0 the process had none of that under those names. A
program's intent was an initiative, the per-period commitment
was a Sprint Goal, and stories did not exist as such: their
acceptance criteria were scattered across SRD items and issues. A reader
had to translate every word, and the translation hid where value was
delivered.

This SRD defines the hierarchy as document types the corpus gate checks
(design.md theme, initiative, epic, feature, story), how progress and
velocity roll up through Parent links, how the Product Backlog is ordered,
and how Sprint Planning takes stories from it.

<!-- field: goals -->
## Goals

- G1: Every planning item is a theme, initiative, epic, feature, or story document, with at most one parent of the kind above it.
- G2: Progress, velocity, and forecasts are computed from story points and the ledger, never typed.
- G3: The Product Backlog is the ranked list of agreed stories, and Sprint Planning takes stories from it.

<!-- req: R1 -->
## R1 - Direction

<!-- req: R1.1 | status: retired | retired_in: 4 -->
### R1.1

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R1.2 | status: retired | retired_in: 4 -->
### R1.2

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R1.3 | status: retired | retired_in: 4 -->
### R1.3

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R1.4 | status: retired | retired_in: 4 -->
### R1.4

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R1.5 | status: retired | retired_in: 4 -->
### R1.5

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R1.6 | status: retired | retired_in: 4 -->
### R1.6

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R1.7 | status: retired | retired_in: 4 -->
### R1.7

Retired in 04.0; replaced by the requirement items R2 to R7 write when release 04.0 is planned (design.md D12).

<!-- req: R2 -->
## R2 - The documents

<!-- req: R2.1 -->
### R2.1

A program's planning items are documents of the five hierarchy types (design.md theme, initiative, epic, feature, story), each an SRD in the pinned parent release's Markdown format (D2) with a status of to_be_agreed, agreed, or retired. Only agreed documents are read by plan, tasks, and roll-up.

<!-- req: R2.2 -->
### R2.2

Themes and initiatives live in the system repository. An epic lives there when it spans teams and in a subsystem repository when one team owns it. Features and stories live in the owning team's repository.

<!-- req: R2.3 -->
### R2.3

No hierarchy document is a unit of work (D4). A story is executed by issues that name it in an Implements line (srd007 R6.5), and those issues and their tasks are the tracker's work.

<!-- req: R2.4 -->
### R2.4

Every hierarchy document may carry an architecture context item listing the ARCHITECTURE components it creates, modifies, or integrates (design.md hierarchy_links, D15): a component of this repository by its bare id, as docs/ARCHITECTURE.md declares it in a <!-- component: id --> heading, and a component of another repository by a URL pinned to a release tag naming that repository's ARCHITECTURE.md and the component as its anchor. The item is optional; a document without it is unaffected, and scrum-master backlog list reports the components a document cites.

<!-- req: R3 -->
## R3 - Parent and roll-up

<!-- req: R3.1 -->
### R3.1

A document names its parent as srd007 R7 states: an initiative's parent is a theme, an epic's an initiative, a feature's an epic, and a story's a feature or an epic. Only a feature must have a parent; an orphan story or epic is valid.

<!-- req: R3.2 -->
### R3.2

A story is done when every one of its acceptance criteria has passing latest evidence on a Sprint Goal (srd006 R10). Until track-goals runs, a story is done when the sprint-end issue of the sprint that listed it reports it complete (srd038 R2).

<!-- req: R3.3 -->
### R3.3

The progress of a feature, epic, or initiative is the story points of its done descendant stories over the story points of all its agreed descendant stories, found through Parent links; a theme reports the progress of its initiatives. Progress is computed, never typed.

<!-- req: R3.4 -->
### R3.4

Roll-up reads the documents of the repository it runs in, the documents their Parent links name by pinned URL, and the descendants in every team repository the system repository's subsystem SRDs name, the program's registry (GH-2726): the stories and features there whose Parent link pins a document in this one, counted in progress with each done from its own repository's sprint-end summaries. A team whose SRD names a scrum repository plans there, so roll-up reads its documents from that repository alone, named for it; a team that names none plans in its component repositories (GH-2738). It reads a team repository through the tracker, the host's repository files on its default branch, else the checkout beside this one; the system SRD, whose components live in the system repository, names no team. A team repository it cannot read is named with the reason, never skipped silently. With no system repository to read the registry from, roll-up reads the team repositories checked out beside this one.

<!-- req: R4 -->
## R4 - Stories

<!-- req: R4.1 -->
### R4.1

A story states who wants what and why in its context items as_a, i_want, and so_that, written from the user's view, and carries points, a relative estimate of its work that is never an estimate of a person (D5).

<!-- req: R4.2 -->
### R4.2

A story's acceptance criteria are its own, and each names its test and, as applicability, the environment where its evidence counts (D8). Its cites context item lists the SRD requirement items its criteria prove, each pinned (D9).

<!-- req: R4.3 -->
### R4.3

tasks turns a story selected onto a Sprint Goal into the issues that execute it, each with an Implements line naming the story.

<!-- req: R5 -->
## R5 - Product Backlog and Sprint Planning

<!-- req: R5.1 -->
### R5.1

A team's Product Backlog is its agreed stories that are not done, ordered by their rank context item, lowest first, then by id. The Product Owner sets rank; changing it is a merged docs change, which is how refinement is recorded.

<!-- req: R5.2 -->
### R5.2

At Sprint Planning, plan in the team's repository (srd031) lists the Product Backlog with the team's velocity and recommends the stories, in rank order, whose points fit it; the Product Owner decides. scrum-master plan backlog writes the chosen stories into the Sprint Goal's Sprint Backlog section and posts a planned event naming them (srd034 R3.5, srd006 R8).

<!-- req: R5.3 -->
### R5.3

A team's velocity is the story points of its stories done per sprint, read from the ledger, or from the sprint-end reports until track-goals runs. Until the team has finished a sprint, plan asks the Product Owner for an assumed velocity.

<!-- req: R6 -->
## R6 - Gate

<!-- req: R6.1 -->
### R6.1

The corpus gate checks every hierarchy document against its type: the required fields and context items, a parent of the right kind that resolves, link targets that resolve (srd007 R8.2), no parent or blocked_by cycle, a story's points, and a criterion without a test or an environment. A context that does not parse, such as a value holding an unquoted colon, or a nested list item written "- item" rather than "1. item", which the parent's formatter refuses as an entry with no key, is one finding naming the cause, not a finding for each item it hides. The parent's corpus audit does not read these directories, so a magefiles gate inside mage audit checks them, as it checks initiatives (srd034 R2.1).

<!-- req: R6.2 -->
### R6.2

The gate refuses an architecture citation (R2.4) of this repository that names no component its docs/ARCHITECTURE.md declares, naming those it does, including when the file is missing or does not parse; and one of another repository that is not a URL pinned to a release tag. Whether a pinned citation's tag and anchor resolve is not checked.

<!-- req: R6.3 -->
### R6.3

The gate compares the plan with the ARCHITECTURE diff: each component of docs/ARCHITECTURE.md added or changed since the latest release, its heading or any text beneath it differing from the file at that release's tag, must be cited by an agreed hierarchy document (R2.4) that was itself added or changed since that release, the plan for the move: an agreement older than the release planned an earlier one. The latest release is the road map's latest release whose status is released, by the tag its release.publication names, read by name so a branch cut before it compares with it too, and each document's path read against the checkout whatever form the directory is given in (GH-114); a corpus tag that only publishes a plan is no release, and a release marked released whose tag is one, its record at that tag naming the same tag and not yet released, is skipped for the release before it and named, under the same policy, with the fix: name the release tag in its publication (GH-117); and with no release released yet every component is added and every agreed document a plan. The file at the tag is read through its textconv filter, so a repository that encrypts its documents at rest compares text. One no document cites and no release shipped is drift, declared intent nobody planned or an architecture gone stale; one no agreed document added or changed since the release cites is uncovered; a component whose status field begins Deferred, declared intended with no plan meant to move it yet, draws neither, whatever its text says; each component is named once (GH-99). A committed docs/architecture-status.md that sorts the components into other sections than the page R8.5 renders now, from every document on disk or from those git tracks, a component missing, extra, or under another heading, is stale, reported the same way, so a document not yet added draws the untracked finding alone (GH-114); its releases and points are not compared, since they depend on the checkouts beside it. The policy's architecture_checks (config format policy) decides how: warn, the default, reports them without failing the gate; refuse makes each a finding. The gate reads the policy from disk, the repository's control/policy.md else the system repository's beside it, and the tag through local git, so it stays within R8.3.

<!-- req: R6.4 -->
### R6.4

A subsystem repository holding hierarchy documents is refused by scrum-master doctor, and by backlog check, until the system repository's docs/ARCHITECTURE.md names it: a component whose id is the component name a subsystem SRD of the system declares for the repository, the value its component label carries, with its dashes read as underscores; the repository's name read the same way is accepted too (design.md D15, GH-90). The system repository is the checkout beside it that holds control/trackers.md, as the tracker's routing finds it; with none, the order is reported unchecked and the gate passes it. backlog check carries it so the merge audit, which runs the gate, refuses a team's plan out of order, not only the check profile in CI.

<!-- req: R6.5 -->
### R6.5

A scrum owns an area, a subsystem and the components its SRD lists, while planned work is a separate tree (GH-2726). backlog check warns for a story or feature in a team's repository that names in architecture a component the subsystem owning the repository does not list, naming the subsystem that lists it, whose work it is. The repository's subsystem is the team SRD listing it, as a component's repository or its scrum repository, by its origin's project path, else by its name; the registry is the SRDs of the system repository beside it, as R6.4 finds it, so with none there is nothing to check.

<!-- req: R6.6 -->
### R6.6

A cross-team epic lives in the system repository and is the technical program manager's; each team's Product Owner writes the stories for its part in the team's repository, with the epic as their parent by pinned URL. backlog check warns for an epic in the system repository whose owner names a Product Owner, and for one whose architecture components all belong to one team's subsystem, which belongs in that team's repository. scrum-master backlog template --kind epic --dir offers owner: technical program manager in the system repository and Product Owner, the subsystem's owner, in a team's. The policy's area_checks (config format policy) decides how R6.5 and R6.6 report: warn, the default, without failing the gate; refuse makes each a finding. Both read only the disk, so they stay within R8.3.

<!-- req: R7 -->
## R7 - Forecasts (direction)

<!-- req: R7.1 -->
### R7.1

Once track-goals runs (06.0), reports render a roadmap forecast, the remaining stories of each epic spread over future sprints at velocity in Blocked-by order, and a burn-up per epic and initiative, story points done against total points; both are committed beside the dashboard, regenerated each run, never edited, and labelled forecasts, not baselines (srd018 R11).

<!-- req: R7.2 -->
### R7.2

Until then, reports render no forecast and no burn-up.

<!-- req: R8 -->
## R8 - Reading and checking the hierarchy

<!-- req: R8.1 -->
### R8.1

scrum-master backlog list reads the hierarchy documents of the repository at --dir and prints each one's id, kind, title, status, parent, and path, and a story's points and rank; --kind keeps one kind. With --json it prints one object per document.

<!-- req: R8.2 -->
### R8.2

scrum-master backlog check runs R6.1's checks on the repository at --dir through the same function the gate in mage audit calls; it also reads the SRDs, counting them among the documents and refusing a requirement item no acceptance criterion traces, and runs the pinned parent spec tool's check, refusing each shape finding but formatting, which scrum-master doctor repairs, so the merge gate refuses what doctor would; the spec tool reads only what git tracks, so the gate also refuses each document under docs/ git does not track, naming it, rather than passing it unchecked; such a document draws that finding alone, the area checks (R6.5, R6.6) reading, like the shape check, only what git tracks (GH-2780). It prints each finding with its file, field, and message, says when the spec tool could not run and why, a document the tool cannot parse being a finding naming it and the cause, not a check that could not run; it also refuses a road-map release record whose status is not specified, implemented, or released, and exits non-zero when there is any finding; with --json it prints the document count, the findings, and why the spec check was skipped.

<!-- req: R8.3 -->
### R8.3

Both commands read only the directory: they open no tracker, use no network, and write nothing.

<!-- req: R8.4 -->
### R8.4

scrum-master backlog guide --name prints a guide this release ships, docs/guides/<name>.md as the release wrote it, so the guide is never older than the profiles that route it. With --with-request it reads the run's request from standard input and prints a line telling the reader to follow the guide, the guide, and then the request, refusing an empty request; this is how a profile routes a guide into its model's context (design.md D14). An unknown name is refused, naming the guides there are. It opens no tracker, uses no network, and writes nothing.

<!-- req: R8.5 -->
### R8.5

scrum-master backlog architecture reports each component of docs/ARCHITECTURE.md in one state: actual, unchanged since the latest release (R6.3), with that release's tag, and with no agreed document citing it whose work is still open, its story points not all done where they are known, or its horizon running past the work the release delivered, since stories not yet written are work too, so neither a component a release declared but no story built (GH-99) nor one a long epic's first stories touched (GH-114) is actual; in_transition, added or changed since it and cited by agreed hierarchy documents (R2.4), with each document's kind, title, the release its work lands in, and its story points done over total (R3.3) from the sprint-end summaries. The release is, of the road map's releases not yet released that deliver the initiative the document rolls up to, the first whose items fall due in or after the last week of the document's horizon, the nearest one it or a parent states; the last of them when the horizon runs past every one; the first when no horizon or due date says more. The points include the agreed stories of the team repositories R3.4 reads whose parent, pinned by URL, rolls up to the document, each counted done from its own repository's sprint-end summaries, and the page names each team repository it could not read; a document with no points and no team repository read says its points are not rolled up rather than none; or intended, added or changed and cited by none, and deferred, never actual, when its status says so (R6.3). It names each citation of a component ARCHITECTURE does not declare, with the document citing it, rather than drop it. With --write it also writes docs/architecture-status.md, a page under a generated-file header that sorts the components into what exists, what is being built and when it lands, and what is intended, so a reader who runs nothing answers those three questions from rendered documents; the architecture-status profile runs it. It opens the tracker only to read the team repositories the registry names (R3.4), uses no network beyond that read, and the page is all it writes; the gate's comparison of the committed page (R6.3) reads only the checkouts beside it. It is specified here rather than with the reports (srd018) because it reads the planning documents, not the tracker's issues.

<!-- req: R8.6 -->
### R8.6

scrum-master backlog components lists the component ids docs/ARCHITECTURE.md declares, in its order, the ids an architecture item may cite (R2.4); with none it says a new component is a design decision. Planning profiles and people check a component with it before naming one (srd047 R1.3, R2.1). It reads only the directory, as R8.3 does.

<!-- field: non_goals -->
## Non-goals

- A baseline or a commitment date; a forecast is regenerated from current knowledge.
- Duration estimates; stories are estimated in relative points and dates come only from velocity.
- Estimating, ranking, or assigning a person (D5); rank orders stories, points belong to the work.
- Tracker-native epics, features, or stories; the hierarchy is documents, and the tracker holds the issues that execute stories.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 | status: retired | retired_in: 4 -->
### AC1

Retired in 04.0 with the direction items it traced.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)
- [R1.5](#r15)
- [R1.6](#r16)
- [R1.7](#r17)

<!-- ac: AC2 -->
### AC2

Each planning item is one of the five document types in the repository R2.2 names, only agreed documents are read, and no hierarchy document is popped or merged as work; its stories are executed by issues that name them; any of them may cite the architecture components it moves, and backlog list reports them.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)

<!-- ac: AC3 -->
### AC3

A story is done only when every criterion has passing evidence, or before track-goals when the sprint-end report says so; an epic with stories of 3, 5, and 8 points of which the 5 is done reports 5 of 16; an epic in the system repository with stories pinned to it in two team repositories the subsystem SRDs name counts both teams' points, and a team repository roll-up cannot read is named.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)

<!-- ac: AC4 -->
### AC4

Every story has a statement, points, and criteria naming a test and an environment, and its cites are pinned; the issues tasks files for a selected story carry Implements naming it.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)

<!-- ac: AC5 -->
### AC5

plan lists a team's not-done agreed stories by rank, recommends those that fit velocity, asks an assumed velocity for a team with no finished sprint, and writes the chosen stories to the Sprint Goal with a planned event naming them; velocity equals the points of the stories done in the last sprint.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC6 -->
### AC6

mage audit fails on a hierarchy document with a missing field, a parent of the wrong kind or that does not resolve, an unresolved link, a parent or blocked_by cycle, a story without points, a criterion without a test or an environment, an architecture citation naming no component of this repository's ARCHITECTURE, or an unpinned citation of another repository's; it warns, and under a policy that refuses fails, on each component added or changed since the latest tag that no agreed document cites, naming drift apart from what is uncovered; doctor refuses a subsystem repository's plan until the system's ARCHITECTURE names it; backlog check warns, and under area_checks refuse fails, for a story naming a component its repository's subsystem does not list, for a system-repository epic a Product Owner owns, and for one whose components all belong to one subsystem; and the epic template offers the technical program manager as owner in the system repository and the team's Product Owner in a team's.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)
- [R6.4](#r64)
- [R6.5](#r65)
- [R6.6](#r66)

<!-- ac: AC7 -->
### AC7

Release 06.0's planning writes the forecast items from R7.1; until then the interim rule of R7.2 holds.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)

<!-- ac: AC8 -->
### AC8

scrum-master backlog list prints every hierarchy document with its id, kind, title, status, parent, path, and a story's points and rank, filtered by --kind; scrum-master backlog check reports exactly the gate's findings on the same documents, names a document git does not track, reports a document the spec tool cannot parse as a finding, refuses an unknown road-map release status, and exits non-zero when there is any, reporting R6.3's warnings apart from its findings; scrum-master backlog guide prints a shipped guide and, with --with-request, the guide before the request it reads, refusing an empty request and an unknown guide; none opens a tracker, uses the network, or writes.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
- [R8.3](#r83)
- [R8.4](#r84)

<!-- ac: AC9 -->
### AC9

scrum-master backlog architecture names each component actual as of the latest tag, in transition with its agreed citing documents, their planned release, and their points done, or intended, counting the points of the stories in every team repository the registry names and naming one it cannot read; with --write the generated page answers what exists, what is being built and when, and what is intended beyond, for a reader who knows no git; scrum-master backlog components lists the declared component ids and opens no tracker.

<!-- field: traces -->
#### Traces

- [R8.5](#r85)
- [R8.6](#r86)
