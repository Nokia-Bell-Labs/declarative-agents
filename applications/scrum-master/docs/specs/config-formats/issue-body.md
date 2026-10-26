---
id: issue-body
title: Issue body conventions
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines the lines and sections scripts read from or write to an issue description or comment. The three work sections (Requirements, Acceptance Criteria, Design Decisions) are the issue body's work format: every issue an outer-loop skill pushes carries them, and implement builds against them whatever the issue's kind. Everything else in an issue body is free text for people. Each convention has one syntax so a script can find it with a line match.

<!-- field: schema -->
## Schema

<!-- schema_entry: upstream_line -->
### upstream_line

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Upstream: PROJECT#IID" on its own line in a description. The issue that produced what this issue consumes (srd007 R1). At most one.

<!-- schema_entry: requires_line -->
### requires_line

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Requires: ARTIFACT >= VERSION" on its own line in a description, with VERSION in semantic-version form (srd007 R2). Zero or more.

<!-- schema_entry: cites_line -->
### cites_line

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Cites: URL" on its own line in a description, where URL is a cross-repository reference pinned to a release tag with a requirement or interface anchor (srd007 R5). Zero or more. A Sprint Goal issue carries one per initiative or epic item it serves. A Cites line may end in " | review" or " | handoff", asking for a review or a handoff decision when a new tag of the cited interface is released (srd014 R9.1, from 06.0).

<!-- schema_entry: link_lines -->
### link_lines

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Blocked-by: TARGET", "Blocks: TARGET", "Depends-on: TARGET", "Relates-to: TARGET", and "Duplicates: TARGET", each on its own line in a description (srd007 R6). TARGET is PROJECT#IID, a same-repository document id, or a pinned URL. Blocks and Blocked-by are the two ends of one link; Blocked-by makes an issue waiting, Depends-on and Relates-to never do. At most one Duplicates line.

<!-- schema_entry: implements_line -->
### implements_line

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Implements: STORY" on its own line in a description, naming the story the issue executes by same-repository id or pinned URL (srd007 R6.5). tasks writes it on every issue it creates from a story. At most one.

<!-- schema_entry: sprint_backlog_section -->
### sprint_backlog_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Sprint Backlog" heading on a Sprint Goal issue followed by one line per story its Sprint Planning selected, "- STORY title (N points)", where STORY is a same-repository id or a pinned URL (srd034). plan writes it; the planned event names the same stories.

<!-- schema_entry: environment_line -->
### environment_line

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Environment: NAME" on its own line in a Sprint Goal issue's description, naming the environment SRD where its evidence counts. Exactly one on a Sprint Goal issue.

<!-- schema_entry: downstream_section -->
### downstream_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Downstream" heading followed by "artifact: NAME" lines naming consumption-map artifacts this issue produces, and optional "review: component::VALUE TEXT" lines asking a component to review the result. Read by handoff on merge.

<!-- schema_entry: requirements_section -->
### requirements_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Requirements" heading followed by a list of what the deliverable must do or contain. Part of the work format; required by admission on every issue an outer-loop skill pushes.

<!-- schema_entry: acceptance_criteria_section -->
### acceptance_criteria_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Acceptance Criteria" heading followed by a list of checkable conditions, each naming the test that proves it and the environment where it counts, as "(test: T; environment: E)" (design.md D8). Part of the work format; the unit is not done until each holds. Required by admission on every issue an outer-loop skill pushes, and on build, experiment, research, and spec issues when policy says so.

<!-- schema_entry: design_decisions_section -->
### design_decisions_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Design Decisions" heading followed by a list of choices already made and their reasons. Part of the work format; optional, present when a choice constrains how the deliverable is built.

<!-- schema_entry: notes_section -->
### notes_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Notes" heading on a Sprint Goal issue, filed empty by tasks and filled by the owner with the details of the work as it proceeds (srd034 R3.3). Free text; no script reads or writes it.

<!-- schema_entry: estimate_line -->
### estimate_line

<!-- field: type -->
#### Type

line

<!-- field: description -->
#### Description

"Estimated LOC: N" on its own line. The completion comment records "Actual LOC: N" against it, posted with scrum-master issue comment.

<!-- schema_entry: handoff_marker -->
### handoff_marker

<!-- field: type -->
#### Type

html comment

<!-- field: description -->
#### Description

"<!-- handoff reference-update from=REPO tag=TAG -->" in the description of a reference-update issue, naming the downstream repository and the tag that caused it (srd014 R8.3); from 06.0 the other issues handoff creates carry "<!-- handoff: PROJECT!MR -->" naming the merge or "<!-- handoff: PROJECT@TAG -->" naming the tag (srd014 R9.1). Makes handoff idempotent (srd014 R5).

<!-- schema_entry: event_marker -->
### event_marker

<!-- field: type -->
#### Type

html comment

<!-- field: description -->
#### Description

"<!-- scrum-master program event=NAME key=value ... -->" in a comment (srd006 R1). Exactly one per event comment.

<!-- schema_entry: ledger_events -->
### ledger_events

<!-- field: type -->
#### Type

html comment

<!-- field: description -->
#### Description

Event markers that make a Sprint Goal issue its own ledger (srd006 R8–R11): "<!-- scrum-master program event=planned release=R requirement=URL story=REF environment=E -->", "<!-- scrum-master program event=evidence criterion=URL test=T environment=E version=V result=pass|fail -->", and "<!-- scrum-master program event=met release=R version=V -->".

<!-- schema_entry: previous_runs_section -->
### previous_runs_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Previous Runs" heading on a recurring issue followed by one line per run, "- PROJECT#IID (YYYY-MM-DD): summary", appended by recur (srd001 R8.3).

<!-- schema_entry: recurrence_section -->
### recurrence_section

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Recurrence" heading on a recurring issue (srd001 R8), saying when the next copy is filed and by what: for a weekly status issue the reports repository's schedule (srd018 R9), for a sprint-end issue scrum-master plan push for the next period (srd038 R1.1).

<!-- schema_entry: tracking_task_list -->
### tracking_task_list

<!-- field: type -->
#### Type

section

<!-- field: description -->
#### Description

A "## Members" heading on a tracking issue followed by task-list lines, "- [ ] PROJECT#IID title" (srd001 R6.1). Handoff appends to it; the tracker renders closed members as done.

<!-- field: config_files -->
## Config files

issue descriptions and comments in the program's tracker group

<!-- field: example -->
## Example

```yaml
|
    Update the reader to the new schema version

    Upstream: group/alpha#41
    Requires: shared-schema >= 0.2.0
    Cites: https://gitlab.example.com/group/system/-/blob/v1.4.0/docs/specs/software-requirements/srd003-ingest.md#R2.1
    Environment: dev

    ## Requirements

    - The reader accepts records at schema 0.2.0
    - The reader keeps accepting records at schema 0.1.0

    ## Acceptance Criteria

    - The reader parses records written at 0.2.0 (test: reader_parses_v020; environment: dev)
    - Records written at 0.1.0 still parse (test: reader_parses_v010; environment: dev)

    ## Design Decisions

    - Dispatch on the record's schema field; no format sniffing

    Estimated LOC: 120

    ## Downstream

    artifact: reader-api
    review: component::architecture Confirm the reader keeps the interface

    <!-- handoff: group/alpha@shared-schema/v0.2.0 -->
```
