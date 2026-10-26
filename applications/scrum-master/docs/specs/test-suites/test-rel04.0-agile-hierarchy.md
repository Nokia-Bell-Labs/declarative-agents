---
id: test-rel04.0-agile-hierarchy
title: Test Suite for Release 04.0 The Agile Hierarchy
release: "04.0"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing
them. The gate stage authors a hierarchy in the system and a team
repository and proves the gate passes it and fails each malformed case. The
planning stage files a Sprint Goal and plans a sprint from ranked stories.
The sprint-end stage closes a Sprint Goal with its stories reported, and
reads an objective issue from before release 04.0 as a Sprint Goal.

<!-- field: traces -->
## Traces

- [rel04.0-uc001-author-the-hierarchy](../use-cases/rel04.0-uc001-author-the-hierarchy.md)
- [rel04.0-uc002-plan-a-sprint-from-stories](../use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md)
- [rel04.0-uc003-sprint-goal-ledger](../use-cases/rel04.0-uc003-sprint-goal-ledger.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>: a beads tracker repository, a system repository with a theme and an initiative, and one Product Owner repository with an epic, a feature, and ranked stories, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.

<!-- field: test_cases -->
## Test cases

<!-- case: The gate checks every hierarchy document | status: implemented -->
### The gate checks every hierarchy document

<!-- field: use_case -->
#### Use case

[rel04.0-uc001-author-the-hierarchy](../use-cases/rel04.0-uc001-author-the-hierarchy.md)

<!-- field: integration_target -->
#### Integration target

mage integration:hierarchyGate

<!-- field: go_test -->
#### Go test

go test ./internal/hierarchy -run '^Test(ValidHierarchyPasses|GateReportsEachMalformedDocument|ObjectiveDocumentIsAnInitiative)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- valid: a theme, initiative, epic, feature, and story pass
- malformed: each missing field, wrong parent kind, unresolved parent or link, cycle, story without points, and criterion without test or environment fails mage audit
- objective_document: read as an initiative and passes

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC2](../software-requirements/srd045-backlog.md#ac2)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC4](../software-requirements/srd045-backlog.md#ac4)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC6](../software-requirements/srd045-backlog.md#ac6)
- [srd007-links](../software-requirements/srd007-links.md) [AC7](../software-requirements/srd007-links.md#ac7)
- [srd007-links](../software-requirements/srd007-links.md) [AC8](../software-requirements/srd007-links.md#ac8)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- case: Roll-up counts done story points | status: implemented -->
### Roll-up counts done story points

<!-- field: use_case -->
#### Use case

[rel04.0-uc001-author-the-hierarchy](../use-cases/rel04.0-uc001-author-the-hierarchy.md)

<!-- field: integration_target -->
#### Integration target

mage integration:hierarchyGate

<!-- field: go_test -->
#### Go test

go test ./internal/hierarchy -run '^TestRollUpCountsDoneStoryPoints$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- epic_progress: 5 of 16 points for stories of 3, 5, and 8 with the 5 done
- unread: a story in a repository roll-up did not read is reported, not counted

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC3](../software-requirements/srd045-backlog.md#ac3)

<!-- case: tasks files one Sprint Goal per team | status: implemented -->
### tasks files one Sprint Goal per team

<!-- field: use_case -->
#### Use case

[rel04.0-uc002-plan-a-sprint-from-stories](../use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md)

<!-- field: integration_target -->
#### Integration target

mage integration:tasks

<!-- field: go_test -->
#### Go test

go test ./internal/goals -run '^TestSprintGoalPerTeam$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- issues: one Sprint Goal per team per release, with pinned Cites, an Environment line, empty Notes and Sprint Backlog sections
- planned_event: one per Sprint Goal
- second_run: no duplicate

<!-- field: traces -->
#### Traces

- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC2](../software-requirements/srd022-tasks.md#ac2)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3)

<!-- case: Sprint Planning lists stories on the Sprint Goal | status: implemented -->
### Sprint Planning lists stories on the Sprint Goal

<!-- field: use_case -->
#### Use case

[rel04.0-uc002-plan-a-sprint-from-stories](../use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md)

<!-- field: integration_target -->
#### Integration target

mage integration:sprintPlanning

<!-- field: go_test -->
#### Go test

go test ./internal/plan -run '^Test(BacklogRanksStoriesWithinVelocity|PushWritesSprintBacklogOnce)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- backlog: not-done agreed stories by rank with points
- recommendation: the stories in rank order whose points fit velocity
- sprint_backlog: the chosen stories with points on the Sprint Goal, and one planned event naming them
- second_push: no change

<!-- field: traces -->
#### Traces

- [srd031-plan](../software-requirements/srd031-plan.md) [AC4](../software-requirements/srd031-plan.md#ac4)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC5](../software-requirements/srd045-backlog.md#ac5)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11)
- [srd006-events](../software-requirements/srd006-events.md) [AC7](../software-requirements/srd006-events.md#ac7)

<!-- case: Link lines are read and linted | status: implemented -->
### Link lines are read and linted

<!-- field: use_case -->
#### Use case

[rel04.0-uc002-plan-a-sprint-from-stories](../use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md)

<!-- field: integration_target -->
#### Integration target

mage integration:program

<!-- field: go_test -->
#### Go test

go test ./internal/links ./internal/program -run '^Test(AgileLinkLines|BlocksAndBlockedByAreOneLink|LintFlagsMalformedLinks)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- lines: Blocked-by, Blocks, Depends-on, Relates-to, Duplicates, and Implements parsed with their targets
- pair: a Blocks line and the matching Blocked-by line are one link
- lint: a malformed or unpinned target and a second Duplicates or Implements line are flagged

<!-- field: traces -->
#### Traces

- [srd007-links](../software-requirements/srd007-links.md) [AC6](../software-requirements/srd007-links.md#ac6)
- [srd007-links](../software-requirements/srd007-links.md) [AC8](../software-requirements/srd007-links.md#ac8)

<!-- case: A Sprint Goal closes at sprint end with its stories reported | status: implemented -->
### A Sprint Goal closes at sprint end with its stories reported

<!-- field: use_case -->
#### Use case

[rel04.0-uc003-sprint-goal-ledger](../use-cases/rel04.0-uc003-sprint-goal-ledger.md)

<!-- field: integration_target -->
#### Integration target

mage integration:sprintEnd

<!-- field: go_test -->
#### Go test

go test ./internal/sprintend -run '^Test(ReportedCompleteClosesWithoutMet|SprintEndRecordsStoriesDone)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- closed: the Sprint Goals reported complete, by the sprint-end merge, with no met event
- stories_done: recorded in the summary and counted toward velocity

<!-- field: traces -->
#### Traces

- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [AC2](../software-requirements/srd038-sprint-end.md#ac2)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC3](../software-requirements/srd045-backlog.md#ac3)

<!-- case: An objective issue is read as a Sprint Goal | status: implemented -->
### An objective issue is read as a Sprint Goal

<!-- field: use_case -->
#### Use case

[rel04.0-uc003-sprint-goal-ledger](../use-cases/rel04.0-uc003-sprint-goal-ledger.md)

<!-- field: integration_target -->
#### Integration target

mage integration:sprintEnd

<!-- field: go_test -->
#### Go test

go test ./internal/beads ./internal/goals ./internal/sprintgoal -run '^Test(ObjectiveIssueIsSprintGoal|ObjectiveMarkerIsRead)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- marker: the objective marker is read as a Sprint Goal's
- ledger: its events replay to the same state
- states: its srd041 rows are the Sprint Goal's

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC11](../software-requirements/srd034-initiatives.md#ac11)
- [srd006-events](../software-requirements/srd006-events.md) [AC10](../software-requirements/srd006-events.md#ac10)
