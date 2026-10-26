---
id: srd040-format-tools
title: "Tooling: the format tools and the format gates"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

The corpus is in the pinned release's Markdown format, and the release's
audit drops a document that strays from it. An author needs the release's
own converter, formatter, and diagnostics at hand, without this
repository's go.mod depending on the release's module. And every file here
is Markdown with tags except the five design.md D2 names, which nothing
holds unless a gate does.

<!-- field: goals -->
## Goals

- G1: An author converts, formats, and checks any corpus document with the pinned release's own library, so mage fmt writes what the audit expects.
- G2: No YAML or JSON file outside design.md D2's list, and no config document that does not read, reaches main.

<!-- req: R1 -->
## R1 - The spec tool

<!-- req: R1.1 -->
### R1.1

The spec tool, in internal/spectool, is built once per pinned release from the release's source in the clone cache, against its Markdown library, and reused until the pin moves. mage convert, fmt, and specCheck run it here, and scrum-master doctor runs it in a program repository at that repository's pin (srd043 R4.4); this repository's go.mod gains no dependency.

<!-- req: R1.2 -->
### R1.2

Each takes a scope over the tracked documents: all, a document type, a directory, or a path.

<!-- req: R1.3 -->
### R1.3

convert writes a YAML corpus document as Markdown in place, removes the YAML, and prints what the conversion reported; fmt rewrites each Markdown document in canonical form; specCheck reports each finding with its fix hint and fails on a finding or on a document fmt would change.

<!-- req: R1.4 -->
### R1.4

fmt and specCheck index the corpus first, so a reference to another document renders and checks as the link the audit's canonical form expects.

<!-- req: R1.5 -->
### R1.5

A hierarchy document, thNNN, inNNN, epNNN, ftNNN, or stNNN-short-name under a themes, initiatives, epics, features, or stories directory, and an objective document, objNNN-short-name under an objectives directory, is read with the release's SRD declaration under the scope of its kind: theme, initiative, epic, feature, story, or objective (design.md schema).

<!-- req: R2 -->
## R2 - Format gates in mage audit

<!-- req: R2.1 -->
### R2.1

A gate test fails when a tracked .yaml, .yml, or .json file is neither one of the files design.md D2 keeps nor a GitLab CI file.

<!-- req: R2.2 -->
### R2.2

A gate test reads every config document this repository holds, the gates file, the rig template, and each config format's example, and fails on one that does not read.

<!-- req: R2.3 -->
### R2.3

A gate test fails when an implemented test suite's go_test names a package or test that does not exist, or when such a suite or an implemented release's proof names a mage target the magefiles do not define.

<!-- field: non_goals -->
## Non-goals

- The corpus audit itself; that is the parent's specification-critic agent (docs/guides/corpus-audit.md).
- Changing the release's declarations; a format this repository needs is fitted to what the release declares.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The spec tool is built once per pinned release and per version of its own source, converts a YAML objective and formats and checks the corpus at that release, and reads hierarchy and objective documents as SRDs.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)
- [R1.5](#r15)

<!-- ac: AC2 -->
### AC2

A planted tracked YAML file fails the gate, every config document this repository holds reads, and a planted suite naming a missing test and a missing mage target fails the suites gate.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
