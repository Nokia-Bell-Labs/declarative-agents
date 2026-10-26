---
id: ft001-grader
title: The grader
status: agreed
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, po-1
- parent: ep001-agent-evaluation

<!-- field: problem -->
## Problem

Teams cannot show that an agent works before it is deployed.

<!-- field: goals -->
## Goals

- G1: Every deployed agent has passed an evaluation a team can read.

<!-- req: R1 -->
## R1 - Scoring

<!-- req: R1.1 -->
### R1.1

The grader scores a trajectory.

<!-- field: non_goals -->
## Non-goals

- Choosing which agents teams deploy.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A score is produced.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- grader_scores

<!-- field: applicability -->
#### Applicability

dev
