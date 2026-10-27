---
id: ai-native-scrum-specifications
title: AI-Native Scrum Specification Index
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Specification index for the AI-native scrum process. The requirements are
complete: 27 SRDs, six config formats, and four semantic models. srd001 to
srd008 define the control plane, the Sprint Goal ledger, pinned references,
and done as evidence in a named environment; srd010 to srd019 define one
script each as a scrum-master subcommand group and contain no question to a person,
srd019 being the track-goals audit; srd020 to srd033 define the five
outer-loop skills, each stating what it reads, what it asks, and which
issues it pushes or events it posts. srd034 makes a Sprint Goal a document
in the system repository before it becomes an issue and files the dated
Sprint Goals; the weekly status issue and the release summary are
srd018's, the sprint-end issue that pushes the release is srd038's, and
the reference-update issue on a downstream release is srd014's. srd035 and srd036 specify the
inner loop on GitLab: specify files one admitted issue whose plane says
whether it is project work or a requirements change, and implement takes
it into its own branch and worktree until its merge closes it.
srd037 is the Agent Plugins 1.0 package of the skills and scrum-master install,
which writes the skills embedded in scrum-master for Cursor, Claude Code, or
GitHub Copilot, and with --runtime fetches the declarative-agents
runtime at the release scrum-master pins. srd039 is the shared reader of corpus and config
documents (internal/corpus), srd040 the format tools and format gates in
magefiles, and srd041 the states and transitions of an issue. srd043 is
scrum-master init and scrum-master onboard, which lay down a program's corpus, and scrum-master
doctor, which brings it up to date after scrum-master is upgraded, and srd044
help, which taught the process from the binary and is now retired: the README init writes carries its process map (GH-37). srd046 is the draft skill, which turns a description into one planning document at any level in conversation with its owner. srd045 makes an epic document the Product Backlog for multi-sprint work, from which plan derives Sprint Goals at a velocity read from the ledger, and holds direction only until 06.0 is planned. srd039 and
srd040 specify shared code and tooling no use case cites, so the road map
lists them under the release that built them. An SRD whose release is
not yet planned holds its direction only (design.md D12): its items are
retired, and an interim rule says what scrum-master does until they are rewritten.
The skills take Spec Kit's names (specify, plan, tasks, implement): tasks
breaks a level into the units below it, down to a story's issues, and
implement builds them. Every repository of a consuming program carries the same
corpus shape (design.md D7). Release 00 (00.0 to 00.3), releases 01.0 to 01.2 and 01.4, 03.0, 03.1, and 04.0 to 04.5 are implemented; 05.0 and 06.0 are specified. Release 03.1 lets each repository name its tracking system, bd, gh, or glab. Release 04.0 aligns the planning documents with the agile hierarchy and renames the objective issue the Sprint Goal. Release 04.1 makes a program repository explain itself and adds scrum-master doctor. Release 04.2 adds the draft skill and scrum-master backlog list and check. Release 04.3 ships scrum-master as release binaries and a container image (eng03) and adds scrum-master install --runtime and doctor's runtime report. Release 04.4 works a story end to end in one worktree and one merge. Release 04.5 keeps weekly status where the work is: reports and rollups written in place, collected per team and for the program, and the status-report and status-rollup skills.

Releases are planned by use case, and a use case cites only the
requirement items its release delivers. Release 00 is the inner loop
on GitLab in four dot releases: 00.0 labels and push, 00.1 pop and merge,
00.2 changes under docs/ through the audit gate, 00.3 scrum-master install. Release 01 builds the
interaction between the technical program manager and the Product
Owners in three dot releases: 01.0 Sprint Goals reach the Product Owner, 01.1 status comes back
weekly, 01.2 the sprint end releases and plans the next period, and 01.4
the agent-legible CLI: help (since retired), the issue group, and scrum-master init and
onboard. A release's major number names its theme, not its order (the
road map lists the build order). 05.0, numbered 01.3 until 2026-10-22,
holds tags that trigger handoff and the summary; 05.1 the generated status and timeline site (srd048), without use cases yet; 06.0, numbered 02.0,
holds evidence, handoff, and cadence, without use cases yet. Every SRD is listed
under the first release whose use case touches it. See
docs/road-map.md.

<!-- field: roadmap_summary | form: "typed table" -->
## Roadmap summary

