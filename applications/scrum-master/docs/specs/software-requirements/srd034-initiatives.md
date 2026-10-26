---
id: srd034-initiatives
title: "Initiatives and the Sprint Goal loop"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

What a program commits to for the coming quarters is decided in a meeting
and lives in slides, so nothing versions it, nothing checks its shape, and
the work downstream cites intent that was never written down. The status
that comes back is read against memory of what was agreed.

The people who receive that intent are sub-project leads and researchers
who already work in their own repositories and will not adopt a new tool
to receive work. A process that asks them to learn one is not followed.

This SRD makes the program's outcomes initiatives, documents written by
the technical program manager in the system repository in the shape of an
SRD and checked by the corpus audit (design.md initiative), and turns them
into one Sprint Goal per team per sprint: a dated issue in the team's own
repository that cites the initiative items the team serves, whose body
carries its own steps, and on which the team's Sprint Planning lists the
stories it selected (srd045). Before release 04.0 an initiative was called
an initiative and a Sprint Goal a Sprint Goal; both are still
read under the old names. What comes back lives with each skill: the weekly
status issue and the release summary with reports (srd018), the sprint-end
issue and the interim close with sprint-end (srd038), and the reference
update with handoff (srd014).

<!-- field: goals -->
## Goals

- G1: Every initiative is a versioned document that the corpus audit checks before any issue cites it.
- G2: Every team receives one dated Sprint Goal per sprint in its own repository whose body says what to do and which stories the sprint selected.
- G3: An owner below the system repository needs no step beyond the dated issues in their repository and implement.

<!-- req: R1 -->
## R1 - Initiative documents

<!-- req: R1.1 -->
### R1.1

An initiative document lives at docs/specs/initiatives/inNNN-short-name.md in the system repository; an objective document at docs/specs/objectives/objNNN-short-name.md, its name before release 04.0, is read as an initiative without change. It is an SRD in the pinned parent release's Markdown format and follows the initiative document type (design.md schema.initiative), using only fields that release declares.

<!-- req: R1.2 -->
### R1.2

It has the shape of an SRD: a problem, goals, requirement groups (R1, R2) with numbered items (R1.1, R1.2), non_goals, and acceptance criteria (AC1, AC2) that trace items. Each requirement group is one outcome; each item is one checkable part of it. Because the anchors are Rn and Rn.m, a Cites line pins an initiative item exactly as it pins a requirement (srd007 R5.1).

<!-- req: R1.3 -->
### R1.3

Its context names its owner by role and the horizon of periods it covers, as the items owner and horizon, and may name its parent theme (srd007 R7), and its frontmatter status is to_be_agreed, agreed, or retired. Only an agreed document is read by plan and tasks.

<!-- req: R1.4 -->
### R1.4

Each requirement group's applicability carries a due item, the end date of the period its outcome counts toward, and an optional delivered_by list of URLs pinned to the downstream release tags that report it (srd007 R5).

<!-- req: R1.5 -->
### R1.5

Each acceptance criterion names its tests and, as its applicability, an environment (D8). Until the program's environment SRDs exist, the environment is one of the chain stages D8 lists and the audit cannot resolve it; the criterion is still required to name one.

<!-- req: R1.6 -->
### R1.6

Item ids are stable once a downstream issue cites them. A changed outcome is a new item or a new document, and the old one is retired, never renumbered.

<!-- req: R2 -->
## R2 - Audit and gate

<!-- req: R2.1 -->
### R2.1

The corpus audit gates initiative documents, and objective documents read as initiatives, in every repository that carries them. It is mage audit, whose hierarchy gate validates each document against R1, because the parent's corpus-audit agent does not read docs/specs/initiatives/ or docs/specs/objectives/: a missing required field or item, an acceptance criterion tracing an item that does not exist, or an unpinned or unresolved reference fails it. mage specCheck holds each document to the parent's SRD declaration.

<!-- req: R2.2 -->
### R2.2

Every repository that carries initiative documents runs the corpus audit on each change to docs/ before it merges, and a failing audit blocks the merge.

<!-- req: R2.3 -->
### R2.3

A repository whose releases carry only documents declares the corpus audit as its release gate check (srd008 R2), so scrum-master release tag tags only an audited corpus (srd015 R2.1).

<!-- req: R3 -->
## R3 - From initiative to Sprint Goal

<!-- req: R3.1 -->
### R3.1

