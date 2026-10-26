---
id: rel05.0-uc002-release-summary
title: A System Release Renders the Deck from the Approved Summary
status: specified
priority: P2
test_suite: test-rel05.0-tag-triggered
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

When the system repository tags a release, the lead has already reviewed
and merged its sprint-end summary at docs/sprint-ends/RELEASE.md. The
release profile's step after the tag renders that approved summary into a
deck with scrum-master reports deck, one slide per outline section within
the configured slide limit, and commits the deck beside the summary. No
branch or review issue opens and no model runs: the summary's review was
the approval. A second run on the same tag commits nothing, and a summary
over the limit names the sections over it instead of being cut.

<!-- field: actor -->
## Actor

The program lead running the release profile in the system repository.

<!-- field: trigger -->
## Trigger

The release profile's tag word pushes a release tag in the system repository, and its step after the tag renders the deck.

<!-- field: preconditions -->
## Preconditions

- The sprint-end summary is merged at docs/sprint-ends/RELEASE.md in the outline-section summary format (srd038 R1.4).
- The reports repository's configuration sets slide_limit, or the default of 10 applies, and holds the deck template.

<!-- field: flow -->
## Flow

- F1: The release profile gates and tags the system release and pushes the tag (srd015 R2.4).
- F2: Its step after the tag runs scrum-master reports deck over docs/sprint-ends/RELEASE.md with the reports repository's template and slide_limit.
- F3: When the outline has more sections than slide_limit, deck exits non-zero naming the sections over it; the release run ends naming the profile to re-run once the summary is shortened, and the tag stands.
- F4: Otherwise the deck is committed beside the summary and pushed; it is regenerated on each run and never edited by hand.
- F5: A second run on the same tag renders the same deck and commits nothing.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd018-reports](../software-requirements/srd018-reports.md) [R10.1](../software-requirements/srd018-reports.md#r101), [R10.2](../software-requirements/srd018-reports.md#r102), [R10.3](../software-requirements/srd018-reports.md#r103) | S1 and S2 trace the deck rendered after the system tag. |
| T2 | [srd018-reports](../software-requirements/srd018-reports.md) [R4.1](../software-requirements/srd018-reports.md#r41) | S1 and S3 trace the deck and its slide limit. |
| T3 | [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R1.4](../software-requirements/srd038-sprint-end.md#r14) | S1 traces the summary's outline format. |
| T4 | [srd015-release](../software-requirements/srd015-release.md) [R2.4](../software-requirements/srd015-release.md#r24) | S3 traces the post-tag step's failure leaving the tag. |
| T5 | [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4), [AC8](../software-requirements/srd018-reports.md#ac8) | S1 to S3 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A system tag renders the deck from the merged sprint-end summary, one slide per outline section, and commits it beside the summary, with no branch, review issue, or model step.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R10.1](../software-requirements/srd018-reports.md#r101)
- [srd018-reports](../software-requirements/srd018-reports.md) [R10.2](../software-requirements/srd018-reports.md#r102)
- [srd018-reports](../software-requirements/srd018-reports.md) [R4.1](../software-requirements/srd018-reports.md#r41)
- [srd038-sprint-end](../software-requirements/srd038-sprint-end.md) [R1.4](../software-requirements/srd038-sprint-end.md#r14)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8)

<!-- sc: S2 -->
### S2

A second run on the same tag commits nothing.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R10.3](../software-requirements/srd018-reports.md#r103)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8)

<!-- sc: S3 -->
### S3

A summary over the slide limit makes the step exit non-zero naming the sections over it, renders nothing truncated, and leaves the tag.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R4.1](../software-requirements/srd018-reports.md#r41)
- [srd015-release](../software-requirements/srd015-release.md) [R2.4](../software-requirements/srd015-release.md#r24)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4)

<!-- field: out_of_scope -->
## Out of scope

- The stakeholder update as its own artifact (srd018 R11.3); it is direction for 06.0.
- Drafting the summary; the sprint-end issue's work does that before the tag (srd038 R1.2).

<!-- field: assumptions -->
## Assumptions

- A1: Every step is a scrum-master word or a profile and spends no model tokens (design.md D10). This use case has no model step.
