---
id: th001-repository-operations
title: Repository operations
status: to_be_agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W38..2026-W52

<!-- field: problem -->
## Problem

The declarative-agents repository grows by hundreds of issues a month, and the way it plans, delivers, and keeps itself healthy lives in session skills and people's heads rather than in a process anyone can read.

<!-- field: goals -->
## Goals

- G1: Every change to the repository is planned, delivered, and recorded through one readable process.
- G2: The repository's own gates stay green as it grows.

<!-- req: R1 -->
## R1 - An operated repository

<!-- req: R1.1 -->
### R1.1

The repository is planned and delivered through scrum-master, and its maintenance is visible in the plan.

<!-- field: non_goals -->
## Non-goals

- Planning the framework's product roadmap, which the specifications and release documents hold.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The repository's work is listed in the planning hierarchy, with its delivery and maintenance under it.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- theme_repository_operations

<!-- field: applicability -->
#### Applicability

dev
