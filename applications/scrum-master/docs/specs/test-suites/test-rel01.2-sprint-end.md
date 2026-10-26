---
id: test-rel01.2-sprint-end
title: Test Suite for Release 01.2 Sprint End Closes the Loop
release: "01.2"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them.
The sprint-end stage works a sprint-end issue in the rig to a
confirmed release behind a passing gate, and proves reported-complete
Sprint Goals close without a met event. The planning stage runs plan
for the next period. The tag-triggered stages are release 05.0's
(test-rel05.0-tag-triggered).

<!-- field: traces -->
## Traces

- [rel01.2-uc001-sprint-end-release](../use-cases/rel01.2-uc001-sprint-end-release.md)
- [rel01.2-uc002-plan-next-period](../use-cases/rel01.2-uc002-plan-next-period.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository, a reports repository, and one Product Owner repository, with a period of fixture reports and Sprint Goals, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.
- The system repository is the documents-only fixture: policy declares its corpus check as the gate of the corpus artifact (in the rig, the rig audit), and the grader gate of a Product Owner repository.
- The tag pipeline reaches the rig's tracker repository.

<!-- field: test_cases -->
## Test cases

<!-- case: The sprint-end issue summarizes, asks, and releases on confirmation | status: implemented -->
### The sprint-end issue summarizes, asks, and releases on confirmation

<!-- field: use_case -->
#### Use case

[rel01.2-uc001-sprint-end-release](../use-cases/rel01.2-uc001-sprint-end-release.md)

<!-- field: integration_target -->
#### Integration target

mage integration:sprintEnd

<!-- field: go_test -->
#### Go test

go test ./internal/sprintend -run '^TestSprintEndReleasesOnlyAfterConfirmation$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- due_date: the milestone end date
- questions: what to change, then confirmation of the release and its version
- summary: committed at docs/sprint-ends/RELEASE.md by the merge that closes the issue
- declined: issue closed by that merge with no tag
- confirmed: tag created on main at the confirmed version, after the merge

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC1](../software-requirements/srd038-sprint-end.md#ac1)

<!-- case: Tags follow a passing gate | status: implemented -->
### Tags follow a passing gate

<!-- field: use_case -->
#### Use case

[rel01.2-uc001-sprint-end-release](../use-cases/rel01.2-uc001-sprint-end-release.md)

<!-- field: integration_target -->
#### Integration target

mage integration:release

<!-- field: go_test -->
#### Go test

go test ./internal/release -run '^Test(GateRunsDeclaredChecks|TagRequiresPassingGateAndIncreasingVersion|DocumentsOnlyGateIsMageAudit|GatePeerReadNeedsAnotherPersonsThumbsUp|GateFailsChecksItCannotRun)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- gate_checks: exactly the declared checks, each posted as an evidence event
- failing_gate: no tag
- tag_after_gate: tpm release tag creates and pushes the tag at the gated revision, with its class in the message and on the release issue
- non_increasing_version: refused
- not_runnable: a contract-tests or scorecard check, or an environment other than local, fails the gate
- peer_read: pending until a thumbs-up or approval comment from someone other than the release issue's author
- documents_only_gate: mage audit

<!-- field: traces -->
#### Traces

- [srd015-release](../software-requirements/srd015-release.md) [AC1](../software-requirements/srd015-release.md#ac1)
- [srd015-release](../software-requirements/srd015-release.md) [AC2](../software-requirements/srd015-release.md#ac2)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- case: Reported-complete Sprint Goals close without a met event | status: implemented -->
### Reported-complete Sprint Goals close without a met event

<!-- field: use_case -->
#### Use case

[rel01.2-uc001-sprint-end-release](../use-cases/rel01.2-uc001-sprint-end-release.md)

<!-- field: integration_target -->
#### Integration target

mage integration:sprintEnd

<!-- field: go_test -->
#### Go test

go test ./internal/sprintend -run '^TestReportedCompleteClosesWithoutMet$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- closed_sprint_goals: the ones reported complete, by the sprint-end merge
- met_events: none
- rollup_state: reported complete

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC2](../software-requirements/srd038-sprint-end.md#ac2)

<!-- case: plan records the next period | status: implemented -->
### plan records the next period

<!-- field: use_case -->
#### Use case

[rel01.2-uc002-plan-next-period](../use-cases/rel01.2-uc002-plan-next-period.md)

<!-- field: integration_target -->
#### Integration target

mage integration:plan

<!-- field: go_test -->
#### Go test

go test ./internal/plan -run '^Test(PlanReleaseRanksSlippedFirst|PlanReleaseWritesTheRoadMapRelease|PlanReleaseReturnsItemsWithoutEvidence)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- questions: initiative items, version, and end date, each with a recommendation
- road_map_change: deliverables and release.publication written, filed, and merged through the inner loop
- plan_tag: the system repository tagged behind mage audit with the tag release.publication names
- milestone: named for the release, end date set
- planned_events: one per Sprint Goal whose release moved
- item_without_test: returned with a comment, not placed
- assignees_and_work_issues: none

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [AC1](../software-requirements/srd031-plan.md#ac1)
- [srd031-plan](../software-requirements/srd031-plan.md) [AC2](../software-requirements/srd031-plan.md#ac2)
- [srd031-plan](../software-requirements/srd031-plan.md) [AC3](../software-requirements/srd031-plan.md#ac3)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3)

<!-- case: The sprint milestone and sprint issue are updated, not duplicated | status: implemented -->
### The sprint milestone and sprint issue are updated, not duplicated

<!-- field: use_case -->
#### Use case

[rel01.2-uc002-plan-next-period](../use-cases/rel01.2-uc002-plan-next-period.md)

<!-- field: integration_target -->
#### Integration target

mage integration:plan

<!-- field: go_test -->
#### Go test

go test ./internal/plan -run '^Test(PushSprintUpdatesMilestoneAndChecklist|PlanPushFilesSprintEndIssuesOnce)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- sprint_issue: one per milestone, checklist item per Sprint Goal
- sprint_end_issues: one per releasing repository, due on the milestone end date
- second_push: dates and checklist updated; nothing created

<!-- field: traces -->
#### Traces

- [srd001-labels](../software-requirements/srd001-labels.md) [AC7](../software-requirements/srd001-labels.md#ac7)