| version | name | use_cases_done | use_cases_total | status |
| --- | --- | --- | --- | --- |
| "00.0" | Labels and push | 1 | 1 | implemented |
| "00.1" | Pop and merge | 1 | 1 | implemented |
| "00.2" | Changes under docs/ | 2 | 2 | implemented |
| "00.3" | Install the skills | 1 | 1 | implemented |
| "01.0" | Sprint Goals reach the Product Owner | 2 | 2 | implemented |
| "01.1" | Status comes back weekly | 2 | 2 | implemented |
| "01.2" | Sprint end closes the loop | 2 | 2 | implemented |
| "01.4" | The agent-legible CLI | 2 | 2 | implemented |
| "03.0" | The tracker becomes beads | 2 | 2 | implemented |
| "03.1" | One tracking system per repository | 1 | 1 | implemented |
| "04.0" | The agile hierarchy | 3 | 3 | implemented |
| "04.1" | A repository that explains itself | 1 | 1 | implemented |
| "04.2" | A planning document drafted in conversation | 1 | 1 | implemented |
| "04.3" | scrum-master ships to machines and to CI | 2 | 2 | implemented |
| "04.4" | Work a story end to end | 1 | 1 | implemented |
| "04.5" | Status reports in place | 2 | 2 | implemented |
| "05.0" | Tags trigger handoff and the summary | 0 | 2 | specified |
| "05.1" | The generated status and timeline site | 0 | 0 | specified |
| "06.0" | Evidence, handoff, and cadence | 0 | 0 | specified |

<!-- field: foundation_document_index -->
## Foundation document index

| id | path |
| --- | --- |
| [ai-native-scrum](VISION.md) | docs/VISION.md |
| [architecture-ai-native-scrum](ARCHITECTURE.md) | docs/ARCHITECTURE.md |
| [design](constitutions/design.md) | docs/constitutions/design.md |
| [ai-native-scrum-roadmap](road-map.md) | docs/road-map.md |
| [eng01-regression-rig](engineering/eng01-regression-rig.md) | docs/engineering/eng01-regression-rig.md |
| [eng02-tpm-and-skills](engineering/eng02-tpm-and-skills.md) | docs/engineering/eng02-tpm-and-skills.md |
| [eng03-releasing-scrum-master](engineering/eng03-releasing-scrum-master.md) | docs/engineering/eng03-releasing-scrum-master.md |
| [eng04-beads-sync](engineering/eng04-beads-sync.md) | docs/engineering/eng04-beads-sync.md |
| agile-mapping | docs/guides/agile-mapping.md |
| ai-native-sdlc | docs/guides/ai-native-sdlc.md |
| corpus-audit | docs/guides/corpus-audit.md |
| plan-vs-architecture | docs/guides/plan-vs-architecture.md |
| process | docs/guides/process.md |
| sdlc-playbook | docs/guides/sdlc-playbook.md |
| spec-kit | docs/guides/spec-kit.md |
| tut01-start-a-program | docs/tutorials/tut01-start-a-program.md |
| tut02-file-and-deliver-an-issue | docs/tutorials/tut02-file-and-deliver-an-issue.md |
| tut03-plan-a-release-and-run-the-sprint | docs/tutorials/tut03-plan-a-release-and-run-the-sprint.md |
| tut04-write-the-backlog | docs/tutorials/tut04-write-the-backlog.md |
| tut05-weekly-status | docs/tutorials/tut05-weekly-status.md |

<!-- field: srd_index -->
## SRD index

