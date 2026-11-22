---
id: program-architecture
title: Program Architecture
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

declarative-agents is one repository holding a runtime, its operator, the language specification agents are written in, a design-patterns paper, and the applications built on them. This document is the index of those parts: each is a component below, and each component with an architecture of its own points to it, where its structure is described in the detail this index has no room for.

<!-- field: interfaces -->
## Interfaces

<!-- field: components -->
## Components

<!-- component: declarative-agents -->
### declarative-agents

The repository's own system corpus and control: this docs/ folder, control/, and the scrum-master profiles vendored in .scrum-master. It is the system repository of a program of one repository.

<!-- field: capabilities -->
#### Capabilities

- plan: themes, initiatives, and epics under docs/specs
- control: the label schema, admission policy, and states in control/

<!-- field: ownership -->
#### Ownership

- docs/ (outside the module folders)
- control/
- .scrum-master/

<!-- component: agent-core -->
### agent-core

The profile-driven runtime: one agent binary that runs any declarative agent from its profile, machine, and tool declarations. Its architecture is agent-core/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- run: an agent from its profile, machine, and tools
- specify: the Markdown corpus format and the spec tooling every module's documents use

<!-- field: ownership -->
#### Ownership

- agent-core/

<!-- component: operator -->
### operator

The Kubernetes operator: the Agent custom resource and the controller that reconciles an Agent into a Deployment. It has no architecture document of its own yet.

<!-- field: capabilities -->
#### Capabilities

- reconcile: an Agent resource into one Deployment

<!-- field: ownership -->
#### Ownership

- operator/

<!-- component: application-dsl-spec -->
### application-dsl-spec

The normative specification of the language agents here are written in, one folder per version, with the audit that checks its statements, evidence, and the conformance citations in docs/conformance.

<!-- field: capabilities -->
#### Capabilities

- specify: the application DSL, version by version
- audit: statement evidence and conformance citations

<!-- field: ownership -->
#### Ownership

- application-dsl-spec/
- docs/conformance/

<!-- component: design-patterns -->
### design-patterns

The design-patterns paper for declarative agents and its pattern language.

<!-- field: capabilities -->
#### Capabilities

- publish: the paper's chapters and pattern-language.yaml

<!-- field: ownership -->
#### Ownership

- design-patterns/

<!-- component: applications -->
### applications

The shared layer of the application modules: what applications/ holds and how its modules relate. Its architecture is applications/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- host: the application modules and the reusable catalog

<!-- field: ownership -->
#### Ownership

- applications/ (outside the application folders)

<!-- component: agent-architecture -->
### agent-architecture

The application that drives the Knowledge Manager documentation agent from its slide deck. Its architecture is applications/agent-architecture/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- deploy: the documentation agent and its applier

<!-- field: ownership -->
#### Ownership

- applications/agent-architecture/

<!-- component: catalog -->
### catalog

The reusable catalog of declarative tool and agent blocks for agent-core applications. Its architecture is applications/catalog/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- provide: canonical tool and agent blocks applications compose

<!-- field: ownership -->
#### Ownership

- applications/catalog/

<!-- component: chatbot-mesh -->
### chatbot-mesh

A routed, multi-RAG, observable chatbot built from declarative agents. Its architecture is applications/chatbot-mesh/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- answer: a chat turn routed across RAG servers

<!-- field: ownership -->
#### Ownership

- applications/chatbot-mesh/

<!-- component: coding-agent -->
### coding-agent

A deployable planner, executor, and critic coding loop built from catalog agents. Its architecture is applications/coding-agent/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- code: plan, execute, and critique a change

<!-- field: ownership -->
#### Ownership

- applications/coding-agent/

<!-- component: scrum-master -->
### scrum-master

The AI-native scrum process as a CLI and model-free agent profiles, released and installed with install.sh. Its architecture is applications/scrum-master/docs/ARCHITECTURE.md.

<!-- field: capabilities -->
#### Capabilities

- deliver: pop and merge issues
- plan and report: the planning hierarchy, status reports, and the site

<!-- field: ownership -->
#### Ownership

- applications/scrum-master/

<!-- component: magefiles -->
### magefiles

The repository's build tooling: the mage targets, including mage audit, which runs every module's gate.

<!-- field: capabilities -->
#### Capabilities

- audit: every module's gates in one run

<!-- field: ownership -->
#### Ownership

- magefiles/
- build/

<!-- field: design_decisions -->
## Design decisions

<!-- field: technology_choices -->
## Technology choices

| component | technology | purpose |
| --- | --- | --- |
| agent-core | Go | one static agent binary |
| operator | Go, controller-runtime | reconciles Agent resources on Kubernetes |
| scrum-master | Go | the CLI and words the profiles run |
| magefiles | mage | the repository's build and audit targets |

<!-- field: project_structure -->
## Project structure

One repository, github.com/Nokia-Bell-Labs/declarative-agents, holds every component, each in the folder its ownership names; the Go modules are joined by go.work.

<!-- field: implementation_status -->
## Implementation status

Every component is built and released from main. The operator, application-dsl-spec, design-patterns, and magefiles have no architecture document of their own yet.

<!-- field: related_documents -->
## Related documents

- docs/VISION.md
- docs/SPECIFICATIONS.md
- docs/road-map.md
- docs/engineering/README.md
- docs/constitutions/design.md
