---
id: states
title: Issue states
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Declares the srd041 issue state machine as data: every transition the beads tracker makes is one row of this file, fired by the row's one actor, with its assignee effect. scrum-master init writes the built-in default to control/states.md, scrum-master doctor restores a missing one, and the tracker reads the program's rows from main on the first transition, so a program can see and extend its own table — an overlay row of its own among them — without a scrum-master change (GH-130). A file that does not parse, or a row the format does not list, refuses every transition with its reason rather than silently falling back to the default. The srd041 invariants (R9) are not rows and always hold: only pop sets an assignee, and only the caller; a person's moves on a taken issue are the assignee's alone.

<!-- field: schema -->
## Schema

<!-- schema_entry: version -->
### version

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

Schema version of this file; 1.

<!-- schema_entry: rows -->
### rows

<!-- field: type -->
#### Type

table with columns from, to, actor, assignee, requires, kind

<!-- field: description -->
#### Description

One row per transition. from is a comma-separated list of states (open, needs-triage, ready, taken, in-progress, closed), or any-open for every state but closed. to is a state, or +overlay / -overlay to set or clear an overlay label such as waiting, blocked, or needs-run. actor is one of person, pop, merge, track-goals, handoff, lint. assignee is the row's effect, none, set, or clear; empty reads as none. requires is a label the issue must carry for the row to apply, or empty. kind is empty or sprint-goal, which restricts the row to Sprint Goal issues.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: states
title: Issue states
---

<!-- field: version -->
## Version

1

<!-- field: rows -->
## Rows

| from | to | actor | assignee | requires | kind |
| --- | --- | --- | --- | --- | --- |
| open | taken | pop | set |  |  |
| taken | ready | person | clear |  |  |
| any-open | +needs-run | handoff | none | type::recurring |  |
| any-open | closed | track-goals | none |  | sprint-goal |
```
