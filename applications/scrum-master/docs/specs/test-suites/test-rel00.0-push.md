---
id: test-rel00.0-push
title: Test Suite for Release 00.0 Labels and Push
release: "00.0"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them. The
suite creates the labels twice and proves the second run changes nothing,
then pushes a request and proves it becomes one admitted, unassigned data
issue.

<!-- field: traces -->
## Traces

- [rel00.0-uc001-push-project-work](../use-cases/rel00.0-uc001-push-project-work.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository and one Product Owner repository, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.

<!-- field: test_cases -->
## Test cases

<!-- case: Labels are created once | status: implemented -->
### Labels are created once

<!-- field: use_case -->
#### Use case

[rel00.0-uc001-push-project-work](../use-cases/rel00.0-uc001-push-project-work.md)

<!-- field: integration_target -->
#### Integration target

mage integration:labels

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestLabelsCreatesOnceAndNeverDeletes$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- first_run: every plane, type, kind (including spec), and state label created
- second_run: nothing changed
- existing_extra_label: kept

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC6](../software-requirements/srd013-program.md#ac6)

<!-- case: Push files an admitted data issue | status: implemented -->
### Push files an admitted data issue

<!-- field: use_case -->
#### Use case

[rel00.0-uc001-push-project-work](../use-cases/rel00.0-uc001-push-project-work.md)

<!-- field: integration_target -->
#### Integration target

mage integration:push

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^(TestPlaneAndKindFollowTheChangedPaths|TestFileSetsActiveSprintMilestone|TestFileLeavesMilestoneWhenNoneOrSeveralActive|TestFileExplicitMilestoneWins|TestFileReportsComponent)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- plane: plane::data for a request naming no docs/ or control/ path
- issue: unassigned, admitted, with kind, component, Requirements, and Acceptance Criteria
- component: reported in the result, given or derived from the subsystem SRDs
- milestone: the active sprint covering today, without --milestone; none and the reason when no sprint or several cover today
- questions: only a missing plane, kind, or test; no confirmation
- skill_lines: at most 50

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [AC1](../software-requirements/srd035-specify.md#ac1)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC2](../software-requirements/srd035-specify.md#ac2)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC3](../software-requirements/srd035-specify.md#ac3)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4)
- [srd013-program](../software-requirements/srd013-program.md) [AC3](../software-requirements/srd013-program.md#ac3)
- [srd001-labels](../software-requirements/srd001-labels.md) [AC3](../software-requirements/srd001-labels.md#ac3)
