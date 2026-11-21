<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Application DSL specification

This module is the normative specification of the language agents in this
repository are written in. Each version of the specification sits in its own
directory. The directory holds the numbered statements in `language.yaml`,
the fixtures that check them, and, for a version that has them, the prose
chapters that cite each statement once.

| Directory | Contents |
|---|---|
| [`1.0/`](1.0/) | The version the engine in this repository implements: chapters `00`–`06`, `language.yaml`, `fixtures/`, `references.yaml`. |
| [`magefiles/`](magefiles/) | The audit and render harness that all versions share. |

## Statement ids

Every version's `language.yaml` is its own namespace for statement ids. An id
such as `R-MACH-001` names a statement within one version. The databases of
two versions never cite each other, and a constitution that cites an id binds
to the version the engine implements.

## Checking and rendering

Run these from this directory:

- `mage audit` validates every version's statement schema, checks that every
  fixture has an owner, runs the acceptance evidence, checks the citations in
  `docs/constitutions/`, and renders the chapters. A citation that does not
  resolve, a statement no chapter cites, or failing evidence fails the gate.
- `mage render` writes the rendered chapters to `1.0/generated-files/`.
- `mage site` builds the static Pages tree under `1.0/generated-files/site/`.
- `mage stats` reports file, line, and statement counts as JSON.
