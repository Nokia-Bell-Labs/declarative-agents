<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# UI panels

Agent UIs are React single-page applications served by the agent itself: a `static_assets` REST binding (`agent-core/internal/tools/rest/definition/server.go`) serves a built Vite bundle from the path the agent's `rest.yaml` declares, with an SPA fallback. Each UI keeps a declarative descriptor, `ui/ui.yaml`, naming its routes, sidebar, and monitored agents. Placement rules live in `docs/engineering/eng02-agent-ui-placement.yaml`.

Before the declarative UX epic the runtime treated every bundle as bytes on disk (a platform bundle such as the observer is now compiled into agent-core), a test cross-checked `ui.yaml` against a hand-written route table, and the applications shared one tokens file. The declarative UX epic (GH-2154) changes each of these; this guide documents the current mechanics and the target model so UI work written now converges toward it.

## The presentation contract

Shared panels bind only to the platform monitor and trace surface that `applications/docs/specs/software-requirements/srd004-declarative-ux.yaml` (R1) pins: `/monitor/state`, `/monitor/machines`, `/monitor/tools/declared`, `/monitor/events/stream`, `/monitor/fleet`, and the two `/query/traces` forms. srd004 names each endpoint's owning SRD and never restates a schema. Panels reach other agents only through `/monitor-proxy/{agent}/{path...}`; a proxy 404 means the agent is not deployed and its panels are hidden. The cohere-demo observer's `/trace-proxy` name is retired (R2.3). Within contract major version 1, owners change responses additively only, and the kit's recorded fixtures are the consumer contract tests (R3).

## The panel model

A panel is the reuse unit: a component plus a manifest naming its id, route, required endpoints, and monitored agents. Panels arrive from three sources.

| Source | Examples | Status |
|---|---|---|
| ui-kit package | trace waterfall and list, machine view, topology, agent card, status bar, fleet (`kitPanelRegistry`) | shipped (GH-2156, GH-2157) |
| the platform, embedded in a tool | the observer UI, built from `applications/ui-kit/observer` and served by the rest tool via `go:embed` when a `static_assets` binding selects `bundle: observer`; the chatbot-mesh observer serves it with no UI files of its own (srd004 R9) | shipped (GH-2158) |
| the application | domain panels an app keeps to itself, including cohere-demo's provenance panel, which reads `/api/v1/documents` (srd004 R4.4) | current practice |

Composition is build-time: an application lists panel packages and its own domain panels, and one bundle is built. `ui.yaml` is the composition input the kit's `AppShell` renders routes and sidebar from (GH-2159, shipped): the kit Vite plugin bundles and validates it, and the application supplies a registry for its domain panels. The Chatbot Mesh chatbot is the worked case: `agents/chatbot/ui/ui.yaml` plus `app/src/panels.ts`, with no route table of its own.

## The kit package

The kit lives at `applications/ui-kit` as the npm package `@declarative-agents/ui-kit` (GH-2259). UIs in this repository depend on it by `file:` path; external repositories depend on the `npm pack` tarball attached to the GitHub release `ui-kit/vX.Y.Z`, which `mage uikit:release` prepares from a clean tree. No registry, credential, or environment variable is involved. The kit README lists the Mage targets.

## ui.yaml version 2

`ui.yaml` is the composition input of the kit shell (srd004 R7). Version 2 adds `version`, `panels`, and `branding` to the version 1 fields; a file without `version` is version 1 and stays valid. `magefiles/uiyaml` validates it in Go, and `applications/ui-kit/schema/ui.v2.schema.json` carries the same structure for charts and the kit build.

Table: ui.yaml fields

| Field | Meaning |
|---|---|
| `version` | `2` to compose panels; absent means 1 |
| `id`, `title`, `source_owner` | identity of the UI and its owning actor |
| `sidebar.title`, `sidebar.groups.<id>.{label, order}` | navigation title and ordered sections |
| `routes[].{id, path, label, action, resource}` | navigable paths that are not composed panels |
| `panels[].id` | panel id and route id; also the application registry key |
| `panels[].package` | `@declarative-agents/ui-kit` for a kit panel, any other name (for example `local`) for a domain panel |
| `panels[].export` | for kit panels, the kit panel manifest id (`trace`, `fleet`, `machine-view`, `topology`, `agent-card`, `status-bar`) |
| `panels[].route` | one lower-case segment such as `/traces`; the last URL segment selects the panel |
| `panels[].{label, sidebar_group, hidden}` | sidebar entry, its group, and whether it is reachable by URL only |
| `panels[].config` | map passed to the panel as `config` |
| `monitored_agents[].{name, label}` | agents panels read through `/monitor-proxy/<name>/` |
| `trace_backend.{name, query_path}` | agent whose proxy serves the trace queries |
| `branding.{title, logo, accent}` | shell title and look |
| `presentation` | application presentation flags |

The validator rejects a duplicate id across routes and panels, two routes on one path, a `sidebar_group` not declared under `sidebar.groups`, panels without `version: 2`, and a kit panel without `export`. A chart must be able to render the file with plain templating: one document, no anchors or aliases, lists of flat maps (`uiyaml.CheckHelmRenderable`). `applications/chatbot-mesh/helm/templates/_chatbot-ui.tpl` is the worked example; `TestChatbotUIRendersAsValidUIYAML` validates its render.

The kit applies the same rules at build time. Its Vite plugin `@declarative-agents/ui-kit/vite` validates `ui.yaml` with `validateUIConfig`, the TypeScript mirror of `magefiles/uiyaml`, fails the build for a panel with no registry entry, and serves the file as `virtual:ui-config`; `AppShell` renders the sidebar and routes from it. The kit README section "Shell" shows the wiring, and `applications/ui-kit/examples/minimal/` is the smallest complete application.

## Design tokens

The canonical tokens file is `applications/ui-kit/src/tokens.css` (GH-2260). Every UI imports it as `@import "@declarative-agents/ui-kit/tokens.css";` through its dependency on the kit — a `file:` path in this repository, the release tarball elsewhere — so no UI reaches it by filesystem path. The design-token drift tests (`magefiles/uidist_test.go`, `applications/catalog/conformance/design_tokens_drift_test.go`) resolve that import through `package.json` and the kit's exports map, and fail on any other `:root` token block under `applications/`.

## Rules that hold now

New UI code binds backends through a client with an injectable base URL rather than hard-coded relative fetches, so the same panel runs same-origin and under a desktop shell later. New shared-looking components (anything a second application would want) go toward the kit rather than into an app's `src/`, even while the kit is pending — keeping them in one file with no app imports makes the later extraction mechanical. Domain panels stay in their application; the epic does not unify them.
