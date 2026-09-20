---
title: Libretto
subtitle: A Declarative Language for Applications, Agents, and Machines
author: Nokia Bell Labs
date: September 2026
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Libretto: A Declarative Language for Applications, Agents, and Machines

Libretto is the declarative language in which applications, their agents, and
their machines are written: `application.yaml`, `profile.yaml`,
`machine.yaml`, tool declarations, and REST definitions. This specification
is structured so that a third party could build a conforming implementation
and so that audit machinery can check conformance mechanically. A libretto is
a declared text that a performance realizes; here the documents are the
declared text and the running instances are the performances (chapter 02
records the naming decision).

Normative statements live in `language.yaml`, each with a stable identifier,
an RFC 2119 requirement level, a conformance target, and acceptance evidence.
The prose chapters cite statements by identifier; no normative content exists
only in prose. The directory keeps its descriptive name,
`application-dsl-spec/`.
