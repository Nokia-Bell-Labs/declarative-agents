---
id: srd019-track-goals
title: "Script: track-goals (scrum-master goals ...)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

A Sprint Goal is met when the tests its stories' acceptance criteria name
pass in the environments they name, on a recorded version. That evidence is
spread across repositories: the Sprint Goal and its stories sit in a team's
repository, the initiative it serves and the requirements its stories cite
in the system repository at a tagged version, the test results in the test
subsystem's pipelines. Nobody reconciles that by hand every night, so
Sprint Goals are declared met when their issues close, and gaps surface at
the review.

track-goals is the audit that does the reconciling. It follows pinned
references from the system repository down through every subsystem and
component, collects the evidence each listed story's acceptance criteria
name, records it on the Sprint Goal as ledger events anchored to each
criterion, closes the Sprint Goal when the evidence passes, keeps every issue's waiting state true to its links, and
fails when evidence is missing or a reference does not resolve. Daily
scrum, sprint review, and the retrospective read its output. Its dashboard
is the program's site (srd048): the ledgers and findings it records are
read on the site's status pages, which render from them, not from a
dashboard of its own.

<!-- field: goals -->
## Goals

- G1: Every open Sprint Goal's ledger reflects the latest evidence for each criterion of its listed stories after one run.
- G2: A Sprint Goal closes when, and only when, its evidence passes.
- G3: Missing evidence and unresolved references fail the audit instead of passing silently.
- G4: The waiting flag on every open issue matches its links after one run.

<!-- req: R1 -->
## R1 - Triggers and common behavior

<!-- req: R1.1 | status: retired | retired_in: 1.2 -->
### R1.1

Retired in 01.2 (design.md D12).

<!-- req: R1.2 | status: retired | retired_in: 1.2 -->
### R1.2

Retired in 01.2 (design.md D12).

<!-- req: R2 -->
## R2 - Follow references

<!-- req: R2.1 | status: retired | retired_in: 1.2 -->
### R2.1

Retired in 01.2 (design.md D12).

<!-- req: R2.2 | derived_from: [{"repo": "github.com/Nokia-Bell-Labs/declarative-agents", "release": "v0.20261022.0", "srd": "srd014-spec-graph", "item": "R3.16", "reason": "track-goals reports unresolvable references with the parent's resolution rule: local, read-only, at the cited release."}] | status: retired | retired_in: 1.2 -->
### R2.2

Retired in 01.2 (design.md D12).

<!-- req: R2.3 | status: retired | retired_in: 1.2 -->
### R2.3

Retired in 01.2 (design.md D12).

<!-- req: R3 -->
## R3 - Collect evidence

<!-- req: R3.1 | status: retired | retired_in: 1.2 -->
### R3.1

Retired in 01.2 (design.md D12).

<!-- req: R3.2 | status: retired | retired_in: 1.2 -->
### R3.2

Retired in 01.2 (design.md D12).

<!-- req: R3.3 | status: retired | retired_in: 1.2 -->
### R3.3

Retired in 01.2 (design.md D12).

<!-- req: R4 -->
## R4 - Close Sprint Goals and drained tracking issues

<!-- req: R4.1 | status: retired | retired_in: 1.2 -->
### R4.1

Retired in 01.2 (design.md D12).

<!-- req: R4.2 | status: retired | retired_in: 1.2 -->
### R4.2

Retired in 01.2 (design.md D12).

<!-- req: R4.3 | status: retired | retired_in: 1.2 -->
### R4.3

Retired in 01.2 (design.md D12).

<!-- req: R5 -->
## R5 - Waiting states

<!-- req: R5.1 | status: retired | retired_in: 1.2 -->
### R5.1

Retired in 01.2 (design.md D12).

<!-- req: R5.2 | status: retired | retired_in: 1.2 -->
### R5.2

Retired in 01.2 (design.md D12).

<!-- req: R5.3 | status: retired | retired_in: 1.2 -->
### R5.3

Retired in 01.2 (design.md D12).

<!-- req: R5.4 | status: retired | retired_in: 1.2 -->
### R5.4

