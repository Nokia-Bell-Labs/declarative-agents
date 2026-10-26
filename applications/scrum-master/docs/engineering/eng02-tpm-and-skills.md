---
id: eng02-tpm-and-skills
title: "scrum-master and Skills: Code Does the Workflow, the Model Does Language"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: introduction -->
## Introduction

Every token the model spends on a step a program could do is cost with no
benefit, and a step the model does is a step that can come out
differently each time. The words exist to take those steps away from
the model. This guideline states where the boundary sits (design.md
D10), what a skill file may contain, and how mage audit holds the line.

<!-- section: the-boundary -->
## The boundary

scrum-master does everything a program can: fetching facts from the tracker and
the repository, deriving state, recommending a choice, filing and
linking issues, validating files, computing aggregates, tagging, and
posting events. Every subcommand takes --json and --dry-run and never
prompts.

The model does three kinds of work, and only these.

| Model task | Example |
|---|---|
| Turn loose conversation into a structured file | The TPM's Sprint Goals discussion into an initiative; a Product Owner's answers into the weekly report |
| Combine several files into one summary | A period's reports into the sprint-end or stakeholder summary |
| Iterate on that content with the person | Revising a summary or an initiative until the person accepts it |

Table: Model tasks.

A choice among options scrum-master computed is not generation. The skill shows
scrum-master's recommendation and relays the person's answer back to scrum-master.

<!-- section: skill-file-shape -->
## Skill file shape

A skill is plugin/skills/<name>/SKILL.md, one skill of the Agent
Plugins 1.0 package plugin/ (srd037). Its front matter follows the
Agent Skills specification: name equal to the directory name and
description saying what the skill does and when to use it, and
nothing agent-specific; every agent reads the same SKILL.md. A skill
file is at most 50 lines, stricter than the 500 lines Agent Skills
recommends. Its
steps are either a scrum-master command,
a question relayed to the person with scrum-master's recommendation, or one of
the three model tasks named as such. Facts come only from scrum-master --json
output; a skill never reads the tracker or the repository on its own.
Anything longer than 50 lines, or any step that computes, filters,
formats, or decides by rule, belongs in scrum-master.

<!-- section: the-audit-gate -->
## The audit gate

mage audit fails when a SKILL.md exceeds 50 lines, when a skill
breaks the Agent Skills front matter rules, or when plugin.json breaks
the Agent Plugins 1.0 manifest rules. The checks are repository gate
tests in magefiles/, run with the rest
of the gate tests before the specification-critic corpus audit. Each
use case marks which of its flow steps are model steps, so a review
sees the model's share of a release at a glance.

<!-- section: moving-a-step-into-tpm -->
## Moving a step into scrum-master

When a skill or a use case flow gives the model a step outside the
three tasks, we file an issue that names the step, specify the scrum-master
subcommand that does it in the owning SRD, and shrink the skill to a
call of that subcommand. The SRD change comes first (D1).

The owning SRD is the one the work belongs to, which is not always a
script SRD. A package can be backed by a skill SRD when the skill is
the only caller of its subcommands: internal/goals implements srd022
tasks, internal/plan srd031 plan, and internal/sprintend
srd038 sprint-end. Shared code is backed by the SRD that states its
contract, such as internal/corpus by srd039.

<!-- field: references -->
## References

- docs/constitutions/design.md D5 and D10
- docs/ARCHITECTURE.md design decision 13 and technology_choices skills
- docs/specs/use-cases/ (flows mark model steps)
