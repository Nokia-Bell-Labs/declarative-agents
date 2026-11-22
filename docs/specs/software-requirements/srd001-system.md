---
id: srd001-system
title: "System: declarative-agents"
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: The technical program manager

<!-- field: problem -->
## Problem

Agents whose workflow is code cannot be validated, reviewed, or changed without rebuilding them; the system makes an agent a declaration one runtime executes.

<!-- field: goals -->
## Goals

- G1: Every agent the repository ships runs from its profile on the one runtime.
- G2: The language agents are written in is specified, with evidence for each statement.
- G3: Every component builds, passes its gates, and is released from main.

<!-- req: R1 -->
## R1 - The runtime

<!-- req: R1.1 -->
### R1.1

agent-core runs any agent from its profile, machine, and tool declarations, and rejects a profile its configuration check refuses.

<!-- req: R2 -->
## R2 - The language

<!-- req: R2.1 -->
### R2.1

application-dsl-spec states the language version by version, and every statement of the version agent-core implements has acceptance evidence that passes.

<!-- req: R3 -->
## R3 - Releases

<!-- req: R3.1 -->
### R3.1

mage audit passes on main, and a release is tagged only when every release gate passes.

<!-- field: non_goals -->
## Non-goals

- New features while the repository is in maintenance mode.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Every shipped profile passes the runtime's configuration check (test: mage audit; environment: local).

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- ac: AC2 -->
### AC2

The implemented language version's audit passes with every statement's evidence (test: mage -d application-dsl-spec audit; environment: local).

<!-- field: traces -->
#### Traces

- [R2.1](#r21)

<!-- ac: AC3 -->
### AC3

The release gate passes before a tag (test: mage tag; environment: local).

<!-- field: traces -->
#### Traces

- [R3.1](#r31)

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| declarative-agents | https://github.com/Nokia-Bell-Labs/declarative-agents.git | / |
| agent-core | https://github.com/Nokia-Bell-Labs/declarative-agents.git | agent-core/ |
| operator | https://github.com/Nokia-Bell-Labs/declarative-agents.git | operator/ |
| application-dsl-spec | https://github.com/Nokia-Bell-Labs/declarative-agents.git | application-dsl-spec/ |
| design-patterns | https://github.com/Nokia-Bell-Labs/declarative-agents.git | design-patterns/ |
| applications | https://github.com/Nokia-Bell-Labs/declarative-agents.git | applications/ |
| agent-architecture | https://github.com/Nokia-Bell-Labs/declarative-agents.git | applications/agent-architecture/ |
| catalog | https://github.com/Nokia-Bell-Labs/declarative-agents.git | applications/catalog/ |
| chatbot-mesh | https://github.com/Nokia-Bell-Labs/declarative-agents.git | applications/chatbot-mesh/ |
| coding-agent | https://github.com/Nokia-Bell-Labs/declarative-agents.git | applications/coding-agent/ |
| scrum-master | https://github.com/Nokia-Bell-Labs/declarative-agents.git | applications/scrum-master/ |
| magefiles | https://github.com/Nokia-Bell-Labs/declarative-agents.git | magefiles/ |
