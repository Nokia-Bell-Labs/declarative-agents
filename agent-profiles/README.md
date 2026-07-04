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

## Runtime contract and authority

This repository does not define how `cmd/agent` bootstraps paths. That contract
lives in **agent-core**: `docs/specs/config-formats/runtime-contract.yaml`,
`docs/specs/software-requirements/srd034-external-agent-profiles.yaml`, and the
constitution set under `docs/constitutions/`. Related work is tracked in
**agent-core** as epic **`agent-core-tj96`** (single configuration
authority), with the file-and-flag documentation milestone **`agent-core-tj96.1`**.

Operators should treat **`--profile`** and **`--directory`** (plus request and
telemetry flags from the runtime contract) as the primary inputs. Profile YAML,
machines, tool selections, and mounts supply the rest.

## Local Usage

Run `cmd/agent` with explicit paths. Replace `/path/to/workspace` with your
workspace.

```bash
agent --profile "$(pwd)/agents/generator/profile.yaml" --directory /path/to/workspace
```

From the **agent-core** checkout, integration Mage targets consume this tree
through the external profile path rules documented there. Read the agent-core
README and runtime contract before wiring CI.

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

Mount this repository read-only at `/profiles` (or another mount point). Pass
that mount path to **`--profile`**. Mount the workspace and pass it to
**`--directory`**.

## Demos and Fixtures

Profile-owned demos live in `demo/`. The Knowledge Manager example uses the same
explicit argv pattern:

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

Core-only runtime fixtures remain in `agent-core` when they exercise reusable
tool implementation behavior rather than a profile-owned sample or suite. REST
runtime conformance fixtures, including standalone REST tool definitions and
OpenAPI documents, stay with `agent-core` until a profile issue explicitly
moves them.

Formal use cases and test suites for profile repository migration and
profile-owned integration tracer bullets are implemented. Checked in assets now
include profile programs, demos, fixtures, release tagging, validation commands,
and Mage integration targets for the Release 07 tracer bullets.

## Release Tags

Profile bundle releases use the repository release revision and a module-scoped
profile tag:

```text
v0.YYYYMMDD.N
agent-profiles/v0.YYYYMMDD.N
```

The root tag identifies the coordinated repository release. The
`agent-profiles/v0.YYYYMMDD.N` tag identifies the profile bundle for callers
that consume this directory independently of the full repository.

After profile changes are ready for mounted-path, checkout, or release-bundle
consumers, create release tags from the repository root on `main`:

```bash
mage tag
```

At tag time, the root target reads existing local root tags for the current date
and creates the next daily revision, such as `v0.20260617.0` or
`v0.20260617.1`. It also creates matching module tags including
`agent-profiles/v0.20260617.N`. Profile bundle tags version this repository's
YAML programs, demos, UI assets, and integration fixtures. Runtime image builds
continue to resolve the root `v0.*` tag family unless the `agent-core` Docker
release target is explicitly overridden.

## Validation

Validation uses an external `agent-core` checkout or runtime image. Local
validation reads every profile-shaped YAML file under `agents/`, including
`profile.yaml`, `profile-*.yaml`, and `*-profile.yaml` variants. It resolves
profile-local files from this repository and checks `/opt/agent-core/tools`
references against the resolved agent-core tree.

```bash
mage validate
```

By default, `mage validate` expects an **agent-core** checkout as a sibling
directory named `agent-core` next to this repository. To point at a different
checkout, use the optional core-root input defined in `magefiles/validation.go`
(constant `agentCoreRootEnv`).

With an `agent-core` image available, run the mounted-profile container smoke
check:

```bash
mage containerSmoke
```

Optional image selection uses the constant `agentCoreImageEnv` in
`magefiles/validation.go` (defaults to `agent-core:latest`).

Before running the profile, the smoke target fails if the image contains
`/opt/agent-core/agents`. It then mounts this repository at `/profiles`, mounts
core-owned tools at `/opt/agent-core/tools`, and runs
`--profile /profiles/agents/jurist/profile.yaml --directory /work`.
