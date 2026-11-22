---
id: ep002-maintenance
title: Maintenance
status: to_be_agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W38..2026-W52
- parent: in001-scrum-run-repository

<!-- field: problem -->
## Problem

Technical debt found while working on the framework was filed issue by issue, with no plan that shows how much there is or what has been retired.

<!-- field: goals -->
## Goals

- G1: main stays green under mage audit.
- G2: Debt is filed under this epic and retired in the sprint it is found, or planned.

<!-- req: R1 -->
## R1 - Green gates

<!-- req: R1.1 -->
### R1.1

Every repository gate runs inside mage audit and passes on main, with no flaking test.

<!-- req: R2 -->
## R2 - Conforming specification corpora

<!-- req: R2.1 -->
### R2.1

Every citation resolves to a requirement item, and every evidence selector matches a test.

<!-- req: R3 -->
## R3 - Dependencies and reproducible releases

<!-- req: R3.1 -->
### R3.1

Dependency advisories are resolved, and a release gate reproduces the same artifacts from the same commit.

<!-- req: R4 -->
## R4 - The repository's own process

<!-- req: R4.1 -->
### R4.1

The repository runs its issue loop with an installed scrum-master release, its corpus conforming to the repository's gates.

<!-- field: non_goals -->
## Non-goals

- New features, which their own epics plan.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

mage audit passes on main.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- epic_maintenance_green_gates

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC2 -->
### AC2

The specification corpus audits report no unresolved citation and no empty evidence selector.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)

<!-- field: tests -->
#### Tests

- epic_maintenance_corpus_conformance

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC3 -->
### AC3

The release gate's reproducibility check passes.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)

<!-- field: tests -->
#### Tests

- epic_maintenance_reproducible_release

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC4 -->
### AC4

.scrum-master/manifest.json is on main and the check profile passes.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)

<!-- field: tests -->
#### Tests

- epic_maintenance_scrum_master_adopted

<!-- field: applicability -->
#### Applicability

dev

<!-- field: issues | form: "tree" -->
## Issues

- "Nokia-Bell-Labs/declarative-agents#2347":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2347"
  - title: "Tech debt: defects found in passing"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2365":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2365"
  - title: "Migrate the 328 grandfathered group citations to requirement items"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2372":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2372"
  - title: "main is red: legacy-group-citations.txt matches no declared document type"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2373":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2373"
  - title: "main is red: a Helm template still emits the retired instantiate keyname"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2375":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2375"
  - title: "A go_test evidence selector that matches no test reports as passing"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2385":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2385"
  - title: "Blueprint and capability-profile releases have no tests: 16 evidence selectors match nothing"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2392":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2392"
  - title: "Narrow the enumerated citations where the prose names a subset"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2399":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2399"
  - title: "Tech debt: specification corpora"
  - who: petar-djukic
  - opened: "2026-09-20"
  - closed: "2026-09-20"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2427":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2427"
  - title: "Tech debt: kindrig"
  - who: petar-djukic
  - opened: "2026-09-21"
  - closed: "2026-09-21"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2441":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2441"
  - title: "Tech debt: agent-core tests"
  - who: petar-djukic
  - opened: "2026-09-21"
  - closed: "2026-09-21"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2459":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2459"
  - title: "Tech debt: magefiles tests are red on main and outside mage audit"
  - who: petar-djukic
  - opened: "2026-09-21"
  - closed: "2026-09-21"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2460":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2460"
  - title: "Tech debt: TestDeployWarnsOnAnUnrecognizableHelmVersion flakes between two warning branches"
  - who: petar-djukic
  - opened: "2026-09-21"
  - closed: "2026-09-21"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2461":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2461"
  - title: "Tech debt: build tooling"
  - who: petar-djukic
  - opened: "2026-09-21"
  - closed: "2026-09-21"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2546":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2546"
  - title: "Tech debt epic: documentation corpus cleanup"
  - who: petar-djukic
  - opened: "2026-10-14"
  - closed: "2026-10-14"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2568":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2568"
  - title: "Shared kind session captures adopted scenario evidence after the scenario's kubeconfig is gone"
  - who: petar-djukic
  - opened: "2026-10-15"
  - closed: "2026-10-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2592":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2592"
  - title: "Tech debt"
  - who: petar-djukic
  - opened: "2026-10-16"
  - closed: "2026-10-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2611":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2611"
  - title: "ParseUseCase rejects flow steps that carry traces; six applications use cases escape the typed checks"
  - who: petar-djukic
  - opened: "2026-10-16"
  - closed: "2026-10-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2627":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2627"
  - title: "Applications test-suite commands parse Markdown corpus documents with yaml.safe_load"
  - who: petar-djukic
  - opened: "2026-10-16"
  - closed: "2026-10-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2629":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2629"
  - title: "Documentation curator renders the Markdown corpus as raw text; e2e and live integration not run since the migration"
  - who: petar-djukic
  - opened: "2026-10-16"
  - closed: "2026-10-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2702":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2702"
  - title: "Tech debt: UI dependency advisories"
  - who: petar-djukic
  - opened: "2026-10-22"
  - closed: "2026-10-22"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2709":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2709"
  - title: "Tech debt: release gate reproducibility"
  - who: petar-djukic
  - opened: "2026-10-22"
  - closed: "2026-10-22"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2937":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2937"
  - title: "Tech debt: application-dsl-spec 1.0 statements"
  - who: petar-djukic
  - opened: "2026-11-21"
  - closed: "2026-11-21"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2943":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2943"
  - title: "Adopt scrum-master for this repository's issue loop"
  - who: petar-djukic
  - opened: "2026-11-22"
  - closed: "2026-11-22"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2945":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2945"
  - title: "The planning documents' issues record is rewritten by agent-core's SRD formatter"
  - who: petar-djukic
  - opened: "2026-11-22"
  - closed: "2026-11-22"
  - state: closed
