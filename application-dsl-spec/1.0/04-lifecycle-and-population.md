<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Lifecycle and population

The lifecycle governs how a single instance comes and goes; the population
invariants govern the set of instances that exist at any moment. This section
takes them in that order.

## The lifecycle square

The lifecycle has two columns — one for persistent workspaces, one for transient
instances — and each column has a creating and a destroying primitive, four
operations in all.

| | Workspace column | Instance column |
|---|---|---|
| create | provision | instantiate |
| destroy | decommission | terminate |

Provision creates a workspace and its identity; decommission destroys both.
Instantiate combines an agent-profile, an identity, and a provisioned workspace
into a running agent-instance; terminate ends that instance. The columns operate
independently: terminating an instance does not touch the provisioned workspace,
and a workspace can stay provisioned with no instance running. Expansion sits
outside the square — the model keeps class-to-class construction separate from
these operations.

Binding connects the columns. An agent-instance binds one workspace for its
lifetime, adopts its identity, and releases it on termination. The workspace
keeps that identity and its contents and stays inactive until the next
agent-instance takes it.

{{statement R-LCM-001}}

{{statement R-LCM-002}}

Runtime-target statements bind running systems, so their acceptance entries are
rig references: each names a test in this repository's integration suites that
exercises the obligation against a real workspace or deployment. The language
gate checks that every reference resolves to a declared test, so a renamed or
deleted rig test fails `mage audit`. The referenced suites run under the
repository's test and integration gates, which own the clusters and rigs the
audit itself does not.

## The population

A population is the set of active instances in a mesh at a given moment —
application-instances, their agent-instances, and the workspaces those agents
bind or leave waiting. The invariants below apply to the whole set, which is
what separates the population target from the runtime target: a conforming
runtime can host a non-conforming population if some external entity starts an
agent-instance no application-profile declares.

The population is closed under the declarations. Every running agent-instance
traces to an agent-profile named among the roots of a governing
application-profile; there is no way into the population except through a
declared root.

{{statement R-POP-001}}

Deciding and acting are distinct roles. One agent-profile's instances determine
the desired state while another's hold the exclusive authority to change it. The
provisioning-workflow-orchestrator shows the separation: it makes the
operational decisions, and the creator alone reaches the deployment interface and
never decides (chatbot-mesh srd004 and srd005).

{{statement R-POP-002}}

Two invariants give workspaces a durable history. Exclusivity limits a workspace
to one bound agent-instance at a time, so no two locations act under the same
identity at once. Orphan legality makes a workspace with no bound agent-instance
a normal state — it is how an identity survives between instantiations, and only
an explicit decommission removes it.

{{statement R-POP-003}}

{{statement R-POP-004}}

An application-profile declares how many agent-instances each root seeds, and the
population count follows that declaration. The grammar for declaring cardinality
lives with the application-profile; the population statement is aligned with it
so the two cannot drift.
