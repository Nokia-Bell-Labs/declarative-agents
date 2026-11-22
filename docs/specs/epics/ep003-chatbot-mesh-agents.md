---
id: ep003-chatbot-mesh-agents
title: The chatbot-mesh's agents as Agent resources
status: to_be_agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W48..2027-W13
- parent: in002-agents-as-resources
- architecture:
  1. chatbot-mesh
  1. operator

<!-- field: problem -->
## Problem

The chatbot-mesh chart hand-writes a Deployment and a Service for each of its plain agent-core processes, and its control plane adds agents by asking the applier to upgrade the whole Helm release.

<!-- field: goals -->
## Goals

- G1: The creator, the provisioning-workflow-orchestrator, and each rag-server run as Agent resources (#2689).
- G2: The control plane's way of adding an agent is decided against Agent resources (#2690).

<!-- req: R1 -->
## R1 - The conversion

<!-- req: R1.1 -->
### R1.1

The chart renders one Agent each for the creator, the orchestrator, and every rag-server, under today's names, and no Deployment or Service for them.

<!-- req: R2 -->
## R2 - The creator and the applier

<!-- req: R2.1 -->
### R2.1

srd005 and srd006 record whether the creator creates Agent resources directly and what remains of the applier.

<!-- field: non_goals -->
## Non-goals

- The observer, whose kubectl proxy sidecar stays (GH-1302).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The chatbot-mesh kind proof passes with the three Agents Ready, and the policy proof passes unchanged.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- epic_chatbot_mesh_agents_ready

<!-- field: applicability -->
#### Applicability

local

<!-- ac: AC2 -->
### AC2

The revised srd005 and srd006 pass the chatbot-mesh audit.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)

<!-- field: tests -->
#### Tests

- epic_creator_applier_revision

<!-- field: applicability -->
#### Applicability

local
