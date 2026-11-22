---
id: in002-agents-as-resources
title: Applications declare their agents as Agent resources
status: agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W48..2027-W13
- parent: th002-declarative-deployment
- architecture:
  1. operator
  1. chatbot-mesh

<!-- field: problem -->
## Problem

The operator derives an agent's ports, probe, monitor label, and bind host from its profile, but no application deploys through it yet, so whether the Agent spec is sufficient is untested.

<!-- field: goals -->
## Goals

- G1: The chatbot-mesh's plain agents run as Agent resources.

<!-- req: R1 -->
## R1 - The chatbot-mesh's agents

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2027-03-31"

<!-- req: R1.1 -->
### R1.1

The chatbot-mesh chart declares its creator, orchestrator, and rag-servers as Agent resources, and the control plane's creator/applier path is revised for them.

<!-- field: non_goals -->
## Non-goals

- Converting the observer, the chatbot, the shared library-chart services, or chroma.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The chatbot-mesh kind proof passes with the three Agents Ready.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- initiative_chatbot_mesh_agents

<!-- field: applicability -->
#### Applicability

local
