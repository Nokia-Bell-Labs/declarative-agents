<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Serving wrappers and blueprints

A serving wrapper turns a capability into a workload. It contributes exactly four things: a serve machine (launch, await control, stop), the lifecycle tool words, the control and monitor REST servers, and the application routes that bind capabilities. Everything else belongs to the capability profiles it hosts.

The target shape exists in the tree: `applications/agent-architecture/agents/applier/` is a complete workload in two files — an 11-line `profile.yaml` referencing the catalog applier's machine, tools, and declarations, plus a `rest.yaml`. When a wrapper grows beyond that, the excess is usually a copy of something shared.

## The serve machine

We do not write serve loops by hand. `agent-core/tools/machines/` ships the templates — `serve-machine-template.yaml`, `monitor-service-machine-template.yaml`, `lifecycle-approval-machine-template.yaml` — and a wrapper's `machine.yaml` reduces to one instantiation with the four word names as arguments. `applications/chatbot-mesh/agents/chatbot/machine.yaml` is the reference instance at 25 lines; the same machine written out by hand runs about 60. Migration of the remaining hand-rolled serve machines is tracked in GH-2168 (planned).

## The lifecycle vocabulary and monitor pair

The six lifecycle words (launch requests, launch control, await control, stop requests, and the exit and stop variants) and the monitor launch/stop pair are the same in every wrapper up to naming. Both are fragment instantiations now. The four lifecycle words come from `agent-core/tools/units/serve-lifecycle-declarations-fragment.yaml`, which takes the names they arrive under as arguments, so the machine that references them needs no change; the monitor pair comes from the monitor fragment beside the agent. A chatbot-mesh wrapper's whole declarations file is those two instantiations and nothing else, about twenty lines where it was a hundred and thirty-five.

The two monitor fragments in the tree are not duplicates of each other. `mesh-monitor-fragment.yaml` is the launch/stop pair for an agent that has its own control server and await word; `catalog/agents/units/monitor-control-fragment.yaml` is the launch/await/stop trio for an agent whose control server is its monitor server. They compose different sets, and fragments do not nest (srd052 R1.2), so the shared monitor words stay written twice.

## The control and monitor servers

Every wrapper exposes the same eight monitor routes and the control server. REST definitions already instantiate fragments — `unit:`, `imports:`, and `instantiate:` work in `rest.yaml` exactly as they do in a declarations file, under the srd052 rules — so this block is a fragment waiting to be written rather than a mechanism waiting to be built. GH-2167 supplies the canonical one; until it lands we copy the block from `applications/chatbot-mesh/agents/chatbot/rest.yaml` unchanged apart from ports and the agent name, because the monitor surface is pinned by the presentation contract of the declarative UX epic (GH-2154).

## The wrapper as a blueprint

The wrappers are one agent with arguments: they differ by agent name, two ports, the four lifecycle word names, and their application routes. That is what an agent blueprint is for. srd055 declares a whole profile once as a fragment with typed parameters, carries its machine template and units with it, and each agent's profile becomes an instance holding a name, the arguments, and only the fields that genuinely differ. Implementation is GH-2123, and the serving wrappers are the profile group it was waiting for.

We do not add a second profile-level templating mechanism beside it. An earlier proposal to generate wrappers from an `application.yaml` promotion entry (GH-2169) was closed for that reason: the deployment entry stays what it is, and the wrapper becomes a blueprint instance.

The shape to converge on already exists in the tree. `applications/agent-architecture/agents/applier/` is a workload in an 11-line `profile.yaml` plus a `rest.yaml`, and a blueprint instance is that with its arguments named.

## Checklist for a new workload

Until blueprints land, a new workload adds: a wrapper `profile.yaml` referencing the capability's files, a serve `machine.yaml` as a template instance, `tools.yaml` listing the lifecycle words, `declarations.yaml` instantiating the monitor fragment plus the lifecycle words, a `rest.yaml` carrying the control and monitor block plus the application routes, and one `roots[]` and one `deployment.entries[]` line in `application.yaml`. After GH-2123, the first five collapse into an instance naming the blueprint and its arguments.
