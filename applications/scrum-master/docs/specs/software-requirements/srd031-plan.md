---
id: srd031-plan
title: "Skill: plan"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Before each three-week period, someone has to say which requirements the
next release will prove. At system level that is the technical program
manager choosing the system requirements the next system release covers;
at subsystem level it is the Product Owner choosing which of the
subsystem's requirements, written to serve the initiative items on its
Sprint Goal, the next subsystem release covers, and which stories the
team's next sprint takes from its Product Backlog. Decided in a meeting, the choice lives in slides,
and the road-map says something else.

plan reads the road-map, the requirements or, in the system
repository, the agreed initiatives, and what track-goals says is
already met (before track-goals runs, what the period's reports say
slipped), asks the owner which requirements go into the next
release, and records the answer in the road-map and on the Sprint Goals it
moves. It works the same at both levels, because every repository carries
the same corpus.

<!-- field: goals -->
## Goals

- G1: Each road-map release names the requirements it will prove before its period starts.
- G2: Only requirements whose acceptance criteria name a test and an environment enter a release.
- G3: The same skill serves the system and every subsystem.

<!-- req: R1 -->
## R1 - Trigger

<!-- req: R1.1 -->
### R1.1

Started before each three-week period by the technical program manager in the system repository, and by each Product Owner in a subsystem repository after the system release is declared.

<!-- req: R2 -->
## R2 - Reads

<!-- req: R2.1 -->
### R2.1

The repository's road-map (docs/road-map.md): its releases, their requirements or, in the system repository, their initiative items (deliverables, design.md schema.roadmap), and their status.

<!-- req: R2.2 -->
### R2.2

The repository's SRDs, in the system repository the agreed initiatives (srd034 R1), and at subsystem level the open Sprint Goal issues filed in the repository, with the initiative items they cite by pinned URL (srd007 R5).

<!-- req: R2.3 -->
### R2.3

scrum-master goals status (srd019) for what is met, proving, or slipped, and the latest release tags. Before track-goals runs in the program, the period aggregate names what slipped: a Sprint Goal its slipped-commitment signal (srd018 R3.2) names, and one its progress shows neither met nor reported complete at the period's end. What slips is each item such a goal cites; a goal that cites no item of an outcome it serves slips the whole outcome, so an item another goal of the same outcome delivered is not carried over.

<!-- req: R2.4 -->
### R2.4

Waiting dependencies as track-goals explains them (srd019), so a requirement that cannot be proved this period is visible before it is chosen.

<!-- req: R3 -->
## R3 - Asks

<!-- req: R3.1 -->
### R3.1

Asks the owner which requirements, or in the system repository which initiative items, enter the next release, recommending slipped ones first, then those that serve the Sprint Goals with the earliest upstream release, then the rest in road-map order. scrum-master plan candidates computes the ranking.

<!-- req: R3.2 -->
### R3.2

Asks for the release's version and end date, recommending the next minor version and the end of the three-week period: the Friday of its third ISO week, the period starting the Monday after the last release's milestone ends, by the tracker's milestone where it finds one in the system's project, the project a system given as a checkout's path names by its origin (GH-141), and else by the ISO weeks the milestone's name spans (GH-117), or else the Monday of the current week, of the next one on a weekend. For the first release, with no milestone to follow, when the unplaced items fall due on a Friday ending a period already begun and not past, that Friday is the recommendation, whatever day of the period the plan is made (GH-66, GH-92).

<!-- req: R4 -->
## R4 - Pushes and posts

<!-- req: R4.1 -->
### R4.1

The release entry in docs/road-map.md, listing its requirements by id, or in the system repository its initiative items in deliverables and its milestone and tag in release.publication, written by scrum-master plan write in the worktree of a plane::management issue filed with specify, and merged through the inner loop, so the plan is a reviewed change. A release plan write adds is a whole record: its status, a title, a line saying what it delivers, on which milestone, under which tag, then its fields in the parent schema's order, deliverables, the previous release's srds, and release; a record it replaces keeps its other fields, reordered the same way. A placeholder sentence scrum-master init left, in the record or the road map's overview, is replaced with text that says what the road map holds. In a team's repository, candidates recommend no end date or tag, which are the system's. The sentence plan write puts in the record's prose, naming its items, milestone, and tag, is rewritten on every write, so a republish's tag reads the same in prose and in release.publication. A release record's status is specified, implemented, or released (design.md roadmap).

<!-- req: R4.2 -->
### R4.2

The tracker milestone named for the release, with its ISO weeks in its description, its end date, and its start on the first working day of the period's first ISO week, created if missing and updated if present (srd001 R7.1), in the program group and in the group of each releasing repository outside it, so every repository's issues find it. The program group is the one named, else the one control/trackers.md records (config format trackers), else the system project's parent group (GH-103).

<!-- req: R4.3 -->
### R4.3

A planned event (srd006 R8) on each Sprint Goal issue whose release changed, counting and citing only the release's items the goal cites in its Cites lines, so goals serving one outcome do not each count the other's; a goal with no Cites lines counts every item of the outcomes it serves (GH-92). Each item is cited at the URL the goal's own Cites line pins, so the event and its goal name the same text; an item the goal does not cite is cited at the new plan's tag (GH-101).

<!-- req: R4.4 -->
### R4.4

Returns each requirement or initiative item whose acceptance criteria lack a test or an environment to its owner with a comment naming what is missing, instead of placing it.

<!-- req: R4.5 -->
### R4.5

In the system repository, once the release entry merges, a documents-only release of the system repository, gated by mage audit and tagged by scrum-master release tag with the tag release.publication names (srd015 R2, srd034 R2.3), so tasks pins its Cites lines to the agreed text.

<!-- req: R4.6 -->
### R4.6

