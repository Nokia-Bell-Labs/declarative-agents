---
id: rel01.1-uc001-weekly-status
title: A Product Owner Reports Weekly Status Through a Dated Issue
status: implemented
priority: P1
test_suite: test-rel01.1-weekly-status
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

Each week a status issue waits in the Product Owner's repository, due the
day after the reminder weekday, so the tracker's due-date notification
reaches the owner on that weekday. The owner pops it; its steps walk each
open Sprint Goal and record status, problems, and complete or not
complete. The result is a report in the reports repository's fixed shape
that tpm reports validate passes, merged through the inner loop, after
which the next week's issue exists with its new due date.

<!-- field: actor -->
## Actor

A Product Owner (a sub-project lead).

<!-- field: trigger -->
## Trigger

The tracker's due-date notification for the week's status issue.

<!-- field: preconditions -->
## Preconditions

- tpm reports init scaffolded the reports repository, its report schema, its validate CI job, and its configuration listing the team and the reminder weekday.
- The Product Owner's repository holds open Sprint Goals filed by rel01.0-uc002-push-objective-issues.

<!-- field: flow -->
## Flow

- F1: The weekly status issue exists in the owner's repository with a Recurrence section, a due date the day after the reminder weekday, and a body that mentions the owner without assigning them.
- F2: The tracker sends its due-date notification to the owner, a participant through the mention.
- F3: The owner runs implement on the status issue; its Requirements list the steps.
- F4: tpm reports week returns as JSON every open Sprint Goal in the repository with its due date and the owner's tracker activity for the week.
- F5: Model step (structure): the model asks the owner about each Sprint Goal and turns the answers into the report in the fixed shape: status, problems, and complete or not complete per Sprint Goal.
- F6: tpm reports validate checks the report; tpm issue merge merges it in the reports repository and closes the status issue in the owner's repository.
- F7: The merge lands and closes the status issue; the reports repository's scheduled tpm reports status-issues run recurs it (tpm issue recur), opening the next week's instance with the next due date and the run appended to Previous Runs.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd018-reports](../software-requirements/srd018-reports.md) [R9.1](../software-requirements/srd018-reports.md#r91), [R9.2](../software-requirements/srd018-reports.md#r92), [R9.3](../software-requirements/srd018-reports.md#r93), [R9.4](../software-requirements/srd018-reports.md#r94) | S1 to S3 trace the weekly status issue. |
| T2 | [srd018-reports](../software-requirements/srd018-reports.md) [R1.1](../software-requirements/srd018-reports.md#r11), [R1.2](../software-requirements/srd018-reports.md#r12), [R1.3](../software-requirements/srd018-reports.md#r13), [R2.1](../software-requirements/srd018-reports.md#r21), [R2.2](../software-requirements/srd018-reports.md#r22), [R6.1](../software-requirements/srd018-reports.md#r61), [R7.1](../software-requirements/srd018-reports.md#r71), [R7.2](../software-requirements/srd018-reports.md#r72), [R8.1](../software-requirements/srd018-reports.md#r81) | S1, S2, and S4 trace the reports repository, validation, and the sprint update audit. |
| T3 | [srd013-program](../software-requirements/srd013-program.md) [R8.1](../software-requirements/srd013-program.md#r81) | S3 traces re-creating the recurring issue. |
| T4 | [srd018-reports](../software-requirements/srd018-reports.md) [AC7](../software-requirements/srd018-reports.md#ac7) | S1 to S3 trace this acceptance criterion. |
| T5 | [srd018-reports](../software-requirements/srd018-reports.md) [AC1](../software-requirements/srd018-reports.md#ac1), [AC2](../software-requirements/srd018-reports.md#ac2), [AC5](../software-requirements/srd018-reports.md#ac5), [AC6](../software-requirements/srd018-reports.md#ac6) | S1, S2, and S4 trace these acceptance criteria. |
| T6 | [srd010-standup](../software-requirements/srd010-standup.md) [AC6](../software-requirements/srd010-standup.md#ac6) | S3 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Each person the reports configuration lists receives one status issue per week, due the day after the reminder weekday, mentioning them without assigning them.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R9.1](../software-requirements/srd018-reports.md#r91)
- [srd018-reports](../software-requirements/srd018-reports.md) [R9.2](../software-requirements/srd018-reports.md#r92)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC7](../software-requirements/srd018-reports.md#ac7)
- [srd018-reports](../software-requirements/srd018-reports.md) [R6.1](../software-requirements/srd018-reports.md#r61)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC5](../software-requirements/srd018-reports.md#ac5)
- [srd018-reports](../software-requirements/srd018-reports.md) [R7.1](../software-requirements/srd018-reports.md#r71)
- [srd018-reports](../software-requirements/srd018-reports.md) [R7.2](../software-requirements/srd018-reports.md#r72)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC6](../software-requirements/srd018-reports.md#ac6)

<!-- sc: S2 -->
### S2

Worked with implement, the issue produces a report with status, problems, and complete or not complete for every open Sprint Goal, which tpm reports validate passes.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R9.3](../software-requirements/srd018-reports.md#r93)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC7](../software-requirements/srd018-reports.md#ac7)
- [srd018-reports](../software-requirements/srd018-reports.md) [R2.1](../software-requirements/srd018-reports.md#r21)
- [srd018-reports](../software-requirements/srd018-reports.md) [R2.2](../software-requirements/srd018-reports.md#r22)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC2](../software-requirements/srd018-reports.md#ac2)
- [srd018-reports](../software-requirements/srd018-reports.md) [R8.1](../software-requirements/srd018-reports.md#r81)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC6](../software-requirements/srd018-reports.md#ac6)

<!-- sc: S3 -->
### S3

The status issue closes by its merge and the next instance exists with the next due date.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R9.4](../software-requirements/srd018-reports.md#r94)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC7](../software-requirements/srd018-reports.md#ac7)
- [srd013-program](../software-requirements/srd013-program.md) [R8.1](../software-requirements/srd013-program.md#r81)
- [srd010-standup](../software-requirements/srd010-standup.md) [AC6](../software-requirements/srd010-standup.md#ac6)

<!-- sc: S4 -->
### S4

tpm reports init creates the reports repository's layout, schema, CI, and configuration once and never overwrites a report.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R1.1](../software-requirements/srd018-reports.md#r11)
- [srd018-reports](../software-requirements/srd018-reports.md) [R1.2](../software-requirements/srd018-reports.md#r12)
- [srd018-reports](../software-requirements/srd018-reports.md) [R1.3](../software-requirements/srd018-reports.md#r13)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC1](../software-requirements/srd018-reports.md#ac1)

<!-- field: out_of_scope -->
## Out of scope

- Status from people the reports configuration does not list; stage one lists the Product Owners only.
- Aggregating the reports; that is rel01.1-uc002-period-rollup.
- The daily standup (srd010 R2 to R7, srd020).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
