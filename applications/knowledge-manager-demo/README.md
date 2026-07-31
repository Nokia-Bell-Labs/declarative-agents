<!-- Copyright (c) 2026 Nokia. All rights reserved. -->

# Knowledge Manager Demo

This standalone application drives the Knowledge Manager documentation agent
from [knowledge-manager.slide](knowledge-manager.slide). The deck starts the
canonical catalog-owned documentation-curator profile, then posts a
lifecycle-exit request to its control server.

The application owns this README, the deck, and the declarative lifecycle-exit
client. `applications/catalog` remains the only owner of the
documentation-curator profile and its UI, while `agent-core` owns the runtime
and builtin tools.

## Source-checkout setup

Run all commands below from `applications/knowledge-manager-demo`. Set the two
ownership roots explicitly when the catalog and runtime are independent
checkouts. These portable defaults select their monorepo locations:

    export AGENT_CATALOG_ROOT="${AGENT_CATALOG_ROOT:-$(cd ../catalog && pwd)}"
    export AGENT_CORE_ROOT="${AGENT_CORE_ROOT:-$(cd ../../agent-core && pwd)}"

`AGENT_CATALOG_ROOT` owns the canonical profile and UI.
`AGENT_CORE_ROOT` owns the development runtime and the core declarations that
the profile names under `/opt/agent-core`.

## Start the Knowledge Manager

Use an `agent` binary built from the selected `AGENT_CORE_ROOT`:

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
18191, monitor on port 18192, and the query surface on port 18193.

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
