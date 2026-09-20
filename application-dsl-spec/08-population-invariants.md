<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 8. Population invariants

## 8.1 The population target

A population is the set of running instances a mesh holds at a moment:
application-instances, their agent-instances, and the workspaces those
agent-instances bind or leave waiting. The invariants in this chapter hold
over the whole set, not over any single instance, which is what separates the
population target from the runtime target: a conforming runtime can still
participate in a non-conforming population if something outside it starts an
agent-instance no application-profile declares.

## 8.2 Closure

The population is closed under the declarations. Every running agent-instance
traces to an agent-profile that a governing application-profile names among
its roots; there is no way into the population except through a declared
root.

{{statement R-POP-001}}

## 8.3 Authority

Deciding and acting are held apart. One agent-profile's instances decide what
the population should look like; a different agent-profile's instances hold
the deployment authority that changes it, and nothing else reaches that
authority. The reference split is the provisioning-workflow-orchestrator,
which decides operations, and the creator, which alone reaches the deployment
interface and never decides (chatbot-mesh srd004 and srd005).

{{statement R-POP-002}}

## 8.4 Workspace exclusivity and orphans

Workspaces make the population's history durable, and two invariants keep
that sound. Exclusivity: a workspace has at most one bound agent-instance at
a time, so an identity never acts from two places at once. Orphan legality: a
workspace with no bound agent-instance is a normal state, not a leak — it is
how identity survives between instantiations, and only an explicit
decommission removes it.

{{statement R-POP-003}}

{{statement R-POP-004}}

## 8.5 Cardinality

An application-profile declares how many agent-instances each root seeds, and
the population's counts follow the declaration. The grammar that declares
cardinality is chapter 05's subject, and its population statement lands with
that grammar so the two cannot drift apart.
