---
id: obj003-dangling-trace
title: Reliable agents the product teams can deploy
status: agreed
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W43..2027-W03

<!-- field: problem -->
## Problem

Product teams can deploy agents, but nobody can show that an agent works
reliably before it is deployed.

<!-- field: goals -->
## Goals

- G1: Every agent a product team deploys has passed an evaluation the team can read.

<!-- req: R1 -->
## R1 - Evaluation of one agent

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-11-06"
- delivered_by:
  1. https://gitlab.example.com/program/delivery/fleet/-/blob/v0.1.0/docs/specs/software-requirements/srd001-eval.md#R1.1

<!-- req: R1.1 -->
### R1.1

A grader scores one agent's trajectories against a written rubric.

<!-- req: R1.2 -->
### R1.2

The score and the rubric are stored with the agent's version.

<!-- req: R2 -->
## R2 - A scorecard a product team reads

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-11-27"

<!-- req: R2.1 -->
### R2.1

One page shows each deployed agent's latest scores.

<!-- field: non_goals -->
## Non-goals

- Choosing which agents product teams deploy.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

One agent's trajectories are scored and the score is stored with its version.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)

<!-- field: tests -->
#### Tests

- grader_scores_fixture_agent

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC2 -->
### AC2

The scorecard lists every deployed agent with its latest score.

<!-- field: traces -->
#### Traces

- R2.9

<!-- field: tests -->
#### Tests

- scorecard_lists_agents

<!-- field: applicability -->
#### Applicability

dev
