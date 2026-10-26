---
id: rel03.1-uc001-choose-the-tracking-system
title: A Program Names Each Repository's Tracking System and Works Through It
status: implemented
priority: P2
test_suite: test-rel03.1-tracking-systems
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A program keeps some repositories' issues in beads and others on GitHub
or GitLab. Its system repository's control/trackers.md names the default
tracking system and the projects or groups that use another. tpm routes
every command to the system serving the project it names, so the same
file, pop, and merge work everywhere: on beads by a local merge, on gh and
glab through a draft pull or merge request that pop opens and merge lands.
Approval before merge is the program's policy, off by default, so a
program with one identity merges its own pull request.

<!-- field: actor -->
## Actor

The technical program manager, who names the tracking systems; any person who files, pops, and merges.

<!-- field: trigger -->
## Trigger

A person runs tpm issue file, pop, or merge for a project.

<!-- field: preconditions -->
## Preconditions

- A system repository whose control/trackers.md names a default system and any overrides (config format trackers).
- The gh or glab CLI logged in as the person, for a project those systems serve; for beads, the tracker repository's clone.
- A local checkout of the project, for pop and merge.

<!-- field: flow -->
## Flow

- F1: tpm reads control/trackers.md from the checkout or its sibling system repository and picks the system serving the project by its longest matching override, else the default; SCRUM_MASTER_TRACKER overrides the default only.
- F2: tpm issue file files the issue on that system with its labels and milestone; on gh, a due or start date is a line of the description, and a group's labels and milestones go to each of its repositories.
- F3: tpm issue pop claims the issue, opens its branch and worktree, and on gh or glab opens a draft pull or merge request with one empty commit.
- F4: The person commits and pushes; when the policy sets admission.require_approval, a reviewer approves the head commit (a pull request review on gh, an approval with a note naming the commit on glab).
- F5: tpm issue merge lands the branch on the host, brings the checkout level, and closes the issue on that merge commit.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.3](../software-requirements/srd042-beads-tracker.md#r13) | S1 traces the routing by control/trackers.md. |
| T2 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.4](../software-requirements/srd042-beads-tracker.md#r14), [R1.5](../software-requirements/srd042-beads-tracker.md#r15) | S2 traces the gh and glab systems. |
| T3 | [srd013-program](../software-requirements/srd013-program.md) [R5.5](../software-requirements/srd013-program.md#r55), [R5.6](../software-requirements/srd013-program.md#r56) | S3 traces approval as the policy's choice. |
| T4 | [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC1](../software-requirements/srd042-beads-tracker.md#ac1), [AC5](../software-requirements/srd042-beads-tracker.md#ac5), [AC6](../software-requirements/srd042-beads-tracker.md#ac6) | S1 and S2 trace these acceptance criteria. |
| T5 | [srd013-program](../software-requirements/srd013-program.md) [AC8](../software-requirements/srd013-program.md#ac8) | S3 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Each command reaches the system control/trackers.md names for its project, the longest override winning, and an ambiguous file is refused, not guessed.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.3](../software-requirements/srd042-beads-tracker.md#r13)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC1](../software-requirements/srd042-beads-tracker.md#ac1)

<!-- sc: S2 -->
### S2

On gh and on glab an issue files, pops into a draft pull or merge request, merges on the host, and closes on that merge commit, through the CLI and the person's own login.

<!-- field: traces -->
#### Traces

- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.4](../software-requirements/srd042-beads-tracker.md#r14)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [R1.5](../software-requirements/srd042-beads-tracker.md#r15)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC5](../software-requirements/srd042-beads-tracker.md#ac5)
- [srd042-beads-tracker](../software-requirements/srd042-beads-tracker.md) [AC6](../software-requirements/srd042-beads-tracker.md#ac6)

<!-- sc: S3 -->
### S3

Under the default policy a one-identity program merges its own pull request; under require_approval, merge waits for another person's approval of the head commit and refuses a stale one.

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [R5.5](../software-requirements/srd013-program.md#r55)
- [srd013-program](../software-requirements/srd013-program.md) [R5.6](../software-requirements/srd013-program.md#r56)
- [srd013-program](../software-requirements/srd013-program.md) [AC8](../software-requirements/srd013-program.md#ac8)

<!-- field: out_of_scope -->
## Out of scope

- Moving a repository's issues from one system to another.
- A host's own branch protection, which applies on top of tpm's policy.
- Single-repository mode, the tracker on a branch of the code repository (GH-131).

<!-- field: assumptions -->
## Assumptions

- A1: tpm keeps no token: gh and glab use the person's existing login.
- A2: Every step is a tpm subcommand or git, and spends no model tokens (design.md D10).
