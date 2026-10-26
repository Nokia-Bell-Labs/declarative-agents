---
id: rel00.3-uc001-install-skills
title: A Developer Installs the Skills for Their Agent
status: implemented
priority: P1
test_suite: test-rel00.3-install
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A Developer starts in a repository with tpm on PATH. tpm install writes the
skills embedded in the binary, an Agent Plugins 1.0 package, as Agent
Skills directories where their agent reads them:
Cursor, Claude Code, or GitHub Copilot. The files belong to the
Developer's tpm, not to the repository, so git ignores them. After a tpm
upgrade, tpm install --check names the outdated files until the skills
are installed again, so a skill never calls a tpm it was not written for.

<!-- field: actor -->
## Actor

A Developer (a member of the program).

<!-- field: trigger -->
## Trigger

A repository without the skills, or a tpm upgrade.

<!-- field: preconditions -->
## Preconditions

- tpm is on PATH.

<!-- field: flow -->
## Flow

- F1: The Developer runs tpm install --agent <cursor|claude|copilot> in the repository; tpm writes every embedded skill to the agent's command directory with the agent's header and a version and checksum stamp.
- F2: tpm lists the installed files in its block in the clone's .git/info/exclude, so git ignores them and the Developer commits nothing; specify and implement now run from the agent.
- F3: After a tpm upgrade, tpm install --check names each outdated file and exits non-zero.
- F4: tpm install updates the files tpm wrote, keeps and reports any a person edited, and --check passes again.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd037-install](../software-requirements/srd037-install.md) [R1.1](../software-requirements/srd037-install.md#r11), [R1.2](../software-requirements/srd037-install.md#r12), [R1.3](../software-requirements/srd037-install.md#r13), [R2.1](../software-requirements/srd037-install.md#r21), [R2.2](../software-requirements/srd037-install.md#r22), [R3.1](../software-requirements/srd037-install.md#r31), [R3.2](../software-requirements/srd037-install.md#r32), [R3.3](../software-requirements/srd037-install.md#r33), [R4.1](../software-requirements/srd037-install.md#r41), [R5.1](../software-requirements/srd037-install.md#r51), [R5.2](../software-requirements/srd037-install.md#r52), [R6.1](../software-requirements/srd037-install.md#r61), [R6.2](../software-requirements/srd037-install.md#r62) | S1 to S3 and S5 trace install. |
| T2 | [srd035-specify](../software-requirements/srd035-specify.md) [R5.3](../software-requirements/srd035-specify.md#r53) | S4 traces the push command's size. |
| T3 | [srd036-implement](../software-requirements/srd036-implement.md) [R4.2](../software-requirements/srd036-implement.md#r42) | S4 traces the pop command's size. |
| T4 | [srd037-install](../software-requirements/srd037-install.md) [AC1](../software-requirements/srd037-install.md#ac1), [AC2](../software-requirements/srd037-install.md#ac2), [AC3](../software-requirements/srd037-install.md#ac3), [AC4](../software-requirements/srd037-install.md#ac4), [AC5](../software-requirements/srd037-install.md#ac5), [AC6](../software-requirements/srd037-install.md#ac6) | S1 to S3 and S5 trace these acceptance criteria. |
| T5 | [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4) | S4 traces this acceptance criterion. |
| T6 | [srd036-implement](../software-requirements/srd036-implement.md) [AC3](../software-requirements/srd036-implement.md#ac3) | S4 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Every embedded skill is installed for the named agents at their paths and headers, each stamped with the tpm version and checksum.

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [R1.1](../software-requirements/srd037-install.md#r11)
- [srd037-install](../software-requirements/srd037-install.md) [R1.2](../software-requirements/srd037-install.md#r12)
- [srd037-install](../software-requirements/srd037-install.md) [R1.3](../software-requirements/srd037-install.md#r13)
- [srd037-install](../software-requirements/srd037-install.md) [AC1](../software-requirements/srd037-install.md#ac1)
- [srd037-install](../software-requirements/srd037-install.md) [R2.1](../software-requirements/srd037-install.md#r21)
- [srd037-install](../software-requirements/srd037-install.md) [R2.2](../software-requirements/srd037-install.md#r22)
- [srd037-install](../software-requirements/srd037-install.md) [AC2](../software-requirements/srd037-install.md#ac2)
- [srd037-install](../software-requirements/srd037-install.md) [R3.1](../software-requirements/srd037-install.md#r31)
- [srd037-install](../software-requirements/srd037-install.md) [AC3](../software-requirements/srd037-install.md#ac3)

<!-- sc: S2 -->
### S2

A re-install changes nothing, keeps an edited file unless --force, and removes a skill no longer embedded only when unedited.

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [R3.2](../software-requirements/srd037-install.md#r32)
- [srd037-install](../software-requirements/srd037-install.md) [R3.3](../software-requirements/srd037-install.md#r33)
- [srd037-install](../software-requirements/srd037-install.md) [AC3](../software-requirements/srd037-install.md#ac3)

<!-- sc: S3 -->
### S3

--check fails on a missing, outdated, edited, stale, or unignored file and passes after install; nothing is written outside the repository's agent command directories and the block in .git/info/exclude.

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [R4.1](../software-requirements/srd037-install.md#r41)
- [srd037-install](../software-requirements/srd037-install.md) [AC4](../software-requirements/srd037-install.md#ac4)
- [srd037-install](../software-requirements/srd037-install.md) [R5.1](../software-requirements/srd037-install.md#r51)
- [srd037-install](../software-requirements/srd037-install.md) [R5.2](../software-requirements/srd037-install.md#r52)
- [srd037-install](../software-requirements/srd037-install.md) [AC5](../software-requirements/srd037-install.md#ac5)

<!-- sc: S5 -->
### S5

Git ignores every installed file and nothing else; the Developer's own skills in the same directories stay tracked.

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [R6.1](../software-requirements/srd037-install.md#r61)
- [srd037-install](../software-requirements/srd037-install.md) [R6.2](../software-requirements/srd037-install.md#r62)
- [srd037-install](../software-requirements/srd037-install.md) [AC6](../software-requirements/srd037-install.md#ac6)

<!-- sc: S4 -->
### S4

The installed push and pop skills are the 50-line SKILL.md files of the conforming plugin package this release embeds.

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [R5.3](../software-requirements/srd035-specify.md#r53)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4)
- [srd036-implement](../software-requirements/srd036-implement.md) [R4.2](../software-requirements/srd036-implement.md#r42)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC3](../software-requirements/srd036-implement.md#ac3)

<!-- field: out_of_scope -->
## Out of scope

- Distributing the tpm binary itself.
- Agents other than Cursor, Claude Code, and GitHub Copilot.

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step.
