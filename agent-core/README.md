# agent-core

A profile-driven runtime for declarative, tool-augmented agents.

## What It Provides

Agent Core packages a single `agent` binary that runs different agents from
YAML configuration. A profile selects the state machine, tool selections, tool
declaration directories, agent-local declarations, and optional workspace
directory. The same runtime drives the generator, evaluator, planner, bench,
and constitution-auditor agents.

The runtime provides the shared machinery those agents need: state-machine
execution, command dispatch with tracing and panic recovery, tool registration,
budget enforcement, LLM integration, prompt assembly, lifecycle checkpointing,
and a standard tool library. Agent behavior lives in `agents/`, `tools/`, and
`docs/specs/`; changing behavior should usually mean changing YAML rather than
adding mode-specific Go code.

## Packages

| Package | Description |
|---------|-------------|
| `internal/runtime/core` | State machine, command dispatch, tool registry, agentic loop, YAML machine config |
| `internal/model/llm` | LLM client interface, conversation management, message types, model profiles |
| `internal/model/llm/ollama` | Ollama adapter satisfying `llm.Client` |
| `internal/model/prompt` | Prompt loading from YAML, system template rendering, manifest serialization |
| `internal/tools/stl` | Standard tool library: file tools, build tools, LLM commands, subprocess, process groups |
| `internal/evaluation` | Evaluator session/point runtime, result artifacts, metrics, convergence, trace analysis |
| `internal/evaluation/bench` | Bench server, UI support, and evaluation launch orchestration |
| `internal/planning` | Planner extraction, graph, materialization, plan parsing, and pipeline orchestration |
| `internal/observability` | Tracing ports, OpenTelemetry adapters, GenAI span helpers, and replay support |
| `internal/support` | CLI, process execution, subprocess, and worktree helper packages |
| `pkg/spec` | Specification graph loader and cross-artifact validator |

Private implementation packages are grouped under `internal/`. See
`package-layout.md` for the migration map and ownership rules. Current internal
domains include `internal/observability` for tracing and telemetry, and
`internal/support` for process, workspace, and CLI helper code.

## Agent Profiles

Profiles are the normal runtime entry points:

| Profile | Purpose |
|---------|---------|
| `agents/generator/profile.yaml` | Run the coding generator loop. |
| `agents/evaluator/profile.yaml` | Run evaluator suites over generator profiles. |
| `agents/planner/profile.yaml` | Run planning and task execution workflows. |
| `agents/bench/profile.yaml` | Serve the bench web UI and launch evaluations. |
| `agents/constitution-auditor/profile.yaml` | Validate the spec corpus. |

Profiles resolve relative paths from their own directory. Current profiles load
shared tool declarations from directories such as `tools/builtin/` and
`tools/exec/`, then add agent-local declarations such as LLM configs or builtin
config overrides. Legacy `--machine`, `--tools`, and `--tools-declaration`
startup remains compatibility behavior; prefer `--profile` for new usage.

## Lifecycle Operations

Checkpointing, suspend/resume, approval gates, history, and rollback are
opt-in lifecycle features. See `lifecycle-rollback.md` for the operator guide,
including `--state-store-dir`, `--resume-checkpoint`, `agent history`,
`agent rollback`, the three-layer state model, and safety rules for irreversible
tools and workspace restore.

## Quick Start

```bash
mage build
bin/agent --profile agents/generator/profile.yaml --directory "$PWD"
```

## Docker Runtime

The repository includes a multi-stage Dockerfile for building a release runtime
image. The builder stage clones Agent Core from GitLab, runs `go test ./...`,
and builds `agent`. The final Alpine runtime image contains only the `agent`
binary, git, common Unix utilities, and shared YAML assets under
`/opt/agent-core`.

The runtime image intentionally excludes the Go toolchain, source checkout,
test dependencies, and `golangci-lint`. Exec tools such as `build`, `vet`,
`lint`, and `test` require those binaries to come from a mounted workspace, a
derived image, or another container/host provisioning step.

The preferred build path is the Mage target:

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
