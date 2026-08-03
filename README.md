# Declarative Agents

Profile-driven runtime and design patterns for declarative, tool-augmented agents.

## Modules

| Directory | Description |
|-----------|-------------|
| [`agent-core/`](agent-core/) | Runtime engine — state machines, tool dispatch, LLM integration, profile loading, and a standard tool library. Go. |
| [`applications/catalog/`](applications/catalog/) | Repository-specific reusable declarative tool and agent blocks, catalog conformance, and catalog integration evidence. |
| [`applications/chatbot-mesh/`](applications/chatbot-mesh/) | Copyable browser-facing chatbot application with routed multi-RAG data plane, provisioning control plane, UX, Helm chart, self-governing specs, and an explicit canonical corpus-ingest build dependency. |
| [`applications/coding-agent/`](applications/coding-agent/) | Deployable planner → executor → critic application composed from canonical catalog blocks, with deterministic profile packaging, a profile-free runtime image, Helm chart, and local/kind integration gates. |
| [`applications/agent-architecture/`](applications/agent-architecture/) | Standalone presentation composition that runs the canonical catalog documentation-curator and serves the Knowledge Manager slide deck. |
| [`design-patterns/`](design-patterns/) | White paper source: *Design Patterns for Declarative Agents* — eleven patterns for building reliable agents (markdown, PlantUML, IEEE build). |
| [`docs/engineering/`](docs/engineering/) | Engineering guidelines that span modules and applications, starting with the standard kind rig for integration tests and demos. |
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

`mage test` rebuilds every tracked shipped UI from a clean lockfile install. It
audits the full build dependency graph and the production-only graph separately;
either scope fails the release gate at any known high or critical vulnerability.

### Persistent integration observability

The persistent OTLP ingress is the canonical collector agent run as a host
process: it receives both trace and metric exports on one gRPC listener and
retains them in its spool, so no docker-compose stack or Prometheus backend is
involved and kind stays the only Docker consumer. Run
`mage observability:up|status|down|reset` from `applications/chatbot-mesh`,
which owns the ingress; its ports and lifecycle are documented in the
[chatbot-mesh README](applications/chatbot-mesh/README.md).

Root releases require every release gate to exit successfully before tagging:
`mage audit`, `mage test`, `agent-core` and `applications/catalog`
`mage integration:all`, catalog `mage conformance` using repository discovery,
and application-owned gates from each application root.
A documented skip reported by a gate is accepted only when that gate exits
successfully. A failed gate cannot be waived; fix the failure and run the gates
again before creating a tag.
The agent-core integration suite is limited to runtime service boundaries:
embedded monitor wiring, Ollama REST and metrics, and Dolt persistence.
Application workflows such as planner-executor-critic run from
`applications/coding-agent`.

`mage tag` requires a clean `main` worktree, records the exact HEAD commit, runs
all gates above itself, and verifies HEAD is unchanged before creating tags.
Revision selection queries the configured remote before choosing N, so a
checkout missing fetched tags still picks the next available revision. It
creates repository tag `v0.YYYYMMDD.N` plus module-scoped tags for
release-relevant directories: `agent-core/v0.YYYYMMDD.N`,
`applications/catalog/v0.YYYYMMDD.N`, its matching legacy compatibility tag
`agent-profiles/v0.YYYYMMDD.N`, `design-patterns/v0.YYYYMMDD.N`, and each
runnable application module, including
`applications/agent-architecture/v0.YYYYMMDD.N`. Git creates the complete
repository/module ref set in one reference transaction, so a conflict or write
failure creates none of the release tags.

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

### applications/chatbot-mesh

```bash
cd applications/chatbot-mesh
mage audit                  # validate the application's spec corpus
mage observability:up       # start the persistent collector ingress its gates need
mage helm:package           # build the installable chart
mage integration:helmSmoke  # prove the packaged mesh on kind
```

The application is copyable with two documented platform dependencies:
agent-core at runtime and the canonical catalog corpus-ingest block at build and
local-integration time. Set `AGENT_CATALOG_ROOT` when the catalog is not in the
monorepo checkout. Its Helm chart is under
[`applications/chatbot-mesh/helm/`](applications/chatbot-mesh/helm/) and its own docs
live under [`applications/chatbot-mesh/docs/`](applications/chatbot-mesh/docs/).

### applications/coding-agent

```bash
cd applications/coding-agent
mage audit                  # validate docs, closure, boot, and test evidence
mage package                # assemble canonical application profile closures
mage image:build            # build the profile-free coding runtime
mage helm:package           # build the installable chart
mage integration:helmSmoke  # prove planner → executor → critic on kind
```

Canonical entry points are
[`agents/application.yaml`](applications/coding-agent/agents/application.yaml),
[`Dockerfile`](applications/coding-agent/Dockerfile), and
[`helm/`](applications/coding-agent/helm/); architecture and operations live under
[`docs/`](applications/coding-agent/docs/).

### applications/agent-architecture

```bash
cd applications/agent-architecture
mage run      # start the canonical catalog documentation-curator
mage present  # serve agent-architecture.slide with the pinned Go present tool
```

The application is a composition-only consumer of
[`applications/catalog/`](applications/catalog/): it owns the presentation and
lifecycle-exit flow but does not copy or recount the documentation-curator.
Setup, ports, and the declarative exit command are documented in the
[application README](applications/agent-architecture/README.md).

## License

BSD 3-Clause — Copyright (c) 2026, Nokia Bell Labs. See [LICENSE](LICENSE).
