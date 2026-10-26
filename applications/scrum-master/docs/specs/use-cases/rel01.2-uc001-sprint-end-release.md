---
id: rel01.2-uc001-sprint-end-release
title: A Sprint-End Issue Summarizes the Period, Asks, and Releases
status: implemented
priority: P1
test_suite: test-rel01.2-sprint-end
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

At the end of each period a sprint-end issue waits in every repository that
releases, due on the milestone end date. The owner pops it; its steps
summarize the period from the reports and the Sprint Goals, ask the
owner what should change, apply the answer, and push the release once the
owner confirms. Sprint Goals the owner reports complete close with that
merge as reported complete, never as met, because track-goals is
not running yet. A documents-only repository's gate is the corpus audit.

<!-- field: actor -->
## Actor

The owner of a releasing repository (a Product Owner, or the technical program manager in the system repository).

<!-- field: trigger -->
## Trigger

The tracker's due-date notification for the period's sprint-end issue.

<!-- field: preconditions -->
## Preconditions

- The period's weekly reports merged (rel01.1-uc001-weekly-status).
- policy declares the repository's release gate checks; a documents-only repository declares mage audit.

<!-- field: flow -->
## Flow

- F1: tpm plan push filed the sprint-end issue when the period was planned, with a Recurrence section and a due date on the milestone end date; in the system repository it references the sprint issue and leaves its checklist to the program lead.
- F2: The owner runs implement on it.
- F3: tpm sprint-end facts returns as JSON the period's aggregate, each Sprint Goal's ledger and reported status, and the version tpm release next recommends.
- F4: Model step (combine): the model drafts the period summary from the period's reports and tpm's output.
- F5: Model step (iterate): the model revises the summary with the owner and applies, in the worktree, the changes the owner names to Sprint Goals or the release.
- F6: The summary is committed at docs/sprint-ends/RELEASE.md, and tpm issue merge merges it with --closes for each Sprint Goal the owner reported complete; the merge closes the sprint-end issue and those Sprint Goals.
- F7: The issue's Requirements ask the owner to confirm the release and its version; with no confirmation, or nothing to release, the merge alone closes the issue and no tag is made.
- F8: Once the merge lands, tpm release gate runs the declared checks on main, the default gate unless the repository releases a declared artifact, with the sprint-end issue as the release issue; tpm release tag creates the tag at the confirmed version only after the gate passes, and a version that does not increase is refused.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R2.3](../software-requirements/srd034-initiatives.md#r23) | S1, S3, and S4 trace the sprint-end issue, the documents-only gate, and the stage-one close. |
| T2 | [srd015-release](../software-requirements/srd015-release.md) [R1.1](../software-requirements/srd015-release.md#r11), [R1.3](../software-requirements/srd015-release.md#r13), [R2.1](../software-requirements/srd015-release.md#r21), [R2.2](../software-requirements/srd015-release.md#r22), [R2.3](../software-requirements/srd015-release.md#r23) | S2 traces the gate and tagging. |
| T3 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2) | S1, S3, and S4 trace these acceptance criteria. |
| T4 | [srd015-release](../software-requirements/srd015-release.md) [AC1](../software-requirements/srd015-release.md#ac1), [AC2](../software-requirements/srd015-release.md#ac2) | S2 traces these acceptance criteria. |
| T5 | [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R1.1](../software-requirements/srd038-sprint-end.md#r11), [R1.2](../software-requirements/srd038-sprint-end.md#r12), [R1.3](../software-requirements/srd038-sprint-end.md#r13), [R2.1](../software-requirements/srd038-sprint-end.md#r21), [R2.2](../software-requirements/srd038-sprint-end.md#r22), [R2.3](../software-requirements/srd038-sprint-end.md#r23) | S1, S3, and S4 trace the sprint-end issue, the documents-only gate, and the stage-one close. |
| T6 | [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC1](../software-requirements/srd038-sprint-end.md#ac1), [AC2](../software-requirements/srd038-sprint-end.md#ac2) | S1, S3, and S4 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Each releasing repository has one sprint-end issue per period due on the milestone end date; worked, it summarizes, asks, and releases only after the owner confirms.

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R1.1](../software-requirements/srd038-sprint-end.md#r11)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R1.2](../software-requirements/srd038-sprint-end.md#r12)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R1.3](../software-requirements/srd038-sprint-end.md#r13)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC1](../software-requirements/srd038-sprint-end.md#ac1)

<!-- sc: S2 -->
### S2

A tag exists only after the declared gate passed for the same revision, with a version greater than the last and its class recorded.

<!-- field: traces -->
#### Traces

- [srd015-release](../software-requirements/srd015-release.md) [R1.1](../software-requirements/srd015-release.md#r11)
- [srd015-release](../software-requirements/srd015-release.md) [R1.3](../software-requirements/srd015-release.md#r13)
- [srd015-release](../software-requirements/srd015-release.md) [AC1](../software-requirements/srd015-release.md#ac1)
- [srd015-release](../software-requirements/srd015-release.md) [R2.1](../software-requirements/srd015-release.md#r21)
- [srd015-release](../software-requirements/srd015-release.md) [R2.2](../software-requirements/srd015-release.md#r22)
- [srd015-release](../software-requirements/srd015-release.md) [R2.3](../software-requirements/srd015-release.md#r23)
- [srd015-release](../software-requirements/srd015-release.md) [AC2](../software-requirements/srd015-release.md#ac2)

<!-- sc: S3 -->
### S3

A documents-only repository releases only a corpus that mage audit passes.

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R2.3](../software-requirements/srd034-initiatives.md#r23)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- sc: S4 -->
### S4

A Sprint Goal reported complete closes by the sprint-end merge with no met event and is shown as reported complete; once track-goals runs, Sprint Goals close only on met.

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.1](../software-requirements/srd038-sprint-end.md#r21)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.2](../software-requirements/srd038-sprint-end.md#r22)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R2.3](../software-requirements/srd038-sprint-end.md#r23)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC2](../software-requirements/srd038-sprint-end.md#ac2)

<!-- field: out_of_scope -->
## Out of scope

- Ticking the sprint issue's exit checklist; the program lead does that (srd001 R7.3).
- Routing the release to consumers; rel05.0-uc001-reference-update covers the slice stage one needs.
- The sprint review and retrospective as skills (srd032, srd033).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
