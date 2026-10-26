---
id: srd035-specify
title: "Skill: specify"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Work enters a program as a sentence someone says: fix this, add that,
change this requirement. An issue written from that sentence by hand
carries whatever labels and sections its author remembered, so admission
sends it back, or it reaches the person who pops it with nothing to build
against.

specify turns one request into one admitted issue in the issue body's
work format. The model does the one thing a program cannot: it structures the
request into the issue body. scrum-master does the rest: it picks the plane, kind,
and component, files the issue unassigned, and lints it. The plane decides
what the issue is: plane::data for project work, plane::management for a
change to the requirements or the plan under docs/, and plane::control for
a change to the process's rules under control/ (srd001 R1-R3).

<!-- field: goals -->
## Goals

- G1: Every request becomes one admitted issue in the issue body's work format.
- G2: An issue's plane and kind say whether it is project work, a design change, a planning change, or a rule change.
- G3: The model writes the body and nothing else.

<!-- req: R1 -->
## R1 - Trigger

<!-- req: R1.1 -->
### R1.1

Started by anyone with a request, in the repository the request concerns.

<!-- req: R2 -->
## R2 - Reads

<!-- req: R2.1 -->
### R2.1

The request as the person states it, and the subsystem SRD that declares the repository's component.

<!-- req: R2.2 -->
### R2.2

The paths the request names. A change to the road-map, a system or subsystem SRD, an initiative, or the sprint is a plane::management issue; any other change under docs/ is a plane::data issue of kind::spec (srd001 R33); a change under control/ is a plane::control issue; anything else is a plane::data issue of the kind the request states.

<!-- req: R3 -->
## R3 - Asks only what is not computed

<!-- req: R3.1 -->
### R3.1

Model step (structure): turns the request into the body, a title and the Requirements, Acceptance Criteria, and optional Design Decisions sections of config format issue-body, each criterion naming its test and environment.

<!-- req: R3.2 -->
### R3.2

Asks the person only for what the request and the repository do not answer: the plane when the request names no path, the kind of a data issue, and the test of a criterion that names none.

<!-- req: R3.3 -->
### R3.3

Asks for no confirmation before filing.

<!-- req: R4 -->
## R4 - Pushes

<!-- req: R4.1 -->
### R4.1

Files the issue with scrum-master issue file (srd013 R3.1): unassigned, with its plane, kind on a data issue, component, and the sprint milestone on a data issue, and linted before it returns.

<!-- req: R4.2 -->
### R4.2

Reports the issue URL, the plane, kind, and component scrum-master labelled it with, and whether admission admitted it, with the reasons when it did not.

<!-- req: R5 -->
## R5 - Refusals

<!-- req: R5.1 -->
### R5.1

Refuses to assign the issue to anyone (srd041 R9.1).

<!-- req: R5.2 -->
### R5.2

Refuses to file a plane::management issue for a person who is not a lead (srd001 R1.2); a kind::spec issue is anyone's.

<!-- req: R5.3 -->
### R5.3

As a skill, is at most 50 lines (design.md D10).

<!-- req: R6 -->
## R6 - The workflow as a profile

<!-- req: R6.1 -->
### R6.1

The workflow is a declarative-agents profile, plugin/agents/specify/ (design.md D14): the machine's states carry the skill's steps, the scrum-master commands are model-dispatched tools with typed parameters, and the model acts only in the Draft state (design.md D10). mage audit validates the profile with the pinned runtime's --validate-config, and scrum-master install renders the skill's SKILL.md from the machine (srd037 R1.4), so an agent that does not run the runtime follows the same steps.

<!-- req: R6.2 -->
### R6.2

Before the issue's body names an ARCHITECTURE component, the specify profile checks each ARCHITECTURE component it would name with scrum-master backlog components (srd045 R8.6) and, for one that is not declared, stops with "this needs a design decision" naming the component and the design skill, filing nothing: planning never invents an architecture element (srd047 R2.1).

<!-- field: non_goals -->
## Non-goals

- Splitting a request into several issues; one request is one issue, and sub-issues arrive with tracking issues.
- Working the issue; that is implement (srd036).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A request naming a planning document becomes a plane::management issue, any other docs/ path a kind::spec data issue, a control/ path a plane::control issue, and any other a plane::data issue with a kind; the body has Requirements and Acceptance Criteria with test and environment.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)
- [R3.1](#r31)

<!-- ac: AC2 -->
### AC2

The skill asks only for a missing plane, kind, or test, and never for confirmation.

<!-- field: traces -->
#### Traces

- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC3 -->
### AC3

The issue is filed unassigned through scrum-master issue file and linted; the skill reports its URL, its plane, kind, and component, and its admission, with reasons.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)

<!-- ac: AC4 -->
### AC4

No issue is assigned; a non-lead's management issue is refused; the skill is at most 50 lines.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC5 -->
### AC5

The specify profile validates with the pinned runtime, and the installed SKILL.md presents its machine's states as its steps; it stops with "this needs a design decision" rather than name an undeclared component.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
