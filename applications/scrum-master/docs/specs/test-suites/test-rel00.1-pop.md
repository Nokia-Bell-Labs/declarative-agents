---
id: test-rel00.1-pop
title: Test Suite for Release 00.1 Pop and Merge
release: "00.1"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them. The
suite pops an admitted issue into its worktree, proves the refusals,
commits, delivers it with tpm issue merge, and proves the issue closed.

<!-- field: traces -->
## Traces

- [rel00.1-uc001-pop-and-merge](../use-cases/rel00.1-uc001-pop-and-merge.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository and one Product Owner repository, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.
- Work reaches the rig's default branches only through tpm issue merge; pushes go to issue branches.

<!-- field: test_cases -->
## Test cases

<!-- case: Pop claims the issue into its own worktree | status: implemented -->
### Pop claims the issue into its own worktree

<!-- field: use_case -->
#### Use case

[rel00.1-uc001-pop-and-merge](../use-cases/rel00.1-uc001-pop-and-merge.md)

<!-- field: integration_target -->
#### Integration target

mage integration:pop

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestPop(AssignsOnlyTheCaller|RefusesHeldOrUnadmitted|CreatesBranchAndWorktree)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- assignees: the caller only
- branch: <iid>-<slug> from the default branch
- worktree: created beside the checkout, reused on a second pop
- refused: an issue assigned to someone else, a closed issue, a non-admitted data issue

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC1](../software-requirements/srd036-implement.md#ac1)
- [srd041-states](../software-requirements/srd041-states.md) [AC5](../software-requirements/srd041-states.md#ac5)

<!-- case: The merge closes the issue | status: implemented -->
### The merge closes the issue

<!-- field: use_case -->
#### Use case

[rel00.1-uc001-pop-and-merge](../use-cases/rel00.1-uc001-pop-and-merge.md)

<!-- field: integration_target -->
#### Integration target

mage integration:pop

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestMerge(DeliversOnBeads|NeedsAnApprovalOfItsTip|WithoutApprovalByDefault)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- merge: under the default policy, no approval needed; under require_approval, refused without another person's approval of the tip; then one --no-ff merge commit on the default branch, citing the issue, pushed
- after_merge: issue closed
- skill: asks nothing, closes nothing itself, at most 50 lines

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC4](../software-requirements/srd013-program.md#ac4)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC2](../software-requirements/srd036-implement.md#ac2)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC3](../software-requirements/srd036-implement.md#ac3)
- [srd041-states](../software-requirements/srd041-states.md) [AC4](../software-requirements/srd041-states.md#ac4)

<!-- case: The worktree carries the checkout's skills | status: implemented -->
### The worktree carries the checkout's skills

<!-- field: use_case -->
#### Use case

[rel00.1-uc001-pop-and-merge](../use-cases/rel00.1-uc001-pop-and-merge.md)

<!-- field: integration_target -->
#### Integration target

mage integration:pop

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestPop(InstallsSkillsIntoWorktree|WithoutInstalledSkillsInstallsNothing)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- installed: the agents installed in the checkout, stamped, in the worktree
- git_status: clean in the worktree after the install
- none_installed: a checkout without skills installs none

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC4](../software-requirements/srd036-implement.md#ac4)
