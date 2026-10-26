---
id: test-rel03.0-beads-tracker
title: Test Suite for Release 03.0 The Tracker Becomes Beads
release: "03.0"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run builds fixture tracker repositories on the local filesystem —
no GitLab and no network — which is itself part of what the release
proves (srd042 R4.2). The pop-and-close stage proves one issue's whole
life against beads; the synchronization stage proves two clones, a
contended pop, and an offline session against one tracker repository.
The tests live in internal/beads and each target runs its test against
fresh fixtures.

<!-- field: traces -->
## Traces

- [rel03.0-uc001-pop-and-close-on-beads](../use-cases/rel03.0-uc001-pop-and-close-on-beads.md)
- [rel03.0-uc002-two-clones-synchronize](../use-cases/rel03.0-uc002-two-clones-synchronize.md)

<!-- field: preconditions -->
## Preconditions

- A fixture builder creates a tracker repository and two working clones in a temporary directory, with ready issues seeded.
- The clones' configuration documents name the beads backend and the fixture tracker repository by path.
- No GitLab instance is reachable; a test that touches the network fails.

<!-- field: test_cases -->
## Test cases

<!-- case: A popped issue closes with its merge commit cited | status: implemented -->
### A popped issue closes with its merge commit cited

<!-- field: use_case -->
#### Use case

[rel03.0-uc001-pop-and-close-on-beads](../use-cases/rel03.0-uc001-pop-and-close-on-beads.md)

<!-- field: integration_target -->
#### Integration target

mage integration:beadsPopClose

<!-- field: go_test -->
#### Go test

go test ./internal/beads -run '^TestPopAndCloseCitesTheMergeCommit$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- assignee: the person who popped, set by the pop alone
- closing_comment: the merge commit hash
- second_close: nothing changes
- tracker_repository: holds the close before the command reports

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC2](../software-requirements/srd042-beads-tracker.md#ac2)

<!-- case: The state machine replays identically on beads | status: implemented -->
### The state machine replays identically on beads

<!-- field: use_case -->
#### Use case

[rel03.0-uc001-pop-and-close-on-beads](../use-cases/rel03.0-uc001-pop-and-close-on-beads.md)

<!-- field: integration_target -->
#### Integration target

mage integration:beadsStates

<!-- field: go_test -->
#### Go test

go test ./internal/beads -run '^TestStateTableReplaysOnBeads$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- transitions: every srd041 table row fires on beads with the same actor and assignee effect
- refused: a transition outside the table is refused and reported

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC1](../software-requirements/srd042-beads-tracker.md#ac1)

<!-- case: Disjoint writes merge and a contended pop keeps the earlier taker | status: implemented -->
### Disjoint writes merge and a contended pop keeps the earlier taker

<!-- field: use_case -->
#### Use case

[rel03.0-uc002-two-clones-synchronize](../use-cases/rel03.0-uc002-two-clones-synchronize.md)

<!-- field: integration_target -->
#### Integration target

mage integration:beadsSync

<!-- field: go_test -->
#### Go test

go test ./internal/beads -run '^TestContendedPopKeepsTheEarlierTaker$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- disjoint_writes: both land in the tracker repository
- contended_issue: one assignee, the earlier push
- later_pop: reports lost to an earlier taker

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3)

<!-- case: An offline clone reads its cache, says so, and reconciles | status: implemented -->
### An offline clone reads its cache, says so, and reconciles

<!-- field: use_case -->
#### Use case

[rel03.0-uc002-two-clones-synchronize](../use-cases/rel03.0-uc002-two-clones-synchronize.md)

<!-- field: integration_target -->
#### Integration target

mage integration:beadsOffline

<!-- field: go_test -->
#### Go test

go test ./internal/beads -run '^TestOfflineReadNamesTheLastPulledCommit$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- offline_read: served from the cache, naming the last pulled commit and time
- offline_pop: marked provisional in the pop's output
- reconciliation: the provisional assignment confirms or reports as lost; queued writes push

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3)

<!-- case: The tracker holds at a program's size | status: implemented -->
### The tracker holds at a program's size

<!-- field: use_case -->
#### Use case

[rel03.0-uc002-two-clones-synchronize](../use-cases/rel03.0-uc002-two-clones-synchronize.md)

<!-- field: integration_target -->
#### Integration target

mage integration:beadsScale

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestScale(SeedIsATrackerFile|SizesReadsTheEnvironment)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- sizes: 1000, 5000, and 10000 issues, each under 2, 5, and 10 concurrent writers
- report: command times alone and under contention, failures, the tracker repository's size, and a fresh clone's time, under .regression/beads-scale
- lost_writes: none; every filed issue is in the tracker once
- duplicate_ids: none; no ID or IID repeats

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3)
