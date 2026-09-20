<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 9. Conformance

## 9.1 Conformance clauses

A conformance claim names a target (section 1.3) and holds when every
statement for that target holds.

Document conformance: a document conforms when it passes every check declared
by the document-target statements applicable to its family — an
application-profile against the chapter 05 statements, an agent-profile
against chapter 06, a machine-profile against chapters 03 and 07, and every
family against the well-formedness floor of chapter 01.

Processor conformance: a processor conforms when it accepts every conforming
document and rejects every non-conforming one. The fixture suites make this
mechanical: each fixture carries an expected verdict, and a processor's
verdicts must match them exactly.

{{statement R-CONF-001}}

Runtime conformance: a runtime conforms when the runtime-target statements
hold over the instances it executes; the rig references on those statements
name the integration evidence.

Population conformance: a population conforms when the chapter 08 invariants
hold over its whole instance set. Population conformance is a property of an
operated system, not of any single component, and a claim names the mesh it
was checked against.

## 9.2 Fixture suite layout

Fixture suites live under `fixtures/document/`, one directory per suite, each
suite owned by exactly one document-target statement. A fixture is a complete
YAML document; its expected verdict — `valid` or `invalid` — is declared by
the statement's acceptance entry in `language.yaml`, next to the check the
verdict is measured against, so the suite and the statement cannot drift
apart.

The language gate enforces the layout: every document-target statement
carries at least one valid and one invalid fixture, every fixture file under
`fixtures/` is referenced by exactly one statement, and a fixture whose
verdict mismatches its check fails `mage audit`.

## 9.3 Claiming conformance

A claim cites the statement identifiers it was checked against, at the
versioned specification it used. Component constitutions under
`docs/constitutions/` are standing claims of this form: each binds one
component to the statements it must uphold, and cites them by identifier.
