---
id: rel04.0-uc002-plan-a-sprint-from-stories
title: A Product Owner Plans a Sprint From the Ranked Stories and the Sprint Goal Lists Them
status: implemented
priority: P1
test_suite: test-rel04.0-agile-hierarchy
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

After tasks files the team's Sprint Goal for the release, the Product
Owner runs plan in the team's repository. tpm lists the team's Product
Backlog, its agreed stories that are not done, by rank with their points,
with the team's velocity and the stories that fit it. The Product Owner
chooses; tpm writes the chosen stories into the Sprint Goal's Sprint
Backlog section and posts a planned event naming them. tasks turns each
story into issues that name it in an Implements line.

<!-- field: actor -->
## Actor

A Product Owner.

<!-- field: trigger -->
## Trigger

tasks filed the team's Sprint Goal for the release.

<!-- field: preconditions -->
## Preconditions

- rel04.0-uc001-author-the-hierarchy merged agreed stories in the team's repository.
- tasks (srd022) filed one Sprint Goal for the team with pinned Cites lines and an empty Sprint Backlog section.

<!-- field: flow -->
## Flow

- F1: tasks files one Sprint Goal issue per team for the release, with one Cites line per initiative item the team serves, an Environment line, empty Notes and Sprint Backlog sections, and a planned event.
- F2: The Product Owner starts plan in the team's repository.
- F3: tpm plan candidates returns the Product Backlog: the agreed stories not yet done, by rank, with points, the team's velocity, and the stories in rank order whose points fit it.
- F4: The skill takes the recommendation or the Product Owner's choice, and asks for an assumed velocity only when the team has finished no sprint.
- F5: tpm plan backlog writes the chosen stories with their points into the Sprint Goal's Sprint Backlog section and posts a planned event naming each story; a second push with the same stories changes nothing.
- F6: tasks turns each chosen story into issues, each with an Implements line naming the story.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd022-tasks](../software-requirements/srd022-tasks.md) [R4.1](../software-requirements/srd022-tasks.md#r41), [R4.3](../software-requirements/srd022-tasks.md#r43) | S1 traces the Sprint Goal tasks files. |
| T2 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.2](../software-requirements/srd034-initiatives.md#r32), [R3.5](../software-requirements/srd034-initiatives.md#r35) | S1 and S3 trace the Sprint Goal's body and its Sprint Backlog. |
| T3 | [srd031-plan](../software-requirements/srd031-plan.md) [R6.2](../software-requirements/srd031-plan.md#r62), [R6.4](../software-requirements/srd031-plan.md#r64), [R6.5](../software-requirements/srd031-plan.md#r65), [R6.6](../software-requirements/srd031-plan.md#r66) | S2 and S3 trace Sprint Planning from stories. |
| T4 | [srd045-backlog](../software-requirements/srd045-backlog.md) [R5.1](../software-requirements/srd045-backlog.md#r51), [R5.2](../software-requirements/srd045-backlog.md#r52), [R5.3](../software-requirements/srd045-backlog.md#r53), [R4.3](../software-requirements/srd045-backlog.md#r43) | S2, S3, and S4 trace the backlog, velocity, and tasks at the story level. |
| T5 | [srd006-events](../software-requirements/srd006-events.md) [R8.1](../software-requirements/srd006-events.md#r81), [R8.2](../software-requirements/srd006-events.md#r82) | S3 traces the planned event naming the stories. |
| T6 | [srd007-links](../software-requirements/srd007-links.md) [R6.1](../software-requirements/srd007-links.md#r61), [R6.5](../software-requirements/srd007-links.md#r65) | S4 traces the Implements line. |
| T7 | [srd022-tasks](../software-requirements/srd022-tasks.md) [AC2](../software-requirements/srd022-tasks.md#ac2) | S1 traces this acceptance criterion. |
| T8 | [srd031-plan](../software-requirements/srd031-plan.md) [AC4](../software-requirements/srd031-plan.md#ac4) | S2 and S3 trace this acceptance criterion. |
| T9 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11) | S3 traces this acceptance criterion. |
| T10 | [srd045-backlog](../software-requirements/srd045-backlog.md) [AC5](../software-requirements/srd045-backlog.md#ac5), [AC4](../software-requirements/srd045-backlog.md#ac4) | S2, S3, and S4 trace these acceptance criteria. |
| T11 | [srd006-events](../software-requirements/srd006-events.md) [AC7](../software-requirements/srd006-events.md#ac7) | S3 traces this acceptance criterion. |
| T12 | [srd007-links](../software-requirements/srd007-links.md) [AC6](../software-requirements/srd007-links.md#ac6) | S4 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Each team receives one Sprint Goal per release with pinned Cites lines, an Environment line, empty Notes and Sprint Backlog sections, and a planned event.

<!-- field: traces -->
#### Traces

- [srd022-tasks](../software-requirements/srd022-tasks.md) [R4.1](../software-requirements/srd022-tasks.md#r41)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R4.3](../software-requirements/srd022-tasks.md#r43)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC2](../software-requirements/srd022-tasks.md#ac2)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.2](../software-requirements/srd034-initiatives.md#r32)

<!-- sc: S2 -->
### S2

plan lists the team's not-done agreed stories by rank with points and velocity, recommends those that fit, and asks an assumed velocity only for a team with no finished sprint.

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [R6.2](../software-requirements/srd031-plan.md#r62)
- [srd031-plan](../software-requirements/srd031-plan.md) [R6.4](../software-requirements/srd031-plan.md#r64)
- [srd031-plan](../software-requirements/srd031-plan.md) [R6.5](../software-requirements/srd031-plan.md#r65)
- [srd031-plan](../software-requirements/srd031-plan.md) [AC4](../software-requirements/srd031-plan.md#ac4)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R5.1](../software-requirements/srd045-backlog.md#r51)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R5.3](../software-requirements/srd045-backlog.md#r53)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC5](../software-requirements/srd045-backlog.md#ac5)

<!-- sc: S3 -->
### S3

The chosen stories appear in the Sprint Goal's Sprint Backlog with their points and in one planned event naming them; a second push changes nothing.

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [R6.6](../software-requirements/srd031-plan.md#r66)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.5](../software-requirements/srd034-initiatives.md#r35)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R5.2](../software-requirements/srd045-backlog.md#r52)
- [srd006-events](../software-requirements/srd006-events.md) [R8.1](../software-requirements/srd006-events.md#r81)
- [srd006-events](../software-requirements/srd006-events.md) [R8.2](../software-requirements/srd006-events.md#r82)
- [srd006-events](../software-requirements/srd006-events.md) [AC7](../software-requirements/srd006-events.md#ac7)

<!-- sc: S4 -->
### S4

Each issue tasks creates for a chosen story carries one Implements line naming it.

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [R4.3](../software-requirements/srd045-backlog.md#r43)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC4](../software-requirements/srd045-backlog.md#ac4)
- [srd007-links](../software-requirements/srd007-links.md) [R6.1](../software-requirements/srd007-links.md#r61)
- [srd007-links](../software-requirements/srd007-links.md) [R6.5](../software-requirements/srd007-links.md#r65)
- [srd007-links](../software-requirements/srd007-links.md) [AC6](../software-requirements/srd007-links.md#ac6)

<!-- field: out_of_scope -->
## Out of scope

- Ranking people or assigning issues; nobody is assigned (srd041 R9.1).
- Evidence on the chosen stories; that is track-goals (06.0).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
- A2: The team's velocity before its first finished sprint is the Product Owner's answer, not a computed fact.
