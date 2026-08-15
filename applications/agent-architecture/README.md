<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Agent Architecture

## Purpose

This standalone application drives the Knowledge Manager documentation agent
from [agent-architecture.slide](agent-architecture.slide). The deck starts the
canonical catalog-owned documentation-curator profile, then posts a
lifecycle-exit request to its control server through the catalog-owned
lifecycle-exit client.

The application owns this README and the deck; it composes catalog-owned
profiles. `applications/catalog` owns the documentation-curator profile and its
UI and the reusable lifecycle-exit client, while `agent-core` owns the runtime
and builtin tools.

## Status

The module status is `implemented` and its ownership is `composition-only`.
The runnable, packaged, Helm-managed, kind-demo, and catalog UI surfaces have
executable evidence. Managed-service conformance is `partial` because the
remaining live lifecycle observations are `dependency_gated`.

## Composition

`agents/application.yaml` is the composition authority for the canonical
documentation-curator, collector, and lifecycle-exit roots and the local
applier composition wrapper. The local wrapper consumes the canonical catalog
applier; it is not an additional implementation.

## Capabilities

- `runnable_module`: `implemented`
- `managed_service`: `partial`
- `packaged`: `implemented`
- `helm_managed`: `implemented`
- `kind_demo`: `implemented`
- `ui`: `implemented` through catalog-owned curator and collector assets

## Ownership Boundaries

The catalog owns curator, collector, lifecycle-exit, applier, and their
canonical UIs. This application owns composition, the applier's local command
bindings, presentation, package derivation, Helm topology, kind workflow, and
end-to-end evidence. It contributes zero agents to repository totals and
reports one composition wrapper separately. Agent-core owns runtime semantics.

## Run or Planned Entry Points

All declared entry points are implemented. Use `mage run` and
`mage presentation` for the local composition, or `mage demo:up` and
`mage demo:down` for the optional Kubernetes demo.

## Verification

From `applications/agent-architecture`, run `go test ./...`, `mage audit`,
`mage stats`, and `mage helm:package`. Root `mage test`, `mage audit`, and
`mage stats` include this module.

## Documentation

The structured design corpus is `docs/VISION.yaml`, `docs/ARCHITECTURE.yaml`,
`docs/road-map.yaml`, and `docs/SPECIFICATIONS.yaml`. Helm details are in
`helm/README.md`.

## Prerequisites (macOS)

Running the local demo needs the monorepo checkout and the tools in the table
below. The sibling checkouts `applications/catalog` and `agent-core` arrive
with the monorepo clone; nothing else has to be fetched.

Table: local demo tools.

| Tool | Version | Install |
|---|---|---|
| Go | 1.26.5 (from `go.mod`) | `brew install go` |
| Mage | 1.17 or later | `brew install mage` |
| git | any recent | ships with Xcode Command Line Tools |
| a web browser | any | ships with macOS |

Homebrew's Go may trail the `go.mod` pin; the default `GOTOOLCHAIN=auto`
downloads the pinned toolchain on first build, so any Go 1.21+ install works
as a bootstrap. No Docker, Kubernetes, or LLM credentials are needed for the
local demo: the profiles run without model calls and bind only local ports.

Deploying the optional Kubernetes demo with `mage demo:up` additionally needs
the tools below, at the minimums `mage doctor` enforces.

Table: Kubernetes demo tools.

| Tool | Minimum | Install |
|---|---|---|
| Docker | 24.0 (Docker Desktop with 6 GiB memory, 4 CPUs) | https://docs.docker.com/desktop/setup/install/mac-install/ |
| kind | 0.32 | `brew install kind` |
| Helm | 3.17 | `brew install helm` |
| kubectl | 1.32 | `brew install kubernetes-cli` |

Run `mage doctor` to verify the Kubernetes toolchain and host resources
without mutating anything.

## Run the demo

Run every command from `applications/agent-architecture`. Steps 1, 2, and 4
each take their own terminal. The trace collector reserves ephemeral loopback
ports, so it can run beside the chatbot-mesh collector.

1. Serve the deck:

       mage presentation

   Open http://127.0.0.1:3999/agent-architecture.slide and follow it; the
   remaining steps mirror the slides.

2. In a second terminal, start the composition, meaning the trace collector
   and the curator together:

       mage run

   This builds the `agent` binary from the sibling `agent-core` checkout,
   starts the collector agent, and starts the canonical
   documentation-curator profile with OTLP export wired to the collector.
   A first run compiles agent-core and takes a minute; later runs are
   faster.

