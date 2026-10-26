---
id: person-config
title: Person configuration
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Holds what one person tells the scripts about themselves: which tracker and group they work in and where the system repository lives. Only the person edits it. Nothing in it assigns work or roles; a person's level and ownership are declared in the SRDs they own.

<!-- field: schema -->
## Schema

<!-- schema_entry: host -->
### host

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracker host; the token comes from the tracker CLI's existing login for this host.

<!-- schema_entry: group -->
### group

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracker group holding the program's projects.

<!-- schema_entry: program_project -->
### program_project

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The project path of the system repository, where the policy, the label schema, and the subsystem SRDs live.

<!-- schema_entry: tracker -->
### tracker

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The project holding the person's Sprint Goals and ad-hoc tasks, when the person keeps them apart from component repositories. Optional.

<!-- schema_entry: state.last_run -->
### state.last_run

<!-- field: type -->
#### Type

timestamp

<!-- field: description -->
#### Description

Written by the standup subcommands, never by the person: when the morning run last completed, so inbox and unblocked report what happened since.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: person
title: Person configuration
---

<!-- field: config_files -->
## Config files

person.md in the person's own configuration directory, outside every repository

<!-- field: host -->
## Host

gitlab.example.com

<!-- field: group -->
## Group

group

<!-- field: program_project -->
## Program project

group/system

<!-- field: tracker -->
## Tracker

group/tasks

<!-- field: state -->
## State

<!-- field: last_run -->
### Last run

2026-10-12 08:30:00+00:00
```
