---
id: rel04.0-uc003-sprint-goal-ledger
title: A Sprint Goal Closes at Sprint End With Its Stories Reported, and Old Objective Issues Read as Sprint Goals
status: implemented
priority: P1
test_suite: test-rel04.0-agile-hierarchy
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

At the sprint's end the owner works the sprint-end issue. tpm sprint-end
facts returns each Sprint Goal's ledger, reported status, and listed
stories; the owner says which stories are done. Before track-goals runs,
the merge closes each reported-complete Sprint Goal with no met event, the
summary records the stories done, those count toward velocity and their
epics' progress, and the rest return to the Product Backlog. A Sprint Goal
issue filed before release 04.0 is read the same way, marker and ledger
included.

<!-- field: actor -->
## Actor

A Product Owner.

<!-- field: trigger -->
## Trigger

The sprint-end issue of the period is due.

<!-- field: preconditions -->
## Preconditions

- rel04.0-uc002-plan-a-sprint-from-stories listed stories on the team's Sprint Goal.
- The program does not yet run track-goals.

<!-- field: flow -->
## Flow

- F1: The owner pops the sprint-end issue; tpm sprint-end facts returns each Sprint Goal's ledger, reported status, and the stories in its Sprint Backlog, reading Sprint Goals as Sprint Goals.
- F2: The skill asks the owner which listed stories are done and what to change.
- F3: The merge of the sprint-end issue closes each Sprint Goal the owner reports complete with no met event, and the summary records the stories done.
- F4: The stories reported done count toward the team's velocity and their epics' progress; the rest return to the Product Backlog for the next Sprint Planning.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.1](../software-requirements/srd038-sprint-end.md#r21), [R2.2](../software-requirements/srd038-sprint-end.md#r22), [R2.4](../software-requirements/srd038-sprint-end.md#r24) | S1 and S2 trace the interim close and the stories reported. |
| T2 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.6](../software-requirements/srd034-initiatives.md#r36), [R3.7](../software-requirements/srd034-initiatives.md#r37) | S1 and S3 trace when a Sprint Goal is met and the objective alias. |
| T3 | [srd006-events](../software-requirements/srd006-events.md) [R11.3](../software-requirements/srd006-events.md#r113) | S3 traces replaying a Sprint Goal's ledger. |
| T4 | [srd045-backlog](../software-requirements/srd045-backlog.md) [R3.2](../software-requirements/srd045-backlog.md#r32), [R3.3](../software-requirements/srd045-backlog.md#r33), [R5.3](../software-requirements/srd045-backlog.md#r53) | S2 traces done stories, roll-up, and velocity. |
| T5 | [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC2](../software-requirements/srd038-sprint-end.md#ac2) | S1 and S2 trace this acceptance criterion. |
| T6 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11) | S1 and S3 trace this acceptance criterion. |
| T7 | [srd006-events](../software-requirements/srd006-events.md) [AC10](../software-requirements/srd006-events.md#ac10) | S3 traces this acceptance criterion. |
| T8 | [srd045-backlog](../software-requirements/srd045-backlog.md) [AC3](../software-requirements/srd045-backlog.md#ac3), [AC5](../software-requirements/srd045-backlog.md#ac5) | S2 traces these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A reported-complete Sprint Goal closes by the sprint-end merge with no met event and is shown as reported complete, not met.

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.1](../software-requirements/srd038-sprint-end.md#r21)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.2](../software-requirements/srd038-sprint-end.md#r22)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC2](../software-requirements/srd038-sprint-end.md#ac2)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.6](../software-requirements/srd034-initiatives.md#r36)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11)

<!-- sc: S2 -->
### S2

The summary records which listed stories are done; those count toward velocity and their epics' progress, and the others return to the Product Backlog.

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.4](../software-requirements/srd038-sprint-end.md#r24)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R3.2](../software-requirements/srd045-backlog.md#r32)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R3.3](../software-requirements/srd045-backlog.md#r33)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R5.3](../software-requirements/srd045-backlog.md#r53)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC3](../software-requirements/srd045-backlog.md#ac3)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC5](../software-requirements/srd045-backlog.md#ac5)

<!-- sc: S3 -->
### S3

An objective issue filed before release 04.0 is listed, closed, and replayed as a Sprint Goal.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.7](../software-requirements/srd034-initiatives.md#r37)
- [srd006-events](../software-requirements/srd006-events.md) [R11.3](../software-requirements/srd006-events.md#r113)
- [srd006-events](../software-requirements/srd006-events.md) [AC10](../software-requirements/srd006-events.md#ac10)

<!-- field: out_of_scope -->
## Out of scope

- Evidence events and met; those are track-goals (06.0).
- Forecasts and burn-ups (srd045 R7).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
