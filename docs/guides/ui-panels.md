<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# UI panels

Agent UIs are React single-page applications served by the agent itself: a `static_assets` REST binding (`agent-core/internal/tools/rest/definition/server.go`) serves a built Vite bundle from the path the agent's `rest.yaml` declares, with an SPA fallback. Each UI keeps a declarative descriptor, `ui/ui.yaml`, naming its routes, sidebar, and monitored agents. Placement rules live in `docs/engineering/eng02-agent-ui-placement.yaml`.

Today the runtime treats the bundle as bytes on disk, `ui.yaml` is cross-checked against the app's routes by a test rather than honored by a shell, and the applications share one tokens file. The declarative UX epic (GH-2154) changes each of these; this guide documents the current mechanics and the target model so UI work written now converges toward it.

## The presentation contract

Shared panels bind only to the platform monitor and trace surface that `applications/docs/specs/software-requirements/srd004-declarative-ux.yaml` (R1) pins: `/monitor/state`, `/monitor/machines`, `/monitor/tools/declared`, `/monitor/events/stream`, `/monitor/fleet`, and the two `/query/traces` forms. srd004 names each endpoint's owning SRD and never restates a schema. Panels reach other agents only through `/monitor-proxy/{agent}/{path...}`; a proxy 404 means the agent is not deployed and its panels are hidden. The cohere-demo observer's `/trace-proxy` name is retired (R2.3). Within contract major version 1, owners change responses additively only, and the kit's recorded fixtures are the consumer contract tests (R3).

## The panel model

A panel is the reuse unit: a component plus a manifest naming its id, route, required endpoints, and monitored agents. Panels arrive from three sources.

| Source | Examples | Status |
|---|---|---|
| ui-kit package | trace waterfall, machine view, topology, agent card, status bar, fleet | planned (GH-2156, GH-2157) |
| the platform, embedded in a tool | the observer UI served by the rest tool via `go:embed`, backed by the observer capability profile | planned (GH-2158, GH-2170) |
| the application | domain panels an app keeps to itself, including cohere-demo's provenance panel, which reads `/api/v1/documents` (srd004 R4.4) | current practice |

Composition is build-time: an application lists panel packages and its own domain panels, and one bundle is built. `ui.yaml` becomes the composition input a generic shell renders routes and sidebar from (GH-2159, planned), replacing the cross-check test.

## Design tokens

The canonical tokens file is `applications/catalog/ui/design-tokens.css`, imported by relative path and enforced by the design-tokens drift test. It moves into the ui-kit package as the kit's first content (GH-2156, planned); consumers then import it from the package instead of by filesystem path, which is the supported form for repositories outside this one.

## Rules that hold now

New UI code binds backends through a client with an injectable base URL rather than hard-coded relative fetches, so the same panel runs same-origin and under a desktop shell later. New shared-looking components (anything a second application would want) go toward the kit rather than into an app's `src/`, even while the kit is pending — keeping them in one file with no app imports makes the later extraction mechanical. Domain panels stay in their application; the epic does not unify them.
