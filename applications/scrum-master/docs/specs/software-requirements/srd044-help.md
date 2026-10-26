---
id: srd044-help
title: "Script: help"
status: retired
retired_in: "5.0"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

- --help and help are two layers that must not disagree. --help stays terse: usage and flags, shaped for a program. help is the prose a reader learns the process from. A gate holds each group's help page to the group's subcommands in the dispatch table.
- The pages are written against the command names of release 01.4, which renames the program group to issue (srd013 R2.2) in the same release, so the prose is written once.
- Pages are sized for an agent's context: one page answers one question, and the top page is the whole process in one read.

<!-- field: problem -->
## Problem

scrum-master's help is a usage listing: group names and flags. An agent or a person
meeting the binary learns nothing from it about the process it implements:
what a Sprint Goal is, how an issue moves from filed to merged, which
command serves which step of the release cycle, or where evidence comes
from. The skills carry some of this, and the corpus all of it, but neither
travels with the binary into a repository that has not yet installed
either. An agent that can run only scrum-master should be able to learn the process
from scrum-master.

<!-- field: goals -->
## Goals

- G1: scrum-master help alone gives a reader the process in one read: the planes, the steps of the release cycle, and the command that serves each step.
- G2: Every group and every load-bearing concept has a page that says what it is for and which commands and SRDs touch it.
- G3: A page that names a command or an SRD that does not exist fails the audit, so the help cannot drift from the binary or the corpus.

<!-- req: R1 -->
## R1 - Pages

<!-- req: R1.1 -->
### R1.1

scrum-master help with no argument prints the process page: the three planes, the ten steps of the release cycle as docs/VISION.md draws them, and for each step the scrum-master group or skill that serves it, where a repository's corpus and control files live, and a pointer to docs/VISION.md.

<!-- req: R1.2 -->
### R1.2

scrum-master help <group> exists for every scrum-master group. It prints what the group is for in the process and who runs it, then each subcommand with one sentence and the SRD that specifies it; flags stay with --help.

<!-- req: R1.3 -->
### R1.3

scrum-master help <concept> exists for at least Sprint Goal, admission, approval, milestone, tracker, evidence, release, and recurring issue. Each prints a paragraph, the commands that touch the concept, and the SRD that specifies it.

<!-- req: R1.4 -->
### R1.4

scrum-master help --list names every page with its one-line summary, and an unknown page name exits non-zero listing the pages.

<!-- req: R2 -->
## R2 - Output

<!-- req: R2.1 -->
### R2.1

Pages are plain text, each at most 60 lines. With --json each page is an object holding its name, summary, text, commands, and SRDs.

<!-- req: R2.2 -->
### R2.2

The pages are embedded in the binary; help reads no file and no tracker, writes nothing, and never prompts.

<!-- req: R3 -->
## R3 - Drift

<!-- req: R3.1 -->
### R3.1

A gate test in mage audit fails when a page names a scrum-master command the dispatch table does not hold, or an SRD the corpus does not hold, or when a group has no page.

<!-- req: R3.2 -->
### R3.2

The subcommands scrum-master help <group> lists are exactly the group's subcommands in the dispatch table.

<!-- field: non_goals -->
## Non-goals

- Replacing --help; usage and flags stay there.
- Documenting the skills; each skill's SKILL.md is its own documentation.
- Rendering the corpus at run time; pages are written prose, held to the corpus by the gate.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

scrum-master help prints the planes, the ten release-cycle steps with the group or skill serving each, the corpus and control locations, and the pointer to VISION; every group and the listed concepts have pages; --list names them all, and an unknown name exits non-zero with the list.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

Every page is plain text of at most 60 lines, --json gives each page's name, summary, text, commands, and SRDs, and help reads no file or tracker, writes nothing, and never prompts.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC3 -->
### AC3

The gate fails on a page naming a missing command or SRD and on a group without a page, and each group page lists exactly the group's subcommands.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
