---
id: rel01.3-uc001-reference-update
title: A Downstream Release Files a Reference-Update Issue Upstream
status: retired
retired_in: "5.0"
replaced_by: ["rel05.0-uc001-reference-update"]
priority: P2
test_suite: test-rel01.3-tag-triggered
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

Retired in 05.0: release 01.3 was renumbered 05.0 on 2026-10-22, and this document continues as rel05.0-uc001-reference-update. This file stays so that pinned references to it resolve.

When a Product Owner's repository tags a release, CI files one issue in
each upstream repository whose initiatives list that repository in
delivered_by, or whose Sprint Goals the release closed. The issue names
the old and new tags and the files and fields to change, so the upstream
owner moves the pinned references through the inner loop. A second run on
the same tag files nothing.

<!-- field: actor -->
## Actor

CI in the downstream repository, on a new release tag.

<!-- field: trigger -->
## Trigger

tpm release tag creates a release tag in a downstream repository (rel01.2-uc001-sprint-end-release).

<!-- field: preconditions -->
## Preconditions

- The upstream initiatives list delivering repositories in delivered_by, or the release closed upstream Sprint Goals.
- CI holds a token scoped to the program group.

<!-- field: flow -->
## Flow

- F1: The tag triggers the downstream repository's CI job, which runs tpm.
- F2: tpm finds the upstream repositories whose initiatives list this repository in delivered_by or whose Sprint Goals the release closed.
- F3: For each, tpm searches for an issue carrying the handoff marker for this tag and stops if one exists.
- F4: Otherwise tpm files one unassigned issue naming the old and new tags, with Requirements listing the files and fields whose pinned references move, and the handoff marker.
- F5: The upstream owner pops the issue; the reference moves in a merged change that shows the old and new tags.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.1](../software-requirements/srd014-handoff.md#r81), [R8.2](../software-requirements/srd014-handoff.md#r82), [R8.3](../software-requirements/srd014-handoff.md#r83) | S1 and S2 trace the reference-update issue. |
| T2 | [srd014-handoff](../software-requirements/srd014-handoff.md) [R5.1](../software-requirements/srd014-handoff.md#r51) | S2 traces the handoff marker's idempotency. |
| T3 | [srd007-links](../software-requirements/srd007-links.md) [R5.5](../software-requirements/srd007-links.md#r55) | S3 traces moving a pinned reference through the inner loop. |
| T4 | [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7) | S1 and S2 trace this acceptance criterion. |
| T6 | [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5) | S3 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A downstream tag files one reference-update issue in each upstream repository that lists or is closed by it, naming old and new tags and the fields to change.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.1](../software-requirements/srd014-handoff.md#r81)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.2](../software-requirements/srd014-handoff.md#r82)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)

<!-- sc: S2 -->
### S2

The issue carries a handoff marker naming the tag, and a second run on the same tag files nothing.

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [R8.3](../software-requirements/srd014-handoff.md#r83)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [R5.1](../software-requirements/srd014-handoff.md#r51)

<!-- sc: S3 -->
### S3

The reference moves only through a reviewed merge with the old and new tags; no script edits it in place.

<!-- field: traces -->
#### Traces

- [srd007-links](../software-requirements/srd007-links.md) [R5.5](../software-requirements/srd007-links.md#r55)
- [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5)

<!-- field: out_of_scope -->
## Out of scope

- Notifying subscribers, consume issues, and superseded events (srd014 R2); stage one needs only the reference update.
- Classifying the release as breaking or compatible.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step.
