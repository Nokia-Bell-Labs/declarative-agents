---
id: rollup
title: Status rollup
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Holds a lead's account of a team's or the program's period in a fixed, checkable shape: what the period's reports add up to, the progress of each Sprint Goal as its ledger states it, the risks and asks with their owners, and what comes next, every claim citing the report or rollup it rests on by repository and commit (srd018 R15). A team rollup reads the merged reports of its subsystem's repositories; the program rollup reads the teams' rollups and the system repository's own reports, and opens a raw report only to cite a signal.

<!-- field: schema -->
## Schema

<!-- schema_entry: front_matter.level -->
### front_matter.level

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

team or program.

<!-- schema_entry: front_matter.subsystem -->
### front_matter.subsystem

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

A team rollup's subsystem SRD id; a program rollup has none.

<!-- schema_entry: front_matter.author -->
### front_matter.author

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracker username of the lead who wrote it: the subsystem's owner, or the technical program manager.

<!-- schema_entry: front_matter.period -->
### front_matter.period

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The ISO weeks it covers: one week, YYYY-Www, or the first and last, YYYY-Www..YYYY-Www. Its last week names the file.

<!-- schema_entry: headlines -->
### headlines

<!-- field: type -->
#### Type

list of entries

<!-- field: description -->
#### Description

The "## Headlines" section: what a reader should take away, each a Markdown list item of key: value pairs separated by semicolons. Keys: headline (free text) and cites.

<!-- schema_entry: progress -->
### progress

<!-- field: type -->
#### Type

list of entries

<!-- field: description -->
#### Description

The "## Progress" section, one entry per Sprint Goal. Keys: sprint_goal (project#iid), component, state (met or reported complete from the Sprint Goal's ledger, srd006, else its latest reported status: on-track, at-risk, blocked, or done), evidence (free text), and cites. Progress is never a count of issues.

<!-- schema_entry: risks_and_asks -->
### risks_and_asks

<!-- field: type -->
#### Type

list of entries

<!-- field: description -->
#### Description

The "## Risks and asks" section. Keys: kind (risk or ask), text, owner (who acts on it: a component, a team, or a role, never a person ranked), and cites. It holds the single entry "none" when there is none.

<!-- schema_entry: next_period -->
### next_period

<!-- field: type -->
#### Type

list of text

<!-- field: description -->
#### Description

The "## Next period" section: what the level commits to next, one plain list item each; "none" when nothing is committed.

<!-- schema_entry: sources -->
### sources

<!-- field: type -->
#### Type

list of entries

<!-- field: description -->
#### Description

The "## Sources" section, one entry per report or rollup the rollup read. Keys: repository (project path), path (the file in it), and commit (the commit that last changed it, as scrum-master reports collect names it).

<!-- field: validation_rules -->
## Validation rules

- The five sections are present, in the order Headlines, Progress, Risks and asks, Next period, Sources.
- Every key named above is present in every entry, and values hold no semicolon.
- Every headline, progress, and risk or ask entry cites one or more sources, comma-separated, each repository@commit, and every one it cites is listed in Sources; scrum-master reports validate fails a rollup where one is not (srd018 R15.2).
- The file name is <last week>-<subsystem>.md for a team, <last week>-program.md for the program.
- A rollup describes components, Sprint Goals, and blockers; it never compares or ranks people (srd018 G3).

<!-- field: config_files -->
## Config files

status/rollups/<YYYY-Qn>/<YYYY-Www>-<subsystem>.md in the repository holding the team's Sprint Goals, and status/rollups/<YYYY-Qn>/<YYYY-Www>-program.md in the system repository, where the quarter is the last week's quarter (srd018 R15.1).

<!-- field: example -->
## Example

```yaml
|
    ---
    level: program
    author: tpm-1
    period: 2026-W42
    ---
    ## Headlines

    - headline: the grader scores the first task end to end; cites: group/product-owners/po-1@4f2c9a1

    ## Progress

    - sprint_goal: group/product-owners/po-1#4; component: grader; state: at-risk; evidence: the scoring API returns stale results; cites: group/product-owners/po-1@4f2c9a1

    ## Risks and asks

    - kind: ask; text: a fix to the scoring API's caching; owner: scoring-api; cites: group/product-owners/po-1@4f2c9a1

    ## Next period

    - score a second task once the scoring API is fixed

    ## Sources

    - repository: group/product-owners/po-1; path: status/rollups/2026-Q4/2026-W42-srd010-grader.md; commit: 4f2c9a1
```
