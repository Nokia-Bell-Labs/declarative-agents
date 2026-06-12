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
runtime image with Go, git, `golangci-lint`, common Unix utilities, and shared
YAML assets under `/opt/agent-core`.

```bash
DOCKER_BUILDKIT=1 docker build \
  --build-arg AGENT_CORE_REF=main \
  -t agent-core:latest .
```

For private HTTPS GitLab access, pass a `.netrc` as a BuildKit secret:

```bash
DOCKER_BUILDKIT=1 docker build \
  --secret id=git_credentials,src="$HOME/.netrc" \
  -t agent-core:latest .
```

The same image can be built with Podman. In environments where the registry
certificate chain is managed outside Podman's trust store, pass
`--tls-verify=false`:

```bash
podman build \
  --tls-verify=false \
  --build-arg AGENT_CORE_REF=main \
  -t agent-core:latest .
```

For private HTTPS GitLab access with Podman, pass a `.netrc` as a build
secret:

```bash
podman build \
  --tls-verify=false \
  --secret id=git_credentials,src="$HOME/.netrc" \
  -t agent-core:latest .
```

The `.netrc` should contain credentials for the GitLab host:

```text
machine gitlabe1.ext.net.nokia.com
  login <username>
  password <token-or-password>
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
If mounted output permissions matter, add `--user "$(id -u):$(id -g)"`;
the image keeps Go caches under `/tmp` so arbitrary user IDs can still run Go
validation commands.

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
