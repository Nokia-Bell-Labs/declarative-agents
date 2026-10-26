---
id: rel03.0-uc001-pop-and-close-on-beads
title: An Issue Is Popped, Delivered, and Closed Against Beads
status: implemented
priority: P2
test_suite: test-rel03.0-beads-tracker
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A person pops a ready issue from the beads tracker, works it in a
worktree branch, and merges locally; the pop skill observes the merge,
closes the issue with the merge commit cited as the closing comment, and
pushes the write to the tracker repository. Every tpm command in the
loop behaves exactly as it does over GitLab, because both backends serve
one tracker boundary; the one difference is what delivery means: a local
merge whose commit cites the issue, in place of a merge request.

<!-- field: actor -->
## Actor

A person in the program, on a clone whose configuration names the beads
backend and the tracker repository.

<!-- field: trigger -->
## Trigger

The person runs the implement skill on a ready issue.

<!-- field: preconditions -->
## Preconditions

- The program's configuration document names the beads backend and the tracker repository's URL.
- The tracker repository holds a ready issue: open, unassigned, admitted, plane data, backlog kind (srd041 R3.1).

<!-- field: flow -->
## Flow

- F1: The pop synchronizes from the tracker repository, reads the ready issue, and assigns the person (srd041 R9.1); the write reaches the tracker repository before the pop reports.
- F2: The person works the issue in a worktree branch named for it.
- F3: The person merges the branch to main locally; the merge commit's message cites the issue id.
- F4: The pop skill's close step observes the merge, closes the issue, and records the merge commit hash as the closing comment.
- F5: The close is pushed to the tracker repository; a second close on the same issue changes nothing.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.1](../software-requirements/srd042-beads-tracker.md#r11), [R1.2](../software-requirements/srd042-beads-tracker.md#r12) | S1 traces the unchanged boundary and state machine. |
| T2 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R2.1](../software-requirements/srd042-beads-tracker.md#r21), [R2.3](../software-requirements/srd042-beads-tracker.md#r23) | S2 traces the mapping and the closing merge commit. |
| T3 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.2](../software-requirements/srd042-beads-tracker.md#r32) | S3 traces push after write. |
| T4 | [srd041-states](../software-requirements/srd041-states.md) [R9.1](../software-requirements/srd041-states.md#r91), [R9.3](../software-requirements/srd041-states.md#r93) | S1 traces the pop and close invariants. |
| T5 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC1](../software-requirements/srd042-beads-tracker.md#ac1) | S1 traces this acceptance criterion. |
| T6 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC2](../software-requirements/srd042-beads-tracker.md#ac2) | S2 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The pop assigns only the person who runs it, the issue closes only on the observed merge, and every transition replays the srd041 table unchanged on beads.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.2](../software-requirements/srd042-beads-tracker.md#r12)
- [srd041-states](../software-requirements/srd041-states.md) [R9.1](../software-requirements/srd041-states.md#r91)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC1](../software-requirements/srd042-beads-tracker.md#ac1)

<!-- sc: S2 -->
### S2

The closed issue carries the merge commit hash as its closing comment, and its title, labels, assignee, and comments read back exactly as written.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R2.3](../software-requirements/srd042-beads-tracker.md#r23)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC2](../software-requirements/srd042-beads-tracker.md#ac2)

<!-- sc: S3 -->
### S3

Every write lands in the tracker repository before the command that made it reports success, and a repeated close changes nothing.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.2](../software-requirements/srd042-beads-tracker.md#r32)

<!-- field: out_of_scope -->
## Out of scope

- The synchronization protocol's conflict rules; rel03.0-uc002 exercises them.
- Migrating existing GitLab issues; a program adopts beads at a release boundary (srd042 non-goals).

<!-- field: assumptions -->
## Assumptions

- A1: Every step is a tpm subcommand or the inner loop, and spends no model tokens (design.md D10). This use case has no model step.
