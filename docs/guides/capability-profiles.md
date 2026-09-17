<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Capability profiles

A capability profile does one job and terminates. Its machine runs from an initial state to a terminal state, it owns no serving lifecycle, no control or monitor servers, and no deployment identity. The serving concerns live in whatever hosts it. This separation is what lets the same behavior appear as a tool inside one agent and as a standalone agent in another application without a rewrite.

Two profiles in the tree model the shape. `applications/coding-agent/agents/executor/` is a request-scoped capability: the role servers around it bind it with `machine_request` and map its terminal states to HTTP statuses. `applications/catalog/agents/runtime-state-reader/` is the monitoring capability as a 110-line agent whose machine is a 14-line instantiation of a shared template.

srd057 defines the capability profile, the consumption forms it supports, and the binding discipline that keeps shared behaviour a named profile. This guide is the working companion to it.

## Writing one

We keep the machine request-scoped: accept input from the invoking word or route, do the work, reach a terminal state that names the outcome. Budget fields (`command_timeout`, `max_iterations`) bound the run. Tools declare side effects and undo so a hosting agent can reason about reversibility. Nothing in the profile may assume it is long-running: no launch/await/stop states, no monitor wiring, no ports.

## Consuming one

The table lists the four forms and when we reach for each.

| Form | Declaration | Choose when |
|---|---|---|
| `self_invoke` | tool word with `config.profile: <profile path>` | process isolation, workspace undo, trace propagation |
| `machine_request` | REST route with `machine_request: {profile, timeout, response.terminal_states}` | the capability is a service another agent or a UI calls |
| serving wrapper | a wrapper profile hosting it, a blueprint instance where the wrappers are one agent with arguments | the capability deserves its own Deployment |

`coding-agent/agents/executor/rest.yaml` shows the `machine_request` form binding a profile path; `applications/catalog/agents/bench/builtin.yaml` shows `self_invoke` with request and output mapping.

There is no in-process form to reach for. `run_point` is the nested-machine boundary's only word and it is evaluation-harness machinery — it requires `point_machine`, `point_tools`, and `point_tool_declarations` and keeps per-point session state (srd019). A capability profile handed to it fails registration, which srd057 R2.4 states and the release 21.0 suite tests.

## Binding discipline

`machine_request` also accepts `profile: profile.yaml` with a `machine:` override — a machine fork of the serving profile itself. We reserve that form for behavior private to one agent. Anything reused across agents or repositories must be a named capability profile, so the reuse is visible in the closure and the copies cannot drift. Converting the existing machine-override bindings that carry shared behavior is tracked in GH-2171 (planned); the survey behind it found 74 of 103 bindings using the override form.

## Current capability inventory

Capabilities being cleaved out of their current hosts under GH-2171 (planned): vector query against the document store, document read and filter, the provisioning and creator request sides, the collector's span intake loop, and the bench experiment launcher. The fleet observer becomes a capability profile under GH-2170 (planned), which also serves the embedded observer UI of GH-2158.
