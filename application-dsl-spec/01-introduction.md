<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 1. Introduction

## 1.1 Scope

We specify a declarative language describing an application, its agents, and
their execution machines. The language comprises three document families: the
application document (`application.yaml`), the agent profile and its
referenced documents (`profile.yaml`, tool declarations, tool selections, REST
definitions), and the machine document (`machine.yaml`, including request
machines and machine fragments). Thus, the language specification defines the
application DSL, with agents and machines as its component grammars.

This specification is implementation-neutral: it binds documents, their
processors, runtimes, and the populations of running instances they produce
— not any particular codebase. Component
constitutions under `docs/constitutions/` bind implementations to this
specification via statement identifiers; this dependency is one-way, with no
implementation citations in this directory.

## 1.2 Normative statements

Each requirement in `language.yaml` is a numbered statement with a stable
identifier, conformance target, and RFC 2119 requirement level (MUST, MUST
NOT, SHOULD, SHOULD NOT, MAY). Every statement includes at least one
acceptance entry—a document fixture with an expected verdict or a named Go
test—and the audit gate fails if evidence is missing or fails. Chapters
reference statements, rendering them in place via stable anchors.

The two statements below seed the pipeline, forming the well-formedness floor
that every subsequent grammar chapter builds upon.

{{statement R-INTRO-001}}

{{statement R-INTRO-002}}

## 1.3 Conformance targets

A conformance claim specifies one of four targets, each tied to a distinct
party. Chapter 09 gives the conformance clauses and fixture suites for each
target.

| Target | Binds | Example obligation |
|---|---|---|
| document | A written profile, application, or machine document | The document is well-formed and carries its required keynames |
| processor | Software that validates and loads documents | Invalid documents are rejected with the specified verdict |
| runtime | Software that executes a loaded profile | Activation and workspace binding follow the lifecycle chapter |
| population | The set of running instances in a mesh | Closure, authority, and workspace-exclusivity invariants hold |

Document and processor targets are verified from fixtures: a document
matches the grammar or not, and a processor rejects an invalid fixture or not.
Runtime and population targets are checked in running systems, with acceptance
entries mapped to integration rigs rather than document fixtures.

## 1.4 Organization

Chapter 02 settles the terminology: the class/instance vocabulary, the ban on
bare nouns in normative text, and the properties/attributes distinction.
Chapter 03 introduces the model, detailing three levels and the relationships
between declared classes and running instances. Chapter 04 covers the
lifecycle primitives, and chapters 05 through 07 the grammar of each document
family. Chapter 08 addresses the invariants that hold across instance
populations, and chapter 09 the conformance clauses for each target together
with the fixture suite layout.
