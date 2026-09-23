<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Mechanized checking

The statements throughout this paper are not prose commentary — each is a
machine-checked obligation, and the build fails when the paper and the checks
drift apart. This closing section describes how that checking works.

A claim of conformance names one of four targets — document, processor, runtime,
or population — and holds when the statements for that target are true. A
document conforms when it passes the checks its family's statements declare:
application-profiles against the application grammar, agent-profiles against the
agent grammar, machine-profiles against the machine grammar, and every family
against the well-formedness floor from the model. A processor conforms when it
accepts every conforming document and rejects every non-conforming one. A
runtime conforms when its runtime-target statements hold over the instances it
runs, with the rig references in those statements standing as the integration
evidence. A population conforms when the population invariants hold across its
whole instance set — a system-wide property, so a claim names the mesh it was
checked against.

{{statement R-CONF-001}}

The document-target checks are driven by fixtures. Fixture suites live under
`fixtures/document/`, one directory per suite, each owned by a single
document-target statement. A fixture is a complete YAML document paired with an
expected verdict — `valid` or `invalid` — declared in that statement's
acceptance entry in `language.yaml`, next to the check it measures. Keeping the
verdict beside the check is what stops the suite and the statement from
diverging. The language gate enforces the arrangement: every document-target
statement carries at least one valid and one invalid fixture, every file under
`fixtures/` is claimed by exactly one statement, and a fixture whose verdict
disagrees with its check fails `mage audit`.

A conformance claim, then, is a set of statement identifiers checked against a
particular version of this paper. The component constitutions under
`docs/constitutions/` are standing claims of exactly this kind: each links a
component to the statements it must satisfy, cited by identifier.
