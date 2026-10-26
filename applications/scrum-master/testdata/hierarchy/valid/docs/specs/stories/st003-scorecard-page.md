---
id: st003-scorecard-page
title: Scorecard page
status: agreed
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, po-1
- parent: ep001-agent-evaluation
- as_a: product team lead
- i_want: scorecard page
- so_that: I can trust an agent before deploying it
- points: 8
- rank: 3
- cites:
  1. https://gitlab.example.com/program/system/-/blob/v1.0.0/docs/specs/software-requirements/srd001-eval.md#R1.1
- relates_to:
  1. st001-score-trajectories

<!-- field: problem -->
## Problem

Teams cannot show that an agent works before it is deployed.

<!-- field: goals -->
## Goals

- G1: Every deployed agent has passed an evaluation a team can read.

<!-- req: R1 -->
## R1 - Detail

<!-- req: R1.1 -->
### R1.1

The story's detail: scorecard page.

<!-- field: non_goals -->
## Non-goals

- Choosing which agents teams deploy.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Scorecard page is shown to work.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- story_st003_scorecard_page

<!-- field: applicability -->
#### Applicability

dev
