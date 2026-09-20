<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 6. Agent-profile grammar

## 6.1 The document

An agent-profile is a `profile.yaml`: the unit of behaviour an agent-instance
realizes. It binds a machine-profile to the tool selections, tool
declarations, REST definitions, and inference configurations the
machine-instance's actions dispatch through. The agent-profile is the class
side of instantiation — chapter 04's primitives take exactly one agent-profile
per agent-instance.

## 6.2 Keynames

{{keyname-table yaml:agent-core/docs/specs/config-formats/agent-profile-format.yaml}}

{{statement R-PROF-001}}

## 6.3 Referenced documents

The reference keynames resolve relative to the agent-profile's directory
unless the runtime contract says otherwise. Tool selections name entries the
tool declarations must declare; REST definitions bind declared tools to
transport; inference configurations select models per state. Each referenced
family has its own format specification under
`agent-core/docs/specs/config-formats/`, and this chapter does not restate
their grammars — the agent-profile grammar owns only the binding keynames in
the table above.

## 6.4 Profile fragments

The `unit`, `params`, and `profile` keynames belong to profile fragments: a
reusable unit whose body is `profile`, expanded into a complete agent-profile
document under the expansion keyname exactly as chapter 03 specifies for
machine-profile documents. The expansion statements R-MODEL-001 and
R-MODEL-002 govern here unchanged.
