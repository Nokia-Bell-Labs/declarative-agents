---
id: rel04.0-uc001-author-the-hierarchy
title: The Technical Program Manager and a Product Owner Write the Backlog Hierarchy Through the Gate
status: implemented
priority: P1
test_suite: test-rel04.0-agile-hierarchy
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The technical program manager writes a theme and an initiative under it in
the system repository; a Product Owner writes an epic under the initiative,
a feature under the epic, and user stories under the feature in the team's
repository, each story with its statement, points, rank, and acceptance
criteria naming a test and an environment. Each document is merged through
the audit gate, and mage audit's hierarchy gate checks every document's
fields, its parent's kind, and its links. An initiative written
before release 04.0 passes as an initiative.

<!-- field: actor -->
## Actor

The technical program manager and a Product Owner.

<!-- field: trigger -->
## Trigger

A program plans work for the coming quarters.

<!-- field: preconditions -->
## Preconditions

- The program's system repository and a team's repository carry the corpus (D7) and run mage audit before each merge that changes docs/ (srd034 R2.2).

<!-- field: flow -->
## Flow

- F1: The technical program manager writes a theme and an initiative naming it as parent in the system repository, through a plane::management issue worked with implement.
- F2: A Product Owner writes an epic naming the initiative as parent by pinned URL, a feature naming the epic, and stories naming the feature, in the team's repository; each story carries as_a, i_want, so_that, points, rank, and acceptance criteria naming a test and an environment.
- F3: mage audit's hierarchy gate reads every hierarchy document, checks its required fields and context items, its parent's kind and resolution, its link targets, and that no parent or blocked_by links form a cycle; a malformed document fails the merge.
- F4: An objective document under docs/specs/objectives/ is read as an initiative and passes the same gate.
- F5: The merged documents are the Product Backlog the team's Sprint Planning reads (rel04.0-uc002-plan-a-sprint-from-stories).

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd045-backlog](../software-requirements/srd045-backlog.md) [R2.1](../software-requirements/srd045-backlog.md#r21), [R2.2](../software-requirements/srd045-backlog.md#r22), [R2.3](../software-requirements/srd045-backlog.md#r23), [R3.1](../software-requirements/srd045-backlog.md#r31), [R4.1](../software-requirements/srd045-backlog.md#r41), [R4.2](../software-requirements/srd045-backlog.md#r42), [R6.1](../software-requirements/srd045-backlog.md#r61) | S1, S2, and S3 trace the documents, their parents, stories, and the gate. |
| T2 | [srd007-links](../software-requirements/srd007-links.md) [R7.1](../software-requirements/srd007-links.md#r71), [R7.2](../software-requirements/srd007-links.md#r72), [R8.2](../software-requirements/srd007-links.md#r82) | S2 and S3 trace links on documents and their checks. |
| T3 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.1](../software-requirements/srd034-initiatives.md#r11), [R1.2](../software-requirements/srd034-initiatives.md#r12), [R1.3](../software-requirements/srd034-initiatives.md#r13), [R2.1](../software-requirements/srd034-initiatives.md#r21) | S4 traces initiatives and the objective alias. |
| T4 | [srd045-backlog](../software-requirements/srd045-backlog.md) [AC2](../software-requirements/srd045-backlog.md#ac2), [AC4](../software-requirements/srd045-backlog.md#ac4), [AC6](../software-requirements/srd045-backlog.md#ac6) | S1, S2, and S3 trace these acceptance criteria. |
| T5 | [srd007-links](../software-requirements/srd007-links.md) [AC7](../software-requirements/srd007-links.md#ac7), [AC8](../software-requirements/srd007-links.md#ac8) | S2 and S3 trace these acceptance criteria. |
| T6 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1), [AC2](../software-requirements/srd034-initiatives.md#ac2) | S4 traces these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Each planning item is a theme, initiative, epic, feature, or story document in the repository its level names, and none is worked as an issue.

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [R2.1](../software-requirements/srd045-backlog.md#r21)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R2.2](../software-requirements/srd045-backlog.md#r22)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R2.3](../software-requirements/srd045-backlog.md#r23)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC2](../software-requirements/srd045-backlog.md#ac2)

<!-- sc: S2 -->
### S2

Each document names at most one parent of the kind above it, a feature always names an epic, and a story carries its statement, points, and criteria naming a test and an environment.

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [R3.1](../software-requirements/srd045-backlog.md#r31)
- [srd007-links](../software-requirements/srd007-links.md) [R7.1](../software-requirements/srd007-links.md#r71)
- [srd007-links](../software-requirements/srd007-links.md) [R7.2](../software-requirements/srd007-links.md#r72)
- [srd007-links](../software-requirements/srd007-links.md) [AC7](../software-requirements/srd007-links.md#ac7)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R4.1](../software-requirements/srd045-backlog.md#r41)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R4.2](../software-requirements/srd045-backlog.md#r42)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC4](../software-requirements/srd045-backlog.md#ac4)

<!-- sc: S3 -->
### S3

mage audit fails on a missing field, a parent of the wrong kind or that does not resolve, an unresolved link, a cycle, a story without points, or a criterion without test and environment.

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [R6.1](../software-requirements/srd045-backlog.md#r61)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC6](../software-requirements/srd045-backlog.md#ac6)
- [srd007-links](../software-requirements/srd007-links.md) [R8.2](../software-requirements/srd007-links.md#r82)
- [srd007-links](../software-requirements/srd007-links.md) [AC8](../software-requirements/srd007-links.md#ac8)

<!-- sc: S4 -->
### S4

An initiative, and an objective document read as one, passes the gate with its outcomes, due items, owner, and horizon.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.1](../software-requirements/srd034-initiatives.md#r11)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.2](../software-requirements/srd034-initiatives.md#r12)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.3](../software-requirements/srd034-initiatives.md#r13)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R2.1](../software-requirements/srd034-initiatives.md#r21)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- field: out_of_scope -->
## Out of scope

- Roll-up across repositories (srd045 R3.4); the gate checks each repository's own documents.
- Evidence on story criteria; that is track-goals (06.0).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
