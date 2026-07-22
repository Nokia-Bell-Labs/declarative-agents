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
mage stats      # output combined LOC stats as JSON
mage clean      # remove generated artifacts in each sub-module
mage tag        # create root and module release tags
```

Each sub-module also has its own mage targets. Run `mage -l` inside any directory with a `magefiles/` folder to list available targets.

Root releases use `mage audit` as the analysis gate and `mage test` as the
unit-test gate. `mage tag` runs from `main` and creates the repository tag
`v0.YYYYMMDD.N` plus module-scoped tags for release-relevant directories:
`agent-core/v0.YYYYMMDD.N`, `agent-profiles/v0.YYYYMMDD.N`, and
`design-patterns/v0.YYYYMMDD.N`.

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

The example is self-contained — copy the directory to run the mesh independently.
Its Helm chart is under [`examples/chatbot-mesh/helm/`](examples/chatbot-mesh/helm/)
and its own docs live under [`examples/chatbot-mesh/docs/`](examples/chatbot-mesh/docs/).

## License

BSD 3-Clause — Copyright (c) 2026, Nokia Bell Labs. See [LICENSE](LICENSE).
