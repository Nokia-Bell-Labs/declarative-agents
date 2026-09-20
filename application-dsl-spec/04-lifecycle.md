<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 4. Lifecycle

## 4.1 The lifecycle square

The lifecycle has two columns, one for persistent workspaces and one for
transient instances. Each column has a creating and destroying primitive,
totaling four operations.

| | Workspace column | Instance column |
|---|---|---|
| create | provision | instantiate |
| destroy | decommission | terminate |

Provision creates a workspace and its identity; decommission destroys both.
Instantiate combines an agent-profile, an identity, and a provisioned
workspace to produce a running agent-instance; terminate ends this instance.
The columns operate independently: terminating an instance does not affect the
provisioned workspace, and a workspace can remain provisioned without a
running instance. Expansion is kept outside the square; chapter 03 maintains
class-to-class construction separate from these operations.

## 4.2 Workspace binding

Binding connects columns. An agent-instance binds one workspace for its
lifecycle, adopting its identity, and relinquishes it upon termination. The
workspace, retaining identity and contents, stays inactive until assigned to
the next agent-instance.

{{statement R-LCM-001}}

{{statement R-LCM-002}}

## 4.3 Evidence for runtime statements

Runtime-target statements bind running systems, so their acceptance entries
are rig references. Each names a test in this repository's integration suites
exercising the obligation against a real workspace or deployment. The language
gate verifies every reference resolves to a declared test, so a renamed or
deleted rig test fails `mage audit`. The referenced suites run under the
repository's test and integration gates, which own the clusters and rigs the
audit does not.
