---
id: srd036-implement
title: "Skill: implement"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

An issue that is worked on the default branch, or in a checkout shared
with other work, mixes changes nobody can review apart, and closing it by
hand says nothing about what changed. The process has one path from an
issue to a change: the person who works it takes it, works on its own
branch in its own worktree, and the merge that lands the branch
closes it.

implement is that path. scrum-master claims the issue for the caller and opens its
branch and worktree (srd013 R7); the work in the worktree is the person's
or their agent's; scrum-master merges the branch, which closes the issue
(srd013 R5). A change under docs/, a design change or a planning change,
passes the corpus gate before it merges (srd034 R2.2).

<!-- field: goals -->
## Goals

- G1: Every issue is worked on its own branch in its own worktree, by the person who took it.
- G2: Only the merge of its branch closes the issue.
- G3: A change under docs/ merges only through the corpus gate, whatever the issue's plane or kind.

<!-- req: R1 -->
## R1 - Trigger

<!-- req: R1.1 -->
### R1.1

Started by the person who will work an open issue, in the issue's repository.

<!-- req: R2 -->
## R2 - Take

<!-- req: R2.1 -->
### R2.1

Runs scrum-master issue pop (srd013 R7), which assigns only the caller and creates or reuses the branch <iid>-<slug> and its worktree. A refusal is shown with its reason and the skill stops.

<!-- req: R2.2 -->
### R2.2

Every change for the issue is made in that worktree.

<!-- req: R2.3 -->
### R2.3

The worktree carries the skills of every agent installed in the checkout pop ran from (srd037), so a person or agent working there can finish the issue with implement. Pop reports as installed only the agents whose files it wrote, none on a worktree that already had them.

<!-- req: R3 -->
## R3 - Finish

<!-- req: R3.1 -->
### R3.1

When the work is committed and pushed, runs scrum-master issue merge (srd013 R5), which runs the corpus gate for a docs/ change, merges the branch, and closes the issue. When the program's policy requires approval, the skill first asks a reviewer to approve the tip with scrum-master issue approve, and asks again when merge refuses an approval of an earlier commit.

<!-- req: R3.2 -->
### R3.2

For any issue whose branch changes docs/, a kind::spec design change or a plane::management planning change alike, runs mage audit before merging and reports its findings instead of merging when it fails.

<!-- req: R3.3 -->
### R3.3

The issue closes when its branch merges (srd041 R7.1); the skill closes nothing itself.

<!-- req: R4 -->
## R4 - Asks and refusals

<!-- req: R4.1 -->
### R4.1

Asks nothing: scrum-master decides whether the issue can be taken, and the issue body holds the steps. A question an issue's own Requirements ask, such as a sprint-end issue's confirmation (srd038 R1.3), belongs to the work, not to pop.

<!-- req: R4.2 -->
### R4.2

As a skill, is at most 50 lines and has no model step; the work in the worktree is the person's or their agent's, not the skill's (design.md D10).

<!-- req: R5 -->
## R5 - A story's issues in one branch

<!-- req: R5.1 -->
### R5.1

scrum-master issue list --implements lists the open issues whose Implements line names a story, or with --epic every story under an epic in the hierarchy, in the stories' rank order and then by IID, and lists apart an issue a Blocked-by line holds behind an open issue.

<!-- req: R5.2 -->
### R5.2

scrum-master issue pop --worktree claims an issue into a worktree an earlier pop opened, on that worktree's branch, with pop's refusals and assignment and without opening a branch or worktree of its own.

<!-- req: R5.3 -->
### R5.3

Given a story, implement pops its first listed issue, claims each further one into the same worktree, builds them in order, each committed separately with its record, and finishes with one merge of that branch whose --closes names every other claimed issue, so the one merge closes them all.

<!-- req: R5.4 -->
### R5.4

Works one story by default, and a whole epic's stories only when the person asks: one story keeps the merge a size one reviewer reads.

<!-- req: R6 -->
## R6 - The workflow as a profile

<!-- req: R6.1 -->
### R6.1

The workflow is a declarative-agents profile, plugin/agents/implement/, whose machine's states are the skill's steps and whose tools are the scrum-master commands the skill runs, each with --json; the model acts only in the Work state (design.md D10, D14). mage audit validates every shipped profile with the pinned runtime's --validate-config.

<!-- req: R6.2 -->
### R6.2

scrum-master install renders the implement SKILL.md from the profile's machine (srd037 R1.4), so an agent that does not run the runtime follows the same steps the runtime executes.

<!-- req: R6.3 -->
### R6.3

A scrum-master refusal ends a run in the Refused terminal state with scrum-master's reason as the run's summary, and pop reports the issue's body, so the Work state holds the Requirements and Acceptance Criteria without a tracker round-trip.

<!-- field: non_goals -->
## Non-goals

- Re-creating a recurring issue after its run; that is release 01.1 (scrum-master issue recur).
- Approving; a reviewer reads the branch and approves it (srd013 R5.6).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Popping an open, admitted issue assigns only the caller and returns its branch and worktree; a refusal stops the skill with its reason.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC2 -->
### AC2

Finishing merges the branch once, which closes the issue; an issue whose branch changes docs/ merges it only when mage audit passes; the skill closes nothing itself.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC3 -->
### AC3

The skill asks nothing, is at most 50 lines, and has no model step.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)

<!-- ac: AC4 -->
### AC4

A worktree pop creates from a checkout with installed skills holds the same agents' skills, stamped and ignored by git; a checkout with none installs none.

<!-- field: traces -->
#### Traces

- [R2.3](#r23)

<!-- ac: AC5 -->
### AC5

A story's open issues list in order with a blocked one apart; the second is claimed into the first's worktree on its branch with no branch opened, an issue someone else holds is refused, and one merge closes every claimed issue.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)
- [R5.4](#r54)

<!-- ac: AC6 -->
### AC6

A run of the profile with the pinned runtime over an admitted issue pops, works, pushes, merges, and closes it; a pop the admission refuses ends the run in Refused with scrum-master's reason; the installed implement SKILL.md presents the machine's states as its steps.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)
