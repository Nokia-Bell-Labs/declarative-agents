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
