---
id: rel01.0-uc001-author-objectives
title: The Technical Program Manager Writes an Audited Initiative
status: implemented
priority: P1
test_suite: test-rel01.0-objectives
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The technical program manager writes the program's Sprint Goals for the
coming periods as an initiative in the system repository and
merges it through the inner loop. The merge runs mage audit, which
runs the parent's specification-critic corpus-audit agent over the
repository; a malformed document does not merge. The merged document is
what the next release plans from and what Sprint Goals cite.

<!-- field: actor -->
## Actor

The technical program manager of a program.

<!-- field: trigger -->
## Trigger

The program agrees what it commits to for the next horizon of periods.

<!-- field: preconditions -->
## Preconditions

- The system repository carries the docs/ corpus, docs/corpus-charter.yaml, and docs/external-pins.yaml (design.md D7).
- The corpus audit's agent is available: in this repository the sibling agent-core; in a program repository, the release its docs/external-pins.yaml pins, in the clone cache (GH-190).
- The system repository runs mage audit before every merge that changes docs/.

<!-- field: flow -->
## Flow

- F1: The technical program manager files an issue for the Sprint Goals and pops it with implement.
- F2: Model step (structure): the model turns the technical program manager's outcomes conversation into docs/specs/objectives/objNNN-short-name.md, an SRD in the parent's format, with a problem, goals, one requirement group per objective with a due item in its applicability, numbered items, non_goals, acceptance criteria naming tests and an environment, the owner role and the horizon in its context, and status to_be_agreed.
- F3: Model step (iterate): the model revises the document with the technical program manager until they accept it; the status moves to agreed when the program agrees it.
- F4: mage audit runs the gate tests and the specification-critic corpus-audit agent of the pinned parent release, locally and before the merge; the agent runs deterministic checks and spends no model tokens.
- F5: A missing required field, an acceptance criterion tracing an absent item, or an unpinned reference fails the audit and blocks the merge.
- F6: The branch merges with the audit passing; the document's item ids are stable from then on.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.1](../software-requirements/srd034-initiatives.md#r11), [R1.2](../software-requirements/srd034-initiatives.md#r12), [R1.3](../software-requirements/srd034-initiatives.md#r13), [R1.4](../software-requirements/srd034-initiatives.md#r14), [R1.5](../software-requirements/srd034-initiatives.md#r15), [R1.6](../software-requirements/srd034-initiatives.md#r16) | S1 and S2 trace the initiative's location, shape, fields, and stable ids. |
| T2 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R2.1](../software-requirements/srd034-initiatives.md#r21), [R2.2](../software-requirements/srd034-initiatives.md#r22) | S3 traces the agent-based audit and its CI gate. |
| T3 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1), [AC2](../software-requirements/srd034-initiatives.md#ac2) | S1 to S3 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The initiative carries every field the initiative type requires, one Sprint Goal per requirement group with a due date, and acceptance criteria that name a test and an environment.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.1](../software-requirements/srd034-initiatives.md#r11)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.2](../software-requirements/srd034-initiatives.md#r12)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.3](../software-requirements/srd034-initiatives.md#r13)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.4](../software-requirements/srd034-initiatives.md#r14)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.5](../software-requirements/srd034-initiatives.md#r15)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1)

<!-- sc: S2 -->
### S2

A changed Sprint Goal is added as a new item or document and the old one retired; no merged item id is renumbered.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R1.6](../software-requirements/srd034-initiatives.md#r16)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1)

<!-- sc: S3 -->
### S3

mage audit runs the specification-critic agent before every merge that changes docs/, fails on a malformed initiative, and blocks its merge.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R2.1](../software-requirements/srd034-initiatives.md#r21)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R2.2](../software-requirements/srd034-initiatives.md#r22)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- field: out_of_scope -->
## Out of scope

- Choosing which initiative items a release proves; that is rel01.2-uc002-plan-next-period.
- Filing Sprint Goals downstream; that is rel01.0-uc002-push-objective-issues.
- A human-readable rendering of the document.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
