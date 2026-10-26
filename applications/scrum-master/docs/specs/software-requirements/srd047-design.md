---
id: srd047-design
title: "Skill: design"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

- Three activities, separated by their output. Design changes the intended state, ARCHITECTURE and docs/engineering/; planning sequences the gap between that and the latest release; implementation moves the actual state (docs/guides/plan-vs-architecture.md). Plan names the second; this skill names the first.
- The design is an ordinary issue. The skill files one issue whose only paths are the intended-state documents, so the change lands through the reviewed merge every change takes, ahead of the plan that cites it, and the bootstrapping order holds (design.md D15).
- Planning stops rather than invents. A plan may cite only what ARCHITECTURE declares (srd045 R2.4, R6.2); when it would need an element that is not there, it stops and names this skill instead of guessing a boundary.

<!-- field: problem -->
## Problem

The word plan carries three meanings: the Scrum scheduling of srd031, a
design step that writes ARCHITECTURE and Design Decisions, and an agent's
own session planning. No skill owns the design step, so Design Decisions
arrive on items as choices already made, and a planning agent that needs a
component ARCHITECTURE does not declare invents one in the plan, where no
one reviews it as architecture.

<!-- field: goals -->
## Goals

- G1: A design decision is made by a named activity whose only output is a change to the intended state, reviewed before implementation.
- G2: Planning never invents an architecture element; it stops and sends the person to design.

<!-- req: R1 -->
## R1 - The design skill

<!-- req: R1.1 -->
### R1.1

The design profile turns one request into one issue whose only paths are docs/ARCHITECTURE.md and files under docs/engineering/, its body naming each component or engineering rule added, changed, or removed with its boundary and interfaces, its Design Decisions with their reasons and the alternatives set aside, and its acceptance criteria. It writes no hierarchy document, SRD, or code; the change lands through the ordinary pop, implement, and reviewed merge.

<!-- req: R1.2 -->
### R1.2

It runs in the system repository for subsystem boundaries, where the technical program manager merges, and in a team's repository for a component's shape, where the Product Owner and the Developers do.

<!-- req: R1.3 -->
### R1.3

Like the other model profiles it reads the plan-vs-architecture guide before acting (srd045 R8.4), checks the existing components with scrum-master backlog components (srd045 R8.6) before naming one, and the model writes only the issue's language (design.md D10, D14).

<!-- req: R2 -->
## R2 - Planning stops for a design decision

<!-- req: R2.1 -->
### R2.1

The specify and tasks profiles check a component with scrum-master backlog components before an issue or document they write names it, and when it is not declared they stop with "this needs a design decision" naming the component and this skill, filing nothing (srd035, srd022).

<!-- req: R2.2 -->
### R2.2

scrum-master plan write refuses a release whose items' documents cite a component of the repository that docs/ARCHITECTURE.md does not declare, naming each with the document citing it and this skill, and writes nothing (srd031 R5.3). The gate refuses such a citation once filed (srd045 R6.2); this stops it before.

<!-- field: non_goals -->
## Non-goals

- Writing the plan; planning sequences what design declared.
- Implementing the design; the implement skill moves the actual state.
- Deciding the design; the person decides, and the model writes the issue.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The design profile files one issue whose paths are only docs/ARCHITECTURE.md and docs/engineering/ files, with Design Decisions, after reading the guide and checking the components, and writes no hierarchy document, SRD, or code.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)

<!-- ac: AC2 -->
### AC2

specify and tasks stop with "this needs a design decision" for an undeclared component, and plan write refuses a release whose documents cite one, naming the component and the design skill.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
