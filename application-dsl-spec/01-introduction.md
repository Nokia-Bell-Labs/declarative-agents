<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 1. Introduction

## 1.1 Scope

We specify the declarative language that describes an application, the agents
it is composed of, and the machines those agents execute. The language spans
three document families: the application document (`application.yaml`), the
agent profile and the documents it references (`profile.yaml`, tool
declarations, tool selections, REST definitions), and the machine document
(`machine.yaml`, including request machines and machine fragments). The
specification of the language is therefore a specification of the application
DSL (domain-specific language), with agents and machines as its component
grammars.

This specification is implementation-neutral. It binds documents, the
processors that load them, the runtimes that execute them, and the populations
of running instances they produce — not any particular codebase. Component
constitutions under `docs/constitutions/` bind implementations to this
specification by citing statement identifiers; the dependency is one-way, and
nothing in this directory cites an implementation.

## 1.2 Normative statements

Every requirement in this specification is a numbered statement in
`language.yaml` with a stable identifier, a conformance target, and a
requirement level drawn from RFC 2119 (MUST, MUST NOT, SHOULD, SHOULD NOT,
MAY). Each statement carries at least one acceptance entry — a document
fixture with an expected verdict, or a named Go test — and the audit gate
fails when a statement lacks evidence or its evidence fails. Chapters cite
statements where the surrounding prose motivates them; the citation renders
the statement in place with a stable anchor.

The two statements below seed the pipeline: they are the well-formedness floor
that every later grammar chapter builds on.

{{statement R-INTRO-001}}

{{statement R-INTRO-002}}

## 1.3 Conformance targets

A conformance claim names one of four targets. The targets partition who or
what the claim binds, and chapter 09 gives each its conformance clauses and
fixture suites.

| Target | Binds | Example obligation |
|---|---|---|
| document | A written profile, application, or machine document | The document is well-formed and carries its required keynames |
| processor | Software that validates and loads documents | Invalid documents are rejected with the specified verdict |
| runtime | Software that executes a loaded profile | Activation and workspace binding follow the lifecycle chapter |
| population | The set of running instances in a mesh | Closure, authority, and workspace-exclusivity invariants hold |

Document and processor targets are checkable from fixtures: a document either
matches the grammar or it does not, and a processor either rejects an invalid
fixture or it does not. Runtime and population targets are checkable from
running systems, and their acceptance entries map to integration rigs rather
than document fixtures.

## 1.4 Organization

Chapter 02 settles terminology: the class/instance vocabulary, the ban on bare
nouns in normative text, and the distinction between properties and
attributes. Chapter 03 gives the model — three levels and the relationships
between declared classes and running instances. Chapter 04 specifies the
lifecycle primitives. Chapters 05 through 07 give the grammar of each document
family. Chapter 08 states the invariants that hold over a population of
instances, and chapter 09 defines conformance for each target together with
the fixture suite layout.
