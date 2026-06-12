# agent-core

A Go framework for building tool-augmented agentic loops.

## What It Provides

agent-core gives you the machinery every agentic system needs: a state machine,
command dispatch with tracing and panic recovery, a tool registry, budget
enforcement, LLM integration, prompt assembly, and a standard tool library.
Domain agents import agent-core and supply their own states, signals, tools, and
transition tables.

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

The repository includes a multi-stage Dockerfile that clones Agent Core from
GitLab during the build, runs `go test ./...`, builds `agent`, and packages a
runtime image with the `agent` binary, git, common Unix utilities, and shared
YAML assets under `/opt/agent-core`. The Go toolchain, source checkout, and
test dependencies stay in the builder stage and are not copied into the runtime
image.

The preferred build path is the Mage target:

```bash
mage docker
```

`mage docker` discovers the latest remote release tag from GitLab, passes it to
the Dockerfile as `AGENT_CORE_REF`, and builds `agent-core:latest`. The target
uses Podman when available, falls back to Docker, and prints the image, release
ref, and engine before building.

Common overrides:

```bash
AGENT_CORE_REF=v0.20260612.1 mage docker
AGENT_CORE_IMAGE=registry.example/agent-core:v0.20260612.1 mage docker
AGENT_CORE_CONTAINER_ENGINE=docker mage docker
AGENT_CORE_REPO=https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core.git mage docker
```

For private HTTPS GitLab access, the target uses `$HOME/.netrc` automatically
when present. Override the path with `AGENT_CORE_NETRC`:

```bash
AGENT_CORE_NETRC=/path/to/netrc mage docker
```

The `.netrc` should contain credentials for the GitLab host:

```text
machine gitlabe1.ext.net.nokia.com
  login <username>
  password <token-or-password>
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
  --secret id=git_credentials,src="$HOME/.netrc" \
  --build-arg AGENT_CORE_REF=v0.20260612.1 \
  -t agent-core:latest .
```

The equivalent lower-level Docker command is:

```bash
DOCKER_BUILDKIT=1 docker build \
  --secret id=git_credentials,src="$HOME/.netrc" \
  --build-arg AGENT_CORE_REF=v0.20260612.1 \
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

Current verification baseline: `mage docker` built `agent-core:latest` from
remote release `v0.20260612.1`, and `podman run --rm agent-core:latest --help`
started the packaged `agent` binary successfully.

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
