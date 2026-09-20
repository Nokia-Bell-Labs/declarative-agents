---
title: Application DSL Specification
subtitle: A Declarative Language for Applications, Agents, and Machines
author: Nokia Bell Labs
date: September 2026
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Application DSL Specification

This specification defines the declarative language in which applications,
their agents, and their machines are written: `application.yaml`,
`profile.yaml`, `machine.yaml`, tool declarations, and REST definitions. It is
structured so that a third party could build a conforming implementation and
so that audit machinery can check conformance mechanically.

Normative statements live in `language.yaml`, each with a stable identifier,
an RFC 2119 requirement level, a conformance target, and acceptance evidence.
The prose chapters cite statements by identifier; no normative content exists
only in prose.

The working title above is descriptive. Chapter 02 settles a coined name for
the language, which becomes the specification title and search token while
this directory keeps its descriptive name.
