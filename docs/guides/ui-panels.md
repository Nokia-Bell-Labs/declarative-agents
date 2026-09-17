<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# UI panels

Agent UIs are React single-page applications served by the agent itself: a `static_assets` REST binding (`agent-core/internal/tools/rest/definition/server.go`) serves a built Vite bundle from the path the agent's `rest.yaml` declares, with an SPA fallback. Each UI keeps a declarative descriptor, `ui/ui.yaml`, naming its routes, sidebar, and monitored agents. Placement rules live in `docs/engineering/eng02-agent-ui-placement.yaml`.

Today the runtime treats the bundle as bytes on disk, `ui.yaml` is cross-checked against the app's routes by a test rather than honored by a shell, and the applications share one tokens file. The declarative UX epic (GH-2154) changes each of these; this guide documents the current mechanics and the target model so UI work written now converges toward it.

## The presentation contract

Shared panels bind only to the platform monitor and trace surface: the monitor views (`state`, `machines`, `tools/declared`, `events/stream`), the fleet view, the trace queries, and the `monitor-proxy/{agent}/{path...}` convention where a proxy 404 means the agent is not deployed and its panel is hidden. Pinning this surface as a versioned contract is the first issue of the epic (GH-2155, planned). Until it lands, we treat those endpoints as frozen: UI code may consume them, backend changes to them are additive only.

## The panel model

A panel is the reuse unit: a component plus a manifest naming its id, route, required endpoints, and monitored agents. Panels arrive from three sources.

| Source | Examples | Status |
|---|---|---|
| ui-kit package | trace waterfall, machine view, topology, agent card, status bar, fleet, provenance | planned (GH-2156, GH-2157) |
| the platform, embedded in a tool | the observer UI served by the rest tool via `go:embed`, backed by the observer capability profile | planned (GH-2158, GH-2170) |
| the application | domain panels an app keeps to itself | current practice |

Composition is build-time: an application lists panel packages and its own domain panels, and one bundle is built. `ui.yaml` becomes the composition input a generic shell renders routes and sidebar from (GH-2159, planned), replacing the cross-check test.

## Design tokens

The canonical tokens file is `applications/catalog/ui/design-tokens.css`, imported by relative path and enforced by the design-tokens drift test. It moves into the ui-kit package as the kit's first content (GH-2156, planned); consumers then import it from the package instead of by filesystem path, which is the supported form for repositories outside this one.

## Rules that hold now

New UI code binds backends through a client with an injectable base URL rather than hard-coded relative fetches, so the same panel runs same-origin and under a desktop shell later. New shared-looking components (anything a second application would want) go toward the kit rather than into an app's `src/`, even while the kit is pending — keeping them in one file with no app imports makes the later extraction mechanical. Domain panels stay in their application; the epic does not unify them.
