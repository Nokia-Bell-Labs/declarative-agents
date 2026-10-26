---
id: rel03.0-uc002-two-clones-synchronize
title: Two Clones Synchronize Through the Tracker Repository
status: implemented
priority: P2
test_suite: test-rel03.0-beads-tracker
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

Two people on separate clones read and write the tracker in the same
window. Each command pulls before reading and pushes after writing, so
disjoint writes merge silently, a contended assignment keeps the earlier
taker with the later pop reported as lost, and a clone that cannot reach
the tracker repository works from its cache, says so by naming its last
pulled commit, and marks an assignment taken offline as provisional
until the next synchronization confirms or loses it.

<!-- field: actor -->
## Actor

Two people in the program, each on their own clone of a repository the
tracker serves.

<!-- field: trigger -->
## Trigger

Both run tpm commands against the same tracker repository within one
working session.

<!-- field: preconditions -->
## Preconditions

- Both clones' configuration documents name the beads backend and the same tracker repository.
- The tracker repository holds ready issues, among them one both people will try to pop.

<!-- field: flow -->
## Flow

- F1: Each person's command pulls the tracker repository before reading; each output names the tracker commit it read.
- F2: Both write to different issues; both pushes land, and the tracker repository's merge carries both writes.
- F3: Both pop the same ready issue; the earlier push wins the assignment, and the later pop reports the issue as lost to an earlier taker (eng04-beads-sync).
- F4: One clone loses its network; its reads serve the cache and name the last pulled commit and time; its pop marks the assignment provisional.
- F5: The offline clone synchronizes again; its provisional assignment confirms, or reports as lost, and its queued writes push.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.1](../software-requirements/srd042-beads-tracker.md#r31), [R3.2](../software-requirements/srd042-beads-tracker.md#r32) | S1 traces the tracker repository as the one source and the pull/push discipline. |
| T2 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.3](../software-requirements/srd042-beads-tracker.md#r33) | S3 traces the stale-clone statement. |
| T3 | [srd041-states](../software-requirements/srd041-states.md) [R9.1](../software-requirements/srd041-states.md#r91), [R9.2](../software-requirements/srd041-states.md#r92) | S2 traces the assignee invariants under conflict. |
| T4 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3) | S1 and S3 trace this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Disjoint writes from two clones both land; every command's output names the tracker commit it read; every write reaches the tracker repository before its command reports success.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.1](../software-requirements/srd042-beads-tracker.md#r31)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.2](../software-requirements/srd042-beads-tracker.md#r32)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3)

<!-- sc: S2 -->
### S2

When two pops contend for one issue, exactly one assignment survives, it belongs to the earlier push, and the later actor is told the issue was lost to an earlier taker.

<!-- field: traces -->
#### Traces

- [srd041-states](../software-requirements/srd041-states.md) [R9.1](../software-requirements/srd041-states.md#r91)
- [srd041-states](../software-requirements/srd041-states.md) [R9.2](../software-requirements/srd041-states.md#r92)

<!-- sc: S3 -->
### S3

A clone that cannot synchronize reads from its cache and says so, naming the last pulled commit and its time; an assignment taken offline is provisional and confirms or reports as lost on the next synchronization.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R3.3](../software-requirements/srd042-beads-tracker.md#r33)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC3](../software-requirements/srd042-beads-tracker.md#ac3)

<!-- field: out_of_scope -->
## Out of scope

- More than two concurrent writers; the rules do not change with the count, and the fixture proves the contended pair.
- A conflict the rules cannot resolve; that stops the push and reports, and is a malformed database, not a scenario.

<!-- field: assumptions -->
## Assumptions

- A1: Every step is a tpm subcommand, and spends no model tokens (design.md D10). This use case has no model step.
