<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Application DSL specification

This module is the normative specification of the language agents in this
repository are written in. Each version of the specification sits in its own
directory. The directory holds the numbered statements in `language.yaml`,
the fixtures that check them, and, for a version that has them, the prose
chapters that cite each statement once.

## Versions

The current version of the language is 2.0. The engine in this repository
implements version 1.0, and is the reference implementation of that version.

| Directory | Contents |
|---|---|
| [`2.0/`](2.0/) | The current version: `language.yaml`, `fixtures/`, and a README. It has no prose chapters. |
| [`1.0/`](1.0/) | The version the engine implements: chapters `00`–`06`, `language.yaml`, `fixtures/`, `references.yaml`, and `COVERAGE.md`, which maps the engine's format references onto the statements. The chapters are the narrative introduction to the language. |
| [`magefiles/`](magefiles/) | The audit and render harness that all versions share. |

Version 1.0 is held to the full audit gate. Each statement has evidence that
runs here: a fixture with a verdict, or a named Go test. Version 2.0 is held
to a reduced gate, covering its schema, fixture coverage and ownership,
fixture shape, and reference resolution, because no engine here could back
its fixture verdicts. A 2.0 statement that no test in this repository
anchors records the gap as a `pending` entry.

Version 1.0 is complete against the engine in this repository at `1.0.0`. It
is frozen except for errata, and new language work belongs to the current
revision.

## Statement ids

Every version's `language.yaml` is its own namespace for statement ids. An id
such as `R-MACH-001` names a statement within one version. The databases of
two versions never cite each other, and a constitution that cites an id binds
to the version the engine implements.

## Checking and rendering

Run these from this directory:

- `mage audit` runs each version's gate. For 1.0, it validates the statement
  schema, checks that every fixture has an owner, runs the acceptance
  evidence, checks the citations in `docs/constitutions/`, and renders the
  chapters. A citation that does not resolve, a statement no chapter cites,
  or failing evidence fails the gate. For 2.0, it runs the reduced gate.
- `mage render` writes the rendered chapters to `1.0/generated-files/`.
- `mage site` builds the static Pages tree under `1.0/generated-files/site/`.
- `mage stats` reports file, line, and statement counts as JSON.
