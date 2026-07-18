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
library. Agent behavior lives in profile YAML plus shared tool declarations;
changing behavior should usually mean changing YAML rather than adding
mode-specific Go code.

## Packages

Core runtime code lives in `internal/runtime/core`. It owns the state machine,
command dispatch, tool registry, agentic loop, and YAML machine config.

Model code lives in `internal/model/llm` and `internal/model/llm/ollama`. Those
packages provide the LLM client interface, conversation types, model profiles,
and the Ollama adapter.

Prompt and tool vocabulary code lives in `internal/model/prompt` and focused
`internal/tools/*` packages. Prompt code loads YAML templates and serializes
tool lists. The tool packages provide file tools, build tools, LLM commands,
subprocess tools, process groups, lifecycle adapters, REST tools, and registry
support.

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

Profiles are normal runtime entry points, but standard agent programs now live
outside this repository. Set `AGENT_PROFILES_ROOT` to an `agent-profiles`
checkout or bundle, then pass explicit paths such as
`$AGENT_PROFILES_ROOT/agents/generator/profile.yaml`,
`$AGENT_PROFILES_ROOT/agents/evaluator/profile.yaml`, or
`$AGENT_PROFILES_ROOT/agents/jurist/profile.yaml`.

Lifecycle operators use the same external profile path shape.
`$AGENT_PROFILES_ROOT/agents/lifecycle/history/profile.yaml` inspects
checkpoint history through `checkpoint_history`.
`$AGENT_PROFILES_ROOT/agents/lifecycle/rollback/profile.yaml` rolls back a
checkpoint through `checkpoint_rollback`. The removed `agent history` and
`agent rollback` aliases are not part of the runtime surface.

Profiles resolve relative paths from their own directory. Current profiles load
shared tool declarations from directories such as `tools/builtin/` and
`tools/exec/`, then add agent-local declarations such as LLM configs or builtin
config overrides. `--profile` is the normal agent configuration flag; machine,
tool selection, declaration, and tool-config paths belong in the profile.

Runtime data stays outside the profile. Pass `--directory` for the workspace,
`--request` for per-run request files, and `--output` for artifacts. These
flags do not identify the agent program.

## Profile UX Integrations

Agent Core owns the generic REST server, `machine_request`, document resource,
static asset, and lifecycle-control runtime behavior. Concrete profile UX
tracer bullets, including the Knowledge Manager documentation-curator profile
and browser workflow, belong to `agent-profiles` with the profile assets they
exercise.

Core package tests should prove reusable runtime contracts without depending on
a shipped profile path. Profile-owned integration suites can still run this
binary with `--profile` and an external profile checkout when they need
end-to-end evidence for a specific agent program.

## Lifecycle Operations

Lifecycle features are opt-in: checkpointing, suspend/resume, approval gates,
history, and rollback. See `lifecycle-rollback.md` for profile examples,
`--dolt-dsn`, `--resume-checkpoint`, request files, receipt-driven rollback,
and Dolt-backed persistence behavior.

For history and rollback, use the universal runtime flags:

```bash
bin/agent --profile "$AGENT_PROFILES_ROOT/agents/lifecycle/history/profile.yaml" \
  --directory "$WORKSPACE" \
  --request requests/history.yaml

bin/agent --profile "$AGENT_PROFILES_ROOT/agents/lifecycle/rollback/profile.yaml" \
  --directory "$WORKSPACE" \
  --request requests/rollback.yaml
```

Lifecycle request files carry values such as `checkpoint: latest` or
`to_iteration: 3`. No lifecycle-only subcommands or checkpoint flags are
exposed by the binary.

Without `--dolt-dsn`, lifecycle persistence uses `NoopCheckpoint` and records no
durable history. Set `--dolt-dsn` to a MySQL-wire DSN for a running `dolt
sql-server` when a run must persist checkpoints for history, resume, or
rollback.

### Local Dolt Server (persistent)

`docker-compose.dolt.yml` runs a local `dolt sql-server` whose storage persists
across container removal, for lifecycle checkpoints and the gated Dolt
integration tests. The `dolt-data` named volume holds the chunk store, so the
container is disposable while the data is not.

```bash
mage dolt:up       # start the server (docker compose up -d)
mage dolt:status   # show the service and the persistent volume
mage dolt:down     # stop and remove the container, keeping the data
mage dolt:reset    # stop and delete the volume, discarding all data
```

