<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 9. Conformance

## 9.1 Conformance clauses

A conformance claim names a target (section 1.3) and holds if all its
statements are true.

A document conforms if it passes all checks declared by applicable
document-target statements for its family. Application-profiles are measured
against chapter 05, agent-profiles against chapter 06, and machine-profiles
against chapters 03 and 07; all families also pass chapter 01's
well-formedness floor.

A processor conforms if it accepts all conforming documents and rejects all
non-conforming ones. The fixture suites automate this. Each fixture includes
an expected verdict, and a processor's verdicts must match exactly.

{{statement R-CONF-001}}

A runtime conforms when its runtime-target statements hold over the instances
it executes. The rig references in those statements name the integration
evidence.

A population conforms when chapter 08 invariants hold across its entire
instance set. Population conformance is a system-wide property rather than of
individual components, and a claim names the mesh it was checked against.

## 9.2 Fixture suite layout

Fixture suites reside in the `fixtures/document/` directory, one directory per
suite, each owned by a single document-target statement. A fixture comprises a
complete YAML document, its expected verdict (`valid` or `invalid`) declared
in the statement's acceptance entry in `language.yaml`, adjacent to the check
it measures against. This setup prevents the suite and statement from
diverging.

The language gate enforces the layout, ensuring each document-target statement
has at least one valid and one invalid fixture. Every fixture file under
`fixtures/` is referenced by exactly one statement. A fixture with a verdict
mismatching its check fails `mage audit`.

## 9.3 Claiming conformance

Claims reference the statement identifiers verified against them, within the
specific version of the specification used. The component constitutions in
`docs/constitutions/` act as standing claims: each links a component to the
required statements, citing their identifiers.
