---
id: srd010-delivery
title: "Subsystem: delivery"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: Product Owner, delivery

<!-- field: problem -->
## Problem

Agents are deployed and watched.

<!-- field: goals -->
## Goals

- G1: Promoted agents run in production.

<!-- req: R1 -->
## R1 - Fleet

<!-- req: R1.1 -->
### R1.1

The fleet runs promoted agents.

<!-- field: non_goals -->
## Non-goals

- Training models.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A promoted agent runs.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| fleet | https://gitlab.example.com/program/delivery/fleet | / |
