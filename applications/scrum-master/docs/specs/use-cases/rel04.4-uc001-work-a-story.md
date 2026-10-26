---
id: rel04.4-uc001-work-a-story
title: A Developer Works a Story End to End in One Worktree and One Merge
status: implemented
priority: P1
test_suite: test-rel04.4-story-loop
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A story chosen at Sprint Planning becomes the issues it needs and is
delivered as one change. tasks proposes and files the issues, each with
an Implements line naming the story. implement lists them in order, pops
the first, claims each further one into the same worktree and branch,
builds them one commit each, and finishes with one merge whose --closes
names every other claimed issue, so the merge closes them all. An issue
waiting on open work is listed apart and stays open.

<!-- field: actor -->
## Actor

A Developer, driving an agent through tasks and implement.

<!-- field: trigger -->
## Trigger

A story is on the team's Sprint Goal and the Developer starts it.

<!-- field: preconditions -->
## Preconditions

- The story is a document the hierarchy gate passes, selected at Sprint Planning onto the team's Sprint Goal.
- A local checkout of the team's repository with its skills installed.

<!-- field: flow -->
## Flow

- F1: tasks reads the story and the issues already implementing it, proposes the issues it still needs in build order, and files each agreed one with an Implements line naming the story.
- F2: tpm issue list --implements lists the story's open issues in order, and an issue blocked by open work apart.
- F3: implement pops the first issue, opening its branch and worktree, and claims each further one into that worktree with tpm issue pop --worktree.
- F4: implement builds the issues in order, one commit each with its record.
- F5: tpm issue merge merges the branch once, with --closes naming every other claimed issue, and closes them all on that merge.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd022-tasks](../software-requirements/srd022-tasks.md) [R6.1](../software-requirements/srd022-tasks.md#r61), [R6.2](../software-requirements/srd022-tasks.md#r62), [R6.3](../software-requirements/srd022-tasks.md#r63) | S1 traces tasks at the story level. |
| T2 | [srd036-implement](../software-requirements/srd036-implement.md) [R5.1](../software-requirements/srd036-implement.md#r51), [R5.2](../software-requirements/srd036-implement.md#r52), [R5.3](../software-requirements/srd036-implement.md#r53), [R5.4](../software-requirements/srd036-implement.md#r54) | S2 traces the list, the claim into one worktree, and the one merge. |
| T3 | [srd022-tasks](../software-requirements/srd022-tasks.md) [AC4](../software-requirements/srd022-tasks.md#ac4) | S1 traces this acceptance criterion. |
| T4 | [srd036-implement](../software-requirements/srd036-implement.md) [AC5](../software-requirements/srd036-implement.md#ac5) | S2 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The story's issues are filed once, each naming the story in an Implements line, in build order.

<!-- field: traces -->
#### Traces

- [srd022-tasks](../software-requirements/srd022-tasks.md) [R6.1](../software-requirements/srd022-tasks.md#r61)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R6.2](../software-requirements/srd022-tasks.md#r62)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [R6.3](../software-requirements/srd022-tasks.md#r63)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC4](../software-requirements/srd022-tasks.md#ac4)

<!-- sc: S2 -->
### S2

The story's open issues list in order with a blocked one apart; the second is claimed into the first's worktree on its branch with no branch opened; an issue someone else holds is refused; and one merge closes every claimed issue while the blocked one stays open.

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [R5.1](../software-requirements/srd036-implement.md#r51)
- [srd036-implement](../software-requirements/srd036-implement.md) [R5.2](../software-requirements/srd036-implement.md#r52)
- [srd036-implement](../software-requirements/srd036-implement.md) [R5.3](../software-requirements/srd036-implement.md#r53)
- [srd036-implement](../software-requirements/srd036-implement.md) [R5.4](../software-requirements/srd036-implement.md#r54)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC5](../software-requirements/srd036-implement.md#ac5)

<!-- field: out_of_scope -->
## Out of scope

- Selecting the story; that is Sprint Planning (tpm plan backlog).
- Evidence on the story's criteria and met; those are track-goals (06.0).
- A whole epic in one run, which implement does only when the person asks.

<!-- field: assumptions -->
## Assumptions

- A1: One story is a change one reviewer reads in one sitting, so it merges once.
- A2: Every tpm step spends no model tokens; the model drafts the issues with the Developer and writes the code (design.md D10).
