# Declarative Agents

Profile-driven runtime and design patterns for declarative, tool-augmented agents.

## Modules

| Directory | Description |
|-----------|-------------|
| [`agent-core/`](agent-core/) | Runtime engine — state machines, tool dispatch, LLM integration, profile loading, and a standard tool library. Go. |
| [`agent-profiles/`](agent-profiles/) | External agent programs and profile YAML assets consumed by agent-core. |
| [`design-patterns/`](design-patterns/) | White paper source: *Design Patterns for Declarative Agents* — eleven patterns for building reliable agents (markdown, PlantUML, IEEE build). |
| [`magefiles/`](magefiles/) | Repository-wide build targets: release tagging, stats aggregation, sub-module dispatch. |

## Build

This repository uses [Mage](https://magefile.org/) for builds. From the repo root:

```bash
mage          # run default target in each sub-module
mage tag      # create a release tag (v0.YYYYMMDD.N)
mage stats    # output combined LOC stats as JSON
```

Each sub-module also has its own mage targets. Run `mage -l` inside any directory with a `magefiles/` folder to list available targets.

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

## License

BSD 3-Clause — Copyright (c) 2026, Nokia Bell Labs. See [LICENSE](LICENSE).