| id | path | title | release |
| --- | --- | --- | --- |
| [srd001-labels](specs/software-requirements/srd001-labels.md) | docs/specs/software-requirements/srd001-labels.md | The label scheme: plane, type, kind, component, and state | 00.0 |
| [srd006-events](specs/software-requirements/srd006-events.md) | docs/specs/software-requirements/srd006-events.md | Comment events | 01.0 |
| [srd007-links](specs/software-requirements/srd007-links.md) | docs/specs/software-requirements/srd007-links.md | Dependency links | 01.0 |
| [srd008-gates](specs/software-requirements/srd008-gates.md) | docs/specs/software-requirements/srd008-gates.md | Definition of done: evidence in a named environment | 06.0 |
| [srd010-standup](specs/software-requirements/srd010-standup.md) | docs/specs/software-requirements/srd010-standup.md | Script: standup (scrum-master standup ...) | 01.1 |
| [srd013-program](specs/software-requirements/srd013-program.md) | docs/specs/software-requirements/srd013-program.md | Script: issue (scrum-master issue ...) | 00.0 |
| [srd014-handoff](specs/software-requirements/srd014-handoff.md) | docs/specs/software-requirements/srd014-handoff.md | Script: handoff (scrum-master handoff ...) | 05.0 |
| [srd015-release](specs/software-requirements/srd015-release.md) | docs/specs/software-requirements/srd015-release.md | Script: release (scrum-master release ...) | 01.2 |
| [srd018-reports](specs/software-requirements/srd018-reports.md) | docs/specs/software-requirements/srd018-reports.md | Script: reports (scrum-master reports ...) | 01.1 |
| [srd019-track-goals](specs/software-requirements/srd019-track-goals.md) | docs/specs/software-requirements/srd019-track-goals.md | Script: track-goals (scrum-master goals ...) | 06.0 |
| [srd020-daily-scrum](specs/software-requirements/srd020-daily-scrum.md) | docs/specs/software-requirements/srd020-daily-scrum.md | Skill: daily-scrum | 06.0 |
| [srd022-tasks](specs/software-requirements/srd022-tasks.md) | docs/specs/software-requirements/srd022-tasks.md | Skill: tasks | 01.0 |
| [srd031-plan](specs/software-requirements/srd031-plan.md) | docs/specs/software-requirements/srd031-plan.md | Skill: plan | 01.2 |
| [srd032-sprint-review](specs/software-requirements/srd032-sprint-review.md) | docs/specs/software-requirements/srd032-sprint-review.md | Skill: sprint-review | 06.0 |
| [srd033-sprint-retrospective](specs/software-requirements/srd033-sprint-retrospective.md) | docs/specs/software-requirements/srd033-sprint-retrospective.md | Skill: sprint-retrospective | 06.0 |
| [srd034-initiatives](specs/software-requirements/srd034-initiatives.md) | docs/specs/software-requirements/srd034-initiatives.md | Initiatives and the Sprint Goal loop | 01.0 |
| [srd035-specify](specs/software-requirements/srd035-specify.md) | docs/specs/software-requirements/srd035-specify.md | Skill: specify | 00.0 |
| [srd036-implement](specs/software-requirements/srd036-implement.md) | docs/specs/software-requirements/srd036-implement.md | Skill: implement | 00.1 |
| [srd037-install](specs/software-requirements/srd037-install.md) | docs/specs/software-requirements/srd037-install.md | Script: install (scrum-master install) | 00.3 |
| [srd038-sprint-end](specs/software-requirements/srd038-sprint-end.md) | docs/specs/software-requirements/srd038-sprint-end.md | Skill: sprint-end | 01.2 |
| [srd039-corpus](specs/software-requirements/srd039-corpus.md) | docs/specs/software-requirements/srd039-corpus.md | Shared: corpus and config documents (internal/corpus) | 01.0 |
| [srd040-format-tools](specs/software-requirements/srd040-format-tools.md) | docs/specs/software-requirements/srd040-format-tools.md | Tooling: mage convert, fmt, specCheck, and the format gates | 01.2 |
| [srd041-states](specs/software-requirements/srd041-states.md) | docs/specs/software-requirements/srd041-states.md | States and transitions of an issue | 00.1 |
| [srd042-beads-tracker](specs/software-requirements/srd042-beads-tracker.md) | docs/specs/software-requirements/srd042-beads-tracker.md | Beads tracker: the program's facts live in a repository | 03.0 |
| [srd043-init-onboard](specs/software-requirements/srd043-init-onboard.md) | docs/specs/software-requirements/srd043-init-onboard.md | Script: init, onboard, and doctor (scrum-master init, scrum-master onboard, scrum-master doctor) | 01.4 |
| [srd044-help](specs/software-requirements/srd044-help.md) | docs/specs/software-requirements/srd044-help.md | Script: help | 01.4 |
| [srd045-backlog](specs/software-requirements/srd045-backlog.md) | docs/specs/software-requirements/srd045-backlog.md | The backlog hierarchy: themes, initiatives, epics, features, and stories | 04.0 |
| [srd046-draft](specs/software-requirements/srd046-draft.md) | docs/specs/software-requirements/srd046-draft.md | Skill: draft | 04.2 |
| [srd047-design](specs/software-requirements/srd047-design.md) | docs/specs/software-requirements/srd047-design.md | Skill: design | 06.0 |
| [srd048-site](specs/software-requirements/srd048-site.md) | docs/specs/software-requirements/srd048-site.md | Skill: site | 05.1 |

<!-- field: config_format_index -->
## Config format index

