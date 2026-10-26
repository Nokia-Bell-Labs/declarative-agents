---
id: rel04.5-uc001-report-in-place
title: A Person Writes Their Weekly Status Report Where They Worked
status: implemented
priority: P1
test_suite: test-rel04.5-status-in-place
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A person's weekly status issue sits in the repository they worked in,
filed from the roster in the system repository. They take it with the
status-report skill: scrum-master gathers their week, what they committed
to last week, and where the report goes; the model asks only what the
tracker cannot know, drafts the report, and writes it once they approve
it; the report merges in that repository through its own issue loop, and
no other repository is written.

<!-- field: actor -->
## Actor

A person the roster lists: a Developer or a Product Owner, driving an agent through the status-report skill.

<!-- field: trigger -->
## Trigger

The week's status issue is due, the day after the roster's reminder weekday.

<!-- field: preconditions -->
## Preconditions

- The system repository holds the roster, control/reports.md, listing the person and the repository they report in.
- A checkout of that repository with the skills installed.

<!-- field: flow -->
## Flow

- F1: scrum-master reports status-issues --control files the person's status issue in their repository for the coming week, once.
- F2: status-report takes the issue, then runs reports week, reports previous, and reports path, and shows what they gathered.
- F3: The model asks, one question at a time, each open Sprint Goal's status and problems and whether it is complete, the week's impact, the blockers, whether last week's Next landed, and what comes next.
- F4: The model drafts the report with Work, Blockers, Impact, and Next, and revises it until the person approves it.
- F5: It writes the report at status/reports in the repository, checks it with reports validate, and commits it; the machine pushes and merges it on the status issue.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd018-reports](../software-requirements/srd018-reports.md) [R12.1](../software-requirements/srd018-reports.md#r121), [R12.2](../software-requirements/srd018-reports.md#r122), [R16.1](../software-requirements/srd018-reports.md#r161), [R16.2](../software-requirements/srd018-reports.md#r162) | S1 traces the roster and the status issue filed in place. |
| T2 | [srd018-reports](../software-requirements/srd018-reports.md) [R14.1](../software-requirements/srd018-reports.md#r141), [R14.2](../software-requirements/srd018-reports.md#r142), [R18.1](../software-requirements/srd018-reports.md#r181) | S2 traces the skill and the words it drafts from. |
| T3 | [srd018-reports](../software-requirements/srd018-reports.md) [AC10](../software-requirements/srd018-reports.md#ac10), [AC12](../software-requirements/srd018-reports.md#ac12), [AC15](../software-requirements/srd018-reports.md#ac15) | S1 and S2 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The person's status issue is filed once in the repository they report in, asking for the report at status/reports there, and a second run files nothing.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R12.1](../software-requirements/srd018-reports.md#r121)
- [srd018-reports](../software-requirements/srd018-reports.md) [R12.2](../software-requirements/srd018-reports.md#r122)
- [srd018-reports](../software-requirements/srd018-reports.md) [R16.1](../software-requirements/srd018-reports.md#r161)
- [srd018-reports](../software-requirements/srd018-reports.md) [R16.2](../software-requirements/srd018-reports.md#r162)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC10](../software-requirements/srd018-reports.md#ac10)

<!-- sc: S2 -->
### S2

The report the person approved merges at status/reports in their repository, passes reports validate, and next week's reports previous returns its Next and Blockers with its commit.

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [R14.1](../software-requirements/srd018-reports.md#r141)
- [srd018-reports](../software-requirements/srd018-reports.md) [R14.2](../software-requirements/srd018-reports.md#r142)
- [srd018-reports](../software-requirements/srd018-reports.md) [R18.1](../software-requirements/srd018-reports.md#r181)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC12](../software-requirements/srd018-reports.md#ac12)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC15](../software-requirements/srd018-reports.md#ac15)

<!-- field: out_of_scope -->
## Out of scope

- Rolling the reports up; that is the status-rollup skill (rel04.5-uc002).
- Rating or comparing people; a report says what its author did and what blocks it.

<!-- field: assumptions -->
## Assumptions

- A1: Every fact comes from a word and spends no model tokens; the model asks for judgment and writes only what the person approves (design.md D10).
- A2: The repository's policy decides whether a reviewer approves the report before it merges; a status issue gets no exemption.
