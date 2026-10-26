---
id: srd022-tasks
title: "Skill: tasks"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Once a release is planned, the owners below have to know what they owe it.
A system release needs each subsystem to satisfy some system requirements;
a subsystem release may need work from its downstream component
repositories; every release needs the test subsystem to provide the suites
and environments that will prove it. Passed down in meetings, these
obligations lose the exact requirement and version they came from, and the
owner below writes requirements against a moving target.

tasks reads the release just planned and files one Sprint Goal issue
for each downstream team, citing the initiative items it serves by URL
pinned to the release tag and naming the environment where its evidence
counts. Each Sprint Goal is the ledger track-goals writes evidence to, and
the team's Sprint Planning lists on it the stories it selected (srd034,
srd045). Before release 04.0 a Sprint Goal was called an objective issue.
Invoking tasks is the decision to file them: it takes every owner
and environment scrum-master can compute, asks only for what it cannot, and pushes
without a further confirmation.

<!-- field: goals -->
## Goals

- G1: Every initiative item of a planned release reaches the downstream team that must serve it, on that team's Sprint Goal.
- G2: Every Sprint Goal cites the initiative items it serves at an exact tag and names the environment where it counts.
- G3: The test subsystem receives a Sprint Goal for the suites and environments each release needs.

<!-- req: R1 -->
## R1 - Trigger

<!-- req: R1.1 -->
### R1.1

Started after plan (srd031) by the technical program manager for a system release, or by a Product Owner whose subsystem has downstream repositories.

<!-- req: R2 -->
## R2 - Reads

<!-- req: R2.1 -->
### R2.1

The release just planned in docs/road-map.md and its release tag, and the requirements it lists.

<!-- req: R2.2 -->
### R2.2

The subsystem SRDs (design.md subsystem_srd) for the downstream owners and their repositories, and the environment SRDs in the test-subsystem repository.

<!-- req: R2.3 -->
### R2.3

Open Sprint Goal issues (and objective issues, read as Sprint Goals) already in the downstream repositories for this release, so a Sprint Goal is updated, not duplicated; and an open Sprint Goal of an earlier release whose Cites lines name an item of this one, which carries that item on: the plan marks the item carried in that goal's repository, and push files nothing for it.

<!-- req: R2.4 -->
### R2.4

The system repository's agreed epics at the release tag, for the area each item falls in (GH-2726): an epic is under an item when its parent is the item's initiative document and its cites name the item, or name nothing of that document; the components its architecture names belong to the team subsystems whose SRDs list them. The system SRD names no team.

<!-- req: R3 -->
## R3 - Asks only what is not computed

<!-- req: R3.1 -->
### R3.1

Takes the owner scrum-master recommends for each initiative item: the Product Owner whose subsystem SRD declares a repository the item's delivered_by names (srd034 R1.4); else every team whose subsystem owns a component the agreed epics under the item name (R2.4), one plan entry per team with a reason naming the epics and components, so each team's one Sprint Goal serves the item; else the only downstream team when the subsystem SRDs declare one team repository, with a reason saying so. delivered_by overrides the area, and the person confirms or changes every recommendation. Asks which downstream team serves an item only when none is recommended, offering the repositories the subsystem SRDs declare.

<!-- req: R3.2 -->
### R3.2

Takes, for each item, the environment of the acceptance criterion tracing it, else of its requirement group's first criterion, else dev, and asks nothing about it. A criterion that also traces items another team serves is copied with a note naming this Sprint Goal's part, its own items only: the other teams' items are theirs to name, and appear nowhere in this Sprint Goal.

<!-- req: R3.3 -->
### R3.3

Asks nothing the road-map, the SRDs, or the tracker already answers, and asks for no confirmation before pushing.

<!-- req: R4 -->
## R4 - Pushes and posts

<!-- req: R4.1 -->
### R4.1

One Sprint Goal issue per downstream team and release, filed with specify in the team's repository, in the issue body's work format, with one Cites line per initiative item it serves pinned to the release tag (srd007 R5), one Environment line, and an empty Sprint Backlog section (srd034 R3.5).

