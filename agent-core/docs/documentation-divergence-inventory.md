# Documentation Divergence Inventory

This inventory records documentation drift found by comparing the current source
tree, agent configuration, and audit output. It is a planning artifact for
`agent-core-9ilp`; follow-up issues should update documentation without changing
runtime behavior unless a follow-up explicitly calls out a source correction.

## Verification Baseline

- Current worktree includes uncommitted Docker preference changes that make
  `mage docker` prefer Docker over Podman when both are installed.
- `go list ./...` reports 29 Go packages.
- `git ls-files '*.yaml'` reports 189 tracked YAML files.
- `mage docker` has built a slim runtime image from remote release
  `v0.20260612.1`; the Docker-built check image reports 77.9 MB and the
  Podman-built `agent-core:latest` reports 54.6 MB.
- Full `mage audit` was not rerun for this inventory issue; it belongs to the
  verification follow-up after documentation edits.

## README Runtime Positioning

Source evidence:
- `cmd/agent/main.go` is the unified binary entry point; generator, planner,
  evaluator, bench, and constitution-auditor behavior is selected through YAML
  profiles and tool declarations.
- `agents/*/profile.yaml` files are the normal runtime entry points.
- `docs/ARCHITECTURE.yaml` now describes the system as a single binary driving
  configured agents.

Documentation drift:
- `README.md` still opens as "A Go framework for building tool-augmented
  agentic loops" and says "Domain agents import agent-core". That no longer
  captures the primary usage model: a universal `agent` runtime plus YAML agent
  profiles.
- The README package table is useful but does not orient readers around the
  active agents (`agents/generator`, `agents/evaluator`, `agents/planner`,
  `agents/bench`, `agents/constitution-auditor`) or profile-first startup.

Recommended edits:
- Refresh the README introduction around Agent Core as a declarative runtime.
- Keep package information concise, but add a current "Agent Profiles" or
  "Runtime Configuration" section that points at active profile files.
- Keep legacy flags out of the happy path and mention them only as
  compatibility behavior when needed.

Follow-up: `agent-core-9ilp.3`.

## Docker And Mage Release Image Workflow

Source evidence:
- `Dockerfile` is a two-stage build: Go source, tests, and build dependencies
  stay in the builder; the runtime stage is Alpine with `agent`, git/Unix
  utilities, and selected YAML config under `/opt/agent-core`.
- `magefiles/docker.go` resolves the latest remote release tag by default,
  passes it as `AGENT_CORE_REF`, uses Docker when installed and Podman as
  fallback, defaults the build secret to repository-local `.netrc`, and prints a
  transparent build settings block plus the exact command.
- `.gitignore` ignores `.netrc`, `podman-build-secret-*`, and
  `magefiles/mage_output_file.go`.

Documentation drift:
- `README.md` contains the only Docker/Mage release-image documentation. The
  broader docs set does not yet mention the containerized runtime packaging,
  release-ref selection, or `/opt/agent-core` shared config asset layout.
- README examples still contain a concrete release ref
  (`v0.20260612.1`). That is useful as a verification note but should not look
  like a permanent recommended version; current source resolves the latest
  remote release dynamically.
- The current runtime image intentionally does not include Go or
  `golangci-lint`, but active exec declarations include `build`, `vet`, `test`,
  and `lint` commands. Documentation should explain whether the container image
  is a minimal agent runtime only, or whether language/toolchain images are
  expected to extend it for code-generation validation.

Recommended edits:
- Update README Docker wording after the current Docker-preference change lands.
- Add a docs/ architecture or runtime-contract note for the release image and
  `/opt/agent-core` asset layout.
- Clarify the minimal runtime image versus language/toolchain requirements for
  exec tools.

Follow-up: `agent-core-9ilp.2`, `agent-core-9ilp.3`.

## Bench Launch Documentation

Source evidence:
- `agents/bench/builtin.yaml` configures `launch_eval` as a boundary word that
  runs the configured evaluator profile and propagates profile/trace settings.
- `docs/specs/config-formats/runtime-contract.yaml` says `launch_eval` reads
  `config.profile` and invokes the child evaluator with `--profile`.
- `agents/bench/machine.yaml` already says experiment execution launches the
  evaluator profile.

Documentation drift:
- `docs/specs/use-cases/rel01.0-uc003-bench-visualization.yaml` still says
  `launch_eval` "spawns the evaluator session machine as a subprocess" in the
  summary, flow, and success criteria. The current boundary is profile-first:
  it spawns an evaluator child agent configured by profile, whose profile owns
  the session machine.

Recommended edits:
- Replace "evaluator session machine subprocess" phrasing with "evaluator
  profile child process" or equivalent, while still noting that the evaluator
  profile runs the session machine internally.

Follow-up: `agent-core-9ilp.2`.

## Tool Declaration File Layout

Source evidence:
- Active profiles use `tool_config_dirs` pointing at individual declaration
  directories such as `tools/builtin/` and `tools/exec/`.
- Aggregate files such as `tools/builtin.yaml` and `tools/exec.yaml` remain in
  the repository as compatibility or historical aggregate inputs.

Documentation drift:
- Several docs still present `tools/builtin.yaml` and `tools/exec.yaml` as the
  primary declaration files (`tool-selection-format.yaml`,
  `tool-declaration-format.yaml`, `tool-vocabulary-audit.yaml`, older comments
  in agent tool selection files). Those references should distinguish active
  profile directory loading from compatibility aggregate files.

Recommended edits:
- Update active config-format docs to prefer `tool_config_dirs` and individual
  declaration directories.
- Keep aggregate YAML references only where explicitly describing compatibility,
  migration history, or historical examples.

Follow-up: `agent-core-9ilp.2`.

## Package And Metrics Counts

Source evidence:
- `go list ./...` reports 29 Go packages.
- `git ls-files '*.yaml'` reports 189 tracked YAML files.
- `package-layout.md` currently lists 29 Go packages and still matches the
  package count.

Documentation drift:
- The old verification baseline in this inventory listed 188 YAML files and
  referred to `agent-core-5zdu`.
- Other generated count references should be refreshed only if `mage stats`
  changes after the current README/docs updates.

Recommended edits:
- Keep `package-layout.md` unless `go list ./...` changes.
- Refresh generated counts in docs that quote YAML or stats totals during the
  verification follow-up.

Follow-up: `agent-core-9ilp.4`.

## User-Facing CLI Help Note

Source evidence:
- `cmd/agent/main.go` top-level `Long` help still says modes are selected by
  `--machine` and `--tools`, while current docs and source behavior prefer
  `--profile`.

Documentation drift:
- This is source user-facing help rather than a docs/ file, so it is outside
  the "align documentation to source code" follow-up unless the team treats CLI
  help as documentation.

Recommended follow-up:
- Consider a source/documentation cleanup issue to update CLI help to the
  profile-first wording. Do not block the docs-only alignment epic on it unless
  the scope expands to user-facing strings in source.
