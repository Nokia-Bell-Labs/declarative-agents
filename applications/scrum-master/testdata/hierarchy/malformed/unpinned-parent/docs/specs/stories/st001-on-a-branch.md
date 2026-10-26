---
id: st001-on-a-branch
title: On a branch
status: agreed
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, po-1
- parent: https://gitlab.example.com/program/system/-/blob/main/docs/specs/epics/ep001-agent-evaluation.md#R1
- as_a: product team lead
- i_want: on a branch
- so_that: I can trust an agent before deploying it
- points: 5
- rank: 1
- cites:
  1. https://gitlab.example.com/program/system/-/blob/v1.0.0/docs/specs/software-requirements/srd001-eval.md#R1.1

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

The story's detail: on a branch.

<!-- field: non_goals -->
## Non-goals

- Choosing which agents teams deploy.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

On a branch is shown to work.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- story_st001_on_a_branch

<!-- field: applicability -->
#### Applicability

dev
