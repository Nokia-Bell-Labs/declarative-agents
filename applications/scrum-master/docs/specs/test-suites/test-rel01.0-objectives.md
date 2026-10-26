---
id: test-rel01.0-objectives
title: Test Suite for Release 01.0 Sprint Goals Reach the Product Owner
release: "01.0"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them.
The audit stage proves that mage audit runs the specification-critic
corpus-audit agent from the pinned parent release and fails on a malformed
initiative. The push stage runs tasks against the rig's
system repository and two Product Owner repositories, and proves the dated, admitted, pinned Sprint Goals and their planned
events, and that a second run changes nothing.

<!-- field: traces -->
## Traces

- [rel01.0-uc001-author-objectives](../use-cases/rel01.0-uc001-author-objectives.md)
- [rel01.0-uc002-push-objective-issues](../use-cases/rel01.0-uc002-push-objective-issues.md)

<!-- field: preconditions -->
## Preconditions

- The corpus audit's agent is available: the sibling agent-core in this repository, else the pinned release in the clone cache (GH-190).
- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository and two Product Owner repositories, each declared by a subsystem SRD fixture, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail; nothing outside the run directory is touched.

<!-- field: test_cases -->
## Test cases

<!-- case: A well-formed initiative passes the agent audit | status: implemented -->
### A well-formed initiative passes the agent audit

<!-- field: use_case -->
#### Use case

[rel01.0-uc001-author-objectives](../use-cases/rel01.0-uc001-author-objectives.md)

<!-- field: integration_target -->
#### Integration target

mage integration:objectivesAudit

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestObjectiveGateRejectsMalformedFixtures$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- agent_terminal_state: Passed
- exit_code: 0
- initiative_items: every requirement group has a due item in its applicability and numbered items

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- case: A malformed initiative fails the audit and blocks the merge | status: implemented -->
### A malformed initiative fails the audit and blocks the merge

<!-- field: use_case -->
#### Use case

[rel01.0-uc001-author-objectives](../use-cases/rel01.0-uc001-author-objectives.md)

<!-- field: integration_target -->
#### Integration target

mage integration:objectivesAudit

<!-- field: go_test -->
#### Go test

go test ./internal/objectives -run '^TestEachMalformedDocumentYieldsExactlyItsFinding$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- missing_field: finding names the document and the field
- dangling_trace: finding names the acceptance criterion and the absent item
- unpinned_reference: finding names the reference
- exit_code: non-zero

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC1](../software-requirements/srd034-initiatives.md#ac1)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- case: tasks files one dated Sprint Goal per owner | status: implemented -->
### tasks files one dated Sprint Goal per owner

<!-- field: use_case -->
#### Use case

[rel01.0-uc002-push-objective-issues](../use-cases/rel01.0-uc002-push-objective-issues.md)

<!-- field: integration_target -->
#### Integration target

mage integration:tasks

<!-- field: go_test -->
#### Go test

go test ./internal/goals -run '^Test(PushFilesOneDatedPinnedObjectiveIssue|DueFallsBackToMilestoneAndDryRunWritesNothing)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- issues_per_owner: 1
- cites_lines: one per initiative item, pinned to the release tag
- environment_line: present
- due_date: the requirement group's due item, or the milestone end date
- notes_section: present and empty
- assignee: none
- planned_events: one per issue

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC3](../software-requirements/srd034-initiatives.md#ac3)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC2](../software-requirements/srd022-tasks.md#ac2)
- [srd006-events](../software-requirements/srd006-events.md) [AC7](../software-requirements/srd006-events.md#ac7)
- [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5)

<!-- case: tasks asks only for a missing owner and refuses unpinned citations | status: implemented -->
### tasks asks only for a missing owner and refuses unpinned citations

<!-- field: use_case -->
#### Use case

[rel01.0-uc002-push-objective-issues](../use-cases/rel01.0-uc002-push-objective-issues.md)

<!-- field: integration_target -->
#### Integration target

mage integration:tasks

<!-- field: go_test -->
#### Go test

go test ./internal/goals -run '^Test(PlanRecommendsOwnerAndEnvironment|UnpinnedCitationAndMissingEnvironmentAreRefused|SecondPushChangesNothingAndKeepsOwnerNotes)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- questions: a repository only for a Sprint Goal with no recommended owner, chosen from the candidates
- unpinned_citation: refused, no issue filed
- missing_environment: refused, no issue filed
- second_run: no new issue and no duplicate event

<!-- field: traces -->
#### Traces

- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC1](../software-requirements/srd022-tasks.md#ac1)
- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC3](../software-requirements/srd022-tasks.md#ac3)

<!-- case: Filing and admission | status: implemented -->
### Filing and admission

<!-- field: use_case -->
#### Use case

[rel01.0-uc002-push-objective-issues](../use-cases/rel01.0-uc002-push-objective-issues.md)

<!-- field: integration_target -->
#### Integration target

mage integration:program

<!-- field: go_test -->
#### Go test

go test ./internal/program ./internal/cli -run '^Test(FileCreatesUnassignedLintedIssueAndDryRunWritesNothing|LintReportsEveryFailure|ApplySetsAndClearsNeedsTriageAndNothingElse|JSONAndDryRunReachTheSubcommand)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- filed_issue: unassigned and admitted
- unpinned_cites: needs-triage with the reason in a comment
- criterion_without_test: needs-triage with the reason in a comment
- flags: --json and --dry-run honoured; no prompt

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC1](../software-requirements/srd013-program.md#ac1)
- [srd013-program](../software-requirements/srd013-program.md) [AC2](../software-requirements/srd013-program.md#ac2)
- [srd013-program](../software-requirements/srd013-program.md) [AC3](../software-requirements/srd013-program.md#ac3)

<!-- case: The owner's only step is implement | status: implemented -->
### The owner's only step is implement

<!-- field: use_case -->
#### Use case

[rel01.0-uc002-push-objective-issues](../use-cases/rel01.0-uc002-push-objective-issues.md)

<!-- field: integration_target -->
#### Integration target

mage integration:tasks

<!-- field: go_test -->
#### Go test

go test ./internal/goals ./magefiles -run '^Test(PushFilesOneDatedPinnedObjectiveIssue|SkillFilesStayWithinLineLimit)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- body_sections: Requirements as ordered steps, Acceptance Criteria, Notes
- external_steps: none

<!-- field: traces -->
#### Traces

- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC9](../software-requirements/srd034-initiatives.md#ac9)