plan (srd031) reads the agreed initiatives with the system SRDs when the technical program manager chooses what the next system release proves, and records the chosen initiative items by id in the road-map release, as its deliverables: a list of inNNN-short-name#Rn.m (or objNNN-short-name#Rn.m), beside the release's tag and milestone under release.publication (design.md roadmap program_release_fields).

<!-- req: R3.2 -->
### R3.2

tasks (srd022) renders one Sprint Goal issue per team for the release from the initiatives: each initiative item is confirmed for a team on its own, so one requirement group's items can go to different teams, and a team's Sprint Goal has one Cites line per initiative item it serves, pinned to the system release tag, the Environment line, Requirements and Acceptance Criteria copied from the document, and the due date set to the requirement group's due field or, without one, the milestone end date plan recorded (srd031 R4.2). A team serving part of a split group titles its Sprint Goal with the group's title and the items it serves, so the teams' goals for one outcome read apart.

<!-- req: R3.3 -->
### R3.3

Each Sprint Goal issue carries an empty Notes section (config format issue-body) that the owner fills with the details of the work as it proceeds. No script writes to it.

<!-- req: R3.4 -->
### R3.4

The Sprint Goal's Requirements section lists the steps the owner takes in order, so the issue is its own instructions; admission (srd013 R1) refuses a Sprint Goal without Requirements and Acceptance Criteria.

<!-- req: R3.5 -->
### R3.5

Each Sprint Goal carries a Sprint Backlog section (config format issue-body), filed empty by tasks. The team's Sprint Planning (plan in the team's repository, srd045 R5.2) writes into it the stories it selected, one line per story with its points, and posts a planned event naming them (srd006 R8).

<!-- req: R3.6 -->
### R3.6

A Sprint Goal is met when every acceptance criterion of every story it lists has passing latest evidence (srd006 R10). Until track-goals runs, the merge of its repository's sprint-end issue closes it as reported complete (srd038 R2).

<!-- req: R3.7 -->
### R3.7

An objective issue, filed before release 04.0, is a Sprint Goal: every script reads it, its marker, and its ledger as a Sprint Goal's (srd006 R11.3). scrum-master files every new one as a Sprint Goal.

<!-- req: R4 -->
## R4 - Weekly status issue

<!-- req: R4.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R9.1"] -->
### R4.1

Retired in 01.2 (design.md D12).

<!-- req: R4.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R9.2"] -->
### R4.2

Retired in 01.2 (design.md D12).

<!-- req: R4.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R9.3"] -->
### R4.3

Retired in 01.2 (design.md D12).

<!-- req: R4.4 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R9.4"] -->
### R4.4

Retired in 01.2 (design.md D12).

<!-- req: R5 -->
## R5 - Sprint-end issue

<!-- req: R5.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end R1.1"] -->
### R5.1

Retired in 01.2 (design.md D12).

<!-- req: R5.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end R1.2"] -->
### R5.2

Retired in 01.2 (design.md D12).

<!-- req: R5.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end R1.3"] -->
### R5.3

Retired in 01.2 (design.md D12).

<!-- req: R6 -->
## R6 - Closing a Sprint Goal before track-goals runs

<!-- req: R6.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end R2.1"] -->
### R6.1

Retired in 01.2 (design.md D12).

<!-- req: R6.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end R2.2"] -->
### R6.2

Retired in 01.2 (design.md D12).

<!-- req: R6.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end R2.3"] -->
### R6.3

Retired in 01.2 (design.md D12).

<!-- req: R7 -->
## R7 - Reference update on a downstream release

<!-- req: R7.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd014-handoff R8.1"] -->
### R7.1

Retired in 01.2 (design.md D12).

<!-- req: R7.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd014-handoff R8.2"] -->
### R7.2

Retired in 01.2 (design.md D12).

<!-- req: R7.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd014-handoff R8.3"] -->
### R7.3

Retired in 01.2 (design.md D12).

<!-- req: R8 -->
## R8 - Summary on a system release

<!-- req: R8.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R10.1"] -->
### R8.1

Retired in 01.2 (design.md D12).

<!-- req: R8.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R10.2"] -->
### R8.2

Retired in 01.2 (design.md D12).

<!-- req: R8.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports R10.3"] -->
### R8.3

Retired in 01.2 (design.md D12).

<!-- req: R9 -->
## R9 - What a downstream owner does

<!-- req: R9.1 -->
### R9.1

