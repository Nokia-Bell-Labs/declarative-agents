---
id: people
title: People on scrums and epics
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Names who works on which scrum and epic, so the site can show them (srd048 R8.8), and who signs off on which scrum and document (srd013 R11.1). Being on a scrum or epic assigns nothing: no issue gets an assignee, no status issue is filed, and no label changes because of it (srd045 R2.5). Reviewing one holds its merges to that person's approval (srd013 R11.2). It is separate from the reports roster (config format reports-config), because everyone the roster lists gets a weekly status issue and a program may list only its leads there. It is control/people.md in the system repository, owned by the technical program manager; a program that keeps none has no people recorded, and the site says so.

<!-- field: schema -->
## Schema

<!-- schema_entry: people -->
### people

<!-- field: type -->
#### Type

list of mapping

<!-- field: description -->
#### Description

One row per person: username (tracker username), scrums (the scrums they are on, comma-separated, each as the site names it, its scrum repository's path below the program's group), epics (the epics they are on, comma-separated, each as the site names it, a team's as name/id and the system's by its id alone), and reviews (the scrums, stories, features, epics, and SRDs they sign off on, comma-separated, named the same way; each has one reviewer). A person on nothing has no row. A name that resolves to nothing is a note in the timeline facts and is left off the pages. scrum-master people allocate, release, review, and unreview write it; a person may edit it too.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: people
title: People
---

<!-- field: people -->
## People

| username | scrums | epics | reviews |
| --- | --- | --- | --- |
| alice | platform | ep001-agent-evaluation, platform/ep002-run-records | platform/ep002-run-records |
| bob | platform, serving | ep001-agent-evaluation | ep001-agent-evaluation, srd045-backlog |
```