scrum-master plan push dates the milestone, sets the sprint issue (srd001 R7) with one checklist item per Sprint Goal, posts the planned events, and files one sprint-end issue (srd038 R1.1) in each releasing repository due on the milestone end date; a second run updates and creates nothing.

<!-- req: R4.7 -->
### R4.7

push decomposes into four words the plan-push profile composes (GH-201, GH-206, design.md D14): scrum-master plan milestone, scrum-master plan sprint-issue, scrum-master plan sprint-ends, and scrum-master plan events. scrum-master plan push runs the same code as one command. Each word rereads the road map and recomputes its scope, and each upserts by marker or ensures by title, so a re-run of a word, the machine, or the command updates and creates nothing, and a word that fails after another is a partial machine state a re-run finishes.

<!-- req: R5 -->
## R5 - Refusals

<!-- req: R5.1 -->
### R5.1

Refuses to place a requirement without a named test and environment, and refuses to assign any issue to a person.

<!-- req: R5.2 -->
### R5.2

Refuses to create work issues; Developers turn each chosen story into issues with tasks.

<!-- req: R5.3 -->
### R5.3

scrum-master plan write refuses a release whose items' documents, or the documents under them, cite a component of the repository that docs/ARCHITECTURE.md does not declare (srd045 R2.4), naming each with the document citing it as a design decision to make first with the design skill (srd047 R2.2), and writes nothing. It refuses too a release whose documents not yet agreed cite a component ARCHITECTURE added or changed since the latest released release that no agreed document plans (srd045 R6.3), naming each with the document and its status, to be agreed first (GH-100).

<!-- req: R6 -->
## R6 - Sprint Planning from stories

<!-- req: R6.1 | status: retired | retired_in: 4 | replaced_by: ["srd031-plan R6.4", "srd045-backlog R5.2"] -->
### R6.1

Retired in 04.0: plan selects stories, not epic items (R6.4).

<!-- req: R6.2 -->
### R6.2

In Scrum terms, the ranking of R3.1 orders the program's initiative items, and the ranking of R6.4 orders a team's Product Backlog; choosing from them is Sprint Planning, and changing a document or a story's rank through the audit gate is backlog refinement.

<!-- req: R6.3 | status: retired | retired_in: 4 -->
### R6.3

Retired in 04.0 with R6.1.

<!-- req: R6.4 -->
### R6.4

In a team's repository, scrum-master plan candidates also lists the team's Product Backlog (srd045 R5.1): its agreed stories that are not done, by rank, with their points, the team's velocity (srd045 R5.3), and, recommended in rank order, the stories whose points fit it. Its release is the system's open one, the first the system's road map has not released, never one of its own (GH-141).

<!-- req: R6.5 -->
### R6.5

Asks the Product Owner which stories enter the sprint, recommending those of R6.4, and asks for an assumed velocity when the team has finished no sprint.

<!-- req: R6.6 -->
### R6.6

scrum-master plan backlog writes the chosen stories, with their points, into the Sprint Backlog section of the team's Sprint Goal and posts a planned event naming them (srd034 R3.5); a second run with the same stories changes nothing.

<!-- req: R7 -->
## R7 - The workflow as a profile

<!-- req: R7.1 -->
### R7.1

The workflow is a declarative-agents profile, plugin/agents/plan/ (design.md D14): the machine's states carry the skill's steps, the scrum-master commands are model-dispatched tools with typed parameters, and the model acts only in the Plan state (design.md D10). mage audit validates the profile with the pinned runtime's --validate-config, and scrum-master install renders the skill's SKILL.md from the machine (srd037 R1.4), so an agent that does not run the runtime follows the same steps.

<!-- field: non_goals -->
## Non-goals

- Estimating capacity per person.
- Filing Sprint Goals downstream; that is tasks (srd022).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Facts come from the road-map, the SRDs, open Sprint Goals with pinned citations (an open goal slips the items it cites, not the rest of their outcome), the team's stories, track-goals status, and dependency explanations; the requirements and the version are asked with recommendations; the skill runs at system and subsystem level alike.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)
- [R3.1](#r31)
- [R3.2](#r32)

<!-- ac: AC2 -->
### AC2

The release entry is merged through the inner loop, the system repository's plan is tagged, the milestone exists with the end date, the sprint issue and the period's sprint-end issues exist once, each moved Sprint Goal gets a planned event, and a requirement or initiative item without test or environment is returned with a comment. A republished release's prose names the same tag as its publication.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)
- [R4.4](#r44)
- [R4.5](#r45)
- [R4.6](#r46)

<!-- ac: AC3 -->
### AC3

No requirement without test and environment is placed, no issue is assigned, no work issue is created, and no release is written whose documents cite an undeclared component.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC4 -->
### AC4

In a team's repository plan lists the not-done agreed stories by rank with points and velocity, recommends those that fit, asks an assumed velocity when the team has none, and writes the chosen stories to the Sprint Goal with a planned event; a second push with the same stories changes nothing.

<!-- field: traces -->
#### Traces

- [R6.2](#r62)
- [R6.4](#r64)
- [R6.5](#r65)
- [R6.6](#r66)

<!-- ac: AC5 | status: retired | retired_in: 4 -->
### AC5

Retired in 04.0 with the direction items it traced.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.3](#r63)

<!-- ac: AC6 -->
### AC6

The plan profile validates with the pinned runtime, and the installed SKILL.md presents its machine's states as its steps.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)

<!-- ac: AC7 -->
### AC7

A run of the plan-push profile with the pinned runtime dates the milestone, files the sprint and sprint-end issues, and posts the planned events, ending in Done; a second run ends in Done and the one-command push after it reports everything unchanged; a road map without the release ends in Refused with scrum-master's reason.

<!-- field: traces -->
#### Traces

- [R4.7](#r47)
