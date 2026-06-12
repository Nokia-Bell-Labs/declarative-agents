# Documentation Divergence Inventory

This inventory records documentation drift found by comparing the current source
tree, agent configuration, and audit output. It is a planning artifact for
`agent-core-5zdu`; follow-up issues should update the referenced docs or audit
rules without changing runtime behavior unless noted.

## Verification Baseline

- `go test ./...` passes.
- `mage stats` runs and reports 30 Go packages, 35,782 Go lines, and 188 YAML files.
- `mage audit` runs but the constitution-auditor reports 48 validation errors.

## Runtime Invocation And Profiles

Source evidence:
- `cmd/agent/main.go` defines `--profile` as the replacement for `--machine`,
  `--tools`, and `--tools-declaration`.
- `cmd/agent/main.go` warns when legacy flags `--machine`, `--tools`,
  `--tools-declaration`, `--model`, and `--ollama-url` are explicitly used.
- `internal/tools/stl/profile.go` resolves `machine`, `tools`,
  `tool_declarations`, `tool_config_dirs`, and `directory` relative to the
  profile file.
- `internal/tools/stl/exectool.go` supports repeatable tool selection files,
  repeatable tool declaration files, and declaration loading from directories.

Documentation drift:
- `docs/specs/config-formats/runtime-contract.yaml` still presents
  `--machine`, `--tools`, and `--tools-declaration` as the primary invocation
  contract, with `--model` and `--ollama-url` as ordinary universal flags.
- `docs/specs/use-cases/rel01.0-uc001-generator-coding.yaml` and
  `docs/specs/use-cases/rel01.0-uc003-bench-visualization.yaml` show legacy
  flag-based invocations as the happy path even though profiles exist for those
  agents.
- Runtime docs do not fully document `tool_config_dirs` directory discovery or
  profile-relative path resolution.

Follow-up: `agent-core-5zdu.2`.

## Evaluator Suite Profiles

Source evidence:
- `internal/evaluation/eval_session.go` documents and implements two suite
  formats: profile-based `profiles: [...]` as preferred, and legacy
  `harnesses` plus `models`.
- `internal/evaluation/eval_session.go` rejects suites that mix profiles with
  harnesses or models.
- `internal/evaluation/eval_clitool.go` invokes child agents with `--profile`
  when `PointContext.ProfilePath` is set; legacy mode falls back to `--model`
  and harness flags.
- `testdata/integration/uc002-evaluator-benchmark/suite.yaml` uses generator
  profiles for Qwen 3.6 35B and 27B.

Documentation drift:
- `docs/specs/software-requirements/srd019-eval-harness.yaml` defines the
  primary suite contract in terms of harnesses and models, and says the session
  iterates `(harness, model, gridPoint, sample, repetition)`.
- `docs/specs/use-cases/rel01.0-uc002-evaluator-benchmark.yaml` describes the
  suite as a grid of harness/model combinations and uses legacy evaluator
  invocation flags.
- UC002 success criterion `S7` traces only to `agents/evaluator/machine.yaml`
  and is reported by `mage audit` as an untraced success criterion.

Follow-up: `agent-core-5zdu.3`.

## Audit Vocabulary And Config Validation

Source evidence:
- `pkg/spec/types.go` defines the canonical `KnownSideEffectKinds` vocabulary.
  It includes `child_process` and `nested_machine_execution`, but not
  `subprocess` or `nested_machine`.
- `agents/planner/builtin.yaml` and `agents/bench/builtin.yaml` currently use
  `kind: subprocess`.
- `agents/evaluator/builtin.yaml` currently uses `kind: nested_machine`.
- `agents/generator/profile.yaml` and `agents/planner/profile.yaml` include
  agent-local LLM declarations, but `mage audit` reports selected `invoke_llm`
  tools as undeclared for generator and planner.
- `agents/evaluator/machine.yaml` is named `evaluator-session`, while the
  directory/profile are named `evaluator`.

Audit findings:
- `machine-name-mismatch`: evaluator directory name vs machine spec name.
- `machine-unresolved-action`: generator dynamic `$tool` transition is treated
  as an unresolved selected action.
- `tool-selection-undeclared`: generator and planner select `invoke_llm` even
  though profile-local declarations provide it.
- `tool-unknown-side-effect-kind`: `subprocess` and `nested_machine` do not
  match the canonical vocabulary.

Follow-up: `agent-core-5zdu.4`.

## Traceability And Coverage

Source/audit evidence:
- `mage audit` reports many `uncovered-req-item` and `uncovered-ac` findings.
- The largest source of current drift is around SRDs that describe LLM
  conversation/prompt behavior, evaluator harness behavior, bench UI behavior,
  and newer tool contracts.
- Several SRDs are reported as orphaned from use-case touchpoints.

Documentation drift:
- Some requirements appear to describe current source behavior but lack
  acceptance criteria or test-suite traces.
- Some older requirements appear to describe legacy behavior and should be
  revised or explicitly scoped as compatibility behavior.

Follow-up: `agent-core-5zdu.5`.

## Package Inventory And Counts

Source evidence:
- `go list ./...` now shows public code under `pkg/spec` only; implementation
  code lives under `internal/runtime`, `internal/tools`, `internal/evaluation`,
  `internal/model`, `internal/planning`, `internal/observability`, and
  `internal/support`.
- `mage stats` reports current line and YAML counts after the internal package
  migration.

Documentation drift:
- Package path references were mostly updated in the previous cleanup, but
  generated counts and package inventory sections still need one source-backed
  refresh against `go list ./...` and `mage stats`.
- Historical notes can mention former paths, but active architecture and spec
  claims should use current paths.

Follow-up: `agent-core-5zdu.6`.
