<!-- Copyright (c) 2026 Nokia. All rights reserved. -->

# Agent Architecture

This standalone application drives the Knowledge Manager documentation agent
from [agent-architecture.slide](agent-architecture.slide). The deck starts the
canonical catalog-owned documentation-curator profile, then posts a
lifecycle-exit request to its control server.

The application owns this README, the deck, and the declarative lifecycle-exit
client. `applications/catalog` remains the only owner of the
documentation-curator profile and its UI, while `agent-core` owns the runtime
and builtin tools.

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

Run every command from `applications/agent-architecture`, except the one-time
port preflight in step 1, which runs from the repository root. Steps 2, 3,
and 5 each take their own terminal.

1. Free the demo ports. The chatbot-mesh persistent observability stack binds
   the ports the demo's trace collector needs (`127.0.0.1:4317` and
   `:18193`); if it is running, `mage run` fails
   with `bind OTLP receiver "ingress": address already in use`. Stop it from
   `applications/chatbot-mesh` with `mage observability:down`, or skip tracing
   with `DEMO_TRACING=false mage run`.

2. Serve the deck:

       mage presentation

   Open http://127.0.0.1:3999/agent-architecture.slide and follow it; the
   remaining steps mirror the slides.

3. In a second terminal, start the composition, meaning the trace collector
   and the curator together:

       mage run

   This builds the `agent` binary from the sibling `agent-core` checkout,
   starts the collector agent, and starts the canonical
   documentation-curator profile with OTLP export wired to the collector.
   A first run compiles agent-core and takes a minute; later runs are
   faster.

4. Browse the running surfaces:

   - documentation at http://localhost:18081;
   - lifecycle control at http://127.0.0.1:18082, with health at
     `/api/lifecycle/health`;
   - monitoring at http://localhost:18084/ui/; and
   - collected traces at http://127.0.0.1:18193/query/traces.

5. In a third terminal, stop the curator through its own API by running the
   lifecycle-exit agent. Set `AGENT_CORE_ROOT` as in Source-checkout setup
   and build the `agent` binary as in Start the Knowledge Manager, then:

       agent --profile "$(pwd)/call-lifecycle-exit/profile.yaml" --directory "$(pwd)" --core-root "$AGENT_CORE_ROOT"

   On success the exit agent reports `terminal state: succeeded`, the curator
   shuts down, and `mage run` stops the collector and returns cleanly. A trailing
   `metric shutdown error … MetricsService` line is expected: the collector
   serves only the OTLP trace service.

## Source-checkout setup

Run all commands below from `applications/agent-architecture`. Set the two
ownership roots explicitly when the catalog and runtime are independent
checkouts. These portable defaults select their monorepo locations:

    export AGENT_CATALOG_ROOT="${AGENT_CATALOG_ROOT:-$(cd ../catalog && pwd)}"
    export AGENT_CORE_ROOT="${AGENT_CORE_ROOT:-$(cd ../../agent-core && pwd)}"

`AGENT_CATALOG_ROOT` owns the canonical profile and UI.
`AGENT_CORE_ROOT` owns the development runtime and the core declarations that
the profile names under `/opt/agent-core`.

## Start the Knowledge Manager

Use an `agent` binary built from the selected `AGENT_CORE_ROOT`. `mage run`
builds its own copy into a private temporary directory, so the manual path
and the exit client need one on `PATH`:

    (cd "$AGENT_CORE_ROOT" && go build -tags production -o agent ./cmd/agent)
    export PATH="$AGENT_CORE_ROOT:$PATH"

Then start the canonical profile:

    agent \
      --profile "$AGENT_CATALOG_ROOT/agents/knowledge-manager/documentation-curator/profile.yaml" \
      --directory "$AGENT_CATALOG_ROOT" \
      --core-root "$AGENT_CORE_ROOT"

The catalog root is also the documentation workspace. The profile serves:

- documentation at `http://localhost:18081`;
- lifecycle control at `http://127.0.0.1:18082`; and
- monitoring at `http://localhost:18084/ui/`.

## Trace collection

By default `mage run` starts the canonical collector agent in spool mode
before the curator and exports the curator's OTLP traces to it. After the
curator exits, the collector is stopped via its control exit route.

While the demo is running, browse collected traces at:

    http://127.0.0.1:18193/query/traces

To disable trace collection:

    DEMO_TRACING=false mage run

The collector binds the OTLP receiver on `127.0.0.1:4317`, control on port
18191, monitor on port 18192, and the query surface on port 18193. The
chatbot-mesh persistent observability stack (`mage observability:up` in
`applications/chatbot-mesh`) binds 4317 and 18193 too; stop it with
`mage observability:down` from that directory before a traced demo run.

## The lifecycle-exit agent

The exit request is a declarative agent under
[call-lifecycle-exit/](call-lifecycle-exit/), not a bespoke HTTP client. Its
machine has one boundary word, `post_exit`, that binds the rest tool to POST the
fixed `{"reason": "demo presentation"}` body to
`/api/lifecycle/exit`; the machine reaches terminal state `Done` when the server
returns HTTP 202 Accepted. The control-server URL is a declared REST client base
(`CURATOR_URL`, default `http://127.0.0.1:18082`), not runtime input, and the
endpoint carries no transport authority (`auth: none`). Run it with:

    agent \
      --profile "$(pwd)/call-lifecycle-exit/profile.yaml" \
      --directory "$(pwd)" \
      --core-root "$AGENT_CORE_ROOT"

Expressing the exit call as a machine rather than a Go binary makes the demo an
instance of the system's own thesis: runtime behavior lives in YAML and is run
by the interpreter. It replaces the former `call_lifecycle_exit/main.go`.

## Installed runtime

An installed runtime already supplies core-owned declarations at
`/opt/agent-core`, so it does not use `AGENT_CORE_ROOT` or `--core-root`. It
still needs an explicit catalog root for the canonical profile:

    agent \
      --profile "$AGENT_CATALOG_ROOT/agents/knowledge-manager/documentation-curator/profile.yaml" \
      --directory "$AGENT_CATALOG_ROOT"

Run the application-owned exit client from this application root:

    agent --profile "$(pwd)/call-lifecycle-exit/profile.yaml" --directory "$(pwd)"

## Kubernetes demo (optional)

This composition also deploys into a persistent kind cluster:

    mage doctor    # preflight: toolchain versions and Docker resources
    mage demo:up   # create or reuse the cluster, build images, install the chart
    mage demo:down # delete only this demo's cluster

`mage demo:up` prints the port-forward command for the curator's control and
documentation ports. Chart packaging and the applier overlay are documented
in [helm/README.md](helm/README.md).
