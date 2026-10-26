---
id: objects
title: Objects of the process
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines every object the process uses by its relation to the issue, the only unit of work (design.md D4). Anything not on this list is not part of the process.

<!-- field: vocabulary -->
## Vocabulary

| term | definition | specified_in | relation |
| --- | --- | --- | --- |
| issue | A commitment to produce one thing. Carries a plane, a kind, a component, and a milestone. | srd001 |  |
| theme | A document in the system repository naming a strategic goal area for months to years. | design.md theme, srd045 | the parent of initiatives; groups initiatives and epics in reports; not itself a unit of work |
| initiative | A document in the system repository stating a multi-team outcome for several quarters, the program's Product Goal, in the shape of an SRD. Its outcome items are what Sprint Goals cite by pinned URL. An objective document is read as an initiative. | design.md initiative, srd034 | child of a theme; parent of epics; cited by Sprint Goals; not itself a unit of work |
| epic | A document holding a large body of work spanning several sprints, in the system repository when it spans teams or in a subsystem repository when one team owns it. | design.md epic, srd045 | child of an initiative; parent of features and stories; progress rolled up from their story points; not itself a unit of work |
| feature | An optional document in the owning team's repository holding a distinct capability within an epic, for two to four sprints. | design.md feature, srd045 | child of exactly one epic; parent of stories |
| story | A user story document in the owning team's repository: who wants what and why, its story points, its acceptance criteria, and the SRD requirement items they prove. Sized to fit one sprint. | design.md story, srd045 | child of a feature or an epic; selected onto a Sprint Goal; executed by issues with an Implements line; not itself a unit of work |
| sprint_goal | The issue tasks files for one team for one sprint, in the team's repository: the goal, the initiative and epic items it serves by pinned URL, and the stories its Sprint Planning selected. Its comment events are its ledger; it is met when every listed story's criteria have passing evidence, or, in a program that does not yet run track-goals, closed by the merge of its repository's sprint-end issue (srd038 R2). An objective issue is read as a Sprint Goal. | srd006, srd019, srd022, srd034 | an issue whose events record planned, evidence, and met |
| sprint_backlog | Not an object. The stories a Sprint Goal lists, with the issues that implement them. | srd034, srd045 | a view over a Sprint Goal and its stories |
| subsystem | A part of the system with one owner (a Product Owner), declared by a subsystem SRD in the system repository that lists its components. | design.md subsystem_srd | a team; receives a Sprint Goal per sprint; groups components |
| component | A repository path declared by a subsystem SRD, owned by Developers. Its name is a component:: label value. | srd001 R21, design.md subsystem_srd | every issue carries exactly one component label |
| environment | One stage of the chain (local, dev, staging, production PoC, customer R&D), declared by an environment SRD in the test-subsystem repository with its owner, trigger, and suites. | srd008, design.md environment_srd | named by every acceptance criterion and every evidence event |
| pinned_set | One release of each subsystem and of the test subsystem, promoted together along the environment chain. | srd008 | the version an evidence event records |
| evidence | An evidence event on a Sprint Goal: a named test's result in a named environment on a pinned set, for one story criterion named by pinned URL. The only input to done. | srd006, srd008 | a comment event on a Sprint Goal issue |
| task | A step inside one issue; a child work item. No labels of its own; never in the backlog. | srd001 R9 | belongs to exactly one issue |
| tracking_issue | An issue labelled type::tracking (type::epic is read as its alias) whose description lists member issues. Never worked directly. | srd001 R6 | an issue that contains references to issues |
| sprint | A three-week period ending in a road-map release; a group milestone and one type::sprint issue holding the exit checklist. | srd001 R7, srd031 | an issue plus the milestone every planned issue of the period carries |
| recurring_issue | A consumer's standing subscription to one artifact; closed and reopened per run. | srd001 R8 | an issue labelled type::recurring |
| audit | An issue with a Recurrence section, worked by the inner loop on a schedule or when a person triggers it; implement re-creates it after each run. Its findings are pushed as issues. | agents, srd018 R6 | an issue re-created per run |
| comment_event | A comment with a machine marker; information, never a commitment and never a state change. | srd006 | attached to exactly one issue |
| link | A vertical or horizontal relation, the agile set: Parent, Blocked-by and Blocks, Depends-on, Relates-to, Duplicates, and Implements, plus Upstream to a producing issue, Requires to a release, and Cites to a requirement or interface pinned to a tag. Issues carry links as body lines, hierarchy documents as context items. | srd007, design.md hierarchy_links | between issues, documents, tags, and pinned file anchors |
| release_tag | A version tag on the producing repository, created only after the artifact's gate passed. | srd008, srd015 | created by a kind::release issue; read by Requires lines and by handoff |
| merge_request | The only way an issue is closed as delivered; closes its issue and that issue's tasks. | srd041 R7, srd008 | closes exactly one issue |
| product_backlog | Not an object. The agreed stories, with the features and epics above them, in the order the owner of the level sets: the Product Owner orders a team's, the technical program manager the program's initiatives and epics. plan's ranking proposes and the owner decides. | srd045, srd031, docs/guides/agile-mapping.md | the stories Sprint Planning selects from |
| backlog | Not an object. The query for open, unassigned, admitted, backlog-kind data issues (the ready state); the Product Backlog is product_backlog. | srd041 R3 | a view over issues |
| dashboard | Not an object. A rendering of the ledger and evidence committed to a repository; never edited. | srd019 | a view over issues |

<!-- field: taxonomy_rules -->
## Taxonomy rules

Two tiers. Planning is a hierarchy of documents: theme, initiative, epic, an optional feature, and story, each with at most one parent, linked by context items and rolled up through Parent. Execution is flat in the tracker: the issue that implements a story, its tasks, and tracking issues that group issues. Levels (system, subsystem, component) are repositories, not issue types; they connect through pinned links and Sprint Goals. Dependencies cross-cut the hierarchy through Blocked-by, Depends-on, and Relates-to; events attach to issues; sprints group issues by milestone.
