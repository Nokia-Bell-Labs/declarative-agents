<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 8. Population invariants

## 8.1 The population target

A population comprises active instances in a mesh at any moment, including
application-instances, their agent-instances, and workspaces these agents bind
to or leave waiting. The invariants in this chapter apply to the entire set,
distinguishing the population target from the runtime target. A conforming
runtime can engage with a non-conforming population if an external entity
starts an agent-instance not defined by any application-profile.

## 8.2 Closure

The population is closed under the declarations: every running agent-instance
traces to an agent-profile explicitly named among the roots of a governing
application-profile. There is no way into the population except through a declared root.

{{statement R-POP-001}}

## 8.3 Authority

Deciding and acting are distinct roles. One agent-profile's instances
determine the desired state, while another's hold exclusive authority to
modify it. The provisioning-workflow-orchestrator exemplifies this separation
by making operational decisions, and the creator alone reaches the deployment
interface and never decides (chatbot-mesh srd004 and srd005).

{{statement R-POP-002}}

## 8.4 Workspace exclusivity and orphans

Workspaces ensure durable population history through two invariants.
Exclusivity limits a workspace to one bound agent-instance at a time,
preventing concurrent identity actions from multiple locations. Orphan
legality makes a workspace without a bound agent-instance a normal state —
it is how identity survives between instantiations, and only an explicit
decommission removes it.

{{statement R-POP-003}}

{{statement R-POP-004}}

## 8.5 Cardinality

An application-profile declares how many agent-instances each root seeds, with
population counts following this declaration. Chapter 05 covers the grammar
for declaring cardinality, and its population statement aligns with this
grammar to prevent drift.
