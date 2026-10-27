---
id: program
title: Program lineage
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Describes a program and its place among programs, so a program of programs is served by the same site at every level (srd048 R8.7, GH-2740). A program that is a team of a larger one is declared there by the larger program's subsystem SRD, whose scrum_repository names this program's system repository; this file says where this program's site is, so the parent can read and link it, and who the parent is, so this program's site links up to it. Every field is optional, and a program without the file stands alone. It is owned by the technical program manager, and a change to it is a plane::control issue and a reviewed merge.

<!-- field: schema -->
## Schema

<!-- schema_entry: site -->
### site

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

This program's site URL, as a browser opens it (the Pages URL of its site repository). A parent program links this program's tile, page, and the documents it serves to it.

<!-- schema_entry: site_repository -->
### site_repository

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The repository holding this program's site, its reports repository, as a project path or URL. A parent program reads this program's published plan, site/data/hierarchy/hierarchy.json, from it on its default branch, to count the documents this program serves its plan with.

<!-- schema_entry: parent -->
### parent

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The parent program's system repository, as a project path or URL, when this program is a team of a larger one. Its site names the parent first in the navigator and on the overview, and gathers the documents whose parent pins a parent's document under a band naming it.

<!-- schema_entry: parent_site -->
### parent_site

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The parent program's site URL. An Up link to a parent's document opens its page there; without it the link opens the pinned document on the forge.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: program
title: Program
---

<!-- field: config_files -->
## Config files

control/program.md in the program repository

<!-- field: site -->
## Site

https://eval.pages.example.com

<!-- field: site_repository -->
## Site repository

acme/eval/eval-reports

<!-- field: parent -->
## Parent

https://gitlab.example.com/acme/portfolio

<!-- field: parent_site -->
## Parent site

https://portfolio.pages.example.com
```
