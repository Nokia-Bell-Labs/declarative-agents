---
id: rel01.4-uc001-bootstrap-a-program
title: The Technical Program Manager Starts a Program and a Repository Joins It
status: implemented
priority: P1
test_suite: test-rel01.4-agent-legible-cli
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The technical program manager starts a program from an empty git
repository. tpm init writes the system repository's corpus skeleton,
control files, and skills, and makes the tracker repository's first
commit, and the parent's corpus audit passes on the result before anyone
has written a line. A Product Owner then runs tpm onboard in an existing
repository: it receives the same corpus skeleton, and the system
repository receives one filed issue asking its owner to list the
repository in a subsystem SRD. Running either command again changes
nothing.

<!-- field: actor -->
## Actor

The technical program manager, then a Product Owner.

<!-- field: trigger -->
## Trigger

A new program, then a repository joining it.

<!-- field: preconditions -->
## Preconditions

- tpm is on PATH.
- An empty git repository for the system repository, an empty tracker repository, and a checkout of the joining repository.

<!-- field: flow -->
## Flow

- F1: The technical program manager runs tpm init --tracker <tracker> --agent claude in the empty repository; tpm writes VISION, ARCHITECTURE, SPECIFICATIONS, a road-map with one release, the design constitution, the charter, the pins, a system SRD, control/labels.md, control/policy.md, and the skills, and reports each file created.
- F2: tpm makes the tracker repository's first commit and creates the program's labels there.
- F3: The parent's corpus audit and the Sprint Goal gate pass on the new repository.
- F4: A Product Owner runs tpm onboard --system <system> --subsystem <srd> in the joining repository's checkout; tpm writes the skeleton without control files and files one issue in the system repository asking for the components-table row.
- F5: Running init and onboard again reports every file kept and files nothing.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.1](../software-requirements/srd043-init-onboard.md#r11), [R1.2](../software-requirements/srd043-init-onboard.md#r12), [R1.3](../software-requirements/srd043-init-onboard.md#r13), [R1.4](../software-requirements/srd043-init-onboard.md#r14), [R1.5](../software-requirements/srd043-init-onboard.md#r15) | S1 and S2 trace init. |
| T2 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R2.1](../software-requirements/srd043-init-onboard.md#r21), [R2.2](../software-requirements/srd043-init-onboard.md#r22), [R2.3](../software-requirements/srd043-init-onboard.md#r23) | S3 traces onboard. |
| T3 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.1](../software-requirements/srd043-init-onboard.md#r31), [R3.2](../software-requirements/srd043-init-onboard.md#r32), [R3.3](../software-requirements/srd043-init-onboard.md#r33), [R3.4](../software-requirements/srd043-init-onboard.md#r34) | S4 traces the behavior both share. |
| T4 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC1](../software-requirements/srd043-init-onboard.md#ac1), [AC2](../software-requirements/srd043-init-onboard.md#ac2), [AC3](../software-requirements/srd043-init-onboard.md#ac3), [AC4](../software-requirements/srd043-init-onboard.md#ac4) | S1 to S4 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

An empty repository becomes a system repository whose corpus, control files, and skills are all present, and the parent's corpus audit and the Sprint Goal gate pass on it as written.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.1](../software-requirements/srd043-init-onboard.md#r11)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.2](../software-requirements/srd043-init-onboard.md#r12)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.3](../software-requirements/srd043-init-onboard.md#r13)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.4](../software-requirements/srd043-init-onboard.md#r14)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC1](../software-requirements/srd043-init-onboard.md#ac1)

<!-- sc: S2 -->
### S2

The tracker repository gets its first commit and the program's labels once, and a tracker repository that has commits is left as it is.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R1.5](../software-requirements/srd043-init-onboard.md#r15)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC2](../software-requirements/srd043-init-onboard.md#ac2)

<!-- sc: S3 -->
### S3

A joining repository gets the corpus skeleton, and the system repository gets exactly one registration issue and no commit.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R2.1](../software-requirements/srd043-init-onboard.md#r21)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R2.2](../software-requirements/srd043-init-onboard.md#r22)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R2.3](../software-requirements/srd043-init-onboard.md#r23)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC3](../software-requirements/srd043-init-onboard.md#ac3)

<!-- sc: S4 -->
### S4

A second run of either command changes nothing, an edited file survives without --force, and no template names a person, organization, product, or project.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.1](../software-requirements/srd043-init-onboard.md#r31)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.2](../software-requirements/srd043-init-onboard.md#r32)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.3](../software-requirements/srd043-init-onboard.md#r33)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R3.4](../software-requirements/srd043-init-onboard.md#r34)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC4](../software-requirements/srd043-init-onboard.md#ac4)

<!-- field: out_of_scope -->
## Out of scope

- Creating the repositories on a hosting service.
- The reports repository (tpm reports init).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands, CI, or the inner loop, and spend no model tokens (design.md D10). This use case has no model step.
