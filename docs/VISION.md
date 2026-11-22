---
id: program-vision
title: Declarative Agents Vision
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: executive_summary -->
## Executive summary

declarative-agents is a profile-driven runtime and a set of design patterns for tool-augmented LLM agents. An agent is a YAML profile: its tools, states, transitions, signals, and budgets. One Go runtime, agent-core, executes whatever loop the profile declares, so changing a workflow is a configuration edit and a new agent needs no new binary. Around the runtime sit its Kubernetes operator, the normative specification of the language agents are written in, the white paper Design Patterns for Declarative Agents, and the applications built from the reusable catalog. It serves researchers and engineers who build, deploy, and study reliable agents. The repository is in maintenance mode and accepts bug fixes only.

<!-- field: problem -->
## Problem

Most agent frameworks bind workflow logic to code, so every loop change forces a rebuild, and what an agent may do is read from source rather than from a declaration a tool can check.

<!-- field: what_this_does -->
## What this does

It delivers one runtime that runs any declared agent, a language specification with acceptance evidence for every statement, an operator that deploys agents from their profiles, a catalog of reusable tool and agent blocks, applications composed from them, and the process that plans and delivers the repository's own work.

<!-- field: why_we_build_this -->
## Why we build this

An agent whose behaviour is data can be validated before it runs, reviewed like configuration, and changed without a release of the runtime; the patterns paper argues that this is what makes agents reliable, and the repository is its evidence.

<!-- field: success_criteria -->
## Success criteria

- profiles_validate: every shipped profile passes the runtime's configuration check in mage audit.
- spec_evidence: every statement of the implemented language version has acceptance evidence that passes.
- green_main: mage audit and the release gate pass on main.

<!-- field: not -->
## Not

- A general agent platform with hosted services; the repository ships source, binaries, and charts.
- A place for new features while in maintenance mode; only bug fixes and maintainer-granted exceptions land.
