---
id: ep001-scrum-master
title: The scrum-master implementation
status: to_be_agreed
authors: petar-djukic
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W44..2026-W48
- parent: in001-scrum-run-repository

<!-- field: problem -->
## Problem

An AI-native scrum process needs its workflows run as model-free agent profiles, so delivery, planning, and reporting are deterministic and cost a coding session two requests per step instead of a re-read of its whole context.

<!-- field: goals -->
## Goals

- G1: scrum-master ships as a release that installs the agent and its profiles on Linux and macOS.
- G2: A program of repositories on GitHub, GitLab, or beads is planned, delivered, and reported through it.

<!-- req: R1 -->
## R1 - Delivery

<!-- req: R1.1 -->
### R1.1

An issue is claimed into its own branch and worktree, and delivered by the one merge that closes it, behind the repository's gates and its reviewers' sign-off.

<!-- req: R2 -->
## R2 - Planning

<!-- req: R2.1 -->
### R2.1

A repository is laid down with init or onboard, its planning hierarchy is drafted and checked, and each story and epic carries the record of the issues that implement it.

<!-- req: R3 -->
## R3 - Reporting

<!-- req: R3.1 -->
### R3.1

Weekly status reports and rollups are filed and written where the work is, and a generated site shows the program's status, timeline, and backlog.

<!-- req: R4 -->
## R4 - Programs across repositories and forges

<!-- req: R4.1 -->
### R4.1

The program's checkouts, releases, and drift are managed from the system repository across GitHub, GitLab, and beads.

<!-- req: R5 -->
## R5 - Releases

<!-- req: R5.1 -->
### R5.1

A release builds against a pinned runtime, passes its gates, and installs with install.sh; the upgrade profile brings a repository to it.

<!-- field: non_goals -->
## Non-goals

- Replacing the forge's own issue tracker or review interface.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

An issue is popped and merged through the merge profile.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- epic_scrum_master_delivery

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC2 -->
### AC2

A repository laid down by init passes the check profile.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)

<!-- field: tests -->
#### Tests

- epic_scrum_master_planning

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC3 -->
### AC3

The site renders a program's status, timeline, and backlog.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)

<!-- field: tests -->
#### Tests

- epic_scrum_master_reporting

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC4 -->
### AC4

roll and reconcile reach every repository the registry names.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)

<!-- field: tests -->
#### Tests

- epic_scrum_master_programs

<!-- field: applicability -->
#### Applicability

dev

<!-- ac: AC5 -->
### AC5

install.sh installs a tagged release.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)

<!-- field: tests -->
#### Tests

- epic_scrum_master_releases

<!-- field: applicability -->
#### Applicability

dev
