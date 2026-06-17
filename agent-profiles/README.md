# agent-profiles

This repository owns the external agent programs and profile assets consumed by
`agent-core`.

Under this root, YAML agent programs sit beside profile-local config,
human-facing assets, demos, and integration fixtures. Runtime code stays
elsewhere. Go packages, builtin tool implementations, the `agent` binary, and
release image logic live in `agent-core`.

## Repository Contract

Profile-owned programs live under `agents/`, grouped by agent family. The
migrated tree includes the coding, evaluation, validation, REST, monitor,
control, lifecycle, and Knowledge Manager profile families. Runnable examples
belong in `demo/`; integration suites and fixture data belong in
`testdata/integration/`.

Documentation under `docs/` records purpose, structure, indexes, roadmap
entries, and issue format rules. Core-owned runtime assets stay in
`agent-core`.

## Local Usage

For local runs, point `agent-core` at this checkout as the profile root:

```bash
export AGENT_PROFILES_ROOT="$(pwd)"
agent --profile "$AGENT_PROFILES_ROOT/agents/generator/profile.yaml" --directory /path/to/workspace
```

`agent-core` tests and Mage integration targets use `AGENT_PROFILES_ROOT` to
resolve profiles, demos, and integration fixtures from this repository.
Profiles reference shared core-owned tool declarations through
`/opt/agent-core/tools`, the installed runtime asset root used by the
agent-core container image.

## Container Usage

In containers, callers mount this repository, check it out, or unpack a release
bundle. The image supplies the `agent` binary plus core-owned runtime assets.
Profiles and workspace files come from the caller.

```bash
docker run --rm \
  -v "$PWD:/profiles:ro" \
  -v "$WORKSPACE:/work" \
  agent-core:latest \
  --profile /profiles/agents/generator/profile.yaml \
  --directory /work
```

Mounted at `/profiles`, this repository plays the same role as
`AGENT_PROFILES_ROOT` on the host.

## Demos and Fixtures

Profile-owned demos live in `demo/`. For Knowledge Manager, the demo starts the
documentation-curator profile from `AGENT_PROFILES_ROOT` or `/profiles` and
uses `AGENT_WORKSPACE` or `/work` as the workspace path:

```bash
docker run --rm \
  -v "$PWD:/profiles:ro" \
  -v "$WORKSPACE:/work" \
  agent-core:latest \
  --profile /profiles/agents/knowledge-manager/documentation-curator/profile.yaml \
  --directory /work
```

Integration fixtures owned by profiles live in `testdata/integration/`:

- `uc001-generator-coding/` contains the generator coding sample workspace.
- `uc002-evaluator-benchmark/` contains the evaluator suite and sample
  workspace. Its profile references resolve from this repository root.
- `rel04-monitor/monitor-rest.yaml` records the monitor profile proof metadata.

`agent-core` Mage integration targets should resolve these files through
`AGENT_PROFILES_ROOT`:

```bash
AGENT_PROFILES_ROOT=/path/to/agent-profiles mage integration:uc001
AGENT_PROFILES_ROOT=/path/to/agent-profiles mage integration:uc002
AGENT_PROFILES_ROOT=/path/to/agent-profiles mage integration:uc004
```

Core-only runtime fixtures remain in `agent-core` when they exercise reusable
tool implementation behavior rather than a profile-owned sample or suite. REST
runtime conformance fixtures, including standalone REST tool definitions and
OpenAPI documents, stay with `agent-core` until a profile issue explicitly
moves them.

Formal use cases and test suites for the profile repository migration remain
tracked follow-up work. Already checked in are the profile assets, demos,
fixtures, release tagging, and validation commands.

## Release Tags

Profile bundle releases use the same revision shape as `agent-core` runtime
releases:

```text
v0.YYYYMMDD.N
```

After profile changes are ready for mounted-path, checkout, or release-bundle
consumers, create a tag from `main`:

```bash
mage tag
```

At tag time, the target reads existing local tags for the current date and
creates the next daily revision, such as `v0.20260617.0` or
`v0.20260617.1`. It does not query `agent-core`; profile bundle tags version
this repository's YAML programs, demos, UI assets, and integration fixtures.
Runtime image tags still belong to `agent-core`.

## Validation

Validation uses an external `agent-core` checkout or runtime image. Local
validation reads every profile-shaped YAML file under `agents/`, including
`profile.yaml`, `profile-*.yaml`, and `*-profile.yaml` variants. It resolves
profile-local files from this repository and resolves `/opt/agent-core/tools`
against `AGENT_CORE_ROOT` without copying agent assets into the core image.

```bash
AGENT_CORE_ROOT=/path/to/agent-core mage validate
```

When `AGENT_CORE_ROOT` is unset, `mage validate` defaults to the sibling
`../agent-core` checkout.

With an `agent-core` image available, run the mounted-profile container smoke
check:

```bash
AGENT_CORE_IMAGE=agent-core:latest \
AGENT_CORE_ROOT=/path/to/agent-core \
mage containerSmoke
```

Before running the profile, the smoke target fails if the image contains
`/opt/agent-core/agents`. It then mounts this repository at `/profiles`, mounts
core-owned tools at `/opt/agent-core/tools`, and runs
`--profile /profiles/agents/jurist/profile.yaml --directory /work`.
