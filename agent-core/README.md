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
`internal/planning`, and `internal/observability`. Those packages support
evaluator runs and read-only artifact queries, planner workflows, tracing ports,
OpenTelemetry adapters, GenAI spans, and replay. The bench application and UI
live in the external `applications/catalog/agents/bench` profile.

Support code lives in `internal/support`. Specification graph loading and
cross-artifact validation live in `pkg/spec`.

Private implementation packages are grouped under `internal/`. See
`package-layout.md` for the migration map and ownership rules. Current internal
domains include `internal/observability` for tracing and telemetry, and
`internal/support` for process, workspace, and CLI helper code.

## Agent Profiles

Profiles are normal runtime entry points, but standard agent programs now live
outside this repository. Set `AGENT_CATALOG_ROOT` to an `applications/catalog`
checkout or bundle, then pass explicit paths such as
`$AGENT_CATALOG_ROOT/agents/executor/profile.yaml`,
`$AGENT_CATALOG_ROOT/agents/critic/profile.yaml`, or
`$AGENT_CATALOG_ROOT/agents/specification-critic/profile.yaml`.

Lifecycle operators use the same external profile path shape.
`$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/history/profile.yaml` inspects
checkpoint history through `checkpoint_history`.
`$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/rollback/profile.yaml` rolls back a
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
and browser workflow, belong to `applications/catalog` with the profile assets they
exercise.

Core package tests prove reusable runtime contracts without depending on a
shipped profile path. They read synthetic fixtures under the package's own
`testdata/`, core-owned integration profiles under
`testdata/integration/profiles/`, or core-owned assets under `tools/`. Profile-owned integration
suites run this binary with `--profile` and an external profile checkout when
they need end-to-end evidence for a specific agent program, and the assertion
that a particular shipped profile is wired a particular way lives in
`applications/catalog` with the assets it reads.

## Lifecycle Operations

Lifecycle features are opt-in: checkpointing, suspend/resume, approval gates,
history, and rollback. See `lifecycle-rollback.md` for profile examples,
`--dolt-dsn`, `--resume-checkpoint`, request files, receipt-driven rollback,
and Dolt-backed persistence behavior.

For history and rollback, use the universal runtime flags:

```bash
bin/agent --profile "$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/history/profile.yaml" \
  --directory "$WORKSPACE" \
  --request requests/history.yaml

bin/agent --profile "$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/rollback/profile.yaml" \
  --directory "$WORKSPACE" \
  --request requests/rollback.yaml
```

Resume a suspended approval fixture through the same root command:

```bash
bin/agent --profile "$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/approval/profile.yaml" \
  --dolt-dsn "$DOLT_DSN" \
  --resume-checkpoint "$RUN_ID" \
  --resume-signal Approved
```

Lifecycle request files carry values such as `checkpoint: latest` or
`to_iteration: 3`. No lifecycle-only subcommands are exposed by the binary;
resume uses the universal flags above, while history and rollback use request
files.

Without `--dolt-dsn`, lifecycle persistence uses `NoopCheckpoint` and records no
durable history. Set `--dolt-dsn` to a MySQL-wire DSN for a running `dolt
sql-server` when a run must persist checkpoints for history, resume, or
rollback.

### Dolt Integration Tests

The gated Dolt checkpoint tests (`cmd/agent/dolt_integration_test.go`) exercise
the real adapter over the MySQL wire protocol. They launch a throwaway `dolt
sql-server` from a prebuilt dolt binary on an ephemeral port for the duration of
each test — no Docker and no manual setup:

```bash
mage integration:dolt   # runs the gated tests, auto-managing the server
```