<!-- req: R4.2 -->
### R4.2

One Sprint Goal issue to the Scrum Master in the test-subsystem repository naming the suites and environments the release needs to produce its evidence.

<!-- req: R4.3 -->
### R4.3

A planned event (srd006 R8) on each Sprint Goal it creates or updates.

<!-- req: R5 -->
## R5 - Refusals

<!-- req: R5.1 -->
### R5.1

Refuses to file a Sprint Goal with an unpinned citation or without an environment.

<!-- req: R5.2 -->
### R5.2

Refuses to write the downstream owner's requirements; the owner writes those with draft and places them with plan.

<!-- req: R5.3 -->
### R5.3

Refuses to copy names or confidential content from inputs.

<!-- req: R6 -->
## R6 - A story into issues

<!-- req: R6.1 -->
### R6.1

At the story level, reads the story a team selected at Sprint Planning through scrum-master backlog list, and the issues already implementing it through scrum-master issue list --implements.

<!-- req: R6.2 -->
### R6.2

Proposes the issues the story still needs, each a change one reviewer reads in one sitting, in build order, each body in the issue body's work format with an Implements line naming the story and a Blocked-by line for an issue that must close first; the model writes the bodies, and nothing is filed before the owner agrees to each.

<!-- req: R6.3 -->
### R6.3

Files each agreed issue with scrum-master issue file under the Sprint Goal's milestone, and shows each issue's reference and lint verdict.

<!-- req: R7 -->
## R7 - The workflow as a profile

<!-- req: R7.1 -->
### R7.1

The workflow is a declarative-agents profile, plugin/agents/tasks/ (design.md D14): the machine's states carry the skill's steps, the scrum-master commands are model-dispatched tools with typed parameters, and the model acts only in the Decompose state (design.md D10). mage audit validates the profile with the pinned runtime's --validate-config, and scrum-master install renders the skill's SKILL.md from the machine (srd037 R1.4), so an agent that does not run the runtime follows the same steps.

<!-- req: R7.2 -->
### R7.2

Before an issue or Sprint Goal names an ARCHITECTURE component, the tasks profile checks each ARCHITECTURE component it would name with scrum-master backlog components (srd045 R8.6) and, for one that is not declared, stops with "this needs a design decision" naming the component and the design skill, filing nothing: planning never invents an architecture element (srd047 R2.1).

<!-- field: non_goals -->
## Non-goals

- Choosing what goes in the release; that is plan (srd031).
- Deciding that a Sprint Goal is met; that is track-goals (srd019).
- Selecting stories; that is the team's Sprint Planning (srd045 R5.2).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Facts come from the planned release, subsystem and environment SRDs, and existing Sprint Goals; the recommended owner and the environment are taken without asking; an item whose delivered_by names no repository goes to every team owning a component its agreed epics name, one entry each, and delivered_by overrides that; only an initiative item with no recommended owner is asked about; no confirmation is asked before the push. An item an open Sprint Goal of an earlier release cites is marked carried, and push files no second goal for it.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)
- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC2 -->
### AC2

Each downstream team receives one Sprint Goal per release with Cites lines pinned to the release tag, an Environment line, and an empty Sprint Backlog section; the test subsystem receives one Sprint Goal for the release's suites and environments; each carries a planned event; a second run creates no duplicate.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)

<!-- ac: AC3 -->
### AC3

An unpinned citation or a missing environment is refused; no downstream requirement is written by the skill; no name or confidential content is copied.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC4 -->
### AC4

A chosen story's issues are filed only after the owner agrees, each admitted, in the issue body's work format, with an Implements line naming the story and the Sprint Goal's milestone.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)

<!-- ac: AC5 -->
### AC5

The tasks profile validates with the pinned runtime, and the installed SKILL.md presents its machine's states as its steps; it stops with "this needs a design decision" rather than name an undeclared component.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
