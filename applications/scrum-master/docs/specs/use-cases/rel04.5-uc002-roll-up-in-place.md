---
id: rel04.5-uc002-roll-up-in-place
title: A Lead Rolls Up a Team's or the Program's Week From Where It Was Written
status: implemented
priority: P1
test_suite: test-rel04.5-status-in-place
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A team's Product Owner and then the technical program manager each take
a rollup issue. The status-rollup skill collects the level's merged
reports, or for the program the teams' rollups, from the repositories
where they were written, through the host's API or a checkout, aggregates
them into signals across repositories, proposes a disposition for each,
and asks the lead only to confirm it; the rollup the lead approves cites
every claim by repository and commit and merges in its own repository.
The site and the next plan read the program rollup.

<!-- field: actor -->
## Actor

A lead: a team's Product Owner for the team rollup, the technical program manager for the program rollup, driving an agent through the status-rollup skill.

<!-- field: trigger -->
## Trigger

The week's rollup issue is due, a working day after the reports it reads.

<!-- field: preconditions -->
## Preconditions

- The system repository holds the roster and the subsystem SRDs naming each team's repositories.
- The week's reports are merged in place (rel04.5-uc001).

<!-- field: flow -->
## Flow

- F1: scrum-master reports status-issues --control files one team rollup issue per subsystem and the program rollup issue, each due after the last, once.
- F2: status-rollup runs reports collect for its level and reports aggregate over the same level, and shows the reports and rollups read, each with its repository and commit, and the signals.
- F3: The model proposes a disposition for each signal and asks the lead only to confirm or change it.
- F4: The model drafts the rollup in the rollup format and revises it until the lead approves it; it writes it, checks it with reports validate, commits it, and files the follow-up signals with reports aggregate --push; the machine pushes and merges it on the rollup issue.
- F5: The site's status pages show the program rollup, and plan's candidates read its progress.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd018-reports](../software-requirements/srd018-reports.md) [R13.1](../software-requirements/srd018-reports.md#r131), [R13.2](../software-requirements/srd018-reports.md#r132), [R13.3](../software-requirements/srd018-reports.md#r133), [R17.1](../software-requirements/srd018-reports.md#r171) | S1 traces the collection and the aggregate across repositories. |
| T2 | [srd018-reports](../software-requirements/srd018-reports.md) [R15.1](../software-requirements/srd018-reports.md#r151), [R15.2](../software-requirements/srd018-reports.md#r152), [R18.2](../software-requirements/srd018-reports.md#r182) | S2 traces the rollup format and the skill. |
| T3 | [srd018-reports](../software-requirements/srd018-reports.md) [R18.3](../software-requirements/srd018-reports.md#r183), [R18.4](../software-requirements/srd018-reports.md#r184) | S3 traces the readers and the move. |
| T4 | [srd018-reports](../software-requirements/srd018-reports.md) [AC11](../software-requirements/srd018-reports.md#ac11), [AC13](../software-requirements/srd018-reports.md#ac13), [AC14](../software-requirements/srd018-reports.md#ac14), [AC16](../software-requirements/srd018-reports.md#ac16), [AC17](../software-requirements/srd018-reports.md#ac17) | S1 to S3 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The collection names every merged report or rollup of the level with its repository and commit, with no clone of a team repository where the host serves files, and the aggregate reports a blocker shared across repositories, a missing report naming its open status issue, and unreported merged work, comparing no people.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R13.1](../software-requirements/srd018-reports.md#r131)
- [srd018-reports](../software-requirements/srd018-reports.md) [R13.2](../software-requirements/srd018-reports.md#r132)
- [srd018-reports](../software-requirements/srd018-reports.md) [R13.3](../software-requirements/srd018-reports.md#r133)
- [srd018-reports](../software-requirements/srd018-reports.md) [R17.1](../software-requirements/srd018-reports.md#r171)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC11](../software-requirements/srd018-reports.md#ac11)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC14](../software-requirements/srd018-reports.md#ac14)

<!-- sc: S2 -->
### S2

The rollup the lead approved merges in its own repository and passes reports validate, every claim citing a source its Sources list.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R15.1](../software-requirements/srd018-reports.md#r151)
- [srd018-reports](../software-requirements/srd018-reports.md) [R15.2](../software-requirements/srd018-reports.md#r152)
- [srd018-reports](../software-requirements/srd018-reports.md) [R18.2](../software-requirements/srd018-reports.md#r182)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC13](../software-requirements/srd018-reports.md#ac13)

<!-- sc: S3 -->
### S3

The site shows the program rollup, the next plan reads its progress, and a program that kept a central reports repository has its roster moved into control/reports.md with that repository still read.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R18.3](../software-requirements/srd018-reports.md#r183)
- [srd018-reports](../software-requirements/srd018-reports.md) [R18.4](../software-requirements/srd018-reports.md#r184)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC16](../software-requirements/srd018-reports.md#ac16)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC17](../software-requirements/srd018-reports.md#ac17)

<!-- field: out_of_scope -->
## Out of scope

- Rendering the program rollup as a slide deck; scrum-master reports deck (R4) is not built yet.
- Ranking or comparing people; a rollup describes components, Sprint Goals, and blockers.

<!-- field: assumptions -->
## Assumptions

- A1: The program rollup reads the teams' rollups and opens a raw report only to cite a signal, so a lead's judgment is not repeated or overridden.
- A2: Every fact comes from a word; the model proposes and the lead decides (design.md D10).
