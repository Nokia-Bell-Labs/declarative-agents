---
id: rel04.1-uc001-orient-and-repair
title: A First-Time Reader Orients in a Program Repository and an Upgraded tpm Brings It Up to Date
status: implemented
priority: P1
test_suite: test-rel04.1-orientation-doctor
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

Someone opens a program repository for the first time. The root README
tells them what the repository is and the order to read it in, each
hierarchy folder's README tells them what its documents are and who owns
them, and AGENTS.md tells an agent which tpm commands and skills operate
there and that every figure is Mermaid inside the Markdown. Later the
technical program manager upgrades tpm and runs tpm doctor in a
repository an older tpm laid down: the files the newer tpm scaffolds
appear, tpm's orientation text is refreshed around what people wrote or
appended to the README and AGENTS.md the repository already had,
the documents are repaired to the pinned format, and the findings it
cannot repair are listed with their fix hints.

<!-- field: actor -->
## Actor

A first-time reader, a person or an agent; then the technical program manager.

<!-- field: trigger -->
## Trigger

A repository opened for the first time; then an upgraded tpm.

<!-- field: preconditions -->
## Preconditions

- tpm is on PATH.
- A system repository and a subsystem repository laid down by tpm init and tpm onboard.
- For the repair, a Go toolchain and the pinned parent release's source in the clone cache.

<!-- field: flow -->
## Flow

- F1: The reader opens the system repository and reads README.md, then docs/specs/themes/README.md, initiatives/README.md, and epics/README.md, and learns each level's meaning, owner, timeframe, and parent before opening a numbered document.
- F2: An agent reads AGENTS.md and learns the tpm command groups and skills at the repository's level, that intent changes through the gate and work enters as an issue, and that every figure is Mermaid in a fenced code block.
- F3: The subsystem repository carries the same root files and a README.md in epics/, features/, and stories/, and none in themes/ or initiatives/; the README.md and AGENTS.md it had before onboard keep their text, with tpm's block appended after it.
- F4: The technical program manager upgrades tpm and runs tpm doctor --dry-run in a repository an older tpm laid down; tpm lists the files it would create, the blocks it would refresh, the documents it would repair, and the installed skills tpm install should update.
- F5: tpm doctor writes the missing files, refreshes the blocks while keeping the text around them, repairs the documents, and lists each finding it cannot repair with its fix hint.
- F6: Running tpm doctor again reports nothing created, appended, refreshed, or repaired.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.6](../software-requirements/srd043-init-onboard.md#r16), [R2.4](../software-requirements/srd043-init-onboard.md#r24) | S1 and S2 trace the orientation files. |
| T2 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.5](../software-requirements/srd043-init-onboard.md#r35), [R4.1](../software-requirements/srd043-init-onboard.md#r41), [R4.2](../software-requirements/srd043-init-onboard.md#r42), [R4.3](../software-requirements/srd043-init-onboard.md#r43), [R4.6](../software-requirements/srd043-init-onboard.md#r46) | S3 traces doctor's files and report. |
| T3 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.4](../software-requirements/srd043-init-onboard.md#r44), [R4.5](../software-requirements/srd043-init-onboard.md#r45) | S4 traces the repair. |
| T4 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC5](../software-requirements/srd043-init-onboard.md#ac5), [AC6](../software-requirements/srd043-init-onboard.md#ac6), [AC7](../software-requirements/srd043-init-onboard.md#ac7) | S1 to S4 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

A first-time reader learns from the root README and the folder READMEs alone what each hierarchy level is, who owns it, how the levels nest, and where to read next.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.6](../software-requirements/srd043-init-onboard.md#r16)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R2.4](../software-requirements/srd043-init-onboard.md#r24)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC5](../software-requirements/srd043-init-onboard.md#ac5)

<!-- sc: S2 -->
### S2

AGENTS.md names every tpm command group and skill at the repository's level, read from the binary, and the Mermaid figure rule.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.6](../software-requirements/srd043-init-onboard.md#r16)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC5](../software-requirements/srd043-init-onboard.md#ac5)

<!-- sc: S3 -->
### S3

tpm doctor adds exactly the missing files, refreshes changed blocks and keeps the text around them, appends the block to an orientation file that has none, reports stale skills without installing them, writes nothing on a dry run, and changes nothing on a second run.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.5](../software-requirements/srd043-init-onboard.md#r35)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.1](../software-requirements/srd043-init-onboard.md#r41)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.2](../software-requirements/srd043-init-onboard.md#r42)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.3](../software-requirements/srd043-init-onboard.md#r43)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.6](../software-requirements/srd043-init-onboard.md#r46)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC6](../software-requirements/srd043-init-onboard.md#ac6)

<!-- sc: S4 -->
### S4

tpm doctor repairs documents to the pinned format so the audit passes, lists what it cannot repair with fix hints, and without Go skips the repair and says why.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.4](../software-requirements/srd043-init-onboard.md#r44)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.5](../software-requirements/srd043-init-onboard.md#r45)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC7](../software-requirements/srd043-init-onboard.md#ac7)

<!-- field: out_of_scope -->
## Out of scope

- Installing or upgrading skills (tpm install).
- Moving a corpus to a newer format release; a person moves the pin and doctor repairs at it.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step; reading is the reader's.