Retired in 01.2 (design.md D12).

<!-- req: R5.5 | status: retired | retired_in: 1.2 -->
### R5.5

Retired in 01.2 (design.md D12).

<!-- req: R5.6 | status: retired | retired_in: 1.2 -->
### R5.6

Retired in 01.2 (design.md D12).

<!-- req: R6 -->
## R6 - scrum-master goals verify

<!-- req: R6.1 | status: retired | retired_in: 1.2 -->
### R6.1

Retired in 01.2 (design.md D12).

<!-- req: R6.2 | status: retired | retired_in: 1.2 -->
### R6.2

Retired in 01.2 (design.md D12).

<!-- req: R7 -->
## R7 - scrum-master goals status

<!-- req: R7.1 | status: retired | retired_in: 1.2 -->
### R7.1

Retired in 01.2 (design.md D12).

<!-- req: R7.2 | status: retired | retired_in: 1.2 -->
### R7.2

Retired in 01.2 (design.md D12).

<!-- req: R7.3 | status: retired | retired_in: 1.2 -->
### R7.3

Retired in 01.2 (design.md D12).

<!-- req: R7.4 | status: retired | retired_in: 1.2 -->
### R7.4

Retired in 01.2 (design.md D12).

<!-- req: R8 -->
## R8 - Audit result

<!-- req: R8.1 | status: retired | retired_in: 1.2 -->
### R8.1

Retired in 01.2 (design.md D12).

<!-- req: R9 -->
## R9 - Direction

<!-- req: R9.1 -->
### R9.1

track-goals follows each Sprint Goal's pinned references and its listed stories, records test results as evidence events anchored to the story criterion each proves, and closes a Sprint Goal with a met event when every listed story's criteria have passing latest evidence in their environments (srd006 R9, R10).

<!-- req: R9.2 -->
### R9.2

It keeps waiting states true from Upstream, Requires, and Cites dependencies, and reports goals status (met, proving, slipped) for planning.

<!-- req: R9.3 -->
### R9.3

Until it runs, a Sprint Goal closes as reported complete by its repository's sprint-end merge (srd038 R2), its stories are done as that merge reports them (srd038 R2.4), and planning takes slipped Sprint Goals from the aggregate's slipped-commitment signal (srd018 R3.2).

<!-- req: R9.4 -->
### R9.4

track-goals also reports each Sprint Goal's progress, and each epic's and initiative's through Parent links (srd045 R3.3): the story points of done stories over the points of all the stories counted, a story being done when every criterion has passing latest evidence. A Sprint Goal's progress reads 100 percent only when its ledger replays to met.

<!-- field: non_goals -->
## Non-goals

- Running tests; the test subsystem's pipelines run them (srd008 R4).
- Promoting pinned sets between environments; that is CI/CD in the test subsystem.
- Judging whether an Increment is acceptable; that is the Product Owner at sprint-review.
- The channel for evidence from customer R&D environments; recorded as an open decision in SPECIFICATIONS.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 | status: retired | retired_in: 1.2 -->
### AC1

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)

<!-- ac: AC2 | status: retired | retired_in: 1.2 -->
### AC2

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC3 | status: retired | retired_in: 1.2 -->
### AC3

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC4 | status: retired | retired_in: 1.2 -->
### AC4

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)

<!-- ac: AC5 | status: retired | retired_in: 1.2 -->
### AC5

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)
- [R5.4](#r54)
- [R5.5](#r55)
- [R5.6](#r56)

<!-- ac: AC6 | status: retired | retired_in: 1.2 -->
### AC6

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)
- [R7.4](#r74)

<!-- ac: AC7 | status: retired | retired_in: 1.2 -->
### AC7

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R8.1](#r81)

<!-- ac: AC8 -->
### AC8

Release 06.0's planning writes track-goals' requirements from R9; until then the interim rule of R9.3 holds.

<!-- field: traces -->
#### Traces

- [R9.1](#r91)
- [R9.2](#r92)
- [R9.3](#r93)
- [R9.4](#r94)
