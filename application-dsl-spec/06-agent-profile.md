<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 6. Agent-profile grammar

## 6.1 The document

An agent-profile, defined in `profile.yaml`, is the behavior unit an
agent-instance realizes. It binds a machine-profile to tool selections, tool
declarations, REST definitions, and inference configurations that dispatch the
machine-instance's actions. The agent-profile is the class side of
instantiation—chapter 04's primitives take one agent-profile per
agent-instance.

## 6.2 Keynames

{{keyname-table yaml:agent-core/docs/specs/config-formats/agent-profile-format.yaml}}

{{statement R-PROF-001}}

## 6.3 Referenced documents

Reference keynames resolve relative to the agent-profile's directory, unless
the runtime contract specifies otherwise. Tool selections must match declared
entries in the tool declarations; REST definitions bind these tools to their
transport mechanisms; inference configurations specify models per state. Each
referenced family has its own format specification under
`agent-core/docs/specs/config-formats/`, and this chapter avoids repeating
their grammars — the agent-profile grammar solely manages binding keynames in
the table above.

## 6.4 Profile fragments

The `unit`, `params`, and `profile` keynames link to profile fragments,
reusable units with body `profile`. These expand into a complete agent-profile
document under the expansion keyname, following chapter 03's rules for
machine-profile documents. Expansion statements R-MODEL-001 and R-MODEL-002
apply unchanged.
