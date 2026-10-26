---
id: test-rel04.4-story-loop
title: Test Suite for Release 04.4 Work a Story End to End
release: "04.4"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The Go tests run against a beads tracker and a code repository on local
disk; the rig runs the built tpm against the regression rig's fixture
repositories. Together they check that a story's issues list in order
with a blocked one apart, that further issues are claimed into the first
one's worktree, and that one merge closes every claimed issue.

<!-- field: traces -->
## Traces

- [rel04.4-uc001-work-a-story](../use-cases/rel04.4-uc001-work-a-story.md)

<!-- field: preconditions -->
## Preconditions

- git; for the rig, the regression rig's tracker and product-owner repositories (eng01).

<!-- field: test_cases -->
## Test cases

<!-- case: a story's issues list in order and one merge closes them | status: implemented -->
### a story's issues list in order and one merge closes them

<!-- field: use_case -->
#### Use case

[rel04.4-uc001-work-a-story](../use-cases/rel04.4-uc001-work-a-story.md)

<!-- field: integration_target -->
#### Integration target

mage integration:story

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestEpicStoriesInRankOrder$|^TestImplementAStory$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- list: the story's open issues by IID, an issue blocked by open work apart; with --epic, every story under the epic in rank order
- claim: the second issue claimed into the first's worktree, on its branch, with no branch or worktree opened; one someone else holds refused
- merge: one merge citing the first issue and closing the others named with --closes
- blocked: the blocked issue still open after the merge

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [AC5](../software-requirements/srd036-implement.md#ac5)

<!-- case: tasks files a story's issues | status: implemented -->
### tasks files a story's issues

<!-- field: use_case -->
#### Use case

[rel04.4-uc001-work-a-story](../use-cases/rel04.4-uc001-work-a-story.md)

<!-- field: integration_target -->
#### Integration target

mage integration:story

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestSkillFilesStayWithinLineLimit$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- filed: each issue with an Implements line naming the story, under the Sprint Goal's milestone
- skill: the tasks skill's story section names tpm backlog list, tpm issue list --implements, and tpm issue file, at most 50 lines

<!-- field: traces -->
#### Traces

- [srd022-tasks](../software-requirements/srd022-tasks.md) [AC4](../software-requirements/srd022-tasks.md#ac4)
