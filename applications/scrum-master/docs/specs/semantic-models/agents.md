---
id: agents
title: Agents and the loops they run
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines what an agent is in this process and the loops every agent runs. The roles model says which human roles exist and the planes model says where rules and intent live; this model says how work gets done between them. There are no role-specific agents: every unit of work is an issue that states its own Requirements, Acceptance Criteria, and Design Decisions, every script talks only to the tracker, and so one inner loop serves every role and every kind of work.

<!-- field: overview -->
## Overview

An agent is a session that runs one skill for the person who started it. It reads and writes the issue tracker and the program's versioned files, and nothing else. What it builds is set by the issue it was given, not by the role of the person who started it.

<!-- field: ownership_rules -->
## Ownership rules

A change the Retrospective keeps is a plane::control issue worked against policy.md through the inner loop and closed by a reviewed merge (srd001 R2.2, srd033). There is no separate path for changing a rule.

<!-- field: overall_findings -->
## Overall findings

Two earlier rosters named role-specific agents: first twelve, one per role, plane, and time scale, then five, one per role and plane. Both re-specified, under each role's name, what specify, implement, and scrum-master release already do, and both needed artifact handoffs between agents to explain work that the tracker already moves. Once every unit of work is an issue carrying its own requirements and acceptance criteria, the role decides only which issues a person pulls, which kind already records. The roster collapses to the two loops and the outer-loop skills that feed them.

<!-- field: operations | form: "tree" -->
## Operations

- inner_loop:
  - steps:
    1. - skill: tasks
       - does: turns each chosen story into the issues that execute it
       - "each in the issue body's work format": null
    1. - skill: specify
       - does: files an issue in the issue body's work format (config format issue-body)
    1. - skill: implement
       - does: takes the issue
       - "assigning only the person who runs it": null
       - "and opens its own branch and worktree (srd036); a story's issues share one branch (srd036 R5)": null
    1. - skill: implement (build)
       - does: builds each issue against its Requirements and Acceptance Criteria
       - "commits with the unit's record embedded": null
       - "and logs actual against estimated size": null
    1. - skill: implement (finish)
       - does: merges the branch, which closes the issue
       - "gated by mage audit for a change under docs/; a reviewer merges it": null
    1. - skill: implement (release issue)
       - does: runs scrum-master release next
       - gate: null
       - "and tag": null
       - "so a tag exists only once the artifact's gate passed (srd008": null
       - "srd015); handoff routes it downstream (srd014)": null
  - used_by: every role, for every kind of work, unchanged.
- audits:
  - definition: Recurring issues (a title starting "Recurring:" or a Recurrence section) popped and worked by the inner loop, on a schedule or when a person triggers one. Findings are pushed as issues; implement (Phase 6) re-creates the recurring issue for the next run.
  - examples:
    1. regression tests and benchmarks against the default branch
    1. contract checks at every artifact boundary (srd008)
    1. - track-goals: follows pinned references, collects evidence, posts ledger events, and closes met Sprint Goals (srd019)
    1. the program dashboard (scrum-master goals status, srd019)
    1. the weekly sprint update and the stakeholder update (srd018 R6)
- outer_loops:
  - definition: The Scrum skills, for the workflows where people decide together. Each reads the tracker, asks people bounded questions, and pushes issues in the issue body's work format for the inner loop to work.
  - skills:
    1. - skill: plan
       - srd: srd031
       - started_by: technical program manager (system) or product owner (subsystem)
       - when: before each three-week period
       - pushes: the road-map release entry and planned events
    1. - skill: tasks
       - srd: srd022
       - started_by: technical program manager or a product owner with downstream repositories
       - when: after plan
       - pushes: Sprint Goal issues citing pinned initiative and epic items
    1. - skill: daily-scrum
       - srd: srd020
       - started_by: each developer
       - when: daily
       - pushes: blocker and interrupt issues
    1. - skill: sprint-review
       - srd: srd032
       - started_by: product owner
       - when: end of each three-week period
       - pushes: follow-up items
    1. - skill: sprint-retrospective
       - srd: srd033
       - started_by: scrum master with the team
       - when: end of each three-week period
       - pushes: policy-change issues

<!-- field: actor_boundaries -->
## Actor boundaries

| id | rule |
| --- | --- |
| A1 | One inner loop for all work. Code, architecture documents, OpenAPI schemas, infrastructure (Terraform, Helm), papers, research, and reviews all run specify, implement, and the reviewed merge. The issue states the deliverable and its acceptance criteria; implement needs no extension per kind of deliverable. |
| A2 | Scripts (scrum-master subcommands) are tools the loops call, not agents. Event-driven scripts (lint, track-goals, handoff) run in CI on tracker events; they are tools the loops depend on, not agents either. |
| A3 | People direct agents; agents never direct other agents. Work moves between sessions only through the tracker, as issues, comments, links, and tags, under the same rules as everyone else. |
| A4 | An agent never assigns work to anyone but the person who ran implement, never approves anything, and never changes a rule except by a merge a person approves. Taking, approving, and rule changes are the three acts the process reserves for people. |
| A5 | The kind label routes pull: it decides who takes an issue, whether it is offered, and how it is counted (srd001 R20.4). It does not change how the work runs. |
