# Grammar Machine Conformance Inventory

This inventory records the conformance audit that produced
`agent-core-9iol` and the final alignment status against
`declarative-programming-pattern.md`.

## Verification Baseline

- `agent-core-9iol.1` through `agent-core-9iol.6` are complete.
- `mage audit` succeeds with selected-tool contract checks wired into
  `pkg/spec.Validate`.
- Warning-era undo strategy drift has been resolved by aligning
  `pkg/spec/checkToolUndoConsistency` with the rollback vocabulary.
- Active agent profiles inspected:
  - `agents/generator/profile.yaml`
  - `agents/generator/profile-qwen35b.yaml`
  - `agents/generator/profile-qwen27b.yaml`
  - `agents/planner/profile.yaml`
  - `agents/evaluator/profile.yaml`
  - `agents/bench/profile.yaml`
  - `agents/jurist/profile.yaml`

## Pattern Requirements Used As Audit Criteria

Source: `declarative-programming-pattern.md`.

- Grammar is data: workflows belong in `machine.yaml` transition tables.
- Words are configured commands: each `ToolDef` is the program interpreted by
  the tool implementation.
- Signals are closed vocabulary: emitted signals must be declared and handled by
  the grammar.
- The engine is fixed: domain behavior should come from grammars and lexicons,
  not conditionals in the runtime.
- Boundary words are explicit: model, human, child-agent, and nested-machine
  boundaries must declare actor configuration, side effects, undo, and signals.
- Undo is word-level: every word has an undo strategy that the engine can reason
  about through the recorded sentence.

## Tool Contract Alignment

Resolved by `agent-core-9iol.2` and `agent-core-9iol.3`.

Active selected tool declarations now carry Grammar Machine word contracts:
`category`, `problem`, `goals`, `requirements`, `non_goals`, `emits`,
`output.schema`, `side_effects`, `reversibility`, `undo`, `errors`, and
`relationships`. The normal spec audit path enforces these fields for selected
tools. Missing contract fields are errors for migrated/default declarations and
warnings only for declarations explicitly classified as legacy.

Relevant source:

- `internal/tools/stl/tool_contracts.go` defines the full contract shape.
- `pkg/spec/validate.go` enforces selected-tool completeness in `Validate`.
- `pkg/spec/corpus.go` resolves active profile selections, including
  agent-local overrides and configured evaluator point tools.

## Legacy And Aggregate Declaration Scope

There are duplicate aggregate declaration files such as `tools/builtin.yaml` and
`tools/exec.yaml` plus individual files under `tools/builtin/` and `tools/exec/`.
The profile-first runtime selects individual declaration directories through
`tool_config_dirs`, so alignment work should prioritize active individual files
and agent-local overrides.

Legacy aggregate files may remain compatibility inputs, but they should not be
the canonical source for new contract metadata unless a follow-up explicitly
keeps them synchronized.

## Child Boundary Configuration Shape

Resolved by `agent-core-9iol.5`.

Child-agent boundary configuration is profile-first. `execute.Config` accepts
`Profile`, `BuildArgs` emits `--profile` when set, and child boundary tools
propagate profile values through planner, bench, launch-eval, and self-invoke
factories. Legacy `machine`, `tools`, `tools_declarations`, `model`, and
`ollama_url` fields remain compatibility-only fallback paths.

## Factory Registration Shape

Resolved by `agent-core-9iol.6`.

The core engine remains generic, and the composition root now centralizes
builtin factory-family wiring in `builtinFactoryCatalog`. Each entry is keyed
by selected tool init names and registers a capability family only when the
active YAML selections require it. Adding a new builtin family still needs
runtime wiring, but the mode-specific branching has been reduced to a single
catalog.

## Undo Strategy Vocabulary

Resolved by `agent-core-9iol.4`.

`pkg/spec/checkToolUndoConsistency` recognizes the structured rollback
strategy vocabulary by reversibility class, including workspace restore,
session/domain restore, conversation truncation/restore, child command undo,
boundary compensation, and compensating actions.

## Documentation Refresh

Resolved by `agent-core-9iol.7`.

Active docs describe the aligned design: profile-first child boundaries,
profile-resolved evaluator terminology, selected-tool contract enforcement,
expanded undo strategy vocabulary, centralized builtin factory registration,
and explicit legacy compatibility paths for older machine/tools invocations.
