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

## 6.4 Blueprints

A blueprint declares an agent-profile once and leaves what varies as typed
parameters. It carries the `params` and `profile` keynames: `params` declares
the parameters, and `profile` holds the body, an agent-profile whose scalar
values may reference `$param(name)`. A blueprint is not itself an
agent-profile document, because its references stay unfilled until arguments
arrive.

An agent-profile document expands a blueprint under the expansion keyname,
supplying the arguments and whatever fields it overrides. We name the
expanding document rather than calling it an instance: section 2.2 reserves
instance for the mesh side, and expansion constructs a class from a class.

{{statement R-PROF-002}}

{{statement R-PROF-003}}

R-MODEL-002 governs the entry itself, in an agent-profile document as in a
machine-profile document: the entry names its blueprint under `fragment` and
its argument bindings under `args`.

The grammar above is the document-target subset of `srd055-agent-blueprints`,
which also settles what this chapter leaves alone — fields present on the
expanding document replace the blueprint's wholesale, paths in the blueprint
body resolve against the blueprint's own directory, and the expanded result
satisfies the rules a hand-written agent-profile does. No processor enforces
any of it today: the loader work is deferred under gh-2123, so these
statements bind documents alone until it lands.
