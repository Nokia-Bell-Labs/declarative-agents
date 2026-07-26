# applications/catalog

This directory is the repository-specific catalog of reusable declarative tool
and agent blocks for `agent-core` applications. Its compatibility surface is the
profile programs and profile-local assets published by `agent-profiles/v0.*`
tags until the coordinated tag migration changes that prefix. `agent-core`
executes those programs; applications under `applications/` compose them by
reference.

Under this root, YAML agent programs sit beside profile-local config,
human-facing assets, demos, and integration fixtures. Runtime code stays
elsewhere. Go packages, builtin tool implementations, the `agent` binary, and
release image logic live in `agent-core`.

## Catalog contract

### Membership rule

An agent belongs under `agents/<family>/` only when it is independently useful
outside one application in this repository or has more than one real consumer.
Application-specific orchestration, endpoint choices, topology, and UI stay
with the application under `applications/<application>/`.

Every agent has one canonical home. Applications reference a catalog member;
they do not fork its machine, declarations, or reusable behavior. A generally
useful improvement found in an application flows upstream to the catalog, as
the chatbot-mesh corpus-ingest trusted-path sequence does. If behavior is useful
only to that application, it stays application-owned.

Relocation tombstones may remain under `agents/` to identify the canonical
application home, but a tombstone is not a shipped catalog member.

### Member obligations

A catalog member owes consumers:

- a canonical profile family under `agents/<family>/`, including every machine,
  tool selection, declaration, REST/config asset, and documented variant needed
  to close its runtime references;
- an SRD under `docs/specs/software-requirements/` defining behavior and
  ownership boundaries;
- deterministic conformance coverage under `conformance/`, plus a live
  integration target when behavior requires an external system;
- parameterization through existing profile-local files, environment-expanded
  declarations/REST definitions, explicit variants, runtime flags, and mounts,
  sufficient for applications to configure the member without editing it;
- portable paths, strict profile validation, formal test evidence, and release
  provenance.

A family that does not yet satisfy those obligations is not promoted as a new
public catalog member. Experimental work remains on `exp/*` until distilled.

### Ownership boundaries

This catalog owns reusable YAML programs, profile-local assets, family SRDs,
conformance, and profile-owned fixtures. `agent-core` owns the interpreter,
builtin implementations, CLI, runtime contracts, and image. Applications own
composition manifests, app-specific wrappers/configuration, end-to-end tests,
packaging, deployment, and operator UX.

`assembler` and `mock` are supported test-time catalog members. The existing
conformance vocabulary continues to classify them as supported test-time
library members. They are independently reusable across profile families and
applications, ship under `agents/`, and carry the same SRD, conformance,
portability, release, and v0 compatibility obligations as every other member.

The rig-subject and `testdata/conformance/` REST/control/lifecycle fixtures are
different: they exercise `agent-core` behavior and remain internal scaffolding
in their current paths. They are not shipped catalog members. A future
move would require its own tracker item, destination contract, and acceptance
criteria.

### Consumer contract

Applications live beside this catalog under `applications/` and name catalog
profiles by root-relative path. They pin a compatible `agent-profiles/v0.*`
release and assemble the transitive closure at package time. The runtime
receives a closed mounted tree; it is not a package resolver or registry.

Consumers may add an application wrapper that selects canonical machine/tools
and supplies app-owned REST or model configuration. Such wrappers must not copy
reusable behavior. The coding application is the closure/provenance reference;
chatbot-mesh corpus-ingest is the wrapper/parameterization reference.

### Compatibility and release evidence

`agent-profiles/v0.YYYYMMDD.N` identifies one immutable catalog bundle under the
current compatibility tag contract. An
application's `compatible_release` means that its references and configuration
were validated against that release family; it is not proof that a dirty or
different checkout is the tagged release. Packaging records both compatibility
and actual source provenance.

The major version is zero: consumers must pin and validate an exact release.
Within `v0.*`, profile paths, selected tool names/signals, terminal states,
request/response shapes, configuration names/defaults, and closure membership
are versioned public behavior. A breaking change requires an explicit migration
note in the family SRD/roadmap and a consumer update in the same coordinated
repository release; silent path or contract breaks are not compatible.

