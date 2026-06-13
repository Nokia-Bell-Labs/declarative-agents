# agent-core

A profile-driven runtime for declarative, tool-augmented agents.

## What It Provides

Agent Core packages a single `agent` binary that runs different agents from
YAML configuration. A profile selects the state machine, tool selections, tool
declaration directories, agent-local declarations, and optional workspace
directory. The same runtime drives the generator, evaluator, planner, bench,
and jurist agents.

Shared runtime machinery includes state-machine execution, command dispatch
with tracing and panic recovery, tool registration, budget enforcement, LLM
integration, prompt assembly, lifecycle checkpointing, and a standard tool
library. Agent behavior lives in `agents/`, `tools/`, and `docs/specs/`;
changing behavior should usually mean changing YAML rather than adding
mode-specific Go code.

## Packages

Core runtime code lives in `internal/runtime/core`. It owns the state machine,
command dispatch, tool registry, agentic loop, and YAML machine config.

Model code lives in `internal/model/llm` and `internal/model/llm/ollama`. Those
packages provide the LLM client interface, conversation types, model profiles,
and the Ollama adapter.

Prompt and tool vocabulary code lives in `internal/model/prompt` and
`internal/tools/stl`. Prompt code loads YAML templates and serializes tool
lists. The STL package provides file tools, build tools, LLM commands,
subprocess tools, process groups, and lifecycle adapters.

Evaluation, planning, and observability code lives in `internal/evaluation`,
`internal/evaluation/bench`, `internal/planning`, and `internal/observability`.
Those packages support evaluator runs, the bench UI, planner workflows, tracing
ports, OpenTelemetry adapters, GenAI spans, and replay.

Support code lives in `internal/support`. Specification graph loading and
cross-artifact validation live in `pkg/spec`.

Private implementation packages are grouped under `internal/`. See
`package-layout.md` for the migration map and ownership rules. Current internal
domains include `internal/observability` for tracing and telemetry, and
`internal/support` for process, workspace, and CLI helper code.

## Agent Profiles

Profiles are the normal runtime entry points. Use
`agents/generator/profile.yaml` for the coding generator loop,
`agents/evaluator/profile.yaml` for evaluator suites,
`agents/planner/profile.yaml` for planning and task execution,
`agents/bench/profile.yaml` for the bench web UI, and
`agents/jurist/profile.yaml` for spec validation.

Lifecycle operators use the same profile path. Use
`agents/lifecycle/history/profile.yaml` to inspect checkpoint history through
`checkpoint_history`. Use `agents/lifecycle/rollback/profile.yaml` to roll back
a checkpoint through `checkpoint_rollback`. The removed `agent history` and
`agent rollback` aliases are not part of the runtime surface.

Profiles resolve relative paths from their own directory. Current profiles load
shared tool declarations from directories such as `tools/builtin/` and
`tools/exec/`, then add agent-local declarations such as LLM configs or builtin
config overrides. Legacy `--machine`, `--tools`, and `--tools-declaration`
startup remains compatibility behavior; prefer `--profile` for new usage.

## Lifecycle Operations

Checkpointing, suspend/resume, approval gates, history, and rollback are
opt-in lifecycle features. See `lifecycle-rollback.md` for the operator guide,
including lifecycle profile examples, `--state-store-dir`,
`--resume-checkpoint`, request input files, the three-layer state model, and
safety rules for irreversible tools and workspace restore.

History and rollback use the universal runtime flags:

```bash
bin/agent --profile agents/lifecycle/history/profile.yaml \
  --state-store-dir "$STATE_DIR" \
  --input requests/history.yaml

bin/agent --profile agents/lifecycle/rollback/profile.yaml \
  --state-store-dir "$STATE_DIR" \
  --directory "$WORKSPACE" \
  --input requests/rollback.yaml
```

The request file carries lifecycle tool input such as `checkpoint: latest` or
`to_iteration: 3`. The binary does not expose lifecycle-only subcommands or
checkpoint flags.

## Quick Start

```bash
mage build
bin/agent --profile agents/generator/profile.yaml --directory "$PWD"
```

## Docker Runtime

Repository builds use a multi-stage Dockerfile for the release runtime image.
During the builder stage, the image clones Agent Core from GitLab, runs
`go test ./...`, and builds `agent`. The final Alpine runtime image contains
only the `agent` binary, git, common Unix utilities, and shared YAML assets under
`/opt/agent-core`.

Runtime images intentionally exclude the Go toolchain, source checkout,
test dependencies, and `golangci-lint`. Exec tools such as `build`, `vet`,
`lint`, and `test` require those binaries to come from a mounted workspace, a
derived image, or another container/host provisioning step.

Build through the Mage target:

```bash
mage docker
```

`mage docker` discovers the latest remote release tag from GitLab, passes it to
the Dockerfile as `AGENT_CORE_REF`, and builds `agent-core:latest`. It uses
Docker when available, falls back to Podman, and prints the resolved build
settings plus the exact Docker/Podman command before building.

Common overrides:

```bash
AGENT_CORE_REF=v0.20260612.N mage docker
AGENT_CORE_IMAGE=registry.example/agent-core:v0.20260612.N mage docker
AGENT_CORE_CONTAINER_ENGINE=docker mage docker
AGENT_CORE_REPO=https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core.git mage docker
```

For private HTTPS GitLab access, put a build-only `.netrc` in the repository
root. It is ignored by git and passed only to the container build:

```bash
mage docker
```

The repository-local `.netrc` should contain credentials for the GitLab host:

```text
machine gitlabe1.ext.net.nokia.com
  login <username>
  password <token-or-password>
```

Set restrictive permissions on the build-only file:

```bash
chmod 600 .netrc
```

Override the path if needed:

```bash
AGENT_CORE_NETRC=/path/to/netrc mage docker
```

Podman builds default to `--tls-verify=false` for environments where the
registry certificate chain is managed outside Podman's trust store. Override
that behavior with:

```bash
AGENT_CORE_TLS_VERIFY=true mage docker
```

The equivalent lower-level Podman command is:

```bash
podman build \
  --tls-verify=false \
  --secret id=git_credentials,src=.netrc \
  --build-arg AGENT_CORE_REF=v0.20260612.N \
  -t agent-core:latest .
```

The equivalent lower-level Docker command is:

```bash
DOCKER_BUILDKIT=1 docker build \
  --progress=plain \
  --secret id=git_credentials,src=.netrc \
  --build-arg AGENT_CORE_REF=v0.20260612.N \
  -t agent-core:latest .
```

An external evaluation repository can mount its local suites, samples, and
evaluator config into `/work` while reusing shared runtime files from the
image:

```bash
podman run --rm \
  -v "$PWD:/work" \
  -w /work \
  agent-core:latest \
  --profile agents/evaluator/profile.yaml \
  --input suites/suite.yaml \
  --output eval-results \
  --directory /work
```

Profiles inside the mounted repository can reference shared image assets with
absolute paths such as `/opt/agent-core/tools/builtin`,
`/opt/agent-core/tools/exec`, and
`/opt/agent-core/agents/generator/profile-qwen27b.yaml`.
If mounted output permissions matter, add `--user "$(id -u):$(id -g)"`.

Recent verification: `mage docker` built `agent-core:latest` from a remote
release, and `podman run --rm agent-core:latest --help` started the packaged
`agent` binary successfully.

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
