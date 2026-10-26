---
id: rel01.0-uc002-push-objective-issues
title: Create-Goals Files Dated Sprint Goals for the Product Owners
status: implemented
priority: P1
test_suite: test-rel01.0-objectives
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

After the system release is planned, the technical program manager runs
tasks, which is the decision to file the release's Sprint Goals. tpm
reads the agreed initiative and the release, recommends the
Product Owner and the environment for each Sprint Goal, and files one
Sprint Goal in each owner's repository. The technical program manager
is asked only for the owner of a Sprint Goal tpm cannot place. The model
generates nothing here. Each
issue cites its initiative items pinned to the release tag, carries the
steps as its Requirements, a due date, and an empty Notes section, and
passes admission. The Product Owner receives it in their own repository
and learns nothing new to take it.

<!-- field: actor -->
## Actor

The technical program manager.

<!-- field: trigger -->
## Trigger

The system release is recorded in docs/road-map.md with its tag.

<!-- field: preconditions -->
## Preconditions

- rel01.0-uc001-author-objectives merged an agreed initiative.
- The road-map release names the initiative items it proves and the tracker milestone carries its end date.
- Subsystem SRDs declare each Product Owner's repository (design.md subsystem_srd).

<!-- field: flow -->
## Flow

- F1: The technical program manager starts tasks in the system repository.
- F2: tpm reads the planned release and its tag, the agreed initiative, the subsystem SRDs, and the Sprint Goals already filed for the release, and returns as JSON each initiative item with a recommended owner (the Product Owner whose subsystem SRD declares the component) and environment.
- F3: The skill takes every recommendation and asks the technical program manager only for the repository of a Sprint Goal with no recommended owner, offering the candidates; it asks for no confirmation.
- F4: tpm renders each Sprint Goal from the document, with one Cites line per item pinned to the release tag, the Environment line, the document's Requirements as ordered steps and its Acceptance Criteria, a due date, and an empty Notes section.
- F5: tpm issue file files each issue in the owner's repository, unassigned; tpm issue lint admits it or sets needs-triage with the reasons.
- F6: tpm posts a planned event on each Sprint Goal it created or updated; a second run creates no duplicate.
- F7: The Product Owner receives a dated issue in their own repository whose body states what to do.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.2](../software-requirements/srd034-initiatives.md#r32), [R3.3](../software-requirements/srd034-initiatives.md#r33), [R3.4](../software-requirements/srd034-initiatives.md#r34), [R9.1](../software-requirements/srd034-initiatives.md#r91) | S1 and S6 trace how a Sprint Goal is rendered and what the owner does with it. |
| T2 | [srd022-tasks](../software-requirements/srd022-tasks.md) [R1.1](../software-requirements/srd022-tasks.md#r11), [R2.1](../software-requirements/srd022-tasks.md#r21), [R2.2](../software-requirements/srd022-tasks.md#r22), [R2.3](../software-requirements/srd022-tasks.md#r23), [R3.1](../software-requirements/srd022-tasks.md#r31), [R3.2](../software-requirements/srd022-tasks.md#r32), [R3.3](../software-requirements/srd022-tasks.md#r33), [R4.1](../software-requirements/srd022-tasks.md#r41), [R4.3](../software-requirements/srd022-tasks.md#r43), [R5.1](../software-requirements/srd022-tasks.md#r51), [R5.2](../software-requirements/srd022-tasks.md#r52), [R5.3](../software-requirements/srd022-tasks.md#r53) | S1, S2, and S5 trace tasks' reads, questions, pushes, and refusals. |
| T3 | [srd013-program](../software-requirements/srd013-program.md) [R1.1](../software-requirements/srd013-program.md#r11), [R1.2](../software-requirements/srd013-program.md#r12), [R1.3](../software-requirements/srd013-program.md#r13), [R2.1](../software-requirements/srd013-program.md#r21), [R3.1](../software-requirements/srd013-program.md#r31) | S3 traces filing and admission. |
| T4 | [srd007-links](../software-requirements/srd007-links.md) [R5.1](../software-requirements/srd007-links.md#r51), [R5.2](../software-requirements/srd007-links.md#r52), [R5.3](../software-requirements/srd007-links.md#r53) | S4 traces pinned citations. |
| T5 | [srd006-events](../software-requirements/srd006-events.md) [R8.1](../software-requirements/srd006-events.md#r81), [R8.2](../software-requirements/srd006-events.md#r82) | S5 traces the planned event. |
| T6 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3), [AC9](../software-requirements/srd034-initiatives.md#ac9) | S1 and S6 trace these acceptance criteria. |
| T7 | [srd022-tasks](../software-requirements/srd022-tasks.md) [AC1](../software-requirements/srd022-tasks.md#ac1), [AC2](../software-requirements/srd022-tasks.md#ac2), [AC3](../software-requirements/srd022-tasks.md#ac3) | S1, S2, and S5 trace these acceptance criteria. |
| T8 | [srd013-program](../software-requirements/srd013-program.md) [AC1](../software-requirements/srd013-program.md#ac1), [AC2](../software-requirements/srd013-program.md#ac2), [AC3](../software-requirements/srd013-program.md#ac3) | S3 traces these acceptance criteria. |
| T9 | [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5) | S4 traces this acceptance criterion. |
| T10 | [srd006-events](../software-requirements/srd006-events.md) [AC7](../software-requirements/srd006-events.md#ac7) | S5 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Each owner receives one Sprint Goal per release with pinned Cites lines, an Environment line, the document's Requirements and Acceptance Criteria, a due date, and an empty Notes section.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.2](../software-requirements/srd034-initiatives.md#r32)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.3](../software-requirements/srd034-initiatives.md#r33)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.4](../software-requirements/srd034-initiatives.md#r34)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R4.1](../software-requirements/srd022-tasks.md#r41)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC2](../software-requirements/srd022-tasks.md#ac2)

<!-- sc: S2 -->
### S2

tasks reads its facts from the release, the documents, and existing Sprint Goals, takes the recommended owner and the environment, asks only for a missing owner, asks no confirmation, and refuses an unpinned citation or a missing environment.

<!-- field: traces -->
#### Traces

- [srd022-tasks](../software-requirements/srd022-tasks.md) [R1.1](../software-requirements/srd022-tasks.md#r11)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R2.1](../software-requirements/srd022-tasks.md#r21)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R2.2](../software-requirements/srd022-tasks.md#r22)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R2.3](../software-requirements/srd022-tasks.md#r23)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R3.1](../software-requirements/srd022-tasks.md#r31)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R3.2](../software-requirements/srd022-tasks.md#r32)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R3.3](../software-requirements/srd022-tasks.md#r33)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC1](../software-requirements/srd022-tasks.md#ac1)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R5.1](../software-requirements/srd022-tasks.md#r51)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R5.2](../software-requirements/srd022-tasks.md#r52)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R5.3](../software-requirements/srd022-tasks.md#r53)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC3](../software-requirements/srd022-tasks.md#ac3)

<!-- sc: S3 -->
### S3

Every Sprint Goal is filed unassigned through tpm issue file and admitted by tpm issue lint, which flags an unpinned Cites line or a criterion without test and environment.

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [R1.1](../software-requirements/srd013-program.md#r11)
- [srd013-program](../software-requirements/srd013-program.md) [R1.2](../software-requirements/srd013-program.md#r12)
- [srd013-program](../software-requirements/srd013-program.md) [R1.3](../software-requirements/srd013-program.md#r13)
- [srd013-program](../software-requirements/srd013-program.md) [AC1](../software-requirements/srd013-program.md#ac1)
- [srd013-program](../software-requirements/srd013-program.md) [R2.1](../software-requirements/srd013-program.md#r21)
- [srd013-program](../software-requirements/srd013-program.md) [AC2](../software-requirements/srd013-program.md#ac2)
- [srd013-program](../software-requirements/srd013-program.md) [R3.1](../software-requirements/srd013-program.md#r31)
- [srd013-program](../software-requirements/srd013-program.md) [AC3](../software-requirements/srd013-program.md#ac3)

<!-- sc: S4 -->
### S4

Each Cites line is a URL pinned to a release tag that resolves to an item of the initiative.

<!-- field: traces -->
#### Traces

- [srd007-links](../software-requirements/srd007-links.md) [R5.1](../software-requirements/srd007-links.md#r51)
- [srd007-links](../software-requirements/srd007-links.md) [R5.2](../software-requirements/srd007-links.md#r52)
- [srd007-links](../software-requirements/srd007-links.md) [R5.3](../software-requirements/srd007-links.md#r53)
- [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5)

<!-- sc: S5 -->
### S5

Each created or updated Sprint Goal carries one planned event naming its release, requirements, and environment.

<!-- field: traces -->
#### Traces

- [srd006-events](../software-requirements/srd006-events.md) [R8.1](../software-requirements/srd006-events.md#r81)
- [srd006-events](../software-requirements/srd006-events.md) [R8.2](../software-requirements/srd006-events.md#r82)
- [srd006-events](../software-requirements/srd006-events.md) [AC7](../software-requirements/srd006-events.md#ac7)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R4.3](../software-requirements/srd022-tasks.md#r43)

<!-- sc: S6 -->
### S6

The Product Owner takes the issue with implement and needs no other tool, channel, or step.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R9.1](../software-requirements/srd034-initiatives.md#r91)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC9](../software-requirements/srd034-initiatives.md#ac9)

<!-- field: out_of_scope -->
## Out of scope

- The Sprint Goal for the test subsystem (srd022 R4.2); stage one has no test subsystem.
- Deciding that a Sprint Goal is met; Sprint Goals close at the sprint end (rel01.2-uc001-sprint-end-release).
- Assigning the issue to anyone.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step.
