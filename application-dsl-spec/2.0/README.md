<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Application DSL specification, version 2.0

This directory holds version 2.0, the current revision of the language.
`language.yaml` contains every normative statement, and `fixtures/` contains
the example specifications that check them. Version 2.0 has no prose
chapters. For a narrative introduction to the language, read the 1.0
chapters in [`../1.0/`](../1.0/). The statements in this directory are the
specification.

The engine in this repository implements version 1.0, not 2.0. See
[Pending evidence](#pending-evidence) for what that means for the evidence
here.

## How to read a statement

Each entry in `language.yaml` has these fields:

- `id` is a stable identifier of the form `R-<FAMILY>-<NNN>`. Identifiers are
  never renumbered or reused. They are scoped to this version's database, so
  `R-MACH-001` here is not the 1.0 statement with the same id.
- `target` is the conformance target the statement binds.
- `level` is the RFC 2119 requirement level: `MUST`, `MUST NOT`, `SHOULD`,
  `SHOULD NOT`, or `MAY`.
- `statement` is the normative text.
- `acceptance` lists the evidence that the statement is checked.

There are four conformance targets:

| Target | Binds |
|---|---|
| `specification` | The YAML an author writes. Version 1.0 called this target `document`. |
| `processor` | Whatever loads and checks specifications. |
| `runtime` | The engine while an agent runs. |
| `population` | The set of running instances as a whole. |

## Evidence

An acceptance entry is one of four assertions:

- `fixture` names a file under `fixtures/`, the check to apply to it, and the
  verdict the check must produce (`valid` or `invalid`). Every `specification`
  statement in force has at least one valid and one invalid fixture, unless a
  `pending` entry says why it does not. Every fixture file belongs to exactly
  one statement.
- `rig` names a Go test in this repository, with the path relative to the
  repository root.
- `go_test` names a Go test the same way.
- `pending` records evidence the statement lacks. Its `note` names what is
  missing. A pending entry is a known gap, not a check.

### Pending evidence

A conforming 2.0 processor would run each fixture verdict through its own
loaders. The engine here implements 1.0, so no 2.0 verdict runs in this
repository. A `rig` entry appears only where a test here already pins the
behaviour the statement describes. Every other statement that needs a test
carries a `pending` entry instead, so the database never claims a check the
repository cannot back.

The audit in [`../magefiles/`](../magefiles/) holds this directory to a reduced
gate, which checks the following:

- the statement schema, including retirement and pending records;
- fixture coverage and ownership;
- that every fixture file parses as YAML;
- that every `fixture`, `rig`, and `go_test` reference resolves to a file or
  declared test.

The audit does not run the fixture checks. It reports how many pending gaps
the database records, so the distance between the language and this engine
stays measurable.

## Retirement

When the language stops making a statement, the statement keeps its id and
text. It gains `status: retired` and a `retired:` note that names the
statements replacing it, if any, and its acceptance stays as a record of
what was checked. Its fixtures move to `fixtures/retired/`, which the audit
resolves but does not judge. No statement in force draws on that directory.

## Fixture layout

| Directory | Holds |
|---|---|
| `fixtures/specification/` | Fixtures owned by statements in force, one subdirectory per rule. |
| `fixtures/retired/` | Fixtures owned by retired statements. |
| `fixtures/units/` | Templates and fragments that fixtures include. Units are scaffolding: no statement owns them and no check judges them. |