The server listens on `127.0.0.1:3306` with a `root` account reachable over TCP
(`DOLT_ROOT_HOST=%`). Point a run at it with
`--dolt-dsn "root@tcp(127.0.0.1:3306)/<database>"`. With the server up, the
gated tests in `cmd/agent/dolt_integration_test.go` run instead of skipping.

## Quick Start

```bash
mage build
AGENT_PROFILES_ROOT=../agent-profiles \
  bin/agent --profile "$AGENT_PROFILES_ROOT/agents/generator/profile.yaml" --directory "$PWD"
```

## Docker Runtime

Repository builds use a multi-stage Dockerfile for the release runtime image.
During the builder stage, the image clones Agent Core from GitLab, runs
`go test ./...`, and builds `agent`. The final Alpine runtime image contains
only the `agent` binary, git, common Unix utilities, and core-owned shared tool
assets under `/opt/agent-core/tools`.

Runtime images intentionally exclude the Go toolchain, source checkout,
test dependencies, `golangci-lint`, and agent profile trees. Exec tools such as
`build`, `vet`, `lint`, and `test` require those binaries to come from a mounted
workspace, a derived image, or another container/host provisioning step. Agent
profiles come from a mounted `agent-profiles` checkout or unpacked profile
bundle.

Build through the Mage target:

```bash
mage docker
```

`mage docker` discovers the latest remote root release tag with the
`v0.YYYYMMDD.N` shape, passes it to the Dockerfile as `AGENT_CORE_REF`, and
builds `agent-core:latest`. Repository releases may also publish a matching
module-scoped tag such as `agent-core/v0.YYYYMMDD.N`, but Docker release
resolution continues to use the root tag family unless `AGENT_CORE_REF`
overrides it. The target requires Docker, and prints the resolved build settings
plus the exact Docker command before building.

Common overrides:

```bash
AGENT_CORE_REF=v0.20260612.N mage docker
AGENT_CORE_IMAGE=registry.example/agent-core:v0.20260612.N mage docker
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

The equivalent lower-level Docker command is:

```bash
DOCKER_BUILDKIT=1 docker build \
  --progress=plain \
  --secret id=git_credentials,src=.netrc \
  --build-arg AGENT_CORE_REF=v0.20260612.N \
  -t agent-core:latest .
```

Run the runtime image with profiles and workspaces mounted separately:

```bash
docker run --rm \
  -v "$AGENT_PROFILES_ROOT:/profiles/agents:ro" \
  -v "$PWD:/work" \
  -w /work \
  agent-core:latest \
  --profile /profiles/agents/generator/profile.yaml \
  --directory /work
```

Evaluator flows use the same profile mount and keep suites/output under the
workspace mount:

```bash
docker run --rm \
  -v "$AGENT_PROFILES_ROOT:/profiles/agents:ro" \
  -v "$PWD:/work" \
  -w /work \
  agent-core:latest \
  --profile /profiles/agents/evaluator/profile.yaml \
  --request suites/suite.yaml \
  --output eval-results \
  --directory /work
```

The image has no fallback profile tree. Running it without `--profile` or with
an absent mounted profile path fails at startup with a profile path error.

Profiles inside the mounted repository can reference shared image assets with
absolute paths such as `/opt/agent-core/tools/builtin` and
`/opt/agent-core/tools/exec`.
If mounted output permissions matter, add `--user "$(id -u):$(id -g)"`.

For integration tests inside a container, build the source-bearing integration
target and mount profile assets from outside the image:

```bash
docker build \
  --target integration \
  --secret id=git_credentials,src=.netrc \
  --build-arg AGENT_CORE_REF=v0.20260612.N \
  -t agent-core-integration:latest .

docker run --rm \
  -v "$AGENT_PROFILES_ROOT:/profiles/agents:ro" \
  -w /src \
  -e AGENT_PROFILES_ROOT=/profiles/agents \
  agent-core-integration:latest \
  mage integration:uc001
```

Recent verification: `mage docker` built `agent-core:latest` from a remote
release, `docker run --rm agent-core:latest --help` started the packaged
`agent` binary, and `docker run --rm agent-core:latest` reported that
`--profile` is required.

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