3. Browse the running surfaces:

   - documentation at http://localhost:18081;
   - lifecycle control at http://127.0.0.1:18082, with health at
     `/api/lifecycle/health`;
   - monitoring at http://localhost:18084/ui/; and
   - collected traces at the query URL printed by `mage run`.

4. In a third terminal, stop the curator through its own API by running the
   catalog-owned lifecycle-exit client. Build the `agent` binary from
   `../../agent-core` as in Start the Knowledge Manager, then:

       agent --profile "../catalog/agents/lifecycle-exit/profile.yaml" --directory "../catalog" --core-root "../../agent-core"

   On success the exit agent reports `terminal state: succeeded`, the curator
   shuts down, and `mage run` stops the collector and returns cleanly. A trailing
   `metric shutdown error … MetricsService` line is expected: the collector
   serves only the OTLP trace service.

## Source-checkout setup

Run all commands below from `applications/agent-architecture`. The demo resolves
two ownership roots: the catalog at `../catalog`, which owns the canonical
profile and UI, and agent-core at `../../agent-core`, which owns the development
runtime and the core declarations the profile names under `/opt/agent-core`.
Those are the monorepo defaults. When the catalog and runtime are independent
checkouts, declare their paths in `demo.yaml` — `catalog_root` and `core_root` —
and substitute them for the literal paths in the commands below. The mage
targets read `demo.yaml`; the demo uses no environment variables.

## Start the Knowledge Manager

Use an `agent` binary built from `../../agent-core`. `mage run` builds its own
copy into a private temporary directory, so the manual path and the exit client
need one on `PATH`:

    (cd ../../agent-core && go build -tags production -o agent ./cmd/agent)
    export PATH="$(cd ../../agent-core && pwd):$PATH"

Then start the canonical profile:

    agent \
      --profile "../catalog/agents/knowledge-manager/documentation-curator/profile.yaml" \
      --directory "../catalog" \
      --core-root "../../agent-core"

The catalog root is also the documentation workspace. The profile serves:

- documentation at `http://localhost:18081`;
- lifecycle control at `http://127.0.0.1:18082`; and
- monitoring at `http://localhost:18084/ui/`.

## Trace collection

By default `mage run` starts the canonical collector agent in spool mode
before the curator and exports the curator's OTLP traces to it. After the
curator exits, the collector is stopped via its control exit route.

While the demo is running, browse collected traces at the query URL printed by
`mage run`.

To disable trace collection, set `tracing: false` in `demo.yaml`, then:

    mage run

The collector reserves distinct ephemeral loopback ports for its OTLP receiver,
control, monitor, and query surfaces. The reservation avoids fixed-port
coordination with other applications while keeping every surface local.

## The lifecycle-exit client

The exit request is a declarative agent under
[`../catalog/agents/lifecycle-exit/`](../catalog/agents/lifecycle-exit/), a
catalog-owned reusable client rather than a bespoke HTTP client. Its machine has
one boundary word, `post_exit`, that binds the rest tool to POST the fixed
`{"reason": "demo presentation"}` body to `/api/lifecycle/exit`; the machine
reaches terminal state `Done` when the server returns HTTP 202 Accepted. The
control-server URL is a declared REST client base (the literal `base_url` in
`../catalog/agents/lifecycle-exit/rest.yaml`, default `http://127.0.0.1:18082`),
not runtime input, and the endpoint carries no transport authority (`auth:
none`). Retargeting to another agent's control server is a single `base_url`
change, no profile copy. Run it with:

    agent \
      --profile "../catalog/agents/lifecycle-exit/profile.yaml" \
      --directory "../catalog" \
      --core-root "../../agent-core"

Expressing the exit call as a machine rather than a Go binary makes it an
instance of the system's own thesis: runtime behavior lives in YAML and is run
by the interpreter. The catalog owns this reusable client; this application's
demo is one consumer.

## Installed runtime

An installed runtime already supplies core-owned declarations at
`/opt/agent-core`, so it does not use `core_root` or `--core-root`. It
still needs an explicit catalog root for the canonical profile:

    agent \
      --profile "../catalog/agents/knowledge-manager/documentation-curator/profile.yaml" \
      --directory "../catalog"

Run the catalog-owned exit client:

    agent --profile "../catalog/agents/lifecycle-exit/profile.yaml" --directory "../catalog"

## Kubernetes demo (optional)

This composition also deploys into a persistent kind cluster:

    mage doctor    # preflight: toolchain versions and Docker resources
    mage demo:up   # create or reuse the cluster, build images, install the chart
    mage demo:down # delete only this demo's cluster

`mage demo:up` prints the port-forward command for the curator's control and
documentation ports. Chart packaging and the applier overlay are documented
in [helm/README.md](helm/README.md).
