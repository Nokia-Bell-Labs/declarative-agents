# Grammar Machine Conformance Inventory

This inventory records the conformance audit that produced
`agent-core-9iol` and the final alignment status against
`declarative-programming-pattern.md`.

## Verification Baseline

`agent-core-9iol.1` through `agent-core-9iol.6` are complete. `mage audit`
succeeds with selected-tool contract checks wired into `pkg/spec.Validate`.
Warning-era undo strategy drift has been resolved by aligning
`pkg/spec/checkToolUndoConsistency` with the rollback vocabulary.

The conformance pass inspected `AGENT_PROFILES_ROOT/generator/profile.yaml`,
`AGENT_PROFILES_ROOT/generator/profile-qwen35b.yaml`,
`AGENT_PROFILES_ROOT/generator/profile-qwen27b.yaml`,
`AGENT_PROFILES_ROOT/planner/profile.yaml`,
`AGENT_PROFILES_ROOT/evaluator/profile.yaml`,
`AGENT_PROFILES_ROOT/bench/profile.yaml`, and
`AGENT_PROFILES_ROOT/jurist/profile.yaml`.

## Pattern Requirements Used As Audit Criteria

The audit criteria come from `declarative-programming-pattern.md`. Workflows
belong in `machine.yaml` transition tables. Each `ToolDef` is the configured
program interpreted by a tool implementation. Emitted signals must be declared
and handled by the grammar. Domain behavior should come from grammars and
lexicons, not runtime conditionals. Boundary words for models, humans,
child-agents, and nested machines must declare actor configuration, side
effects, undo, and signals. Undo remains word-level so the engine can reason
about the recorded sentence.

## Tool Contract Alignment

`agent-core-9iol.2` and `agent-core-9iol.3` resolved this area. Active
selected tool declarations now carry Grammar Machine word contracts.
They include `category`, `problem`, `goals`, `requirements`, `non_goals`, `emits`,
`output.schema`, `side_effects`, `reversibility`, `undo`, `errors`, and
`relationships`. The normal spec audit path enforces these fields for selected
tools. Missing contract fields are errors for migrated/default declarations and
warnings only for declarations explicitly classified as legacy.

`internal/tools/stl/tool_contracts.go` defines the full contract shape.
`pkg/spec/validate.go` enforces selected-tool completeness in `Validate`.
`pkg/spec/corpus.go` resolves active profile selections, including agent-local
overrides and configured evaluator point tools.

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

`agent-core-9iol.5` resolved child-agent boundary configuration. The shape is
profile-first. `execute.Config` accepts
`Profile`, `BuildArgs` emits `--profile` when set, and child boundary tools
propagate profile values through planner, bench, launch-eval, and self-invoke
factories. Legacy `machine`, `tools`, `tools_declarations`, `model`, and
`ollama_url` fields remain compatibility-only fallback paths.

## Factory Registration Shape

`agent-core-9iol.6` resolved factory registration shape. The core engine
remains generic, and the composition root now centralizes
builtin factory-family wiring in `builtinFactoryCatalog`. Each entry maps
selected tool init names and registers a capability family only when the
active YAML selections require it. Adding a new builtin family still needs
runtime wiring, but the mode-specific branching has been reduced to a single
catalog.

## Undo Strategy Vocabulary

`agent-core-9iol.4` resolved undo strategy vocabulary.
`pkg/spec/checkToolUndoConsistency` recognizes the structured rollback
strategy vocabulary by reversibility class, including workspace restore,
session/domain restore, conversation truncation/restore, child command undo,
boundary compensation, and compensating actions.

## Documentation Refresh

`agent-core-9iol.7` resolved the documentation refresh. Active docs describe the
aligned design through profile-first child boundaries,
profile-resolved evaluator terminology, selected-tool contract enforcement,
expanded undo strategy vocabulary, centralized builtin factory registration, and
explicit legacy compatibility paths for older machine/tools invocations.
