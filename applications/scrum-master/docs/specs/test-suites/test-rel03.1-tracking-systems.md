---
id: test-rel03.1-tracking-systems
title: Test Suite for Release 03.1 One Tracking System per Repository
release: "03.1"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The Go tests run offline: the routing tests parse control/trackers.md and
open systems through stub openers, and the gh and glab tests run tpm's
file, pop, approve, and merge against a fake GitHub and a fake GitLab with
two logins that merge in a real git origin. The opt-in rigs run the same
loop against GitHub and GitLab themselves, with the person's gh or glab
login; they never join mage regression, since they need the network and a
host account.

<!-- field: traces -->
## Traces

- [rel03.1-uc001-choose-the-tracking-system](../use-cases/rel03.1-uc001-choose-the-tracking-system.md)

<!-- field: preconditions -->
## Preconditions

- For the Go tests: git; no network.
- For mage integration:github: gh logged in, GH_TOKEN for the organization, and its private rig repository.
- For mage integration:glab: glab logged in and GLAB_RIG_GROUP naming the group the rig works in.

<!-- field: test_cases -->
## Test cases

<!-- case: control/trackers.md routes each project | status: implemented -->
### control/trackers.md routes each project

<!-- field: use_case -->
#### Use case

[rel03.1-uc001-choose-the-tracking-system](../use-cases/rel03.1-uc001-choose-the-tracking-system.md)

<!-- field: integration_target -->
#### Integration target

mage integration:pop

<!-- field: go_test -->
#### Go test

go test ./internal/routing ./internal/tracker

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- default: a project no override names is served by the default system
- override: the longest matching path wins
- environment: SCRUM_MASTER_TRACKER replaces the default only
- refused: a path mapped twice, an unknown system, or bd named as an override
- found: the file in the checkout, else in a sibling checkout
- rig: every command of mage integration:pop reaches bd through the default, with no override

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC1](../software-requirements/srd042-beads-tracker.md#ac1)

<!-- case: the gh system files, pops, and merges through GitHub | status: implemented -->
### the gh system files, pops, and merges through GitHub

<!-- field: use_case -->
#### Use case

[rel03.1-uc001-choose-the-tracking-system](../use-cases/rel03.1-uc001-choose-the-tracking-system.md)

<!-- field: integration_target -->
#### Integration target

mage integration:github

<!-- field: go_test -->
#### Go test

go test ./internal/github -run '^TestIssuesRoundTrip$|^TestGroupMilestonesAndLabelsGoToEachRepository$|^TestDeliveryThroughPullRequests$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- file: the issue on GitHub with its labels, milestone, and due date read back
- pop: the branch with a draft pull request, once
- approval: the author's own refused by GitHub; another person's review of the head recorded
- merge: under require_approval, refused with no approval and with a stale one; under the default policy, the author's own pull request merged
- close: the issue closed on the merge commit

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC5](../software-requirements/srd042-beads-tracker.md#ac5)
- [srd013-program](../software-requirements/srd013-program.md) [AC8](../software-requirements/srd013-program.md#ac8)

<!-- case: the glab system files, pops, and merges through GitLab | status: implemented -->
### the glab system files, pops, and merges through GitLab

<!-- field: use_case -->
#### Use case

[rel03.1-uc001-choose-the-tracking-system](../use-cases/rel03.1-uc001-choose-the-tracking-system.md)

<!-- field: integration_target -->
#### Integration target

mage integration:glab

<!-- field: go_test -->
#### Go test

go test ./internal/gitlab -run '^TestIssuesRoundTrip$|^TestDeliveryThroughMergeRequests$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- file: the issue on GitLab under a group milestone its project finds
- pop: the branch with a draft merge request
- approval: another person's approval pinned to the head commit by a note naming it
- merge: under require_approval, refused with no approval and with a stale one; under the default policy, the author's own merge request merged once GitLab finishes checking it
- close: the issue closed on the merge commit

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC6](../software-requirements/srd042-beads-tracker.md#ac6)
- [srd013-program](../software-requirements/srd013-program.md) [AC8](../software-requirements/srd013-program.md#ac8)

<!-- case: a program of one repository keeps its tracker on a branch | status: implemented -->
### a program of one repository keeps its tracker on a branch

<!-- field: use_case -->
#### Use case

[rel03.1-uc001-choose-the-tracking-system](../use-cases/rel03.1-uc001-choose-the-tracking-system.md)

<!-- field: integration_target -->
#### Integration target

mage integration:singleRepo

<!-- field: go_test -->
#### Go test

go test ./internal/beads ./internal/routing ./internal/scaffold

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- init: the init profile, with tracker naming the repository's own remote and tracker_branch scrum-master/tracker, makes the branch as an orphan holding the config file and writes the branch to control/trackers.md
- file: the issue is written to the tracker branch
- merge: the popped issue's branch merges into main and the issue closes on the tracker branch
- main: no tracker commit reaches main, and main never holds .beads/
- default: with no tracker_branch, the tracker is the tracker repository's main, as before

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC2](../software-requirements/srd043-init-onboard.md#ac2)
