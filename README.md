# Declarative Agents

Profile-driven runtime and design patterns for declarative, tool-augmented agents.

## Modules

| Directory | Description |
|-----------|-------------|
| [`agent-core/`](agent-core/) | Runtime engine — state machines, tool dispatch, LLM integration, profile loading, and a standard tool library. Go. |
| [`agent-profiles/`](agent-profiles/) | External agent programs and profile YAML assets consumed by agent-core. |
| [`examples/chatbot-mesh/`](examples/chatbot-mesh/) | Standalone, copyable example: the browser-facing chatbot mesh — a chatbot agent that fans one query embedding out to N Chroma-backed RAG servers and routes to a chat LLM, plus a control plane (a coordinator and creator that turn a provisioning intent into an ingest and a rollout through a deployment API). Ships its own docs/specs, agents, ux SPA, and Helm chart, runs on the agent-core image, and self-governs its corpus with `mage audit`. |
| [`design-patterns/`](design-patterns/) | White paper source: *Design Patterns for Declarative Agents* — eleven patterns for building reliable agents (markdown, PlantUML, IEEE build). |
| [`docs/engineering/`](docs/engineering/) | Engineering guidelines that span modules and examples, starting with the standard kind rig for integration tests and demos. |
| [`magefiles/`](magefiles/) | Repository-wide build targets: release tagging, stats aggregation, sub-module dispatch. |

## Build

This repository uses [Mage](https://magefile.org/) for builds. From the repo root:

```bash
mage            # run default target in each sub-module
mage build      # build artifacts in each sub-module
mage audit      # run the release analysis gate in each sub-module
mage test       # run tests for applicable sub-modules
mage stats      # combined LOC and per-agent stats (states, transitions, tools, YAML) as JSON
mage clean      # remove generated artifacts in each sub-module
mage tag        # create root and module release tags
```

Each sub-module also has its own mage targets. Run `mage -l` inside any directory with a `magefiles/` folder to list available targets.

### Persistent integration observability

Start the shared OTLP ingress and retained Jaeger and Prometheus backends before
telemetry-required integrations. `down` keeps backend volumes; only `reset`
deletes them.

```bash
mage observability:up
mage observability:status
mage observability:down
mage observability:reset
```

Defaults expose OTLP gRPC on `4317`, OTLP HTTP on `4318`, Collector health on
`13133`, Jaeger query on `16686`, and Prometheus query on `9090`. Override them
with `DA_OTEL_GRPC_PORT`, `DA_OTEL_HTTP_PORT`, `DA_OTEL_HEALTH_PORT`,
`DA_JAEGER_QUERY_PORT`, and `DA_PROMETHEUS_QUERY_PORT`. Integration targets may
reuse a healthy stack but do not stop or reset it.

Root releases require every release gate to exit successfully before tagging:
`mage audit`, `mage test`, both `agent-core` and `agent-profiles`
`mage integration:all`, and `agent-profiles` `mage conformance` with
`AGENT_CORE_ROOT` set to the release checkout. A documented skip reported by a
gate is accepted only when that gate exits successfully. A failed gate cannot
be waived; fix the failure and run the gates again before creating a tag.

`mage tag` requires a clean `main` worktree, records the exact HEAD commit, runs
all gates above itself, and verifies HEAD is unchanged before creating tags. It
creates repository tag `v0.YYYYMMDD.N` plus module-scoped tags for
release-relevant directories: `agent-core/v0.YYYYMMDD.N`,
`agent-profiles/v0.YYYYMMDD.N`, and `design-patterns/v0.YYYYMMDD.N`. Git creates
the complete repository/module ref set in one reference transaction, so a
conflict or write failure creates none of the release tags.

### agent-core

```bash
cd agent-core
mage build    # compile cmd/ binaries into bin/
mage lint     # run golangci-lint
mage stats    # LOC and YAML breakdowns (JSON)
```

### design-patterns

```bash
cd design-patterns
mage figures  # render PlantUML diagrams to PNG
mage pdf      # compile IEEE two-column PDF
mage clean    # remove generated artifacts
```

### examples/chatbot-mesh

```bash
cd examples/chatbot-mesh
mage audit    # run the jurist over this example's spec corpus (self-governing)
```

The example is copyable with two documented platform dependencies: agent-core
at runtime and the canonical agent-profiles corpus-ingest program at build and
local-integration time. Set `AGENT_PROFILES_ROOT` when the profile library is
not in the monorepo checkout. Its Helm chart is under
[`examples/chatbot-mesh/helm/`](examples/chatbot-mesh/helm/) and its own docs
live under [`examples/chatbot-mesh/docs/`](examples/chatbot-mesh/docs/).

## License

BSD 3-Clause — Copyright (c) 2026, Nokia Bell Labs. See [LICENSE](LICENSE).
