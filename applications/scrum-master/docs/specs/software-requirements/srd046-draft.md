---
id: srd046-draft
title: "Skill: draft"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

- One skill serves every level. A theme, an initiative, an epic, a feature, and a story share one document shape (srd045 R2.1): context items, problem, goals, requirement groups, non-goals, and acceptance criteria. What differs by level is the parent kind, the location, and the context items the owner must decide, so the skill carries that difference as a table instead of five skills.
- The skill discusses before it writes. A description says what someone wants; it rarely settles the parent, the horizon, the owner, the scope boundary, or how each criterion is tested, and those are the owner's decisions (D5). Asking them first, with a recommended answer for each, costs one exchange and spares a rewrite.
- Facts and checks are scrum-master's (D10). The skill reads the existing hierarchy with scrum-master backlog list and checks what it wrote with scrum-master backlog check (srd045 R8); the model writes only the document's language and iterates on it with the person.
- Children reuse the skill. Drafting an initiative's epics or an epic's stories is the same task one level down, so the skill offers it and runs again per child.

<!-- field: problem -->
## Problem

A planning document starts as a loose description: a paragraph, a page of
notes, or a section of a longer document. Turning it into a theme,
initiative, epic, feature, or story that fits the program means choosing
its parent, avoiding overlap with what exists, filling the fields its
level needs, and settling what only the owner can decide. Done by hand
from the design constitution, the document often reaches the gate with a
missing field, a parent of the wrong kind, or a criterion with no test,
and the decisions it embeds were never put to the owner.

The seeding brief (README) generates a whole program in one pass from a
system description. It does not help with one document at a time, and it
asks the person nothing.

<!-- field: goals -->
## Goals

- G1: Any description becomes one hierarchy document of the chosen level that passes the gate, in the repository and location its level requires.
- G2: Every decision the description leaves open is put to the person, with a recommended answer, before the document is written; the model writes language and nothing else.
- G3: The hierarchy stays connected: each document has the right parent, overlap with an existing document is named, and a document too large or too small for its level is caught before it is written.

<!-- req: R1 -->
## R1 - Trigger

<!-- req: R1.1 -->
### R1.1

Started by the person who owns the level: the technical program manager for a theme, an initiative, or an epic that spans teams, in the system repository; a Product Owner for an epic, a feature, or a story, in the team's repository (srd045 R2.2). An engineering document is started by the owner of the repository it describes: the technical program manager in the system repository, a Product Owner in a team's.

<!-- req: R1.2 -->
### R1.2

Its input is a description (a file or text the person gives) and the level to write.

<!-- req: R2 -->
## R2 - Reads

<!-- req: R2.1 -->
### R2.1

The description, and the level's fields and context items from docs/constitutions/design.md.

<!-- req: R2.2 -->
### R2.2

The repository's hierarchy documents from scrum-master backlog list --json (srd045 R8.1): their ids, kinds, titles, statuses, and parents, to propose a parent and an id and to find overlap.

<!-- req: R3 -->
## R3 - Asks only what is not computed

<!-- req: R3.1 -->
### R3.1

Model step (structure): states in a few lines what it understood the item to be, and proposes its id, title, and parent.

<!-- req: R3.2 -->
### R3.2

Asks the person only what the description and the repository do not settle and the level needs: for a theme, its horizon and the outcomes that show progress; for an initiative, its horizon, one requirement group per milestone with its due date, and the owning teams; for an epic, its owner, a horizon longer than one sprint, its dependencies, and its non-goals; for a feature, its owner, its boundary, and its tests; for a story, its as a, I want, and so that, its points and rank, each criterion's test and environment, and its cites; for an engineering document, the part it describes, its structure, runtime, and deployment, and the decisions it explains, one section each.

<!-- req: R3.3 -->
### R3.3

Asks at most five questions at a time, each with a recommended answer the person can accept as given.

<!-- req: R3.4 -->
### R3.4

Names an existing document the description overlaps, and a scope too large or too small for the level, and proposes a split, a merge, or another level.

<!-- req: R3.5 -->
### R3.5

Writes nothing until the person answers or tells it to use its recommendations, and asks for no other confirmation.

<!-- req: R4 -->
## R4 - Writes

<!-- req: R4.1 -->
### R4.1

Model step (structure): writes the document at its level's location in the parent release's format, with status to_be_agreed; an engineering document, whose format carries no status, is written at docs/engineering/engNN-short-name.md and named in ARCHITECTURE.md's related documents.

<!-- req: R4.2 -->
### R4.2

Runs scrum-master backlog check (srd045 R8.2) and fixes every finding before it shows the document.

<!-- req: R4.3 -->
### R4.3

Model step (iterate): shows the document and revises it with the person until they are satisfied, running scrum-master backlog check after each revision.

<!-- req: R4.4 -->
### R4.4

Offers to draft the next level down, initiatives under a theme, epics under an initiative, features or stories under an epic, and drafts each child with this skill.

<!-- req: R5 -->
## R5 - Refusals

<!-- req: R5.1 -->
### R5.1

Never sets a status other than to_be_agreed; agreeing a document is its owner's merged change (srd045 R2.1).

<!-- req: R5.2 -->
### R5.2

Files, commits, and merges nothing; the document enters main through the repository's own issue, as any change under docs/ does (srd035, srd036).

<!-- req: R5.3 -->
### R5.3

Refuses to estimate, rank, or assign a person; points and rank belong to the work, and the person gives them (D5).

<!-- req: R5.4 -->
### R5.4

As a skill, is at most 50 lines (design.md D10).

<!-- req: R6 -->
## R6 - The workflow as a profile

<!-- req: R6.1 -->
### R6.1

The workflow is a declarative-agents profile, plugin/agents/draft/ (design.md D14): the machine's states carry the skill's steps, the scrum-master commands are model-dispatched tools with typed parameters, and the model acts only in the Converse state (design.md D10). mage audit validates the profile with the pinned runtime's --validate-config, and scrum-master install renders the skill's SKILL.md from the machine (srd037 R1.4), so an agent that does not run the runtime follows the same steps.

<!-- field: non_goals -->
## Non-goals

- Generating a whole program in one pass; that is the README's seeding brief.
- Agreeing, planning, or scheduling the document; agreement is the owner's, and plan reads only agreed documents (srd031).
- Turning a story into issues; that is tasks at the story level (srd045 R4.3).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Started by the level's owner with a description and a level, the skill reads the description, the level's fields, and the repository's hierarchy through scrum-master backlog list.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC2 -->
### AC2

Before writing, the skill states its understanding, proposes an id, title, and parent, asks at most five of the level's open questions at a time with a recommendation each, names any overlap or scope mismatch, and writes nothing until the person answers or accepts the recommendations.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)
- [R3.5](#r35)

<!-- ac: AC3 -->
### AC3

The document is written at its level's location as to_be_agreed (an engineering document at docs/engineering/ with no status, named in ARCHITECTURE's related documents), passes scrum-master backlog check before it is shown, is revised with the person with a check after each revision, and the skill offers to draft its children with the same skill.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)
- [R4.4](#r44)

<!-- ac: AC4 -->
### AC4

The skill sets no status but to_be_agreed, commits nothing, estimates no person, and is at most 50 lines.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)
- [R5.4](#r54)

<!-- ac: AC5 -->
### AC5

The draft profile validates with the pinned runtime, and the installed SKILL.md presents its machine's states as its steps.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
