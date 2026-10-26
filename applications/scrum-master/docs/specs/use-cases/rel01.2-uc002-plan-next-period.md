---
id: rel01.2-uc002-plan-next-period
title: The Technical Program Manager Plans the Next Period's Sprint Goals
status: implemented
priority: P1
test_suite: test-rel01.2-sprint-end
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

Before the next period starts, the technical program manager runs
plan in the system repository. It reads the road-map, the agreed
initiatives, the system SRDs, and the Sprint Goals with their
reported status, and asks which initiative items the next release proves
and what its version and end date are. It records the answer as a
reviewed change to the road-map, sets the milestone, updates the sprint
issue's exit checklist, and posts planned events on Sprint Goals whose
release moved. tasks then files the next period's Sprint Goals.

<!-- field: actor -->
## Actor

The technical program manager.

<!-- field: trigger -->
## Trigger

The current period's sprint-end issues have closed.

<!-- field: preconditions -->
## Preconditions

- rel01.2-uc001-sprint-end-release closed the period.
- The system repository holds agreed initiatives (rel01.0-uc001-author-objectives).

<!-- field: flow -->
## Flow

- F1: The technical program manager files a plane::management planning issue with specify and pops it in the system repository before the period starts, then starts plan in its worktree.
- F2: tpm plan candidates reads docs/road-map.md, the SRDs, the agreed initiatives, the open Sprint Goals, and the period aggregate's slipped-commitment signal, and returns as JSON the candidate initiative items ranked slipped first, with the recommended version and end date.
- F3: The skill shows the candidates and relays the technical program manager's choice of items, version, and end date back to tpm.
- F4: tpm returns any chosen item whose acceptance criteria lack a test or an environment to its owner with a comment, instead of placing it.
- F5: tpm plan write puts the release entry into docs/road-map.md in the worktree: its initiative items in deliverables, and its milestone and tag in release.publication; the change merges through the inner loop.
- F6: Once it merges, tpm release gate and tpm release tag release the system repository behind mage audit with the tag release.publication names, so the plan is published at a tag.
- F7: tpm plan push creates or updates the milestone, named for the release with its ISO weeks in its description and its end date, and the sprint issue with one exit checklist item per Sprint Goal, posts a planned event on each Sprint Goal whose release changed, and files the period's sprint-end issues in each releasing repository.
- F8: The technical program manager runs tasks for the new release (rel01.0-uc002-push-objective-issues).

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd031-plan](../software-requirements/srd031-plan.md) [R1.1](../software-requirements/srd031-plan.md#r11), [R2.1](../software-requirements/srd031-plan.md#r21), [R2.2](../software-requirements/srd031-plan.md#r22), [R3.1](../software-requirements/srd031-plan.md#r31), [R3.2](../software-requirements/srd031-plan.md#r32), [R4.1](../software-requirements/srd031-plan.md#r41), [R4.2](../software-requirements/srd031-plan.md#r42), [R4.3](../software-requirements/srd031-plan.md#r43), [R4.4](../software-requirements/srd031-plan.md#r44), [R4.5](../software-requirements/srd031-plan.md#r45), [R4.6](../software-requirements/srd031-plan.md#r46), [R5.1](../software-requirements/srd031-plan.md#r51), [R5.2](../software-requirements/srd031-plan.md#r52) | S1 to S3 trace plan's reads, questions, pushes, and refusals. |
| T2 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.1](../software-requirements/srd034-initiatives.md#r31) | S1 traces planning from initiatives. |
| T3 | [srd001-labels](../software-requirements/srd001-labels.md) [R7.1](../software-requirements/srd001-labels.md#r71), [R7.2](../software-requirements/srd001-labels.md#r72), [R7.4](../software-requirements/srd001-labels.md#r74) | S4 traces the sprint milestone and sprint issue. |
| T4 | [srd031-plan](../software-requirements/srd031-plan.md) [AC1](../software-requirements/srd031-plan.md#ac1), [AC2](../software-requirements/srd031-plan.md#ac2), [AC3](../software-requirements/srd031-plan.md#ac3) | S1 to S3 trace these acceptance criteria. |
| T5 | [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3) | S1 traces this acceptance criterion. |
| T6 | [srd001-labels](../software-requirements/srd001-labels.md) [AC7](../software-requirements/srd001-labels.md#ac7) | S4 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

plan reads the road-map, SRDs, initiatives, open Sprint Goals, and the period's slipped commitments, and asks only the items, version, and end date with recommendations.

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [R1.1](../software-requirements/srd031-plan.md#r11)
- [srd031-plan](../software-requirements/srd031-plan.md) [R2.1](../software-requirements/srd031-plan.md#r21)
- [srd031-plan](../software-requirements/srd031-plan.md) [R2.2](../software-requirements/srd031-plan.md#r22)
- [srd031-plan](../software-requirements/srd031-plan.md) [R3.1](../software-requirements/srd031-plan.md#r31)
- [srd031-plan](../software-requirements/srd031-plan.md) [R3.2](../software-requirements/srd031-plan.md#r32)
- [srd031-plan](../software-requirements/srd031-plan.md) [AC1](../software-requirements/srd031-plan.md#ac1)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [R3.1](../software-requirements/srd034-initiatives.md#r31)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3)

<!-- sc: S2 -->
### S2

The release entry merges through the inner loop, the plan is tagged in the system repository, the milestone carries the end date, the period's sprint-end issues exist, moved Sprint Goals get planned events, and an item without test or environment is returned with a comment.

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [R4.1](../software-requirements/srd031-plan.md#r41)
- [srd031-plan](../software-requirements/srd031-plan.md) [R4.2](../software-requirements/srd031-plan.md#r42)
- [srd031-plan](../software-requirements/srd031-plan.md) [R4.3](../software-requirements/srd031-plan.md#r43)
- [srd031-plan](../software-requirements/srd031-plan.md) [R4.4](../software-requirements/srd031-plan.md#r44)
- [srd031-plan](../software-requirements/srd031-plan.md) [R4.5](../software-requirements/srd031-plan.md#r45)
- [srd031-plan](../software-requirements/srd031-plan.md) [R4.6](../software-requirements/srd031-plan.md#r46)
- [srd031-plan](../software-requirements/srd031-plan.md) [AC2](../software-requirements/srd031-plan.md#ac2)

<!-- sc: S3 -->
### S3

No item without test and environment is placed, no issue is assigned, and no work issue is created.

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [R5.1](../software-requirements/srd031-plan.md#r51)
- [srd031-plan](../software-requirements/srd031-plan.md) [R5.2](../software-requirements/srd031-plan.md#r52)
- [srd031-plan](../software-requirements/srd031-plan.md) [AC3](../software-requirements/srd031-plan.md#ac3)

<!-- sc: S4 -->
### S4

The sprint is one milestone and one sprint issue whose exit checklist lists the release's Sprint Goals; planning it again updates them and creates nothing new.

<!-- field: traces -->
#### Traces

- [srd001-labels](../software-requirements/srd001-labels.md) [R7.1](../software-requirements/srd001-labels.md#r71)
- [srd001-labels](../software-requirements/srd001-labels.md) [R7.2](../software-requirements/srd001-labels.md#r72)
- [srd001-labels](../software-requirements/srd001-labels.md) [R7.4](../software-requirements/srd001-labels.md#r74)
- [srd001-labels](../software-requirements/srd001-labels.md) [AC7](../software-requirements/srd001-labels.md#ac7)

<!-- field: out_of_scope -->
## Out of scope

- Reading met, proving, or slipped state and waiting dependencies from track-goals (srd031 R2.3, R2.4); stage one plans from reported status.
- Ticking or closing the sprint issue (srd001 R7.3).
- Writing new initiatives; that is rel01.0-uc001-author-objectives.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step.