The tests require only a `dolt` binary on `PATH` (install from
<https://docs.dolthub.com/introduction/installation>) or an explicit
`AGENT_CORE_DOLT_BIN` override; they skip cleanly when no binary is found, so
`go test ./...` stays green on machines without dolt.

### Persistent Dolt Server (production)

Production runs point `--dolt-dsn` at a long-lived `dolt sql-server` over the
MySQL wire protocol. Start one directly with the dolt binary and give root TCP
access from the host:

```bash
DOLT_ROOT_HOST=% dolt sql-server --host 127.0.0.1 --port 3306 --data-dir <dir>
```

Then point a run at it with
`--dolt-dsn "root@tcp(127.0.0.1:3306)/<database>"`. Storage persists in
`<dir>`, so the server is disposable while the data is not.

## Quick Start

```bash
mage build
AGENT_CATALOG_ROOT=../applications/catalog \
  bin/agent --profile "$AGENT_CATALOG_ROOT/agents/executor/profile.yaml" --directory "$PWD"
```

## Docker Runtime

Repository builds use a multi-stage Dockerfile for the release runtime image.
During the builder stage, the image clones Agent Core from GitLab, runs
`go test ./...`, and builds `agent`. The final Alpine runtime image contains
only the `agent` binary, git, common Unix utilities (including GNU `find` from
`findutils`, required by `list_files`), and core-owned shared tool assets under
`/opt/agent-core/tools`.

Runtime images intentionally exclude the Go toolchain, source checkout,
test dependencies, `golangci-lint`, and agent profile trees. Exec tools such as
`build`, `vet`, `lint`, and `test` require those binaries to come from a mounted
workspace, a derived image, or another container/host provisioning step. Agent
profiles come from a mounted `applications/catalog` checkout or unpacked profile
bundle.

Build through the Mage target:

```bash
mage docker
```

`mage docker` discovers the latest remote root release tag with the
`v0.YYYYMMDD.N` shape, passes it to the Dockerfile as `AGENT_CORE_REF`, and
builds `agent-core:latest`. Repository releases may also publish a matching
module-scoped tag such as `agent-core/v0.YYYYMMDD.N`, but Docker release
resolution continues to use the root tag family unless `release_ref` in
`demo.yaml` overrides it. The target requires Docker, and prints the resolved
build settings plus the exact Docker command before building.

To override release or container settings, uncomment the relevant keys in
`demo.yaml`:

```yaml
release_ref: v0.20260612.N
release_repo: https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core.git
container_image: registry.example/agent-core:v0.20260612.N
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

Override the path in `demo.yaml` if needed:

```yaml
container_netrc: /path/to/netrc
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
  -v "$AGENT_CATALOG_ROOT:/profiles/agents:ro" \
  -v "$PWD:/work" \
  -w /work \
  agent-core:latest \
  --profile /profiles/agents/executor/profile.yaml \
  --directory /work
```

Evaluator flows use the same profile mount and keep suites/output under the
workspace mount:

```bash
docker run --rm \
  -v "$AGENT_CATALOG_ROOT:/profiles/agents:ro" \
  -v "$PWD:/work" \
  -w /work \
  agent-core:latest \
  --profile /profiles/agents/critic/profile.yaml \
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

Recent verification: `mage docker` built `agent-core:latest` from a remote
release, `docker run --rm agent-core:latest --help` started the packaged
`agent` binary, and `docker run --rm agent-core:latest` reported that
`--profile` is required.

## Browser End-to-End Tests

The external documentation-curator profile carries the repository's browser
test suite under
`../applications/catalog/agents/knowledge-manager/documentation-curator/ui/docs`.
We depend on `puppeteer-core`
rather than `puppeteer`, so npm downloads no browser. The host supplies one,
and the test receives its path through the explicit `--executable-path`
argument. Table 1 lists what a machine needs before the suite runs.

Table 1: Browser test prerequisites

| Requirement | Detail |
|---|---|
| Node dependencies | `npm ci` inside the profile's `ui/docs` directory |
| A browser | System Chrome or Chromium, installed by the host |
| Browser path | Passed to the npm script with `--executable-path` |

Run the suite from inside the package directory:

```bash
cd ../applications/catalog/agents/knowledge-manager/documentation-curator/ui/docs
npm ci
npm run test:e2e:machine-request -- \
  --executable-path="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
```

Two details cost more time than they should. The scripts are ES modules, and
ES module resolution ignores `NODE_PATH`, so a script that lives outside the
package cannot reach its `node_modules` that way; run it from the package
directory, or point its import at an absolute path. And `puppeteer-core`
launches nothing without `executablePath`, so the suite requires the explicit
`--executable-path` argument and reports a clear configuration error when it is
missing. The package script supplies the stable base URL and artifact directory
defaults. Append `--base-url=URL` or `--artifact-dir=PATH` after `--` to
override either value.

We choose `puppeteer-core` deliberately. The full `puppeteer` package
downloads its own Chromium on every install, which we do not want in CI images
or on developer machines that already carry a browser. Adding `puppeteer` to
resolve a missing-browser error reverses that choice; pass the browser path
explicitly instead.

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
