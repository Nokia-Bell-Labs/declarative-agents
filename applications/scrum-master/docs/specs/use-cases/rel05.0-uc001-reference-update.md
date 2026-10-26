---
id: rel05.0-uc001-reference-update
title: A Downstream Release Files a Reference-Update Issue Upstream
status: specified
priority: P2
test_suite: test-rel05.0-tag-triggered
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

When a Product Owner's repository tags a release, the release profile's
handoff step files one issue in each upstream repository whose
initiatives list that repository in delivered_by, or whose Sprint Goals
the release closed. The issue names the tag the upstream pins now, the new
tag, and the files and fields to change, so the upstream owner moves the
pinned references through the inner loop. Each repository's issues go
through the tracking system it is routed to, bd, gh, or glab. A second run
on the same tag files nothing, and an issue already open for an earlier
tag takes the new one.

<!-- field: actor -->
## Actor

The person running the release profile in the downstream repository, or a tag pipeline on the release image running the handoff profile as a service user.

<!-- field: trigger -->
## Trigger

The release profile's tag word pushes a release tag in a downstream repository (rel01.2-uc001-sprint-end-release), and its handoff step runs scrum-master handoff on-release.

<!-- field: preconditions -->
## Preconditions

- The upstream initiatives list delivering repositories in delivered_by, or the release closed upstream Sprint Goals.
- The person can reach each repository's tracking system as control/trackers.md routes it: the tracker clone for bd, the gh or glab CLI's login for gh and glab.
- The upstream repository's checkout sits beside the downstream one.
- On the pipeline path only, the service user holds a credential scoped to the tracking system.

<!-- field: flow -->
## Flow

- F1: The release profile tags the release and pushes the tag; its handoff step runs scrum-master handoff on-release with the person's access.
- F2: Handoff finds the upstream repositories whose initiatives list this repository in delivered_by by pinned URL, or whose Sprint Goals the release closed, from those Sprint Goals' Cites lines.
- F3: For each, handoff searches the upstream repository's issues in every state, through its routed tracking system, for the marker naming this tag, and stops if one exists.
- F4: When a reference-update issue for an earlier tag is open, handoff adds the new tag and a comment to it; otherwise it files one unassigned issue naming the pinned old tag and the new tag, with Requirements listing the files and fields whose pinned references move, and the marker.
- F5: When a tracking system is unreachable or the upstream checkout is missing, handoff files nothing, exits non-zero, and the release run ends naming the handoff profile to re-run; the tag stands.
- F6: The upstream owner pops the issue; the reference moves in a merged change that shows the old and new tags.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.1](../software-requirements/srd014-handoff.md#r81), [R8.2](../software-requirements/srd014-handoff.md#r82), [R8.3](../software-requirements/srd014-handoff.md#r83) | S1 and S2 trace the reference-update issue. |
| T2 | [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.4](../software-requirements/srd014-handoff.md#r84), [R8.5](../software-requirements/srd014-handoff.md#r85) | S3 and S4 trace the open-issue rule and the refusal. |
| T3 | [srd014-handoff](../software-requirements/srd014-handoff.md) [R7.1](../software-requirements/srd014-handoff.md#r71), [R7.2](../software-requirements/srd014-handoff.md#r72) | S4 and S5 trace the trigger and the routed tracking systems. |
| T4 | [srd014-handoff](../software-requirements/srd014-handoff.md) [R5.1](../software-requirements/srd014-handoff.md#r51) | S2 traces the marker's idempotency. |
| T5 | [srd007-links](../software-requirements/srd007-links.md) [R5.5](../software-requirements/srd007-links.md#r55) | S6 traces moving a pinned reference through the inner loop. |
| T6 | [srd014-handoff](../software-requirements/srd014-handoff.md) [AC6](../software-requirements/srd014-handoff.md#ac6), [AC7](../software-requirements/srd014-handoff.md#ac7) | S1 to S5 trace these acceptance criteria. |
| T7 | [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5) | S6 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A downstream tag files one reference-update issue in each upstream repository that lists or is closed by it, naming the pinned old tag, the new tag, and the fields to change.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.1](../software-requirements/srd014-handoff.md#r81)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.2](../software-requirements/srd014-handoff.md#r82)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)

<!-- sc: S2 -->
### S2

The issue carries the marker naming the tag, and a second run on the same tag files nothing, whether the first issue is open or closed.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.3](../software-requirements/srd014-handoff.md#r83)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [R5.1](../software-requirements/srd014-handoff.md#r51)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)

<!-- sc: S3 -->
### S3

An upstream repository with a reference-update issue open for an earlier tag gets the new tag and a comment on that issue, not a sibling, on bd, gh, and glab alike.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.4](../software-requirements/srd014-handoff.md#r84)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)

<!-- sc: S4 -->
### S4

An unreachable tracking system or a missing upstream checkout files nothing, the release run names the handoff profile to re-run, and the tag stands; the handoff profile's re-run files what was owed.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.5](../software-requirements/srd014-handoff.md#r85)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [R7.1](../software-requirements/srd014-handoff.md#r71)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC6](../software-requirements/srd014-handoff.md#ac6)

<!-- sc: S5 -->
### S5

A downstream repository on one tracking system and an upstream repository on another each have their issues read and written through their own.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R7.2](../software-requirements/srd014-handoff.md#r72)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC6](../software-requirements/srd014-handoff.md#ac6)

<!-- sc: S6 -->
### S6

The reference moves only through a reviewed merge with the old and new tags; no script edits it in place.

<!-- field: traces -->
#### Traces

- [srd007-links](../software-requirements/srd007-links.md) [R5.5](../software-requirements/srd007-links.md#r55)
- [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5)

<!-- field: out_of_scope -->
## Out of scope

- Notifying subscribers, consume issues, and superseded events (srd014 R9); 05.0 builds only the reference update.
- Classifying the release as breaking or compatible.

<!-- field: assumptions -->
## Assumptions

- A1: Every step is a scrum-master word, a profile, or the inner loop, and spends no model tokens (design.md D10). This use case has no model step.
