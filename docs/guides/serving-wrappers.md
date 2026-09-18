<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Serving wrappers and blueprints

A serving wrapper turns a capability into a workload. It contributes exactly four things: a serve machine (launch, await control, stop), the lifecycle tool words, the control and monitor REST servers, and the application routes that bind capabilities. Everything else belongs to the capability profiles it hosts.

The target shape exists in the tree: `applications/agent-architecture/agents/applier/` is a complete workload in two files — an 11-line `profile.yaml` referencing the catalog applier's machine, tools, and declarations, plus a `rest.yaml`. When a wrapper grows beyond that, the excess is usually a copy of something shared.

## The serve machine

We do not write serve loops by hand. `agent-core/tools/machines/` ships the templates — `serve-machine-template.yaml`, `monitor-service-machine-template.yaml`, `lifecycle-approval-machine-template.yaml` — and a wrapper's `machine.yaml` reduces to one instantiation. The serve template takes the four word names, a `workflow` value for the machine's metric label, and an optional `command_timeout`; its transitions carry the phase labels every serve agent shares, so an instance's telemetry names the agent and the phase without writing either out. `applications/chatbot-mesh/agents/chatbot/machine.yaml` is the reference instance; documentation-curator shows the label and budget arguments in use.

A machine that is the serve lifecycle plus extra states — a poll loop, an intake loop, a launch sequence — is not an instance, because an instance carries only its name, purpose and arguments (srd054 R1.2). The extra states are a capability waiting to be cleaved out (GH-2170, GH-2188), and the wrapper becomes an instance when they go. Bench is the worked case: its Validating and Launching pair became the `bench-experiments` capability, its `machine.yaml` is now one instantiation of the serve template, and `TestBenchMachineIsServeTemplateInstance` fails if a bench state ever returns to the host.

## The lifecycle vocabulary and monitor pair

The six lifecycle words (launch requests, launch control, await control, stop requests, and the exit and stop variants) and the monitor launch/stop pair are the same in every wrapper up to naming. Both live in agent-core now. The four lifecycle words come from `agent-core/tools/units/serve-lifecycle-declarations-fragment.yaml`, which takes the names they arrive under as arguments, so the machine that references them needs no change. The monitor pair comes from `agent-core/tools/units/monitor-rest-declarations.yaml` by plain import: the monitor server fragment always names its server `monitor`, so the pair has nothing to vary. A serving agent's whole declarations file is that import and that instantiation, about twenty lines where it was a hundred and thirty-five, and catalog's applier takes the same shape as the chatbot-mesh agents (GH-2180).

The monitor pair and catalog's monitor control fragment are not duplicates of each other. The pair is for an agent that has its own control server and await word; `catalog/agents/units/monitor-control-fragment.yaml` is the launch/await/stop trio for an agent whose control server is its monitor server. They compose different sets, and fragments do not nest (srd052 R1.2), so the shared monitor words stay written twice.

## The control and monitor servers

Every wrapper exposes the same eight monitor routes and the control server. The monitor server is now one instantiation of `agent-core/tools/rest/units/monitor-server-fragment.yaml`, and it lives in a `monitor-rest.yaml` that only the agent's own profile lists, because a `rest.yaml` shared with a request profile would make the instantiation an unused import there (srd052 R3.1). The control server stays written out: its name differs per agent, and instantiating it under `as` would rename its endpoints and with them the generated OpenAPI operation ids, which is a worse trade than the ten lines it saves.

## The wrapper as a blueprint

The wrappers are one agent with arguments: they differ by agent name, two ports, the four lifecycle word names, and their application routes. That is what an agent blueprint is for. srd055 declares a whole profile once as a fragment with typed parameters, carries its machine template and units with it, and each agent's profile becomes an instance holding a name, the arguments, and only the fields that genuinely differ. Implementation is GH-2123, and the serving wrappers are the profile group it was waiting for.

We do not add a second profile-level templating mechanism beside it. An earlier proposal to generate wrappers from an `application.yaml` promotion entry (GH-2169) was closed for that reason: the deployment entry stays what it is, and the wrapper becomes a blueprint instance.

The shape to converge on already exists in the tree. `applications/agent-architecture/agents/applier/` is a workload in an 11-line `profile.yaml` plus a `rest.yaml`, and a blueprint instance is that with its arguments named.

## Checklist for a new workload

Until blueprints land, a new workload adds: a wrapper `profile.yaml` referencing the capability's files, a serve `machine.yaml` as a template instance, `tools.yaml` listing the lifecycle words, `declarations.yaml` importing the monitor pair and instantiating the lifecycle words, a `rest.yaml` carrying the control and monitor block plus the application routes, and one `roots[]` and one `deployment.entries[]` line in `application.yaml`. After GH-2123, the first five collapse into an instance naming the blueprint and its arguments.
