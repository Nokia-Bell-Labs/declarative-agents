---
id: st002-second
title: Second
status: agreed
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, po-1
- parent: https://gitlab.example.com/program/system/-/blob/v1.0.0/docs/specs/epics/ep001-agent-evaluation.md#R1
- as_a: product team lead
- i_want: second
- so_that: I can trust an agent before deploying it
- points: 5
- rank: 1
- cites:
  1. https://gitlab.example.com/program/system/-/blob/v1.0.0/docs/specs/software-requirements/srd001-eval.md#R1.1
- blocked_by:
  1. st001-first

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

The story's detail: second.

<!-- field: non_goals -->
## Non-goals

- Choosing which agents teams deploy.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Second is shown to work.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- story_st002_second

<!-- field: applicability -->
#### Applicability

dev
