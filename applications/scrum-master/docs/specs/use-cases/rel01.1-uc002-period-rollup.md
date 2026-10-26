---
id: rel01.1-uc002-period-rollup
title: The Technical Program Manager Aggregates a Period of Status
status: implemented
priority: P2
test_suite: test-rel01.1-weekly-status
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The technical program manager runs tpm reports aggregate over the week's
or the period's reports. It groups the items by component and computes the
signals no single report shows: shared and aging blockers, slipping
commitments, missing reports, contradictions with the tracker, and merged
work nobody reported. It states progress by Sprint Goal, never by person.
A signal that needs action becomes an issue.

<!-- field: actor -->
## Actor

The technical program manager.

<!-- field: trigger -->
## Trigger

A week or a period of reports has merged in the reports repository.

<!-- field: preconditions -->
## Preconditions

- rel01.1-uc001-weekly-status merged the week's reports.
- The reports repository's configuration names the policy thresholds for blocker age and slips.

<!-- field: flow -->
## Flow

- F1: The technical program manager runs tpm reports aggregate for the week or the period.
- F2: tpm reads every report in range and groups their items by component.
- F3: tpm computes the signals and checks each report against the tracker, including a report calling a Sprint Goal complete with no met event.
- F4: tpm states each Sprint Goal's progress from its ledger and its reported status, and expresses no signal as a comparison of people.
- F5: With --push, tpm reports aggregate files each signal that needs action as an issue in the issue body's work format in the system repository.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd018-reports](../software-requirements/srd018-reports.md) [R3.1](../software-requirements/srd018-reports.md#r31), [R3.2](../software-requirements/srd018-reports.md#r32), [R3.4](../software-requirements/srd018-reports.md#r34), [R3.5](../software-requirements/srd018-reports.md#r35), [R3.6](../software-requirements/srd018-reports.md#r36), [R5.1](../software-requirements/srd018-reports.md#r51), [R6.3](../software-requirements/srd018-reports.md#r63) | S1 to S4 trace aggregation, its conventions, and pushed signals. |
| T2 | [srd018-reports](../software-requirements/srd018-reports.md) [AC3](../software-requirements/srd018-reports.md#ac3), [AC4](../software-requirements/srd018-reports.md#ac4), [AC5](../software-requirements/srd018-reports.md#ac5) | S1 to S4 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

aggregate groups the range's items by component and reports every listed signal by component or blocker, never ranking people.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R3.1](../software-requirements/srd018-reports.md#r31)
- [srd018-reports](../software-requirements/srd018-reports.md) [R3.2](../software-requirements/srd018-reports.md#r32)
- [srd018-reports](../software-requirements/srd018-reports.md) [R3.4](../software-requirements/srd018-reports.md#r34)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC3](../software-requirements/srd018-reports.md#ac3)

<!-- sc: S2 -->
### S2

Progress is stated per Sprint Goal from its ledger and its reported status, not as counts of closed issues.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R3.5](../software-requirements/srd018-reports.md#r35)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC3](../software-requirements/srd018-reports.md#ac3)

<!-- sc: S3 -->
### S3

aggregate honours --json and --dry-run and never prompts.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R5.1](../software-requirements/srd018-reports.md#r51)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4)

<!-- sc: S4 -->
### S4

An actionable signal becomes a pushed issue, not prose in a report.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R6.3](../software-requirements/srd018-reports.md#r63)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC5](../software-requirements/srd018-reports.md#ac5)
- [srd018-reports](../software-requirements/srd018-reports.md) [R3.6](../software-requirements/srd018-reports.md#r36)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC3](../software-requirements/srd018-reports.md#ac3)

<!-- field: out_of_scope -->
## Out of scope

- Dashboard signals from track-goals (srd018 R3.3); track-goals is not running in stage one.
- Rendering the deck and the stakeholder update; that is rel05.0-uc002-release-summary.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step.