| id | path | title |
| --- | --- | --- |
| [report](specs/config-formats/report.md) | docs/specs/config-formats/report.md | Weekly status report |
| [reports-config](specs/config-formats/reports-config.md) | docs/specs/config-formats/reports-config.md | Reports roster |
| [rollup](specs/config-formats/rollup.md) | docs/specs/config-formats/rollup.md | Status rollup |
| [issue-body](specs/config-formats/issue-body.md) | docs/specs/config-formats/issue-body.md | Issue body conventions |
| [labels](specs/config-formats/labels.md) | docs/specs/config-formats/labels.md | Label schema |
| [person-config](specs/config-formats/person-config.md) | docs/specs/config-formats/person-config.md | Person configuration |
| [policy](specs/config-formats/policy.md) | docs/specs/config-formats/policy.md | Process policy |
| [program](specs/config-formats/program.md) | docs/specs/config-formats/program.md | Program lineage |
| [states](specs/config-formats/states.md) | docs/specs/config-formats/states.md | Issue states |
| [trackers](specs/config-formats/trackers.md) | docs/specs/config-formats/trackers.md | Tracking systems |

<!-- field: semantic_model_index -->
## Semantic model index

| id | path | title |
| --- | --- | --- |
| [agents](specs/semantic-models/agents.md) | docs/specs/semantic-models/agents.md | Agents and the loops they run |
| [objects](specs/semantic-models/objects.md) | docs/specs/semantic-models/objects.md | Objects of the process |
| [planes](specs/semantic-models/planes.md) | docs/specs/semantic-models/planes.md | The three planes |
| [roles](specs/semantic-models/roles.md) | docs/specs/semantic-models/roles.md | Roles and levels |

<!-- field: use_case_index -->
## Use case index

