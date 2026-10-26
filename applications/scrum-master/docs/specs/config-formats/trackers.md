---
id: trackers
title: Tracking systems
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Names the tracking system each repository of a program uses: bd, whose issues live in a tracker repository; gh, GitHub's issues and pull requests; or glab, GitLab's issues and merge requests. A tracking system is one package: the issue record, how a branch is reviewed and merged, and where the records go. Every change to a repository's files goes through the same loop whichever system serves it (srd042). The file lives in the system repository at control/trackers.md; scrum-master reads it from the checkout it runs in, else from the one sibling checkout that holds it; worktrees of one repository, as pop opens beside a checkout, count as that one sibling, read from its main checkout. A change to it is a plane::control issue and a reviewed merge.

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

<!-- schema_entry: group -->
### group

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The program group: where scrum-master init creates the program's labels and the issue prefix comes from, and where plan push dates each release's milestone; issue labels and plan push default to it. scrum-master init writes the one --group names. Without it, plan push dates milestones in the system project's parent group.

<!-- schema_entry: default.system -->
### default.system

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracking system of every repository no override names: bd, gh, or glab. SCRUM_MASTER_TRACKER replaces it for one person's run, never an override: a git URL or path names bd with that tracker repository, and gh or glab names that system.

<!-- schema_entry: default.tracker_repository -->
### default.tracker_repository

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The tracker repository's URL when the default is bd; scrum-master init writes the one it seeds.

<!-- schema_entry: default.tracker_branch -->
### default.tracker_branch

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The branch of the tracker repository that holds .beads/ when the default is bd; main when absent. A program of one repository names a dedicated branch of its own repository, such as scrum-master/tracker, which is never merged into its other branches. scrum-master init writes the one it seeds, and SCRUM_MASTER_TRACKER_BRANCH replaces it for one person's run.

<!-- schema_entry: overrides -->
### overrides

<!-- field: type -->
#### Type

tree of path: system

<!-- field: description -->
#### Description

A project or group path and the system serving it, gh or glab: a group's entry serves every project under it that no closer entry names. A path appears once, so a repository has exactly one tracking system and project#iid names one issue.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: trackers
title: Tracking systems
---

<!-- field: version -->
## Version

1

<!-- field: group -->
## Group

acme

<!-- field: default -->
## Default

<!-- field: system -->
### System

bd

<!-- field: tracker_repository -->
### Tracker repository

https://github.com/acme/program-tracker.git

<!-- field: tracker_branch -->
### Tracker branch

main

<!-- field: overrides | form: "tree" -->
## Overrides

- acme/delivery/fleet: gh
- acme/scoring: glab
```
