---
id: srd039-corpus
title: "Shared: reading the corpus and config documents"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

scrum-master reads a program's road map, SRDs, and initiatives, and its own
configuration files, all in the marker form the pinned parent
declarative-agents release declares. This repository consumes that release
only through its audit and its format tools, never as a Go module, so scrum-master
carries its own reader for the form. A reader that drifts from what the
release's converter and formatter write reads a document wrong without
saying so.

<!-- field: goals -->
## Goals

- G1: scrum-master reads every corpus document and config document a program holds in the form the pinned release writes.
- G2: A config document scrum-master writes reads back as the value it was written from.

<!-- req: R1 -->
## R1 - Markdown documents

<!-- req: R1.1 -->
### R1.1

A document opens with YAML frontmatter between two lines of three dashes; a document without it is refused.

<!-- req: R1.2 -->
### R1.2

Each heading tagged by the marker comment above it is a section: a field (field: name) or a record (kind: id), nested by heading level. A marker's further segments are attributes, and a license comment is not a marker.

<!-- req: R1.3 -->
### R1.3

Headings and markers inside a fenced code block are text, not structure.

<!-- req: R1.4 -->
### R1.4

A field's body reads as text, a bullet list, a pipe table whose escaped pipe is a literal pipe, or a tree of key and value items with nested lists, the forms the release prints.

<!-- req: R1.5 -->
### R1.5

A reference link reads as its text, and a field the release prints as None reads as empty.

<!-- req: R1.6 -->
### R1.6

A document names a key when a record id or a field is that key; a pinned reference's anchor resolves that way (srd007 R5.2).

<!-- req: R2 -->
## R2 - Reading from a repository

<!-- req: R2.1 -->
### R2.1

A document is read at its Markdown path and ref through the tracker; a missing document is not found, naming the path.

<!-- req: R2.2 -->
### R2.2

A path that is not Markdown is refused, and a directory listing keeps only its Markdown documents.

<!-- req: R3 -->
## R3 - Config documents

<!-- req: R3.1 -->
### R3.1

A config document (design.md D11) reads as a value: a field of fields is a mapping, a field of records is a list of mappings with each record's id as name, a table is a list of mappings, a bullet list is a list, and other text is one value. Values resolve as integers, booleans, durations, or text, and None as empty.

<!-- req: R3.2 -->
### R3.2

A value writes as a config document: mappings as nested fields, flat lists of records as tables, lists of records that nest as named records, lists as bullets, and empty values as None; reading it back gives the value it was written from.

<!-- field: non_goals -->
## Non-goals

- Checking a document against its declaration; that is the parent's audit and the format tools (srd040).
- Writing corpus documents other than the road map's release record (srd031) and config documents.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A document the release's converter and formatter wrote, with records, fields, lists, tables, trees, links, None, and a fenced block, reads into the sections and values it holds, and a document without frontmatter is refused.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)
- [R1.5](#r15)
- [R1.6](#r16)

<!-- ac: AC2 -->
### AC2

A Markdown document is read from the tracker by its path, a missing one is not found, and a YAML path is refused.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC3 -->
### AC3

Every config format's example, this repository's gates file, and the rig template read as their values, and a value written as a config document reads back unchanged.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
