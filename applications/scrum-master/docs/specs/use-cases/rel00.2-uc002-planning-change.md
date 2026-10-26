---
id: rel00.2-uc002-planning-change
title: A Lead Changes the Plan Through a Management Issue and the Same Gate
status: implemented
priority: P2
test_suite: test-rel00.2-docs-change
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A lead (the technical program manager or a sub-project lead) changes the
plan: the road-map, a system or subsystem SRD, or an initiative.
specify files it as a plane::management issue, lead-only and out of the
backlog. The lead pops it and merges the docs/ change through the same
audit gate as a design change. A Sprint Goal tasks filed is
worked this way by the Product Owner who received it.

<!-- field: actor -->
## Actor

A lead, or the Product Owner who received a Sprint Goal.

<!-- field: trigger -->
## Trigger

A change to a planning document, or a Sprint Goal received.

<!-- field: preconditions -->
## Preconditions

- Push and pop work (rel00.0-uc001-push-project-work, rel00.1-uc001-pop-and-merge).

<!-- field: flow -->
## Flow

- F1: The lead runs specify with a request that names a planning document.
- F2: Model step (structure): the model turns the request into the issue body.
- F3: tpm sets plane::management, refuses the issue when the person is not a lead, and files it unassigned; it never enters the backlog.
- F4: The lead, or for a Sprint Goal the receiving Product Owner, pops it and changes the documents in the worktree.
- F5: implement runs mage audit and merges only when it passes; the merge closes the issue.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd035-specify](../software-requirements/srd035-specify.md) [R5.2](../software-requirements/srd035-specify.md#r52) | S1 traces the lead-only rule. |
| T2 | [srd001-labels](../software-requirements/srd001-labels.md) [R1.1](../software-requirements/srd001-labels.md#r11), [R1.2](../software-requirements/srd001-labels.md#r12), [R1.3](../software-requirements/srd001-labels.md#r13), [R1.4](../software-requirements/srd001-labels.md#r14) | S1 traces the management plane. |
| T3 | [srd036-implement](../software-requirements/srd036-implement.md) [R2.1](../software-requirements/srd036-implement.md#r21) | S2 traces pop. |
| T4 | [srd013-program](../software-requirements/srd013-program.md) [R7.1](../software-requirements/srd013-program.md#r71) | S2 traces the claim. |
| T5 | [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4) | S1 traces this acceptance criterion. |
| T6 | [srd001-labels](../software-requirements/srd001-labels.md) [AC1](../software-requirements/srd001-labels.md#ac1) | S1 traces this acceptance criterion. |
| T7 | [srd036-implement](../software-requirements/srd036-implement.md) [AC1](../software-requirements/srd036-implement.md#ac1) | S2 traces this acceptance criterion. |
| T8 | [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7) | S2 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A request naming a planning document becomes a plane::management issue filed by a lead and kept out of the backlog; a non-lead's is refused; tasks Sprint Goals are the other management issues scripts file.

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [R5.2](../software-requirements/srd035-specify.md#r52)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4)
- [srd001-labels](../software-requirements/srd001-labels.md) [R1.1](../software-requirements/srd001-labels.md#r11)
- [srd001-labels](../software-requirements/srd001-labels.md) [R1.2](../software-requirements/srd001-labels.md#r12)
- [srd001-labels](../software-requirements/srd001-labels.md) [R1.3](../software-requirements/srd001-labels.md#r13)
- [srd001-labels](../software-requirements/srd001-labels.md) [R1.4](../software-requirements/srd001-labels.md#r14)
- [srd001-labels](../software-requirements/srd001-labels.md) [AC1](../software-requirements/srd001-labels.md#ac1)

<!-- sc: S2 -->
### S2

The lead or the receiving Product Owner pops the issue, and its branch merges only through mage audit.

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [R2.1](../software-requirements/srd036-implement.md#r21)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC1](../software-requirements/srd036-implement.md#ac1)
- [srd013-program](../software-requirements/srd013-program.md) [R7.1](../software-requirements/srd013-program.md#r71)
- [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7)

<!-- field: out_of_scope -->
## Out of scope

- Changes to control/ policy files (plane::control); their review rule is srd001 R2.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
