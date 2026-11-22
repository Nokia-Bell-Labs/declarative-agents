---
id: in001-scrum-run-repository
title: Run this repository with an AI-native scrum process and keep it healthy
status: agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W38..2026-W52
- parent: th001-repository-operations

<!-- field: problem -->
## Problem

The repository develops scrum-master but ran its own issue loop with session skills, and its maintenance work was filed issue by issue with nothing tying it together.

<!-- field: goals -->
## Goals

- G1: The repository runs its issue loop with an installed scrum-master release.
- G2: Technical debt is retired under a plan, and main stays green.

<!-- req: R1 -->
## R1 - The scrum-master implementation

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-11-27"

<!-- req: R1.1 -->
### R1.1

scrum-master is built, released, and installable, covering delivery, planning, reporting, and multi-repository programs.

<!-- req: R2 -->
## R2 - Maintenance

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-12-31"

<!-- req: R2.1 -->
### R2.1

The repository's gates run green on main, and the debt found in passing is filed and retired.

<!-- req: R3 -->
## R3 - The agent-core public package

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2027-01-29"

<!-- req: R3.1 -->
### R3.1

agent-core offers a public package that runs the agent's command line and registers an application's in-process commands, and scrum-master builds its agent from its own main over it, with agent-core as an ordinary module dependency (#2756).

<!-- field: non_goals -->
## Non-goals

- New agent-core runtime features, which their own specifications plan.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A scrum-master release is installed, and the repository's issues are popped and merged through it.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- initiative_scrum_master_release_installed

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC2 -->
### AC2

mage audit passes on main.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)

<!-- field: tests -->
#### Tests

- initiative_mage_audit_green

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC3 -->
### AC3

A test binary registers an in-process command through the public package and a profile run calls it, and scrum-master's agent builds with no overlay.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)

<!-- field: tests -->
#### Tests

- initiative_agent_core_public_package

<!-- field: applicability -->
#### Applicability

dev