Support states are intentionally simple: canonical members on `main` and in a
release tag are the supported v0 surface; work on `exp/*` is unsupported and
non-consumable; relocation tombstones and internal conformance fixtures are
informational/internal, not catalog APIs. Supported test-time members
such as assembler and mock remain part of the versioned public surface. This
avoids a second hand-maintained status registry that could drift from real
profile paths, conformance, SRDs, and tags.

Release evidence is:

```bash
go test ./...
mage validate
mage audit
mage conformance       # deterministic release gate; never performs live inference
mage liveConformance   # explicit live-model opt-in; unavailable exact models skip
mage integration:all
mage containerSmoke   # when the agent-core image prerequisite is present
```

`go test ./...`, `mage test`, and `mage conformance` do not initiate model
inference merely because Ollama or a declared model is installed. The six live
paths (the three executor variants, planner, the REST Ollama variant, and the
chatbot source router) require `AGENT_PROFILES_LIVE_CONFORMANCE=1`; `mage
liveConformance` sets that opt-in. Live runs still require each test's exact
declared model and never substitute another model. They use a five-minute
per-run default, configurable with a positive Go duration such as
`AGENT_PROFILES_LIVE_TIMEOUT=10m mage liveConformance`.

`mage validate` derives its profile inventory from real profile-shaped files
under `agents/`; family conformance and specification indexes provide the
machine-readable behavioral evidence. This documentation does not introduce a
second catalog, package resolver, or runtime orchestrator.

Documentation under `docs/` records catalog purpose, structure, indexes,
roadmap entries, and issue format rules. Core-owned runtime assets stay in
`agent-core`.

## Experiment Branches

We keep in-progress agent experiments off `main` and out of this repository's
permanent record. Each experiment lives on a short-lived branch named
`exp/<slug>`, checked out as a worktree, and disappears when the branch is
deleted:

```bash
git fetch origin
git worktree add ../exp-<slug> exp/<slug>
```

The `exp/` prefix separates experiment branches from `gh-*` work branches and
makes stale experiments easy to list:

```bash
git branch -r --list 'origin/exp/*'
```

Experiment branches never merge into `main`. When an experiment produces a
profile or demo worth keeping, we distill it onto a fresh `gh-*` branch through
the normal issue and pull request flow, targeting `agents/<family>/` or
`demo/`. The experiment branch itself is discarded.

We never open a pull request from an `exp/*` branch. GitHub retains pull
request head commits permanently through `refs/pull/*`, which defeats later
cleanup of experiment history.

