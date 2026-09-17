<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Serving wrappers and workload promotion

A serving wrapper turns a capability into a workload. It contributes exactly four things: a serve machine (launch, await control, stop), the lifecycle tool words, the control and monitor REST servers, and the application routes that bind capabilities. Everything else belongs to the capability profiles it hosts.

The target shape exists in the tree: `applications/agent-architecture/agents/applier/` is a complete workload in two files — an 11-line `profile.yaml` referencing the catalog applier's machine, tools, and declarations, plus a `rest.yaml`. When a wrapper grows beyond that, the excess is usually a copy of something shared.

## The serve machine

We do not write serve loops by hand. `agent-core/tools/machines/` ships the templates — `serve-machine-template.yaml`, `monitor-service-machine-template.yaml`, `lifecycle-approval-machine-template.yaml` — and a wrapper's `machine.yaml` reduces to one instantiation with the four word names as arguments. `applications/chatbot-mesh/agents/chatbot/machine.yaml` is the reference instance at 25 lines; the same machine written out by hand runs about 60. Migration of the remaining hand-rolled serve machines is tracked in GH-2168 (planned).

## The lifecycle vocabulary and monitor pair

The six lifecycle words (launch requests, launch control, await control, stop requests, and the exit and stop variants) and the monitor launch/stop pair are the same in every wrapper up to naming. They become fragment instantiations: the monitor pair from the shared monitor fragment (the chatbot-mesh chatbot already instantiates it), and the lifecycle words from a serve-lifecycle declarations fragment planned in GH-2166. Until that fragment ships, new wrappers copy the chatbot-mesh chatbot's declarations rather than a demo repository's — the monorepo copy is the shortest and closest to the fragment's target content.

## The control and monitor servers

Every wrapper exposes the same eight monitor routes and the control server. This block becomes a REST fragment once `rest.yaml` supports instantiation (GH-2167, planned). Until then we copy the block from `applications/chatbot-mesh/agents/chatbot/rest.yaml` unchanged apart from ports and the agent name, because the monitor OpenAPI surface is pinned by the presentation contract of the declarative UX epic (GH-2154).

## Promotion

Promotion makes the wrapper disappear. A deployment entry in `application.yaml` names the capability profile and the staging pipeline generates the wrapper into the staged closure:

```yaml
deployment:
  entries:
    - id: observer
      promote:
        profile: agents/observer/profile.yaml
        rest_extra: agents/observer/rest.yaml
      workload: observer
      mount_path: /profiles
```

This is planned work (GH-2169); the section records the target shape so wrappers written today converge toward it. Generation happens at profile-staging time in `pkg/profilestage`, the generated files land in `helm/profiles/` like hand-written ones, and the generic binary needs no change. The parity gate is `--dump-config` equality with the wrapper the entry replaces.

## Checklist for a new workload

Until promotion lands, a new workload adds: a wrapper `profile.yaml` referencing the capability's files, a serve `machine.yaml` as a template instance, `tools.yaml` listing the lifecycle words, `declarations.yaml` instantiating the monitor fragment plus the lifecycle words, a `rest.yaml` with the copied control/monitor block plus the application routes, and one `roots[]` plus one `deployment.entries[]` line in `application.yaml`. After GH-2169, the same workload is the one deployment entry.