| id | title | release | status | test_suite | path |
| --- | --- | --- | --- | --- | --- |
| [rel00.0-uc001-push-project-work](specs/use-cases/rel00.0-uc001-push-project-work.md) | A Developer Pushes a Request and It Becomes an Admitted Issue | 00.0 | implemented | [test-rel00.0-push](specs/test-suites/test-rel00.0-push.md) | docs/specs/use-cases/rel00.0-uc001-push-project-work.md |
| [rel00.1-uc001-pop-and-merge](specs/use-cases/rel00.1-uc001-pop-and-merge.md) | A Developer Pops an Issue into a Worktree and Its Merge Closes It | 00.1 | implemented | [test-rel00.1-pop](specs/test-suites/test-rel00.1-pop.md) | docs/specs/use-cases/rel00.1-uc001-pop-and-merge.md |
| [rel00.2-uc001-spec-change](specs/use-cases/rel00.2-uc001-spec-change.md) | A Developer Changes a Requirement During Design Through the Audit Gate | 00.2 | implemented | [test-rel00.2-docs-change](specs/test-suites/test-rel00.2-docs-change.md) | docs/specs/use-cases/rel00.2-uc001-spec-change.md |
| [rel00.2-uc002-planning-change](specs/use-cases/rel00.2-uc002-planning-change.md) | A Lead Changes the Plan Through a Management Issue and the Same Gate | 00.2 | implemented | [test-rel00.2-docs-change](specs/test-suites/test-rel00.2-docs-change.md) | docs/specs/use-cases/rel00.2-uc002-planning-change.md |
| [rel00.3-uc001-install-skills](specs/use-cases/rel00.3-uc001-install-skills.md) | A Developer Installs the Skills for Their Agent | 00.3 | implemented | [test-rel00.3-install](specs/test-suites/test-rel00.3-install.md) | docs/specs/use-cases/rel00.3-uc001-install-skills.md |
| [rel01.0-uc001-author-objectives](specs/use-cases/rel01.0-uc001-author-objectives.md) | The Technical Program Manager Writes an Audited Initiative | 01.0 | implemented | [test-rel01.0-objectives](specs/test-suites/test-rel01.0-objectives.md) | docs/specs/use-cases/rel01.0-uc001-author-objectives.md |
| [rel01.0-uc002-push-objective-issues](specs/use-cases/rel01.0-uc002-push-objective-issues.md) | Create-Goals Files Dated Sprint Goals for the Product Owners | 01.0 | implemented | [test-rel01.0-objectives](specs/test-suites/test-rel01.0-objectives.md) | docs/specs/use-cases/rel01.0-uc002-push-objective-issues.md |
| [rel01.1-uc001-weekly-status](specs/use-cases/rel01.1-uc001-weekly-status.md) | A Product Owner Reports Weekly Status Through a Dated Issue | 01.1 | implemented | [test-rel01.1-weekly-status](specs/test-suites/test-rel01.1-weekly-status.md) | docs/specs/use-cases/rel01.1-uc001-weekly-status.md |
| [rel01.1-uc002-period-rollup](specs/use-cases/rel01.1-uc002-period-rollup.md) | The Technical Program Manager Aggregates a Period of Status | 01.1 | implemented | [test-rel01.1-weekly-status](specs/test-suites/test-rel01.1-weekly-status.md) | docs/specs/use-cases/rel01.1-uc002-period-rollup.md |
| [rel01.2-uc001-sprint-end-release](specs/use-cases/rel01.2-uc001-sprint-end-release.md) | A Sprint-End Issue Summarizes the Period, Asks, and Releases | 01.2 | implemented | [test-rel01.2-sprint-end](specs/test-suites/test-rel01.2-sprint-end.md) | docs/specs/use-cases/rel01.2-uc001-sprint-end-release.md |
| [rel01.2-uc002-plan-next-period](specs/use-cases/rel01.2-uc002-plan-next-period.md) | The Technical Program Manager Plans the Next Period's Sprint Goals | 01.2 | implemented | [test-rel01.2-sprint-end](specs/test-suites/test-rel01.2-sprint-end.md) | docs/specs/use-cases/rel01.2-uc002-plan-next-period.md |
| [rel01.4-uc001-bootstrap-a-program](specs/use-cases/rel01.4-uc001-bootstrap-a-program.md) | The Technical Program Manager Starts a Program and a Repository Joins It | 01.4 | implemented | [test-rel01.4-agent-legible-cli](specs/test-suites/test-rel01.4-agent-legible-cli.md) | docs/specs/use-cases/rel01.4-uc001-bootstrap-a-program.md |
| [rel01.4-uc002-agent-learns-the-cli](specs/use-cases/rel01.4-uc002-agent-learns-the-cli.md) | An Agent Learns the Process from tpm and Works an Issue | 01.4 | implemented | [test-rel01.4-agent-legible-cli](specs/test-suites/test-rel01.4-agent-legible-cli.md) | docs/specs/use-cases/rel01.4-uc002-agent-learns-the-cli.md |
| [rel03.0-uc001-pop-and-close-on-beads](specs/use-cases/rel03.0-uc001-pop-and-close-on-beads.md) | An Issue Is Popped, Delivered, and Closed Against Beads | 03.0 | implemented | [test-rel03.0-beads-tracker](specs/test-suites/test-rel03.0-beads-tracker.md) | docs/specs/use-cases/rel03.0-uc001-pop-and-close-on-beads.md |
| [rel03.0-uc002-two-clones-synchronize](specs/use-cases/rel03.0-uc002-two-clones-synchronize.md) | Two Clones Synchronize Through the Tracker Repository | 03.0 | implemented | [test-rel03.0-beads-tracker](specs/test-suites/test-rel03.0-beads-tracker.md) | docs/specs/use-cases/rel03.0-uc002-two-clones-synchronize.md |
| [rel03.1-uc001-choose-the-tracking-system](specs/use-cases/rel03.1-uc001-choose-the-tracking-system.md) | A Program Names Each Repository's Tracking System and Works Through It | 03.1 | implemented | [test-rel03.1-tracking-systems](specs/test-suites/test-rel03.1-tracking-systems.md) | docs/specs/use-cases/rel03.1-uc001-choose-the-tracking-system.md |
| [rel04.0-uc001-author-the-hierarchy](specs/use-cases/rel04.0-uc001-author-the-hierarchy.md) | The Technical Program Manager and a Product Owner Write the Backlog Hierarchy Through the Gate | 04.0 | implemented | [test-rel04.0-agile-hierarchy](specs/test-suites/test-rel04.0-agile-hierarchy.md) | docs/specs/use-cases/rel04.0-uc001-author-the-hierarchy.md |
| [rel04.0-uc002-plan-a-sprint-from-stories](specs/use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md) | A Product Owner Plans a Sprint From the Ranked Stories and the Sprint Goal Lists Them | 04.0 | implemented | [test-rel04.0-agile-hierarchy](specs/test-suites/test-rel04.0-agile-hierarchy.md) | docs/specs/use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md |
| [rel04.0-uc003-sprint-goal-ledger](specs/use-cases/rel04.0-uc003-sprint-goal-ledger.md) | A Sprint Goal Closes at Sprint End With Its Stories Reported, and Old Objective Issues Read as Sprint Goals | 04.0 | implemented | [test-rel04.0-agile-hierarchy](specs/test-suites/test-rel04.0-agile-hierarchy.md) | docs/specs/use-cases/rel04.0-uc003-sprint-goal-ledger.md |
| [rel04.1-uc001-orient-and-repair](specs/use-cases/rel04.1-uc001-orient-and-repair.md) | A First-Time Reader Orients in a Program Repository and an Upgraded tpm Brings It Up to Date | 04.1 | implemented | [test-rel04.1-orientation-doctor](specs/test-suites/test-rel04.1-orientation-doctor.md) | docs/specs/use-cases/rel04.1-uc001-orient-and-repair.md |
| [rel04.2-uc001-draft-a-planning-document](specs/use-cases/rel04.2-uc001-draft-a-planning-document.md) | An Owner Drafts a Planning Document from a Description in Conversation | 04.2 | implemented | [test-rel04.2-draft](specs/test-suites/test-rel04.2-draft.md) | docs/specs/use-cases/rel04.2-uc001-draft-a-planning-document.md |
| [rel04.3-uc001-install-without-go](specs/use-cases/rel04.3-uc001-install-without-go.md) | A Developer with No Go Toolchain Installs tpm and Readies the Pinned Runtime | 04.3 | implemented | [test-rel04.3-distribution](specs/test-suites/test-rel04.3-distribution.md) | docs/specs/use-cases/rel04.3-uc001-install-without-go.md |
| [rel04.3-uc002-ci-job-on-the-image](specs/use-cases/rel04.3-uc002-ci-job-on-the-image.md) | A CI Job Runs tpm and the Pinned Runtime on the Release's Image | 04.3 | implemented | [test-rel04.3-distribution](specs/test-suites/test-rel04.3-distribution.md) | docs/specs/use-cases/rel04.3-uc002-ci-job-on-the-image.md |
| [rel04.4-uc001-work-a-story](specs/use-cases/rel04.4-uc001-work-a-story.md) | A Developer Works a Story End to End in One Worktree and One Merge | 04.4 | implemented | [test-rel04.4-story-loop](specs/test-suites/test-rel04.4-story-loop.md) | docs/specs/use-cases/rel04.4-uc001-work-a-story.md |
| [rel04.5-uc001-report-in-place](specs/use-cases/rel04.5-uc001-report-in-place.md) | A Person Writes Their Weekly Status Report Where They Worked | 04.5 | implemented | [test-rel04.5-status-in-place](specs/test-suites/test-rel04.5-status-in-place.md) | docs/specs/use-cases/rel04.5-uc001-report-in-place.md |
| [rel04.5-uc002-roll-up-in-place](specs/use-cases/rel04.5-uc002-roll-up-in-place.md) | A Lead Rolls Up a Team's or the Program's Week From Where It Was Written | 04.5 | implemented | [test-rel04.5-status-in-place](specs/test-suites/test-rel04.5-status-in-place.md) | docs/specs/use-cases/rel04.5-uc002-roll-up-in-place.md |
| [rel05.0-uc001-reference-update](specs/use-cases/rel05.0-uc001-reference-update.md) | A Downstream Release Files a Reference-Update Issue Upstream | 05.0 | specified | [test-rel05.0-tag-triggered](specs/test-suites/test-rel05.0-tag-triggered.md) | docs/specs/use-cases/rel05.0-uc001-reference-update.md |
| [rel05.0-uc002-release-summary](specs/use-cases/rel05.0-uc002-release-summary.md) | A System Release Renders the Deck from the Approved Summary | 05.0 | specified | [test-rel05.0-tag-triggered](specs/test-suites/test-rel05.0-tag-triggered.md) | docs/specs/use-cases/rel05.0-uc002-release-summary.md |

