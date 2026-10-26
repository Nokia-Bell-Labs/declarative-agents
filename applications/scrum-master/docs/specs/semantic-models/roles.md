---
id: roles
title: Roles and levels
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines the four roles of the process by the level each owns, what each writes, and when its level is done. Every repository carries the same corpus (design.md D7); roles differ only by which SRDs they own. Skill SRDs name these roles as their actors.

<!-- field: ownership_rules -->
## Ownership rules

| level | owner | writes | done_when |
| --- | --- | --- | --- |
| system | technical_program_manager | System SRDs and interface YAMLs in the system repository, and the subsystem SRDs that declare each subsystem's owner and components. | System tests pass on the pinned set in the final environment before customer handoff (srd008). |
| subsystem | product_owner | The subsystem's epics, features, and stories, the requirements its stories cite, and the subsystem's road-map. | Its stories' criteria pass in the environment each names (srd008). |
| component | developer | Issues from chosen stories with tasks, then code and documents through specify and implement. | Component tests pass in the local environment and the branch is reviewed and merged (srd008). |
| test_subsystem | scrum_master | Environment SRDs, pipeline SRDs, and test-suite SRDs in the test-subsystem repository. | Known-good sets pass and known-bad sets fail in every environment that runs them (srd008). |

<!-- field: roles | form: "keyed table" -->
## Roles

| id | was | is | runs |
| --- | --- | --- | --- |
| technical_program_manager | Manages cross-team dependencies in spreadsheets. | "Owns the system level: the system requirements and interfaces, the subsystem SRDs that say who owns what, the label schema, and the policy. Writes the program's themes and initiatives and its cross-team epics, declares each system release before its three-week period, and files each team's Sprint Goal citing the initiative and epic items it serves. Owns clarity and process, not the delivery outcomes of the subsystems." | [plan, tasks] |
| product_owner | Writes human-facing stories and manages the backlog. | Owns a subsystem and orders its Product Backlog. Writes its epics, features, and user stories, each story with acceptance criteria that name a test and an environment, and the requirements those criteria prove, using draft and implement on its docs; selects stories onto the team's Sprint Goal at Sprint Planning; and judges the Increment. | [plan, tasks, draft, implement, sprint-review] |
| developer | Writes code. | Owns components. Turns each chosen story into issues with tasks, works them through the inner loop, and reviews branches for architecture, security, and anything the tests cannot see. In a program of developers and researchers, also runs experiments, writes reports, and reviews others' artifacts; the kind label says which. | [tasks, daily-scrum, specify, implement, scrum-master release] |
| scrum_master | Runs standups, removes blockers, counts points. | "Owns the test subsystem: the environments of the chain, the pipelines that promote pinned sets between them, and the suites that produce evidence, each specified as an SRD. Owns the audits, track-goals among them, and facilitates the Retrospective. There is no scrum master agent." | [sprint-retrospective, track-goals, audits] |

<!-- field: relationships -->
## Relationships

Each role's relationship to the Scrum Guide (2020); docs/guides/agile-mapping.md maps every event, artifact, and commitment.

| role | scrum_guide | fit | rationale |
| --- | --- | --- | --- |
| technical_program_manager | none; the Guide has no role above one team | addition | A program of several teams needs one owner of the system's requirements, interfaces, and releases, who files each team's Sprint Goal citing the initiative and epic items it serves; the nearest counterparts are Nexus's integration work and SAFe's Release Train Engineer with Solution Management. It orders the program's backlog and never assigns work. |
| product_owner | Product Owner | aligned, with an addition | Orders the subsystem's backlog, owns its goals, and judges the Increment; also writes requirements with an agent, because acceptance criteria that name a test and an environment are what make done checkable. |
| developer | Developers | aligned | Create the Increment, own the Sprint Backlog through tasks, hold to the Definition of Done, and adapt daily. |
| scrum_master | Scrum Master | deviation | The administrative half of the role (standups, the board, counting points, chasing blockers) is scripts; what remains is the part of the team's effectiveness that is a system to own: the environments, suites, and audits that decide done, and the Retrospective. Coaching stays a person's work. |

<!-- field: actor_boundaries | form: "tree" -->
## Actor boundaries

- interface: "What crosses between levels: a requirement, an interface, its released version, and the repositories that cite it by pinned URL. Owned by the level above."
- interior: "How a level meets the Sprint Goals it receives: its own epics, features, stories, requirements, issues, and implementation. Owned by that level's owner."
- rule: An owner specifies interfaces for the level below and never its interior; a level below changes an interface only through a plane::management or plane::control issue in the repository that owns it.
- push_and_pull: "Intent is pushed down as initiatives, epics, and Sprint Goals, and work is pulled. plan declares the next release at each level; tasks files one Sprint Goal per team, citing the initiative and epic items it serves at the release tag; the team's Sprint Planning selects stories onto it and Developers turn each story into issues with tasks. Nobody assigns an issue: people pop issues for themselves. A Sprint Goal is met on evidence for its stories' criteria, collected by track-goals."
