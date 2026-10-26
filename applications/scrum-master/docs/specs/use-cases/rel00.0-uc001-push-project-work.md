---
id: rel00.0-uc001-push-project-work
title: A Developer Pushes a Request and It Becomes an Admitted Issue
status: implemented
priority: P1
test_suite: test-rel00.0-push
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The program's labels exist first, created from control/labels.md. A
Developer asks for a piece of project work; specify turns the request
into one admitted, unassigned plane::data issue in the issue body's work format,
with its kind and component, in the backlog. The model writes the body and
nothing else; tpm picks the labels, files, and lints.

<!-- field: actor -->
## Actor

A Developer (a member of the program).

<!-- field: trigger -->
## Trigger

A request for project work in a component's repository.

<!-- field: preconditions -->
## Preconditions

- The program group exists.
- A subsystem SRD declares the repository's component.

<!-- field: flow -->
## Flow

- F1: tpm issue labels creates or updates the program's group labels from control/labels.md or the defaults; a second run changes nothing.
- F2: The Developer runs specify with the request.
- F3: Model step (structure): the model turns the request into a title and the Requirements and Acceptance Criteria sections, each criterion naming its test and environment.
- F4: tpm sets plane::data because the request names no docs/ or control/ path, the kind the request states or the person names when asked, and the component from the subsystem SRD; tpm issue file files the issue unassigned and lints it, and it enters the backlog.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd013-program](../software-requirements/srd013-program.md) [R3.1](../software-requirements/srd013-program.md#r31), [R3.3](../software-requirements/srd013-program.md#r33), [R6.1](../software-requirements/srd013-program.md#r61), [R6.2](../software-requirements/srd013-program.md#r62) | S1 and S2 trace labels and filing. |
| T2 | [srd035-specify](../software-requirements/srd035-specify.md) [R1.1](../software-requirements/srd035-specify.md#r11), [R2.1](../software-requirements/srd035-specify.md#r21), [R2.2](../software-requirements/srd035-specify.md#r22), [R3.1](../software-requirements/srd035-specify.md#r31), [R3.2](../software-requirements/srd035-specify.md#r32), [R3.3](../software-requirements/srd035-specify.md#r33), [R4.1](../software-requirements/srd035-specify.md#r41), [R4.2](../software-requirements/srd035-specify.md#r42), [R5.1](../software-requirements/srd035-specify.md#r51), [R5.3](../software-requirements/srd035-specify.md#r53) | S2 and S3 trace push. |
| T3 | [srd001-labels](../software-requirements/srd001-labels.md) [R3.1](../software-requirements/srd001-labels.md#r31), [R3.2](../software-requirements/srd001-labels.md#r32), [R3.3](../software-requirements/srd001-labels.md#r33), [R3.4](../software-requirements/srd001-labels.md#r34) | S2 traces the data plane. |
| T4 | [srd013-program](../software-requirements/srd013-program.md) [AC3](../software-requirements/srd013-program.md#ac3), [AC6](../software-requirements/srd013-program.md#ac6) | S1 and S2 trace these acceptance criteria. |
| T5 | [srd035-specify](../software-requirements/srd035-specify.md) [AC1](../software-requirements/srd035-specify.md#ac1), [AC2](../software-requirements/srd035-specify.md#ac2), [AC3](../software-requirements/srd035-specify.md#ac3), [AC4](../software-requirements/srd035-specify.md#ac4) | S2 and S3 trace these acceptance criteria. |
| T6 | [srd001-labels](../software-requirements/srd001-labels.md) [AC3](../software-requirements/srd001-labels.md#ac3) | S2 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The program's labels exist once, created from control/labels.md or the defaults, and a second run changes nothing.

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [R6.1](../software-requirements/srd013-program.md#r61)
- [srd013-program](../software-requirements/srd013-program.md) [R6.2](../software-requirements/srd013-program.md#r62)
- [srd013-program](../software-requirements/srd013-program.md) [AC6](../software-requirements/srd013-program.md#ac6)

<!-- sc: S2 -->
### S2

A request that names no docs/ or control/ path becomes one admitted, unassigned plane::data issue with a kind, a component, Requirements, and Acceptance Criteria naming test and environment, filed without a confirmation step.

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [R1.1](../software-requirements/srd035-specify.md#r11)
- [srd035-specify](../software-requirements/srd035-specify.md) [R2.1](../software-requirements/srd035-specify.md#r21)
- [srd035-specify](../software-requirements/srd035-specify.md) [R2.2](../software-requirements/srd035-specify.md#r22)
- [srd035-specify](../software-requirements/srd035-specify.md) [R3.1](../software-requirements/srd035-specify.md#r31)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC1](../software-requirements/srd035-specify.md#ac1)
- [srd035-specify](../software-requirements/srd035-specify.md) [R3.2](../software-requirements/srd035-specify.md#r32)
- [srd035-specify](../software-requirements/srd035-specify.md) [R3.3](../software-requirements/srd035-specify.md#r33)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC2](../software-requirements/srd035-specify.md#ac2)
- [srd035-specify](../software-requirements/srd035-specify.md) [R4.1](../software-requirements/srd035-specify.md#r41)
- [srd035-specify](../software-requirements/srd035-specify.md) [R4.2](../software-requirements/srd035-specify.md#r42)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC3](../software-requirements/srd035-specify.md#ac3)
- [srd013-program](../software-requirements/srd013-program.md) [R3.1](../software-requirements/srd013-program.md#r31)
- [srd013-program](../software-requirements/srd013-program.md) [R3.3](../software-requirements/srd013-program.md#r33)
- [srd013-program](../software-requirements/srd013-program.md) [AC3](../software-requirements/srd013-program.md#ac3)
- [srd001-labels](../software-requirements/srd001-labels.md) [R3.1](../software-requirements/srd001-labels.md#r31)
- [srd001-labels](../software-requirements/srd001-labels.md) [R3.2](../software-requirements/srd001-labels.md#r32)
- [srd001-labels](../software-requirements/srd001-labels.md) [R3.3](../software-requirements/srd001-labels.md#r33)
- [srd001-labels](../software-requirements/srd001-labels.md) [R3.4](../software-requirements/srd001-labels.md#r34)
- [srd001-labels](../software-requirements/srd001-labels.md) [AC3](../software-requirements/srd001-labels.md#ac3)

<!-- sc: S3 -->
### S3

No issue is assigned by push, and the specify skill is at most 50 lines.

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [R5.1](../software-requirements/srd035-specify.md#r51)
- [srd035-specify](../software-requirements/srd035-specify.md) [R5.3](../software-requirements/srd035-specify.md#r53)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4)

<!-- field: out_of_scope -->
## Out of scope

- Taking and working the issue; that is rel00.1-uc001-pop-and-merge.
- Changes under docs/; that is release 00.2.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
