---
id: report
title: Weekly status report
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Holds one person's status for one week in a fixed, machine-checkable shape, so scrum-master reports validate can check it and scrum-master reports aggregate can read a week or a period of them against the tracker (srd018 R2, R3). The body is what the person said; the front matter is an index derived from the body, so the two cannot drift.

<!-- field: schema -->
## Schema

<!-- schema_entry: front_matter.author -->
### front_matter.author

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracker username of the person the report is about; it matches the file name.

<!-- schema_entry: front_matter.sprint -->
### front_matter.sprint

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The ISO week the report covers, YYYY-Www; it matches the file name.

<!-- schema_entry: front_matter.components -->
### front_matter.components

<!-- field: type -->
#### Type

list of string

<!-- field: description -->
#### Description

Every component named by an entry of Work or Blockers, sorted, without repeats.

<!-- schema_entry: front_matter.blocked-on -->
### front_matter.blocked-on

<!-- field: type -->
#### Type

list of string

<!-- field: description -->
#### Description

Every waiting-on value of Blockers, sorted, without repeats; empty when there are no blockers.

<!-- schema_entry: work -->
### work

<!-- field: type -->
#### Type

list of entries

<!-- field: description -->
#### Description

The "## Work" section, one entry per open Sprint Goal issue in the person's repository, each a Markdown list item of key: value pairs separated by semicolons. Keys: sprint_goal (the issue reference, project#iid; a report written before release 04.0 names it objective, which is read as an alias), component, status (on-track, at-risk, blocked, or done), complete (yes or no), and problems (free text, "none" when there are none).

<!-- schema_entry: blockers -->
### blockers

<!-- field: type -->
#### Type

list of entries

<!-- field: description -->
#### Description

The "## Blockers" section, one entry per blocker, in the same key: value form. Keys: blocker (free text), component (the component blocked), since (YYYY-MM-DD, the day the blocker began), and waiting-on (the component or issue reference it waits on). The section holds the single entry "none" when nothing blocks.

<!-- schema_entry: impact -->
### impact

<!-- field: type -->
#### Type

list of text

<!-- field: description -->
#### Description

The "## Impact" section: why the week's work mattered, in the author's words, one plain list item each. Required in a report written in place, under status/, as its status issue asks; a central reports repository's report may omit it and reads as before (srd018 R12.1, GH-2778).

<!-- schema_entry: next -->
### next

<!-- field: type -->
#### Type

list of text

<!-- field: description -->
#### Description

The "## Next" section, required in a report written in place and optional in a central reports repository's (GH-2778): what the author commits to for the coming week, one plain list item each, which next week's report answers; scrum-master reports previous returns it with the Blockers (srd018 R14.1).

<!-- field: validation_rules -->
## Validation rules

- Both sections, Work and Blockers, are present in every report, even when empty of work or blockers: Blockers then holds the single entry "none" (GH-116).
- A report written in place, under status/, also has Impact and Next; scrum-master reports validate names each one missing (GH-2778).
- Every key named above is present in every entry, and values hold no semicolon.
- The front matter equals what the body derives; scrum-master reports validate fails a report where it does not (srd018 R2.2).
- A report says what its author did and what blocks it; it never rates another person.

<!-- field: config_files -->
## Config files

status/reports/<YYYY-Qn>/<YYYY-Www>-<username>.md in the repository where the person worked, one file per listed person per ISO week, where the quarter is the ISO week's quarter (srd018 R12.1); reports/<YYYY-Qn>/<YYYY-Www>-<username>.md in a central reports repository a program has not moved from.

<!-- field: example -->
## Example

```yaml
|
    ---
    author: po-1
    sprint: 2026-W42
    components: [grader]
    blocked-on: [scoring-api]
    ---
    ## Work

    - sprint_goal: group/product-owners/po-1#4; component: grader; status: at-risk; complete: no; problems: the scoring API returns stale results
    - sprint_goal: group/product-owners/po-1#5; component: grader; status: done; complete: yes; problems: none

    ## Blockers

    - blocker: stale results from the scoring API; component: grader; since: 2026-10-08; waiting-on: scoring-api

    ## Impact

    - the grader scores the first task end to end, so the scorecard has real numbers

    ## Next

    - score a second task once the scoring API is fixed
```
