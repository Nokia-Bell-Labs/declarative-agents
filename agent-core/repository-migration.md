# Repository Migration Plan

This repository is large enough that the first migration should reduce tracked
artifacts and clarify ownership boundaries before splitting code across
repositories. A physical multi-repo split will be easier, safer, and more
reversible after the current repository is organized around stable package and
asset boundaries.

## Current Shape

`agent-core` is primarily a Go runtime that builds the `agent` binary. The
runtime loads agent behavior from YAML profiles, tool declarations, and specs.
The repository also contains evaluation tooling, two Vite UIs, generated UI
bundles, local binaries, and Beads/Dolt tracker data.

The source tree is not the main size problem by itself. The largest contributors
are generated or stateful artifacts:

- `.git` is very large, which suggests large files or generated artifacts were
  committed in history.
- `.beads` contains embedded Dolt data and backup state.
- `internal/evaluation/bench/ui/node_modules` is tracked.
- UI `dist` bundles are checked in under internal UI directories.
- Local binaries such as `agent`, `eval-analyze`, and `bin/` increase checkout
  size when present locally.

The migration should therefore start with repository hygiene, then move to
logical boundaries, and only then split repositories if that still helps.

## Goals

- Keep the core runtime small, cloneable, and easy to release.
- Separate product behavior from runtime implementation.
- Keep generated assets and local dependency caches out of source control.
- Let evaluation, specs, and UIs evolve without forcing every runtime change to
  carry their weight.
- Preserve a simple developer workflow during the transition.

## Non-Goals

- Do not immediately split everything into independent repositories.
- Do not introduce compatibility layers for boundaries that have not shipped.
- Do not rewrite package architecture just to match a new folder layout.
- Do not treat `.beads/issues.jsonl` as the source of truth for issue data.

## Phase 1: Repository Hygiene

This phase should happen before moving code. It reduces current checkout size and
prevents the same problem from returning.

1. Stop tracking generated and local dependency artifacts.
   - Remove tracked `node_modules` directories.
   - Remove tracked UI `dist` directories unless a release process explicitly
     requires committed static assets.
   - Keep `package-lock.json` files for reproducible UI installs.
   - Keep root binaries ignored and avoid committing generated binaries.

2. Tighten ignore rules.
   - Ignore all nested `node_modules/`.
   - Ignore all nested UI `dist/` outputs by default.
   - Ignore local build outputs under `bin/`.
   - Keep Beads/Dolt runtime storage out of normal source commits unless a
     specific tracker file is intentionally versioned.

3. Decide what to do with existing history.
   - If clone size matters, rewrite history with `git filter-repo` or BFG to
     remove large generated files and dependency directories.
   - Coordinate this rewrite with anyone else using the repository because it
     changes commit identities.
   - If history rewrite is too disruptive, clean the tree going forward and
     accept that existing clones remain large until a fresh repository is cut.

4. Add verification checks.
   - Add a CI or pre-commit check that fails if `node_modules`, generated UI
     bundles, local binaries, or large transient files are staged.
   - Document the expected UI build commands instead of committing build output.

## Phase 2: Establish In-Repository Boundaries

Before creating multiple repositories, organize the current tree so each domain
has clear ownership and build rules.

Recommended target layout:

```text
core/
  cmd/
  internal/runtime/
  internal/model/
  internal/tools/
  internal/support/
  internal/observability/
  pkg/spec/

profiles/
  agents/
  tools/

specs/
  docs/specs/
  docs/SPECIFICATIONS.yaml
  docs/road-map.yaml

evaluation/
  internal/evaluation/
  testdata/integration/
  evaluator suites and samples

ui/
  documentation/
  bench/

build/
  magefiles/
  Dockerfile
```

This layout can be introduced incrementally. The important part is to make the
dependency direction explicit:

- `core` should not depend on `evaluation`.
- `core` should be able to run profiles from an external path.
- `profiles` may depend on tool names and runtime contracts exposed by `core`.
- `evaluation` may invoke the `agent` binary or depend on a small public API.
- `ui` should build static assets as part of release packaging, not as committed
  source.

## Phase 3: Split Candidates

After the in-repository boundaries are stable, split only the parts that benefit
from independent ownership, releases, or clone size.

### Keep: `agent-core`