<!-- field: test_suite_index | form: "typed table" -->
## Test suite index

| id | title | release | test_case_count | traces | path |
| --- | --- | --- | --- | --- | --- |
| [test-rel00.0-push](specs/test-suites/test-rel00.0-push.md) | Test Suite for Release 00.0 Labels and Push | "00.0" | 2 | [[rel00.0-uc001-push-project-work](specs/use-cases/rel00.0-uc001-push-project-work.md)] | docs/specs/test-suites/test-rel00.0-push.md |
| [test-rel00.1-pop](specs/test-suites/test-rel00.1-pop.md) | Test Suite for Release 00.1 Pop and Merge | "00.1" | 3 | [[rel00.1-uc001-pop-and-merge](specs/use-cases/rel00.1-uc001-pop-and-merge.md)] | docs/specs/test-suites/test-rel00.1-pop.md |
| [test-rel00.2-docs-change](specs/test-suites/test-rel00.2-docs-change.md) | Test Suite for Release 00.2 Changes Under docs/ | "00.2" | 3 | [[rel00.2-uc001-spec-change](specs/use-cases/rel00.2-uc001-spec-change.md), [rel00.2-uc002-planning-change](specs/use-cases/rel00.2-uc002-planning-change.md)] | docs/specs/test-suites/test-rel00.2-docs-change.md |
| [test-rel00.3-install](specs/test-suites/test-rel00.3-install.md) | Test Suite for Release 00.3 Install the Skills | "00.3" | 6 | [[rel00.3-uc001-install-skills](specs/use-cases/rel00.3-uc001-install-skills.md)] | docs/specs/test-suites/test-rel00.3-install.md |
| [test-rel01.0-objectives](specs/test-suites/test-rel01.0-objectives.md) | Test Suite for Release 01.0 Sprint Goals Reach the Product Owner | "01.0" | 6 | [[rel01.0-uc001-author-objectives](specs/use-cases/rel01.0-uc001-author-objectives.md), [rel01.0-uc002-push-objective-issues](specs/use-cases/rel01.0-uc002-push-objective-issues.md)] | docs/specs/test-suites/test-rel01.0-objectives.md |
| [test-rel01.1-weekly-status](specs/test-suites/test-rel01.1-weekly-status.md) | Test Suite for Release 01.1 Status Comes Back Weekly | "01.1" | 6 | [[rel01.1-uc001-weekly-status](specs/use-cases/rel01.1-uc001-weekly-status.md), [rel01.1-uc002-period-rollup](specs/use-cases/rel01.1-uc002-period-rollup.md)] | docs/specs/test-suites/test-rel01.1-weekly-status.md |
| [test-rel01.2-sprint-end](specs/test-suites/test-rel01.2-sprint-end.md) | Test Suite for Release 01.2 Sprint End Closes the Loop | "01.2" | 5 | [[rel01.2-uc001-sprint-end-release](specs/use-cases/rel01.2-uc001-sprint-end-release.md), [rel01.2-uc002-plan-next-period](specs/use-cases/rel01.2-uc002-plan-next-period.md)] | docs/specs/test-suites/test-rel01.2-sprint-end.md |
| [test-rel01.4-agent-legible-cli](specs/test-suites/test-rel01.4-agent-legible-cli.md) | Test Suite for Release 01.4 The Agent-Legible CLI | "01.4" | 8 | [[rel01.4-uc001-bootstrap-a-program](specs/use-cases/rel01.4-uc001-bootstrap-a-program.md), [rel01.4-uc002-agent-learns-the-cli](specs/use-cases/rel01.4-uc002-agent-learns-the-cli.md)] | docs/specs/test-suites/test-rel01.4-agent-legible-cli.md |
| [test-rel03.0-beads-tracker](specs/test-suites/test-rel03.0-beads-tracker.md) | Test Suite for Release 03.0 The Tracker Becomes Beads | "03.0" | 4 | [[rel03.0-uc001-pop-and-close-on-beads](specs/use-cases/rel03.0-uc001-pop-and-close-on-beads.md), [rel03.0-uc002-two-clones-synchronize](specs/use-cases/rel03.0-uc002-two-clones-synchronize.md)] | docs/specs/test-suites/test-rel03.0-beads-tracker.md |
| [test-rel03.1-tracking-systems](specs/test-suites/test-rel03.1-tracking-systems.md) | Test Suite for Release 03.1 One Tracking System per Repository | "03.1" | 3 | [[rel03.1-uc001-choose-the-tracking-system](specs/use-cases/rel03.1-uc001-choose-the-tracking-system.md)] | docs/specs/test-suites/test-rel03.1-tracking-systems.md |
| [test-rel04.0-agile-hierarchy](specs/test-suites/test-rel04.0-agile-hierarchy.md) | Test Suite for Release 04.0 The Agile Hierarchy | "04.0" | 7 | [[rel04.0-uc001-author-the-hierarchy](specs/use-cases/rel04.0-uc001-author-the-hierarchy.md), [rel04.0-uc002-plan-a-sprint-from-stories](specs/use-cases/rel04.0-uc002-plan-a-sprint-from-stories.md), [rel04.0-uc003-sprint-goal-ledger](specs/use-cases/rel04.0-uc003-sprint-goal-ledger.md)] | docs/specs/test-suites/test-rel04.0-agile-hierarchy.md |
| [test-rel04.1-orientation-doctor](specs/test-suites/test-rel04.1-orientation-doctor.md) | Test Suite for Release 04.1 A Repository That Explains Itself | "04.1" | 4 | [[rel04.1-uc001-orient-and-repair](specs/use-cases/rel04.1-uc001-orient-and-repair.md)] | docs/specs/test-suites/test-rel04.1-orientation-doctor.md |
| [test-rel04.2-draft](specs/test-suites/test-rel04.2-draft.md) | Test Suite for Release 04.2 A Planning Document Drafted in Conversation | "04.2" | 3 | [[rel04.2-uc001-draft-a-planning-document](specs/use-cases/rel04.2-uc001-draft-a-planning-document.md)] | docs/specs/test-suites/test-rel04.2-draft.md |
| [test-rel04.3-distribution](specs/test-suites/test-rel04.3-distribution.md) | Test Suite for Release 04.3 tpm Ships to Machines and to CI | "04.3" | 4 | [[rel04.3-uc001-install-without-go](specs/use-cases/rel04.3-uc001-install-without-go.md), [rel04.3-uc002-ci-job-on-the-image](specs/use-cases/rel04.3-uc002-ci-job-on-the-image.md)] | docs/specs/test-suites/test-rel04.3-distribution.md |
| [test-rel04.4-story-loop](specs/test-suites/test-rel04.4-story-loop.md) | Test Suite for Release 04.4 Work a Story End to End | "04.4" | 2 | [[rel04.4-uc001-work-a-story](specs/use-cases/rel04.4-uc001-work-a-story.md)] | docs/specs/test-suites/test-rel04.4-story-loop.md |
| [test-rel04.5-status-in-place](specs/test-suites/test-rel04.5-status-in-place.md) | Test Suite for Release 04.5 Status Reports in Place | "04.5" | 2 | [[rel04.5-uc001-report-in-place](specs/use-cases/rel04.5-uc001-report-in-place.md), [rel04.5-uc002-roll-up-in-place](specs/use-cases/rel04.5-uc002-roll-up-in-place.md)] | docs/specs/test-suites/test-rel04.5-status-in-place.md |
| [test-rel05.0-tag-triggered](specs/test-suites/test-rel05.0-tag-triggered.md) | Test Suite for Release 05.0 Tags Trigger Handoff and the Summary | "05.0" | 2 | [[rel05.0-uc001-reference-update](specs/use-cases/rel05.0-uc001-reference-update.md), [rel05.0-uc002-release-summary](specs/use-cases/rel05.0-uc002-release-summary.md)] | docs/specs/test-suites/test-rel05.0-tag-triggered.md |

