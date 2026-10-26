---
id: srd033-sprint-retrospective
title: "Skill: sprint-retrospective"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Every other event changes the work. The retrospective is the one where the
team changes how the work is done. In this process, how the work is done
is the control plane: the gates, the scoring, the admission rules, the
definition of done, the label schema. A retrospective that ends in good
intentions changes none of it; the next sprint runs on the same policy
files and the same problems recur.

Traditional retrospectives also start from memory and opinion. The audits
already measured where work waited, how long it took, what was reopened,
and which gates failed. The retrospective starts from those measurements
and ends in policy-change issues, which the inner loop works against
policy.md like any other issue.

<!-- field: goals -->
## Goals

- G1: The retrospective starts from the sprint's audit metrics, not from recollection.
- G2: Every agreed improvement becomes a plane::control issue worked through the inner loop; there is no separate path for rule changes.
- G3: The retrospective judges the process, never people.

<!-- req: R1 -->
## R1 - Trigger

<!-- req: R1.1 | status: retired | retired_in: 1.2 -->
### R1.1

Retired in 01.2 (design.md D12).

<!-- req: R2 -->
## R2 - Reads

<!-- req: R2.1 | status: retired | retired_in: 1.2 -->
### R2.1

Retired in 01.2 (design.md D12).

<!-- req: R2.2 | status: retired | retired_in: 1.2 -->
### R2.2

Retired in 01.2 (design.md D12).

<!-- req: R2.3 | status: retired | retired_in: 1.2 -->
### R2.3

Retired in 01.2 (design.md D12).

<!-- req: R3 -->
## R3 - Asks

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
## R4 - Pushes

<!-- req: R4.1 | status: retired | retired_in: 1.2 -->
### R4.1

Retired in 01.2 (design.md D12).

<!-- req: R4.2 | status: retired | retired_in: 1.2 -->
### R4.2

Retired in 01.2 (design.md D12).

<!-- req: R5 -->
## R5 - Refusals

<!-- req: R5.1 | status: retired | retired_in: 1.2 -->
### R5.1

Retired in 01.2 (design.md D12).

<!-- req: R5.2 | status: retired | retired_in: 1.2 -->
### R5.2

Retired in 01.2 (design.md D12).

<!-- req: R5.3 | status: retired | retired_in: 1.2 -->
### R5.3

Retired in 01.2 (design.md D12).

<!-- req: R6 -->
## R6 - Direction

<!-- req: R6.1 -->
### R6.1

After the review, sprint-retrospective reads the period's audit metrics and pushes at most three plane::control issues that change the policy for the next period.

<!-- req: R6.2 -->
### R6.2

Until it is written, a change to the policy is a plane::control issue filed with specify (srd001 R2).

<!-- field: non_goals -->
## Non-goals

- Changing Sprint Goals; that is plan and tasks.
- Team morale or interpersonal discussion; the skill structures the process facts and leaves the rest of the conversation to the people in it.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 | status: retired | retired_in: 1.2 -->
### AC1

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC2 | status: retired | retired_in: 1.2 -->
### AC2

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R5.3](#r53)

<!-- ac: AC3 | status: retired | retired_in: 1.2 -->
### AC3

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R5.1](#r51)
- [R5.2](#r52)

<!-- ac: AC4 -->
### AC4

Release 06.0's planning writes this skill's requirements from R6; until then the interim rule of R6.2 holds.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
