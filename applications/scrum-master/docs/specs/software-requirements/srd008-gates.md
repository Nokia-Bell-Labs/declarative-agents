---
id: srd008-gates
title: "Definition of done: evidence in a named environment"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Agents produce changes faster than people read them, and an issue
that closes says only that someone stopped working on it. Some artifacts
pass every unit test and still fail where they run: a schema that compiles
but breaks a consumer, a subsystem that installs cleanly but not as an
upgrade on a running system, an agent profile that is valid but behaves
worse. If done means closed issues, those artifacts reach consumers. If
the checks live in each person's head, they differ by person and by day.

We define done as evidence: a named test passed in a named environment on
a recorded version. Each level of the program has its own done, the
environments form one chain from a developer's machine to a customer's
R&D environment, and promotion along that chain is automation in the test
subsystem. Two gates remain where people judge: a person reviews a
merge, required when the program's policy says so, and a person approves
every handoff to a customer.

<!-- field: goals -->
## Goals

- G1: Nothing merges without a person's review.
- G2: Done at every level is test evidence in a named environment, never a closed issue or a ticked checklist.
- G3: Promotion between environments is automated and declared; handoff to a customer is a human decision.
- G4: The checks for each artifact are declared once, in policy, and are the same for everyone.

<!-- req: R1 -->
## R1 - Merge gate

<!-- req: R1.1 | status: retired | retired_in: 1.2 -->
### R1.1

Retired in 01.2 (design.md D12).

<!-- req: R1.2 | status: retired | retired_in: 1.2 -->
### R1.2

Retired in 01.2 (design.md D12).

<!-- req: R1.3 | status: retired | retired_in: 1.2 -->
### R1.3

Retired in 01.2 (design.md D12).

<!-- req: R1.4 -->
### R1.4

The reviewer looks for what passes the checks and is still wrong: scaling, security, drift from the architecture, a shortcut the acceptance criteria did not forbid. The reviewer reads the branch against the issue's Acceptance Criteria, and approves its tip with scrum-master issue approve. Whether a merge waits for that approval is the program's choice: with the policy's admission.require_approval set, scrum-master issue merge refuses without it; by default it merges without one and records the approval when one was given (srd013 R5.5, R5.6).

<!-- req: R2 -->
## R2 - Release gate

<!-- req: R2.1 -->
### R2.1

A release gate runs the checks the policy declares for the artifact (definition_of_done.<artifact>), or definition_of_done.default when the artifact has no entry. Each check names the environment it runs in.

<!-- req: R2.2 -->
### R2.2

The check kinds are tests (a suite of the artifact or of the test subsystem), contract-tests (the citing repositories' contract tests against the candidate, srd007 R5.4), scorecard (an evaluation of the candidate against the currently released version, with a threshold), peer-read (a person other than the author approves the release issue), and custom (a named command the program supplies).

<!-- req: R2.3 -->
### R2.3

The gate passes only if every declared check passes. Each check's result is recorded as an evidence event naming the requirement, test, environment, version, and result.

<!-- req: R2.4 -->
### R2.4

No tag is created without a passing release gate. A failed gate leaves the release issue open with the failing evidence recorded.

<!-- req: R3 -->
## R3 - Done by level

<!-- req: R3.1 | status: retired | retired_in: 1.2 -->
### R3.1

Retired in 01.2 (design.md D12).

<!-- req: R3.2 | status: retired | retired_in: 1.2 -->
### R3.2

Retired in 01.2 (design.md D12).

<!-- req: R3.3 | status: retired | retired_in: 1.2 -->
### R3.3

Retired in 01.2 (design.md D12).

<!-- req: R3.4 | status: retired | retired_in: 1.2 -->
### R3.4

Retired in 01.2 (design.md D12).

<!-- req: R3.5 | status: retired | retired_in: 1.2 -->
### R3.5

Retired in 01.2 (design.md D12).

<!-- req: R4 -->
## R4 - The environment chain

<!-- req: R4.1 | status: retired | retired_in: 1.2 -->
### R4.1

Retired in 01.2 (design.md D12).

<!-- req: R4.2 | status: retired | retired_in: 1.2 -->
### R4.2

Retired in 01.2 (design.md D12).

<!-- req: R4.3 | status: retired | retired_in: 1.2 -->
### R4.3

Retired in 01.2 (design.md D12).

<!-- req: R4.4 | status: retired | retired_in: 1.2 -->
### R4.4

Retired in 01.2 (design.md D12).

<!-- req: R5 -->
## R5 - Customer handoff

<!-- req: R5.1 | status: retired | retired_in: 1.2 -->
### R5.1

Retired in 01.2 (design.md D12).

<!-- req: R5.2 | status: retired | retired_in: 1.2 -->
### R5.2

Retired in 01.2 (design.md D12).

<!-- req: R6 -->
## R6 - Gates are policy

<!-- req: R6.1 -->
### R6.1

Gate definitions live only in the policy file and in environment SRDs. A script that applies a check not declared there, or skips one that is, is a defect.

<!-- req: R6.2 -->
### R6.2

Changing a gate is a plane::control change (srd001 R2): an issue and a reviewed merge.

<!-- req: R6.3 -->
### R6.3

A scorecard check names its evaluator and threshold in policy; the evaluator is supplied by the program and is outside this corpus.

<!-- req: R7 -->
## R7 - Direction

<!-- req: R7.1 -->
### R7.1

Done is decided by level from evidence events alone: a component, a subsystem Sprint Goal, a system release, and the test subsystem each get a done rule when release 06.0 is planned.

<!-- req: R7.2 -->
### R7.2

A pinned set of releases moves through the environment chain (local, dev, staging, production PoC, customer R&D) by CI in the test subsystem, only on passing evidence, and into a customer environment only with a person's approval.

<!-- req: R7.3 -->
### R7.3

Until then, a merge closes its issue under the repository's own merge settings and, for a change under docs/, the corpus audit (srd036 R3.2); no scrum-master command claims done by level or promotion.

<!-- field: non_goals -->
## Non-goals

- Defining any program's tests, contract tests, evaluators, or environments.
- Automatic merging; a person merges.
- The channel by which evidence returns from customer R&D environments; recorded as an open decision in SPECIFICATIONS.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 | status: retired | retired_in: 1.2 -->
### AC1

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

Releasing an artifact runs exactly the checks its policy entry (or the default) declares, each in its named environment, records each as an evidence event, and creates a tag only if all pass.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)

<!-- ac: AC3 | status: retired | retired_in: 1.2 -->
### AC3

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)
- [R3.5](#r35)

<!-- ac: AC4 | status: retired | retired_in: 1.2 -->
### AC4

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)
- [R4.4](#r44)

<!-- ac: AC5 | status: retired | retired_in: 1.2 -->
### AC5

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)

<!-- ac: AC6 -->
### AC6

Gate definitions exist only in policy and environment SRDs, a gate change goes through a control-plane issue, and a scorecard check names its evaluator and threshold.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)

<!-- ac: AC7 -->
### AC7

Under a policy with require_approval, scrum-master merges no change without another person's approval of the exact commit it merges; under the default policy it merges without one and reports the approver when the tip carries an approval (srd013 R5.5).

<!-- field: traces -->
#### Traces

- [R1.4](#r14)

<!-- ac: AC8 -->
### AC8

Release 06.0's planning writes done by level, promotion, handoff, and the merge approvals from R7; until then the interim rule of R7.3 holds.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)
