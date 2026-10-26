---
id: rel01.4-uc002-agent-learns-the-cli
title: An Agent Learns the Process from tpm and Works an Issue
status: implemented
priority: P1
test_suite: test-rel01.4-agent-legible-cli
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

An agent with tpm on PATH and nothing else learns the process from the
binary. tpm help gives it the planes, the steps of the release cycle, and
the command that serves each; tpm help issue and tpm help Sprint Goal say
what the issue lifecycle and a Sprint Goal are. It then files and pops an
issue with tpm issue, the group named for what every subcommand acts on.
A skill installed from an earlier tpm that still calls tpm program fails
with a message naming tpm issue and tpm install, and tpm issue recur
opens the next instance of a recurring issue, finding the instances an
earlier tpm opened.

<!-- field: actor -->
## Actor

An agent working for a Developer.

<!-- field: trigger -->
## Trigger

An agent meets tpm for the first time, or after an upgrade to release 01.4.

<!-- field: preconditions -->
## Preconditions

- tpm of release 01.4 is on PATH.
- A program whose tracker holds an admitted issue and a recurring issue an earlier tpm opened.

<!-- field: flow -->
## Flow

- F1: The agent runs tpm help and reads the process page: planes, the ten release-cycle steps with the group or skill serving each, and the pointer to docs/VISION.md.
- F2: The agent runs tpm help issue and tpm help Sprint Goal, and tpm help --list for the other pages.
- F3: The agent runs tpm issue pop on the admitted issue and works it in the worktree tpm returns.
- F4: A skill still installed from an earlier tpm runs tpm program pop; tpm exits non-zero naming tpm issue and tpm install, and tpm install updates the skill.
- F5: The recurring issue's run ends; tpm issue recur closes it, logs the run, and opens the next instance once.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd044-help](../software-requirements/srd044-help.md) [R1.1](../software-requirements/srd044-help.md#r11), [R1.2](../software-requirements/srd044-help.md#r12), [R1.3](../software-requirements/srd044-help.md#r13), [R1.4](../software-requirements/srd044-help.md#r14), [R2.1](../software-requirements/srd044-help.md#r21), [R2.2](../software-requirements/srd044-help.md#r22), [R3.1](../software-requirements/srd044-help.md#r31), [R3.2](../software-requirements/srd044-help.md#r32) | S1 and S2 trace help. |
| T2 | [srd013-program](../software-requirements/srd013-program.md) [R2.1](../software-requirements/srd013-program.md#r21), [R2.2](../software-requirements/srd013-program.md#r22) | S3 traces the group's name and the refusal of the old one. |
| T3 | [srd013-program](../software-requirements/srd013-program.md) [R8.1](../software-requirements/srd013-program.md#r81), [R8.2](../software-requirements/srd013-program.md#r82) | S4 traces recur in the issue group. |
| T4 | [srd044-help](../software-requirements/srd044-help.md) [AC1](../software-requirements/srd044-help.md#ac1), [AC2](../software-requirements/srd044-help.md#ac2), [AC3](../software-requirements/srd044-help.md#ac3) | S1 and S2 trace these acceptance criteria. |
| T5 | [srd013-program](../software-requirements/srd013-program.md) [AC2](../software-requirements/srd013-program.md#ac2), [AC9](../software-requirements/srd013-program.md#ac9) | S3 and S4 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

tpm help alone gives the process in one page, and every group and the listed concepts have a page of at most 60 lines, also as JSON.

<!-- field: traces -->
#### Traces

- [srd044-help](../software-requirements/srd044-help.md) [R1.1](../software-requirements/srd044-help.md#r11)
- [srd044-help](../software-requirements/srd044-help.md) [R1.2](../software-requirements/srd044-help.md#r12)
- [srd044-help](../software-requirements/srd044-help.md) [R1.3](../software-requirements/srd044-help.md#r13)
- [srd044-help](../software-requirements/srd044-help.md) [R1.4](../software-requirements/srd044-help.md#r14)
- [srd044-help](../software-requirements/srd044-help.md) [R2.1](../software-requirements/srd044-help.md#r21)
- [srd044-help](../software-requirements/srd044-help.md) [R2.2](../software-requirements/srd044-help.md#r22)
- [srd044-help](../software-requirements/srd044-help.md) [AC1](../software-requirements/srd044-help.md#ac1)
- [srd044-help](../software-requirements/srd044-help.md) [AC2](../software-requirements/srd044-help.md#ac2)

<!-- sc: S2 -->
### S2

The audit fails when a help page names a missing command or SRD, or a group has no page, so the help never drifts from the binary or the corpus.

<!-- field: traces -->
#### Traces

- [srd044-help](../software-requirements/srd044-help.md) [R3.1](../software-requirements/srd044-help.md#r31)
- [srd044-help](../software-requirements/srd044-help.md) [R3.2](../software-requirements/srd044-help.md#r32)
- [srd044-help](../software-requirements/srd044-help.md) [AC3](../software-requirements/srd044-help.md#ac3)

<!-- sc: S3 -->
### S3

Every issue subcommand runs under tpm issue, and tpm program runs nothing and names its fix.

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [R2.1](../software-requirements/srd013-program.md#r21)
- [srd013-program](../software-requirements/srd013-program.md) [R2.2](../software-requirements/srd013-program.md#r22)
- [srd013-program](../software-requirements/srd013-program.md) [AC2](../software-requirements/srd013-program.md#ac2)

<!-- sc: S4 -->
### S4

tpm issue recur opens the next instance of a recurring issue exactly once, including one whose earlier instance an earlier tpm opened.

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [R8.1](../software-requirements/srd013-program.md#r81)
- [srd013-program](../software-requirements/srd013-program.md) [R8.2](../software-requirements/srd013-program.md#r82)
- [srd013-program](../software-requirements/srd013-program.md) [AC9](../software-requirements/srd013-program.md#ac9)

<!-- field: out_of_scope -->
## Out of scope

- The rest of the subcommand groups' rationalization, which stays with 06.0 (SPECIFICATIONS GAP6).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). Reading the help pages is the agent's, and costs only the pages' tokens.
