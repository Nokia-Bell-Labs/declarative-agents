---
id: reports-config
title: Reports repository configuration
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Names who reports status and when, so scrum-master reports status-issues files one weekly status issue per listed person and scrum-master reports aggregate knows whose report is missing (srd018 R3.2, R7; srd018 R9.1, R4.2). The escalation thresholds aggregate uses stay in policy.escalation, where the rest of the program's rules live.

<!-- field: schema -->
## Schema

<!-- schema_entry: team -->
### team

<!-- field: type -->
#### Type

list of mapping

<!-- field: description -->
#### Description

The people who report: username (tracker username) and project (the repository whose open Sprint Goals they report on and where their status issue is filed). Stage one lists the Product Owners.

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

The system repository's project path: where the policy lives and where aggregate --push files actionable signals.

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

reports.md at the root of the reports repository, written once by scrum-master reports init

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

<!-- field: slide_limit -->
## Slide limit

10
```
