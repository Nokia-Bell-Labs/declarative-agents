---
id: reports-config
title: Reports roster
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Names who reports status, in which repository, and when, so scrum-master reports status-issues files one weekly status issue per listed person where they report, a team rollup issue per subsystem, and the program rollup issue, and scrum-master reports aggregate knows whose report is missing (srd018 R3.2, R7, R9.1, R12, R16). The technical program manager owns it in the system repository; a program that has not moved its roster there keeps it in a central reports repository (srd018 R12.2). The escalation thresholds aggregate uses stay in policy.escalation, where the rest of the program's rules live.

<!-- field: schema -->
## Schema

<!-- schema_entry: team -->
### team

<!-- field: type -->
#### Type

list of mapping

<!-- field: description -->
#### Description

The people who report: username (tracker username) and project (the repository they work in: whose open Sprint Goals they report on, where their status issue is filed, and where their report is written and merged, srd018 R12.1). A person's team is the subsystem whose SRD lists that repository.

<!-- schema_entry: reminder_weekday -->
### reminder_weekday

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The weekday the tracker's due-date notification should arrive; the status issue is due the day after (srd018 R9.2). Default wednesday.

<!-- schema_entry: status_issue_title -->
### status_issue_title

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The status issue's title with {week} replaced by the ISO week, YYYY-Www. Default "Recurring: weekly status {week}".

<!-- schema_entry: system -->
### system

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The system repository's project path: where the policy lives and where aggregate --push files actionable signals. A roster read from the system repository's control/ names it without this entry.

<!-- schema_entry: team_rollup_title -->
### team_rollup_title

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

A team rollup issue's title with {subsystem} replaced by the subsystem SRD's id and {week} by the ISO week (srd018 R16.1). Default "Recurring: team rollup {subsystem} {week}".

<!-- schema_entry: program_rollup_title -->
### program_rollup_title

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The program rollup issue's title with {week} replaced by the ISO week. Default "Recurring: program rollup {week}".

<!-- schema_entry: program_lead -->
### program_lead

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracker username of the person the program rollup issue is for, the technical program manager. Empty is the person running status-issues.

<!-- schema_entry: report_due_days -->
### report_due_days

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

How many days after the reminder weekday a status issue is due. Default 1, the day after.

<!-- schema_entry: team_rollup_due_days -->
### team_rollup_due_days

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

How many working days after the reports a team rollup issue is due. Default 1.

<!-- schema_entry: program_rollup_due_days -->
### program_rollup_due_days

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

How many working days after the team rollups the program rollup issue is due. Default 1.

<!-- schema_entry: reports -->
### reports

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

A central reports repository's project path whose reports scrum-master reports collect and previous still read, for the release that moves a program's reports into place, so its history stays readable (srd018 R12.2). Empty reads none.

<!-- schema_entry: slide_limit -->
### slide_limit

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

The most slides scrum-master reports deck renders from a summary, one per outline section. A summary whose outline has more sections makes deck exit non-zero naming the sections over the limit; nothing is truncated (srd018 R4.1). Default 10.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: reports
title: Reports configuration
---

<!-- field: config_files -->
## Config files

control/reports.md in the system repository, owned by the technical program manager; reports.md at the root of a central reports repository where a program has not moved it

<!-- field: team -->
## Team

| username | project |
| --- | --- |
| po-1 | group/product-owners/po-1 |
| po-2 | group/product-owners/po-2 |

<!-- field: reminder_weekday -->
## Reminder weekday

wednesday

<!-- field: status_issue_title -->
## Status issue title

Recurring: weekly status {week}

<!-- field: system -->
## System

group/system

<!-- field: program_lead -->
## Program lead

tpm-1

<!-- field: report_due_days -->
## Report due days

1

<!-- field: team_rollup_due_days -->
## Team rollup due days

1

<!-- field: program_rollup_due_days -->
## Program rollup due days

1

<!-- field: slide_limit -->
## Slide limit

10
```