<!-- field: coverage_gaps -->
## Coverage gaps

| id | gap |
| --- | --- |
| GAP1 | Use cases exist for releases 00, 01, 03, and 04, and for 05.0, which is specified. Release 06.0 has none: srd008, srd019, srd020, srd032, and srd033 hold its direction only (design.md D12), and their items and use cases are written when 06.0 is planned. |
| GAP2 | Test suites exist for releases 00, 01, 03, 04, and 05.0. test-rel00.0-push, test-rel00.1-pop, test-rel00.2-docs-change, test-rel00.3-install, test-rel01.0-objectives, test-rel01.1-weekly-status, test-rel01.2-sprint-end, test-rel01.4-agent-legible-cli, and the suites of 03.0, 03.1, and 04.0 to 04.5 are implemented; test-rel05.0-tag-triggered is specified, and acceptance criteria outside them have no test case yet. |
| GAP3 | The audit is the parent's specification-critic corpus-audit agent, run by mage audit after the repository gate tests. It does not yet run in CI on changes to docs/ (srd034 R2.2); scrum-master issue merge runs it before each merge; this repository has no pipeline. |
| GAP4 | Open decision. The promotion trigger for each environment (each subsystem release, nightly from the latest releases, or both) is declared per environment SRD; no default is chosen yet. |
| GAP5 | Open decision. The channel by which evidence from customer R&D environments returns to the Sprint Goal ledger is not specified (srd008, srd019 non-goals). |
| GAP6 | Open decision. Release 01.4 renames the program group to issue, moves recur into it from standup, and adds init, onboard, and help (srd013, srd043, srd044). The other subcommand groups (standup, goals, handoff, release, reports, plan, sprint-end) are rationalized when 06.0 rewrites track-goals (srd019). |
| GAP7 | The corpus's own SRDs omit test and environment on acceptance criteria (design.md srd acceptance_criteria_note) until the test subsystem's environment SRDs exist. |
| GAP8 | The parent corpus audit does not read the hierarchy directories (docs/specs/themes/, initiatives/, epics/, features/, stories/, and objectives/), so a magefiles gate test validates those documents against design.md and srd045 R6.1 inside mage audit (srd034 R2.1). A hierarchy document is scrum-master's type written as an SRD in the parent's format, so the gate is scrum-master's own, not a gap the parent closes. |
| GAP9 | scrum-master issue lint does not yet run in CI on issue creation or on a schedule (srd013 R1.3); that needs a released agent the program's pipelines can run, which the image provides (eng03). |
| GAP11 | Open decision. srd041 has lint rows only out of open and needs-triage, so an issue already past admission that later fails a re-lint has no transition. On GitLab and on beads lint sets or clears the needs-triage label alone and leaves the state; whether such an issue should leave the backlog is undecided. |
| GAP12 | Roll-up across repositories (srd045 R3.4): a story in another repository whose parent is in this one is not discovered from the parent until a program registry names every repository; roll-up counts what it reads and says so. |
