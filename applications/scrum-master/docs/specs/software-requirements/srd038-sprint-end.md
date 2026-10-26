---
id: srd038-sprint-end
title: "Skill: sprint-end"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

At a period's end each releasing repository has to say what the period
produced, let its owner change what should change, and release only what
the owner confirms. Until track-goals runs, a Sprint Goal the owner reports
complete also has to close somewhere without passing for evidence, and the
stories it listed have to be reported done or not, because velocity counts
them. A sprint-end issue does all three: scrum-master files it when the period is
planned, its steps run through the inner loop, and its merge closes it and
the Sprint Goals the owner reports complete.

<!-- field: goals -->
## Goals

- G1: Each releasing repository ends each period with one sprint-end issue that summarizes the period, asks the owner, and releases only on confirmation, after its summary merges.
- G2: Before track-goals runs, a Sprint Goal reported complete closes by the sprint-end merge as reported complete, never as met, and each story it listed is reported done or not.

<!-- req: R1 -->
## R1 - Sprint-end issue

<!-- req: R1.1 -->
### R1.1

A sprint-end issue is a recurring audit filed once per period in each repository that releases, by scrum-master plan push when the period is planned (srd031 R4.6), due on the milestone end date. In the system repository it references the sprint issue (srd001 R7) and leaves its checklist to the program lead.

<!-- req: R1.2 -->
### R1.2

Popped, its Requirements direct the work in order: summarize the period from scrum-master sprint-end facts, which returns the period's aggregate (srd018 R3), the team's own in a team repository and the program's in the system repository (GH-116), each Sprint Goal's ledger, reported status, and listed stories (in the system repository, the Sprint Goals of every team repository the subsystem SRDs declare), and the version scrum-master release next recommends from the checkout's commits since its latest tag, with its reason (srd015 R3.1); ask the owner whether anything in the summary, the Sprint Goals, or the release should change, and which listed stories are done; apply the changes the owner names in the worktree; commit the summary at docs/sprint-ends/RELEASE.md; merge with scrum-master issue merge, listing each Sprint Goal the owner reports complete with Closes (R2.1); once it merges, push the release from main with scrum-master release gate and scrum-master release tag, the sprint-end issue being the release issue.

<!-- req: R1.3 -->
### R1.3

The owner's confirmation of the release and its version is asked before the release is pushed and never assumed. The release is gated and tagged on main after the sprint-end merge lands, so the tag names a merged revision; the gate is the policy's default definition_of_done unless the repository releases an artifact it declares (srd008 R2). A repository with nothing to release closes the sprint-end issue by that merge alone, with the summary committed and no tag (a merge close, srd041 R9.3).

<!-- req: R1.4 -->
### R1.4

In the system repository, the summary at docs/sprint-ends/RELEASE.md follows the outline-section summary format the reports repository's deck template reads, one section per slide, so the release profile's step after the system tag renders the deck from it (srd018 R4.1, R10.1).

<!-- req: R2 -->
## R2 - Closing a Sprint Goal before track-goals runs

<!-- req: R2.1 -->
### R2.1

In a program that does not yet run track-goals, a Sprint Goal issue (or an objective issue, read as one) closes by the merge of its repository's sprint-end issue when the owner reports it complete; that merge lists it with --closes. This is a merge close (srd041 R9.3).

<!-- req: R2.2 -->
### R2.2

Such a close posts no met event; the merge posts a reported_complete event naming the release and the merge commit on the Sprint Goal's ledger instead, and scrum-master sprint-end facts reads it as reported complete, its outcome field naming which holds: met, reported complete, or not complete. A Sprint Goal another sprint-end merge already closed, as a team's and the program's merges may both name, is reported and skipped, not refused, and is still listed as reported complete: its ledger keeps the event the first merge posted, or gets one now when it has neither that nor a met event. The roll-up states the Sprint Goal as reported complete, distinct from met (srd018 R3.5), so a reported close is never shown as evidence, and the aggregate does not count it as a contradiction of the ledger (srd018 R3.2).

<!-- req: R2.3 -->
### R2.3

Once track-goals runs in the program, R2.1 no longer applies and Sprint Goals close on met (srd019).

<!-- req: R2.4 -->
### R2.4

The sprint-end summary records, for each Sprint Goal it closes, which of the stories listed in its Sprint Backlog the owner reports done, in a Stories done section scrum-master sprint-end stories writes, each story as project#story, since story ids repeat across teams. The summary is in the format design.md sprint_end_summary declares, and scrum-master issue merge refuses one that is not. Until track-goals runs, those stories are done for roll-up and velocity (srd045 R3.2, R5.3), and the rest return to the Product Backlog.

<!-- req: R3 -->
## R3 - The workflow as a profile

<!-- req: R3.1 -->
### R3.1

The workflow is a declarative-agents profile, plugin/agents/sprint-end/ (design.md D14): the machine's states carry the skill's steps, the scrum-master commands are model-dispatched tools with typed parameters, and the model acts only in the Summarize state (design.md D10). mage audit validates the profile with the pinned runtime's --validate-config, and scrum-master install renders the skill's SKILL.md from the machine (srd037 R1.4), so an agent that does not run the runtime follows the same steps.

<!-- field: non_goals -->
## Non-goals

- Closing Sprint Goals on evidence; that is track-goals (srd019).
- Planning the next period; that is plan (srd031).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Each releasing repository has one sprint-end issue per period, filed by scrum-master plan push and due on the milestone end date; worked, it summarizes the period, asks the owner what to change, and closes by the merge that commits the summary; the release is gated and tagged on main only after that merge and the owner's confirmation, or not at all; in the system repository the summary's outline is the deck's.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

Without track-goals, a reported-complete Sprint Goal closes by the sprint-end merge, carries no met event, appears in the roll-up and in the facts' outcome field as reported complete, not met, and raises no contradiction signal; the summary records which listed stories are done, and the others return to the Product Backlog; with track-goals it closes only on met.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)

<!-- ac: AC3 -->
### AC3

The sprint-end profile validates with the pinned runtime, and the installed SKILL.md presents its machine's states as its steps.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
