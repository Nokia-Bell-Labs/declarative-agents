---
id: rel01.3-uc002-release-summary
title: A System Release Drafts the Summary for Review
status: retired
retired_in: "5.0"
replaced_by: ["rel05.0-uc002-release-summary"]
priority: P2
test_suite: test-rel01.3-tag-triggered
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

Retired in 05.0: release 01.3 was renumbered 05.0 on 2026-10-22, and this document continues as rel05.0-uc002-release-summary. This file stays so that pinned references to it resolve.

When the system repository tags a release, a CI pipeline runs tpm reports
aggregate over the reports since the previous system release and tpm
reports deck over the result, and opens one summary for review in the reports
repository with the computed aggregate and deck, linked to the open
stakeholder update issue. The pipeline spends no model tokens. When the
lead works the stakeholder update, the model drafts the summary prose from
the reports and the aggregate and iterates on it with the lead; the
lead's review is the approval, and nothing is published unreviewed.

<!-- field: actor -->
## Actor

CI in the system repository, on a new release tag.

<!-- field: trigger -->
## Trigger

tpm release tag creates a release tag in the system repository.

<!-- field: preconditions -->
## Preconditions

- The reports repository holds the reports since the previous system release tag.
- A stakeholder update issue is open in the reports repository.

<!-- field: flow -->
## Flow

- F1: The tag triggers the system repository's summary pipeline, which runs tpm.
- F2: tpm reports aggregate computes the signals over the reports between the previous system release tag and this one.
- F3: tpm reports deck renders the computed aggregate into the deck template, within the configured slide limit.
- F4: tpm pushes one branch and files one review issue in the reports repository with the computed aggregate and the deck, linked to the stakeholder update issue; if one exists for this tag, it opens none.
- F5: Model step (combine): when the lead pops the stakeholder update issue, the model drafts the summary prose from the reports and the computed aggregate.
- F6: Model step (iterate): the model revises the outline and the summary with the lead; the merge closes the stakeholder update issue.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd018-reports](../software-requirements/srd018-reports.md) [R10.1](../software-requirements/srd018-reports.md#r101), [R10.2](../software-requirements/srd018-reports.md#r102), [R10.3](../software-requirements/srd018-reports.md#r103) | S1 and S2 trace the release-triggered summary pipeline. |
| T2 | [srd018-reports](../software-requirements/srd018-reports.md) [R4.1](../software-requirements/srd018-reports.md#r41) | S1 and S3 trace the deck. |
| T3 | [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8) | S1 and S2 trace this acceptance criterion. |
| T4 | [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4), [AC5](../software-requirements/srd018-reports.md#ac5) | S1 and S3 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A system tag runs aggregate and deck in CI and pushes one branch, with one review issue, holding the computed aggregate and deck, linked to the stakeholder update issue.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R10.1](../software-requirements/srd018-reports.md#r101)
- [srd018-reports](../software-requirements/srd018-reports.md) [R10.2](../software-requirements/srd018-reports.md#r102)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8)
- [srd018-reports](../software-requirements/srd018-reports.md) [R4.1](../software-requirements/srd018-reports.md#r41)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4)

<!-- sc: S2 -->
### S2

A second run on the same tag pushes no branch and files no issue.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R10.3](../software-requirements/srd018-reports.md#r103)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8)

<!-- sc: S3 -->
### S3

The model drafts the summary prose and iterates on it with the lead; the stakeholder update closes by the reviewed merge, with the lead's approval of the outline and no comparison of people.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [AC5](../software-requirements/srd018-reports.md#ac5)

<!-- field: out_of_scope -->
## Out of scope

- Generating prose in CI; the model writes only when the lead works the stakeholder update.
- Distribution outside the tracker.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10).
