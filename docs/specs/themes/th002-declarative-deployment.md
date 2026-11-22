---
id: th002-declarative-deployment
title: Declarative deployment
status: agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W38..2027-W13
- architecture:
  1. operator

<!-- field: problem -->
## Problem

An application's agents are deployed through hand-written Deployments and Services, so what runs on a cluster is not read from the agents' own declarations.

<!-- field: goals -->
## Goals

- G1: An agent deploys from its profile as one Agent resource.

<!-- req: R1 -->
## R1 - Agents deployed from their declarations

<!-- req: R1.1 -->
### R1.1

The operator deploys an agent from its profile, and the applications declare their agents as Agent resources.

<!-- field: non_goals -->
## Non-goals

- Managing workloads that are not agent-core processes.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

An application's agents run on a cluster as Agent resources.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- theme_agents_as_resources

<!-- field: applicability -->
#### Applicability

local