The core repository should contain the runtime, binary entry point, public Go
packages, runtime tool implementations, observability support, Docker packaging,
and minimal smoke tests.

Expected contents:

- `cmd/agent`
- `internal/runtime`
- `internal/model`
- `internal/tools`
- `internal/support`
- `internal/observability`
- `pkg/spec`
- root `go.mod` and `go.sum`
- runtime Dockerfile and Mage build targets

### Split Candidate: `agent-profiles`

Move agent behavior and runtime declarations into a profiles/assets repository.

Expected contents:

- `agents/`
- `tools/`
- profile-local request examples
- behavior-level documentation

Consumption options:

- checkout path passed to `--profile`
- release tarball unpacked beside the binary
- Docker image layer copied into `/opt/agent-core`
- git submodule or subtree during transition

This split works well because the README already treats agent behavior as YAML
and declarations rather than mode-specific Go code.

### Split Candidate: `agent-specs`

Move specs and validation corpus if they are owned or reviewed separately from
runtime code.

Expected contents:

- `docs/specs/`
- `docs/SPECIFICATIONS.yaml`
- `docs/road-map.yaml`
- spec validation fixtures

Keep `pkg/spec` in `agent-core` unless the spec model is needed by other
projects as an independent Go module. If that happens, extract it later as a
small library with a stable API.

### Split Candidate: `agent-evaluation`

Move evaluator workflows, benchmark suites, samples, and bench-specific tooling.

Expected contents:

- `internal/evaluation`
- evaluator request files and suites
- integration testdata used only by evaluator workflows
- benchmark reporting code

The preferred dependency is to invoke a released `agent` binary from
`agent-core`. Importing internal Go packages across repositories should be
avoided. If evaluation needs shared code, promote a narrow API into `pkg/` first.

### Split Candidate: `agent-ui`

Move the Vite applications into a frontend-oriented repository or top-level
workspace.

Expected contents:

- documentation curator UI
- bench UI
- e2e tests for browser behavior
- frontend package management and build scripts

Release packaging can copy built assets into the runtime image or embed them at
build time. Built `dist` output should normally be produced by CI, not committed.

## Phase 4: Release And Consumption Model

Choose one release model before extracting repositories.

### Option A: Runtime Image Owns Assembly

`agent-core` builds the binary and release image. CI fetches pinned versions of
profiles, specs, and UI assets, then assembles `/opt/agent-core`.

This is the simplest operational model for users who run containers.

### Option B: Binary Plus Asset Bundle

`agent-core` releases the binary. `agent-profiles` and `agent-specs` release
versioned asset bundles. Users install the binary and choose an asset bundle.

This is better when profiles need to move faster than the runtime.

### Option C: Monorepo With Clean Boundaries

Keep one repository but enforce boundaries and stop tracking generated artifacts.

This may be enough if the primary pain is clone size and noisy diffs rather than
different team ownership.

## Suggested Sequence

1. Remove tracked generated artifacts and dependency caches.
2. Update `.gitignore` and add a guard against committing them again.
3. Decide whether to rewrite git history.
4. Move UI build output to CI or release packaging.
5. Make profiles load cleanly from external paths.
6. Promote any cross-boundary Go APIs from `internal/` to `pkg/` only when a real
   external consumer needs them.
7. Move evaluation to a top-level boundary or separate module.
8. Move profiles and specs to separate repositories only after path-based
   consumption works locally.
9. Update Docker and Mage targets to assemble the runtime from pinned asset
   versions.
10. Document the new developer workflow and release process.

## Risks

- Rewriting history will disrupt existing clones and branches.
- Moving Go packages too early can create unstable public APIs.
- Splitting profiles before external profile paths are tested can break normal
  agent startup.
- Extracting evaluation while it still imports `internal/` packages will force
  awkward API exposure.
- Separate repositories add release coordination cost, so each split should have
  a clear owner or cadence benefit.

## Recommendation

Start with Phase 1 and Phase 2. They address the current repository size and make
future extraction safer. If the repository still feels too large or ownership is
clearly diverging after that, split in this order:

1. `agent-profiles`
2. `agent-evaluation`
3. `agent-ui`
4. `agent-specs`

Keep `agent-core` focused on the Go runtime and binary. Treat everything else as
versioned inputs consumed by that runtime.
