<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 4. Lifecycle

## 4.1 The lifecycle square

The lifecycle has two columns because the language has two kinds of durable
thing on the instance side: workspaces, which persist, and instances, which
come and go. Each column has a creating primitive and a destroying primitive,
giving four operations.

| | Workspace column | Instance column |
|---|---|---|
| create | provision | instantiate |
| destroy | decommission | terminate |

Provision creates a workspace and the identity it carries; decommission
destroys both, deliberately and explicitly. Instantiate takes an
agent-profile, an identity, and a provisioned workspace and produces a
running agent-instance; terminate ends the agent-instance. The columns are
independent: terminating an agent-instance leaves its workspace provisioned,
and a provisioned workspace does not imply a running agent-instance. Nothing
in the square is expansion — chapter 03 keeps class-to-class construction
apart from every operation here.

## 4.2 Workspace binding

Binding is the hinge between the columns. An agent-instance binds exactly one
workspace for its whole life, takes its identity from it, and releases it at
termination; the workspace then waits, with its identity and contents intact,
for the next agent-instance.

{{statement R-LCM-001}}

{{statement R-LCM-002}}

## 4.3 Evidence for runtime statements

Runtime-target statements bind running systems, so their acceptance entries
are rig references: each names a test in this repository's integration suites
that exercises the obligation against a real workspace or a real deployment.
The language gate verifies every reference resolves to a declared test, so a
renamed or deleted rig test fails `mage audit`; the referenced suites
themselves run under the repository's test and integration gates, which own
the clusters and rigs the audit does not.