The whole interaction surface for an owner below the system repository is: issues with due dates in their own repository, the tracker's due-date notifications, the steps inside each issue's body, and implement. No requirement of this SRD adds a tool, a notification channel, or a step outside an issue.

<!-- req: R10 -->
## R10 - Direction: dates and progress

<!-- req: R10.1 | status: retired | retired_in: 4 | replaced_by: ["srd045-backlog R3.1", "srd007-links R6.5"] -->
### R10.1

Retired in 04.0: a Sprint Goal serves initiative items and lists stories, whose parents are epics and features (srd045 R3.1).

<!-- req: R10.2 -->
### R10.2

A Sprint Goal's dates and progress are derived, never typed: its target date is the due item of R1.4; its planned start is the start of the milestone its planned event names; its completion is the time of its met event; its progress is the story points of its listed stories that are done over the points of all its listed stories (srd045 R3.2, R3.3).

<!-- req: R10.3 | status: retired | retired_in: 4 -->
### R10.3

Retired in 04.0 with R10.1.

<!-- field: non_goals -->
## Non-goals

- Choosing what goes in a release; that is plan (srd031).
- Deciding that a Sprint Goal is met on evidence; that is track-goals (srd019).
- Ranking or comparing people in any status or summary (srd018 G3).
- A notification service of its own; reminders are the tracker's due-date notifications.
- Creating the repositories that sub-projects own; repository layout is declared by subsystem SRDs (D7).
- The rest of handoff and release fan-out (srd014, srd015) beyond R7.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

An initiative document at docs/specs/initiatives/, or an objective document at docs/specs/objectives/, has every field the initiative document type requires, one outcome per requirement group, a due item per group, stable item ids, an owner role, a horizon, a status, and acceptance criteria naming tests and an environment; a Cites line can pin any of its items.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)
- [R1.5](#r15)
- [R1.6](#r16)

<!-- ac: AC2 -->
### AC2

The corpus audit fails on a malformed initiative document, runs before every merge that changes docs/, and is the release gate check of a documents-only repository.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC3 -->
### AC3

plan places initiative items by id; tasks files one Sprint Goal per team with pinned Cites lines, an Environment line, the document's Requirements and Acceptance Criteria as ordered steps, a due date from the document or the milestone, empty Notes and Sprint Backlog sections; admission refuses one without Requirements and Acceptance Criteria. Two teams serving one split group get Sprint Goal titles that name their items.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)

<!-- ac: AC4 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports AC7"] -->
### AC4

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)
- [R4.4](#r44)

<!-- ac: AC5 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end AC1"] -->
### AC5

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC6 | status: retired | retired_in: 1.2 | replaced_by: ["srd038-sprint-end AC2"] -->
### AC6

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)

<!-- ac: AC7 | status: retired | retired_in: 1.2 | replaced_by: ["srd014-handoff AC7"] -->
### AC7

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)

<!-- ac: AC8 | status: retired | retired_in: 1.2 | replaced_by: ["srd018-reports AC8"] -->
### AC8

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
- [R8.3](#r83)

<!-- ac: AC9 -->
### AC9

Nothing this SRD specifies asks a downstream owner to use a tool, channel, or step other than dated issues in their repository, due-date notifications, steps in issue bodies, and implement.

<!-- field: traces -->
#### Traces

- [R9.1](#r91)

<!-- ac: AC10 -->
### AC10

A Sprint Goal's target date is its due item, its planned start the start of its milestone, its completion the time of its met event, and its progress the done share of its listed stories' points; none of them is typed.

<!-- field: traces -->
#### Traces

- [R10.2](#r102)

<!-- ac: AC11 -->
### AC11

tasks files each Sprint Goal with an empty Sprint Backlog section; the team's Sprint Planning writes the selected stories with their points into it and posts a planned event naming them; a Sprint Goal is met only when every listed story's criteria have passing evidence, and before track-goals runs the sprint-end merge closes it as reported complete; an objective issue filed before 04.0 is read as a Sprint Goal, marker and ledger included.

<!-- field: traces -->
#### Traces

- [R3.5](#r35)
- [R3.6](#r36)
- [R3.7](#r37)

<!-- ac: AC12 | status: retired | retired_in: 4 -->
### AC12

Retired in 04.0 with the direction items it traced.

<!-- field: traces -->
#### Traces

- [R10.1](#r101)
- [R10.3](#r103)
