---
id: rel04.2-uc001-draft-a-planning-document
title: An Owner Drafts a Planning Document from a Description in Conversation
status: implemented
priority: P1
test_suite: test-rel04.2-draft
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The owner of a level has a description of something the program should
plan: a paragraph, notes, or a section of a longer document. They run the
draft skill with the description and the level. The skill lists the
repository's hierarchy with tpm backlog list, says what it understood,
proposes an id, a title, and a parent, and asks the few questions the
description leaves open, each with a recommended answer, naming any
overlap with an existing document. Once the owner answers, it writes the
document as to_be_agreed, checks it with tpm backlog check, revises it with
the owner, and offers to draft the document's children the same way.

<!-- field: actor -->
## Actor

The owner of the level: the technical program manager for a theme, an initiative, or a cross-team epic; a Product Owner for an epic, a feature, or a story.

<!-- field: trigger -->
## Trigger

A description of work the program should plan, and the level to write it at.

<!-- field: preconditions -->
## Preconditions

- tpm is on PATH and the draft skill is installed (tpm install).
- The repository was laid down by tpm init or tpm onboard, so its hierarchy folders and design.md exist.

<!-- field: flow -->
## Flow

- F1: The owner runs the draft skill with the description and the level.
- F2: tpm backlog list --json gives the repository's hierarchy documents; tpm help backlog gives the level's fields.
- F3: Model step: the skill states its understanding, proposes the id, title, and parent, asks at most five open questions with recommendations, and names overlap or a scope mismatch; it writes nothing yet.
- F4: The owner answers, or tells the skill to use its recommendations.
- F5: Model step: the skill writes the document as to_be_agreed, runs tpm backlog check, and fixes every finding.
- F6: Model step: the skill shows the document and revises it with the owner, checking after each revision.
- F7: The skill offers to draft the document's children, and drafts each one the same way.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd045-backlog](../software-requirements/srd045-backlog.md) [R8.1](../software-requirements/srd045-backlog.md#r81), [R8.2](../software-requirements/srd045-backlog.md#r82), [R8.3](../software-requirements/srd045-backlog.md#r83) | S1 traces the list and check commands. |
| T2 | [srd046-draft](../software-requirements/srd046-draft.md) [R2.2](../software-requirements/srd046-draft.md#r22), [R3.2](../software-requirements/srd046-draft.md#r32), [R3.5](../software-requirements/srd046-draft.md#r35) | S2 traces the discussion before writing. |
| T3 | [srd046-draft](../software-requirements/srd046-draft.md) [R4.1](../software-requirements/srd046-draft.md#r41), [R4.2](../software-requirements/srd046-draft.md#r42), [R5.1](../software-requirements/srd046-draft.md#r51), [R5.4](../software-requirements/srd046-draft.md#r54) | S3 traces the written, checked document and the skill's limits. |
| T4 | [srd045-backlog](../software-requirements/srd045-backlog.md) [AC8](../software-requirements/srd045-backlog.md#ac8) | S1 traces this acceptance criterion. |
| T5 | [srd046-draft](../software-requirements/srd046-draft.md) [AC1](../software-requirements/srd046-draft.md#ac1), [AC2](../software-requirements/srd046-draft.md#ac2), [AC3](../software-requirements/srd046-draft.md#ac3), [AC4](../software-requirements/srd046-draft.md#ac4) | S2 and S3 trace these acceptance criteria. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

tpm backlog list names every hierarchy document of the repository with its id, kind, title, status, parent, and path, and tpm backlog check reports exactly the gate's findings, exiting non-zero when there is any, without opening a tracker or writing.

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [R8.1](../software-requirements/srd045-backlog.md#r81)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R8.2](../software-requirements/srd045-backlog.md#r82)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [R8.3](../software-requirements/srd045-backlog.md#r83)
- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC8](../software-requirements/srd045-backlog.md#ac8)

<!-- sc: S2 -->
### S2

Before writing, the skill reads the hierarchy with tpm backlog list, proposes a parent, and asks the level's open questions with recommendations, writing nothing until the owner answers.

<!-- field: traces -->
#### Traces

- [srd046-draft](../software-requirements/srd046-draft.md) [R2.2](../software-requirements/srd046-draft.md#r22)
- [srd046-draft](../software-requirements/srd046-draft.md) [R3.2](../software-requirements/srd046-draft.md#r32)
- [srd046-draft](../software-requirements/srd046-draft.md) [R3.5](../software-requirements/srd046-draft.md#r35)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC1](../software-requirements/srd046-draft.md#ac1)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC2](../software-requirements/srd046-draft.md#ac2)

<!-- sc: S3 -->
### S3

The document the skill writes is to_be_agreed and passes tpm backlog check, and the skill is installed for every agent within its 50-line limit.

<!-- field: traces -->
#### Traces

- [srd046-draft](../software-requirements/srd046-draft.md) [R4.1](../software-requirements/srd046-draft.md#r41)
- [srd046-draft](../software-requirements/srd046-draft.md) [R4.2](../software-requirements/srd046-draft.md#r42)
- [srd046-draft](../software-requirements/srd046-draft.md) [R5.1](../software-requirements/srd046-draft.md#r51)
- [srd046-draft](../software-requirements/srd046-draft.md) [R5.4](../software-requirements/srd046-draft.md#r54)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC3](../software-requirements/srd046-draft.md#ac3)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC4](../software-requirements/srd046-draft.md#ac4)

<!-- field: out_of_scope -->
## Out of scope

- Agreeing the document, and planning it into a release (srd031).
- Generating a whole program from a system description (the README's seeding brief).

<!-- field: assumptions -->
## Assumptions

- A1: Steps not marked as model steps are tpm subcommands and spend no model tokens (design.md D10). F3, F5, and F6 are the model steps: structuring the description into a proposal and a document, and iterating on the document with the owner.
