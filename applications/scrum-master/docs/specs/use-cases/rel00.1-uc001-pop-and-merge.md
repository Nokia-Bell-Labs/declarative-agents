---
id: rel00.1-uc001-pop-and-merge
title: A Developer Pops an Issue into a Worktree and Its Merge Closes It
status: implemented
priority: P1
test_suite: test-rel00.1-pop
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A Developer takes an admitted issue with implement. tpm claims it for them
alone and opens its own branch and worktree; the work is done and
committed there; tpm merges the branch, which closes the issue, and a
reviewer merges it. The skill asks nothing and closes nothing itself.

<!-- field: actor -->
## Actor

A Developer (a member of the program).

<!-- field: trigger -->
## Trigger

An admitted issue the Developer chooses to work.

<!-- field: preconditions -->
## Preconditions

- An admitted issue exists (rel00.0-uc001-push-project-work).

<!-- field: flow -->
## Flow

- F1: The Developer runs implement on the issue; tpm issue pop assigns only them and creates the branch <iid>-<slug> and its worktree, or refuses an issue someone else holds, a closed issue, or a data issue admission did not admit.
- F2: The work is done and committed in the worktree and pushed; it is the Developer's or their agent's, not the skill's.
- F3: tpm issue merge merges the branch with a merge commit citing the issue, pushes it, and closes the issue.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd013-program](../software-requirements/srd013-program.md) [R5.1](../software-requirements/srd013-program.md#r51), [R5.2](../software-requirements/srd013-program.md#r52), [R7.1](../software-requirements/srd013-program.md#r71), [R7.2](../software-requirements/srd013-program.md#r72), [R7.3](../software-requirements/srd013-program.md#r73), [R7.4](../software-requirements/srd013-program.md#r74) | S1, S2, and S3 trace pop and the merge. |
| T2 | [srd036-implement](../software-requirements/srd036-implement.md) [R1.1](../software-requirements/srd036-implement.md#r11), [R2.1](../software-requirements/srd036-implement.md#r21), [R2.2](../software-requirements/srd036-implement.md#r22), [R2.3](../software-requirements/srd036-implement.md#r23), [R3.1](../software-requirements/srd036-implement.md#r31), [R3.3](../software-requirements/srd036-implement.md#r33), [R4.1](../software-requirements/srd036-implement.md#r41), [R4.2](../software-requirements/srd036-implement.md#r42) | S1, S2, and S3 trace the skill. |
| T3 | [srd041-states](../software-requirements/srd041-states.md) [R7.1](../software-requirements/srd041-states.md#r71), [R9.1](../software-requirements/srd041-states.md#r91) | S1 and S2 trace closing and the assignee rule. |
| T4 | [srd013-program](../software-requirements/srd013-program.md) [AC4](../software-requirements/srd013-program.md#ac4), [AC7](../software-requirements/srd013-program.md#ac7) | S1 and S2 trace these acceptance criteria. |
| T5 | [srd036-implement](../software-requirements/srd036-implement.md) [AC1](../software-requirements/srd036-implement.md#ac1), [AC2](../software-requirements/srd036-implement.md#ac2), [AC3](../software-requirements/srd036-implement.md#ac3), [AC4](../software-requirements/srd036-implement.md#ac4) | S1, S2, and S3 trace these acceptance criteria. |
| T6 | [srd041-states](../software-requirements/srd041-states.md) [AC4](../software-requirements/srd041-states.md#ac4), [AC5](../software-requirements/srd041-states.md#ac5) | S1 and S2 trace these acceptance criteria. |
| T7 | [srd037-install](../software-requirements/srd037-install.md) [R6.3](../software-requirements/srd037-install.md#r63) | S3 traces the install into the worktree. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Pop assigns only the person who runs it and opens the issue's own branch and worktree; an issue someone else holds, a closed issue, or a data issue admission did not admit is refused.

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [R1.1](../software-requirements/srd036-implement.md#r11)
- [srd036-implement](../software-requirements/srd036-implement.md) [R2.1](../software-requirements/srd036-implement.md#r21)
- [srd036-implement](../software-requirements/srd036-implement.md) [R2.2](../software-requirements/srd036-implement.md#r22)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC1](../software-requirements/srd036-implement.md#ac1)
- [srd013-program](../software-requirements/srd013-program.md) [R7.1](../software-requirements/srd013-program.md#r71)
- [srd013-program](../software-requirements/srd013-program.md) [R7.2](../software-requirements/srd013-program.md#r72)
- [srd013-program](../software-requirements/srd013-program.md) [R7.3](../software-requirements/srd013-program.md#r73)
- [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7)
- [srd041-states](../software-requirements/srd041-states.md) [R9.1](../software-requirements/srd041-states.md#r91)
- [srd041-states](../software-requirements/srd041-states.md) [AC5](../software-requirements/srd041-states.md#ac5)

<!-- sc: S3 -->
### S3

The worktree pop opens carries the skills installed in the checkout, so the issue can be finished from there.

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [R2.3](../software-requirements/srd036-implement.md#r23)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC4](../software-requirements/srd036-implement.md#ac4)
- [srd013-program](../software-requirements/srd013-program.md) [R7.4](../software-requirements/srd013-program.md#r74)
- [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7)
- [srd037-install](../software-requirements/srd037-install.md) [R6.3](../software-requirements/srd037-install.md#r63)

<!-- sc: S2 -->
### S2

The merge tpm makes closes the issue; the skill closes nothing itself and asks nothing.

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [R3.1](../software-requirements/srd036-implement.md#r31)
- [srd036-implement](../software-requirements/srd036-implement.md) [R3.3](../software-requirements/srd036-implement.md#r33)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC2](../software-requirements/srd036-implement.md#ac2)
- [srd036-implement](../software-requirements/srd036-implement.md) [R4.1](../software-requirements/srd036-implement.md#r41)
- [srd036-implement](../software-requirements/srd036-implement.md) [R4.2](../software-requirements/srd036-implement.md#r42)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC3](../software-requirements/srd036-implement.md#ac3)
- [srd013-program](../software-requirements/srd013-program.md) [R5.1](../software-requirements/srd013-program.md#r51)
- [srd013-program](../software-requirements/srd013-program.md) [R5.2](../software-requirements/srd013-program.md#r52)
- [srd013-program](../software-requirements/srd013-program.md) [AC4](../software-requirements/srd013-program.md#ac4)
- [srd041-states](../software-requirements/srd041-states.md) [R7.1](../software-requirements/srd041-states.md#r71)
- [srd041-states](../software-requirements/srd041-states.md) [AC4](../software-requirements/srd041-states.md#ac4)

<!-- field: out_of_scope -->
## Out of scope

- Working a story's issues in one branch; that is implement at the story level (srd036 R5).
- The corpus gate on a docs/ change; that is release 00.2.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
