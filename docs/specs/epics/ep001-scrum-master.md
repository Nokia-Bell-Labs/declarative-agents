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
- architecture:
  1. scrum-master

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

<!-- field: issues | form: "tree" -->
## Issues

- "Nokia-Bell-Labs/declarative-agents#2713":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2713"
  - title: "Add the scrum-master application and the agent-core in-process tool support"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2715":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2715"
  - title: "scrum-master corpus pins a parent release tag this repository does not have"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2717":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2717"
  - title: "scrum-master: add five hands-on tutorials under docs/tutorials/"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2719":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2719"
  - title: "scrum-master: add an upgrade profile that brings a program repository to the installed release"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2721":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2721"
  - title: "scrum-master: make the release image opt-in instead of pushing to ghcr.io/nokia-bell-labs"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2723":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2723"
  - title: "scrum-master site: resizable navigator, timeline frame after a lapsed session, the plan as cards, and visible levels"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2725":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2725"
  - title: "scrum-master: add status-report and status-rollup skills, with reports kept where the work is and collected per team and program"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2726":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2726"
  - title: "scrum-master: support cross-team epics, with roll-up across the scrums' repositories and Sprint Goals routed by area"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-26"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2730":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2730"
  - title: "Site: the navigator divider does not drag in windows narrower than 769 pixels"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2732":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2732"
  - title: "Site: a page per scrum, showing the epics and stories each team works on"
  - who: petar-djukic
  - opened: "2026-10-26"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2734":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2734"
  - title: "Site: the navigator divider is invisible until hovered"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2736":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2736"
  - title: "Docs: tutorials and README catch up with routing by area, rollups in place, and the scrum pages"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2738":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2738"
  - title: "A scrum repository for a team's planning, apart from its code repositories"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2740":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2740"
  - title: "Programs of programs: a sub-program on the parent's site, and a link up to the parent"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2742":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2742"
  - title: "A sub-program's program rollup is its team rollup at the level above"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2743":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2743"
  - title: "scrum-master: find the system checkout when checkouts mirror the forge's group layout"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2744":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2744"
  - title: "scrum-master: the one-component admission rule depends on whether control is read from disk or through the tracker"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2745":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2745"
  - title: "scrum-master: onboard a scrum repository, and the subsystem SRD of a new team"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2749":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2749"
  - title: "Install fewer, coarser skills: workflows and four group skills, every profile still shipped"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2751":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2751"
  - title: "scrum-master v0.20261026.6 release has no agent binaries, so install.sh fails"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2752":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2752"
  - title: "scrum-master v0.20261026.6: merge refuses every popped worktree, taking its folder name for the project"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2754":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2754"
  - title: "scrum-master: build, gate, and release against a pinned runtime release, and check it out of tree"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2755":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2755"
  - title: "scrum-master: set the runtime's identity and its own each in one place, and derive the module path"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2759":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2759"
  - title: "scrum-master release: the out-of-tree gate fails its own module-path guard"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2761":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2761"
  - title: "scrum-master: fetch a pinned runtime's source with git, from any forge"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2763":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2763"
  - title: "scrum-master: tests and a gate that assume the default forge, a module path's text, and no runtime pin"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2765":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2765"
  - title: "scrum-master: a runtime cloned with git lacks the releases the corpus's derived_from entries cite"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2767":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2767"
  - title: "scrum-master install.sh cannot download a private GitLab project's release with a token"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2769":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2769"
  - title: "scrum-master: a word given its system as a checkout's path refuses: no checkout of the project"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2771":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2771"
  - title: "scrum-master check reports pin drift on every corpus built against a pinned runtime"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2772":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2772"
  - title: "scrum-master goals plan routes an item to teams whose epics do not cite it"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2774":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2774"
  - title: "scrum-master: status reports written in place make an unchanged corpus look releasable"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2775":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2775"
  - title: "scrum-master: aggregate, sprint-end-facts and site choose different report sources, silently"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2776":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2776"
  - title: "scrum-master: the site's unreported-merged-work signal reaches back before the program began"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2777":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2777"
  - title: "scrum-master: reports-path names a different file and branch from the status issue"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2778":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2778"
  - title: "scrum-master: the weekly report's sections differ between the status issue, its schema, and reports-validate"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2779":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2779"
  - title: "scrum-master: a subsystem with no reporters gets a rollup issue and a first-release recommendation"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2780":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2780"
  - title: "scrum-master: backlog check warns on a document it refuses as untracked"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2781":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2781"
  - title: "scrum-master: small output inconsistencies in check, init, plan-push and the site"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-27"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2790":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2790"
  - title: "scrum-master: reports validate passes an incomplete in-place report named by an absolute path"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2791":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2791"
  - title: "scrum-master: aggregate without control cannot find in-place reports from a site-only reports checkout"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2792":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2792"
  - title: "scrum-master: pop, reports validate, and check still print null for empty lists"
  - who: petar-djukic
  - opened: "2026-10-27"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2796":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2796"
  - title: "scrum-master: merge leaves the issue's worktree and branches behind"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2799":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2799"
  - title: "scrum-master: reports source names a checkout as \".\", and a control-plane merge reads as unreported work"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2801":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2801"
  - title: "scrum-master: merge of an empty branch takes an earlier merge citing the same reference as its own"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2803":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2803"
  - title: "scrum-master: checkout lookup should find repositories nested in subgroups (mirrored group layout)"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2809":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2809"
  - title: "scrum-master: use the acme example group in the checkout, program, and routing tests"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2811":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2811"
  - title: "scrum-master: support the program template with initiative repositories (registry says what plan level each repository holds)"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2812":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2812"
  - title: "scrum-master: one word lays down and updates every registered repository's checkout from the system checkout"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2813":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2813"
  - title: "scrum-master: roll a release across the program: an upgrade merge request in every registered repository"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-10-28"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2821":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2821"
  - title: "The repository-wide release gates are red in the scrum-master regression lane"
  - who: petar-djukic
  - opened: "2026-10-28"
  - closed: "2026-11-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2822":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2822"
  - title: "scrum-master: reconcile the program against its forge: a sweep that files an issue per drifted repository"
  - who: petar-djukic
  - opened: "2026-10-29"
  - closed: "2026-10-29"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2824":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2824"
  - title: "scrum-master: plan push files sprint-end issues in an initiative repository"
  - who: petar-djukic
  - opened: "2026-10-29"
  - closed: "2026-10-29"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2826":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2826"
  - title: "scrum-master: story pages show no requirements the story cites"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2828":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2828"
  - title: "Architecture site pages and component SRDs of their own (spec)"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2830":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2830"
  - title: "The site's architecture word: a page per subsystem and component (srd048 R9)"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2832":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2832"
  - title: "The site machine lacks the architecture word's state, and the site golden tree is stale (GH-2830)"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2834":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2834"
  - title: "issue-file refuses on a system repository without subsystem SRDs"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2836":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2836"
  - title: "The gantt pages open with the interactive timeline in the shell, and mermaid charts scroll instead of squashing"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2837":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2837"
  - title: "The program's vision is ambient on the site: the overview opens with it, the navigator and each team's page link it"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2840":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2840"
  - title: "A corpus copy links the plain docs/ paths it names"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2841":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2841"
  - title: "The interactive timeline clips to a chosen sprint range"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2844":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2844"
  - title: "A story carries its issue ledger, written by the issue words and reconciled"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2845":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2845"
  - title: "The site's backlog pages draw the story ledgers from the checkouts"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2851":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2851"
  - title: "The site declarations stay under the style gate's line limit"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2853":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2853"
  - title: "The ledger's new requirement items trace to acceptance criteria"
  - who: petar-djukic
  - opened: "2026-11-01"
  - closed: "2026-11-01"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2855":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2855"
  - title: "The backlog's embedded view is a kanban board"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-02"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2857":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2857"
  - title: "A backlog card names its issue by its short ref"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-02"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2859":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2859"
  - title: "The timeline word reuses committed tagged facts, and the page fetches a tag's plan on demand"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-02"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2861":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2861"
  - title: "A pushed release tag can ship with no GitHub release, so install.sh cannot install it"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-02"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2863":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2863"
  - title: "A program names who is on each epic and scrum, and the site shows them"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-02"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2864":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2864"
  - title: "An issue can name an epic directly, without a story"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2865":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2865"
  - title: "An open epic has a matching issue on GitHub, GitLab, and beads"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2866":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2866"
  - title: "A person can assign an issue to someone else, and see what is assigned to them"
  - who: petar-djukic
  - opened: "2026-11-02"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2869":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2869"
  - title: "A named person signs off on an issue or document, with a default when no one is named"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2871":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2871"
  - title: "A skill can run people allocate, release, list, and issue link"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2874":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2874"
  - title: "A document names its authors"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2877":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2877"
  - title: "An epic's mirror needs no sign-off to record, doesn't rewrite on GitLab, and stays out of --mine"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2879":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2879"
  - title: "A reviewer can sign off on their own work, so a one-person repository isn't locked out"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2881":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2881"
  - title: "The site stays current, a missing checkout doesn't stop it, and a lone clone finds its program"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2883":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2883"
  - title: "GitLab requests set their own Content-Type, so a merge isn't refused on an older glab"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2885":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2885"
  - title: "Pop works without variables and lists what can be taken, and a fresh clone has one way to get started"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2887":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2887"
  - title: "A recorded story or epic fails its own repository's corpus gate"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2889":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2889"
  - title: "An upgrade is refused for problems it did not cause, and fails on a repository with no agent directories"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2891":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2891"
  - title: "The formatter rewrites a story's issues record into a form the record cannot read"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2893":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2893"
  - title: "backlog list pulls first and shows each story's issues from the tracker"
  - who: petar-djukic
  - opened: "2026-11-03"
  - closed: "2026-11-03"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2895":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2895"
  - title: "The site links epics and stories directly, each on a page that filters them"
  - who: petar-djukic
  - opened: "2026-11-04"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2896":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2896"
  - title: "The timeline filters by theme and initiative, keeps a bounded height, and resizes its name column"
  - who: petar-djukic
  - opened: "2026-11-04"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2899":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2899"
  - title: "An older agent cannot downgrade a repository, and a skill run names the drift"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2901":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2901"
  - title: "Cross-repository links parse on GitHub, not only in GitLab's form"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2903":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2903"
  - title: "On GitHub a group's labels, milestones and reconcile sweep reach the program's repositories, not the whole account"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2904":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2904"
  - title: "setup finds a program on a private personal account, can be told where it is, and reports when it couldn't"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2905":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2905"
  - title: "init accepts gh and glab as its tracker"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2906":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2906"
  - title: "merge --branch fetches a branch that exists only on origin"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2907":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2907"
  - title: "issue link names a story in the team's scrum repository from a code repository"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2908":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2908"
  - title: "A milestone created on GitHub is due on the day asked for"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2915":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2915"
  - title: "A program's reports repository takes a release like the rest of the program"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2917":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2917"
  - title: "roll files its upgrade issues on the active sprint milestone"
  - who: petar-djukic
  - opened: "2026-11-05"
  - closed: "2026-11-05"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2919":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2919"
  - title: "The spec tool reads the generated site's corpus copies as hierarchy documents"
  - who: petar-djukic
  - opened: "2026-11-06"
  - closed: "2026-11-06"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2921":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2921"
  - title: "The spec tool reads testdata fixtures as corpus documents"
  - who: petar-djukic
  - opened: "2026-11-07"
  - closed: "2026-11-07"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2923":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2923"
  - title: "reconcile's group sweep reports the system SRD's other repositories and declared non-program repositories as unregistered"
  - who: petar-djukic
  - opened: "2026-11-13"
  - closed: "2026-11-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2926":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2926"
  - title: "The regression rig's sprint milestone is due on a fixed date, so every rig target filing on it fails after 2026-11-06"
  - who: petar-djukic
  - opened: "2026-11-16"
  - closed: "2026-11-16"
  - state: closed
- "Nokia-Bell-Labs/declarative-agents#2928":
  - url: "https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2928"
  - title: "scrum-master site: link stories to the components their work lands in, both ways"
  - who: petar-djukic
  - opened: "2026-11-16"
  - closed: "2026-11-16"
  - state: closed
