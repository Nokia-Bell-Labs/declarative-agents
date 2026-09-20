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

`applications/catalog/agents/planner/builtin.yaml` shows the `self_invoke` form in `invoke_executor`. `coding-agent/agents/executor/rest.yaml` shows the `machine_request` form binding a profile path, and bench shows it launching work that outlives the request: the `experiments` route in `applications/catalog/agents/bench/rest.yaml` binds `bench/experiments-profile.yaml`, whose one word starts the critic as a detached child through `start_service` and returns a handle while the run continues (GH-2188).

There is no in-process form to reach for. `run_point` is the nested-machine boundary's only word and it is evaluation-harness machinery — it requires `point_machine`, `point_tools`, and `point_tool_declarations` and keeps per-point session state (srd019). A capability profile handed to it fails registration, which srd057 R2.4 states and the release 21.0 suite tests.

## Binding discipline

`machine_request` also accepts `profile: profile.yaml` with a `machine:` override — a machine fork of the serving profile itself. We reserve that form for behavior private to one agent. Anything reused across agents or repositories must be a named capability profile, so the reuse is visible in the closure and the copies cannot drift. The survey behind GH-2171 found 74 of 103 bindings using the override form. GH-2171 closed as decided: most of those overrides are private to their agent, and the one cleave this repository owed shipped in GH-2188.

## Current capability inventory

GH-2171 measured six candidate cleaves, and each ended differently. We record the outcomes because each one states a rule a future cleave will meet.

Table: GH-2171 cleave outcomes

| Candidate | Outcome |
|---|---|
| Bench experiment launcher | Shipped as `bench-experiments` (GH-2188). Its host is now a serve-machine-template instance. |
| Collector span intake | Not extractable: the intake loop is a never-ending rotation whose lifecycle await is one of its legs, which srd057 R1.1 rules out. |
| Vector query, provisioning and creator request sides | Decided against. Their SRDs (srd013, srd016, srd017) record each agent's move out of the catalog, and `applications/catalog/README.md` makes membership an owner's decision, not a side effect of a cleave. |
| Document read, scenario rigs | Owned by the consumer repositories (petar-djukic/cohere-demo#738, petar-djukic/agentic-wiki-mesh#125). |

The fleet observer is not among them, and the reason is worth recording. Its poll loop never reaches a terminal state, and the loop is driven by the lifecycle await itself: `AwaitTimedOut` routes to `Discovering`, so the interval timer and the service lifecycle are one machine by design. R1.1 rules that out, and the forms that remain would each spawn a process per poll interval. What the observer shares instead is its whole agent: the catalog owns its machine, words, and selection, and a mesh wraps them with its own REST surface (GH-2170), the way the applications wrap the catalog's applier.

Where a unit lives, what earns it a shared location, and which reuse form fits a given repetition are conventions, not mechanics: [eng03-declaration-standard-library](../engineering/eng03-declaration-standard-library.yaml) states them.