Experiment tasks live on the branch, not in the GitHub tracker, so deleting
the branch removes code, history, and task state in one operation. We track
them with [beads](https://github.com/steveyegge/beads) (`bd`), whose task
database is committed under `.beads/` on the experiment branch. GitHub issues
for experiment
tasks would outlive the experiment as permanent tracker entries. Because
`exp/*` branches never merge, `.beads/` never reaches `main`. When an
experiment task turns into durable work, we promote it to a GitHub issue
through the normal issue flow; distilled `gh-*` branches carry the profile
and demo files, never the beads data.

Pushed experiment branches are visible to everyone with access to this
repository and reach every clone that fetches them. Deleting an experiment
branch removes it from branch lists, fetches, and fresh
clones, but the commits stay addressable by SHA on GitHub until garbage
collection runs, and the fork network can retain pushed objects. We treat
every push as potentially permanent: no secrets, no sensitive data. GitHub
Support can purge objects on request when removal matters.

When an experiment concludes, we delete the branch and prune local state in
every clone that checked it out:

```bash
git push origin --delete exp/<slug>
git worktree remove ../exp-<slug>
git branch -D exp/<slug>
```

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
agent --profile "$(pwd)/agents/executor/profile.yaml" --directory /path/to/workspace
```

Application tooling that consumes this source tree uses
`AGENT_CATALOG_ROOT`. Relative values are resolved once against the command's
startup directory. During Release 99.0 only, `AGENT_PROFILES_ROOT` is accepted
as a deprecated alias when `AGENT_CATALOG_ROOT` is unset. If both are set, they
must clean to the same absolute path; otherwise the command fails before doing
work. The alias is removed after Release 99.0. These variables select a source
checkout and do not change the packaged `/profiles` mount, compatibility tag,
or recorded source provenance.

From the **agent-core** checkout, integration Mage targets consume this tree
through the external profile path rules documented there. Read the agent-core
README and runtime contract before wiring CI.

## Container Usage

In containers, callers mount this catalog, check it out, or unpack a release
bundle. The image supplies the `agent` binary plus core-owned runtime assets.
Profiles and workspace files come from the caller.

```bash
docker run --rm \
  -v "$PWD:/profiles:ro" \
  -v "$WORKSPACE:/work" \
  agent-core:latest \
  --profile /profiles/agents/executor/profile.yaml \
  --directory /work
```

Mount this catalog read-only at `/profiles` (or another mount point). Pass
that mount path to **`--profile`**. Mount the workspace and pass it to
**`--directory`**.

## Application reference and packaging contract

Applications consume profiles by reference; they do not copy canonical catalog
profiles into their source trees. An application manifest names each entry
profile with a path relative to this catalog root and pins a compatible
`agent-profiles/v0.*` release. Packaging, not the runtime, resolves the complete
closure into a tree mounted at `/profiles`.

Reference resolution follows the runtime's existing path surfaces:

- entry references and values beginning `agents/` are relative to this root;
- profile-local machine, tool selection, declarations, config directories, and
  REST files are relative to the YAML file that declares them;
- child `profile` and critic `point_*` declaration references are transitive;
- `/opt/agent-core/...` references remain external runtime assets and are not
  copied;
- copied files retain their root-relative destination, so a declaration that
  names `agents/executor/profile.yaml` still resolves at
  `/profiles/agents/executor/profile.yaml`.

Packagers must reject repository traversal, disallowed absolute paths, runtime
reference globs, symlinks, dangling references, and conflicting destination
paths. Output must be deterministic and record the sorted file closure plus
source provenance. A clean checkout exactly at the pinned tag may record
`kind: release`. Any other checkout records `kind: checkout`, its commit (or an
explicit unversioned fixture marker), dirty state when applicable, and the
compatible release separately. Compatibility is not release provenance.

Catalog profiles expose configuration through profile-local declarations,
machines, explicit variants, and the existing runtime flags and mounts. An
application packager may select or co-generate those existing assets, but this
contract introduces no placeholders or template substitution. Application
values must not be committed into canonical catalog profiles.

`applications/coding-agent/agents/application.yaml` and its `mage package` target
are the reference implementation. Its closure includes planner, executor,
critic session, and `critic/profile-workspace.yaml`; the application directory
contains composition/config inventory only, not copied catalog programs.

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
  workspace. Its profile references resolve from this catalog root.
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
`agent-profiles/v0.YYYYMMDD.N` tag remains the compatibility identifier for
callers that consume this catalog independently of the full repository. Tag
prefix migration is owned by the coordinated release work.

After profile changes are ready for mounted-path, checkout, or release-bundle
consumers, create release tags from the repository root on `main`:

```bash
mage tag
```

At tag time, the root target reads existing local root tags for the current date
and creates the next daily revision, such as `v0.20260617.0` or
`v0.20260617.1`. It also creates matching module tags including
`agent-profiles/v0.20260617.N`. Profile bundle tags version this catalog's
YAML programs, demos, UI assets, and integration fixtures. Runtime image builds
continue to resolve the root `v0.*` tag family unless the `agent-core` Docker
release target is explicitly overridden.

## Validation

Validation uses an external `agent-core` checkout or runtime image. Catalog-local
validation reads every profile-shaped YAML file under `agents/`, including
`profile.yaml`, `profile-*.yaml`, and `*-profile.yaml` variants. It resolves
profile-local files from this catalog and checks `/opt/agent-core/tools`
references against the resolved agent-core tree.

```bash
mage validate
```

After the source move, run repository-local validation with
`AGENT_CORE_ROOT="$(git rev-parse --show-toplevel)/agent-core" mage validate`.
When `AGENT_CORE_ROOT` is unset, catalog Mage targets resolve the monorepo
checkout at `../../agent-core` from this owner root. Relative
`AGENT_CORE_ROOT` values are also interpreted from this owner root.

With an `agent-core` image available, run the mounted-profile container smoke
check:

```bash
mage containerSmoke
```

Optional image selection uses the constant `agentCoreImageEnv` in
`magefiles/validation.go` (defaults to `agent-core:latest`).

Before running the profile, the smoke target fails if the image contains
`/opt/agent-core/agents`. It then mounts this catalog at `/profiles`, mounts
core-owned tools at `/opt/agent-core/tools`, and runs
`--profile /profiles/agents/jurist/profile.yaml --directory /work`.
