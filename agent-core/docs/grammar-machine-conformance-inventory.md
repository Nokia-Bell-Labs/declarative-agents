# Grammar Machine Conformance Inventory

This inventory records active gaps between the current implementation and
`declarative-programming-pattern.md`. It is the planning baseline for
`agent-core-9iol`.

## Verification Baseline

- `bd ready` shows `agent-core-9iol.1` as the first unblocked child issue.
- `mage audit` succeeds with `validate: ... OK`, but still reports warning-level
  `tool-undo-mismatch`, orphan/touchpoint, and uncovered-AC findings.
- Active agent profiles inspected:
  - `agents/generator/profile.yaml`
  - `agents/generator/profile-qwen35b.yaml`
  - `agents/generator/profile-qwen27b.yaml`
  - `agents/planner/profile.yaml`
  - `agents/evaluator/profile.yaml`
  - `agents/bench/profile.yaml`
  - `agents/constitution-auditor/profile.yaml`

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

## Active Tool Contract Gaps

The active profiles select 50 unique declaration entries when evaluator point
tools are included. A generated scan against the same field set used by
`internal/tools/stl.ValidateToolContracts` found:

- 44 entries missing `problem`, `goals`, `non_goals`, `errors`, and
  `relationships`.
- 40 entries missing `output.schema`.
- 6 entries missing structural fields: `category`, `side_effects`,
  `reversibility.classification`, and `undo`.

High-priority active gaps:

- Generator profiles select `build`, `vet`, `lint`, and `test` from
  `tools/exec/*.yaml`. These declarations have `binary`, `args`, `emits`, and
  description, but lack category, output schema, structured side effects,
  reversibility, undo, errors, and relationships.
- `agents/evaluator/profile.yaml` selects `parse_suite_config` from
  `agents/evaluator/builtin.yaml`. The active override declares only name,
  init, visibility, emits, description, and config.
- `agents/bench/profile.yaml` selects `serve_ui` from
  `agents/bench/builtin.yaml`. The active override declares only name, init,
  visibility, emits, description, and config.
- Evaluator point tools selected through `agents/evaluator/tools-point.yaml`
  mostly have side-effect and undo metadata, but many still lack prose contract
  fields and output schemas.
- Model-specific `invoke_llm` declarations under `agents/*/llm/*.yaml` are
  active profile-local configuration but mostly omit full word contract metadata.

Relevant source:

- `internal/tools/stl/tool_contracts.go` defines the contract validator fields:
  `problem`, `goals`, `requirements`, `non_goals`, `emits`, `output.schema`,
  `side_effects`, `reversibility`, `undo`, and `relationships`.
- `internal/tools/stl/configload_test.go` currently asserts that selected
  declarations still produce warn-only contract migration findings.

Follow-up: `agent-core-9iol.2`.

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

Top-level runtime invocation is profile-first, but child-agent boundaries still
use machine/tools/tool-declaration triples:

- `internal/support/execute/execute.go` models child execution as `Machine`,
  `Tools`, `ToolDeclarations`, `Model`, and `OllamaURL`, and `BuildArgs`
  emits `--machine`, `--tools`, `--tools-declaration`, `--model`, and
  `--ollama-url`.
- `agents/planner/builtin.yaml` configures `execute_task` with
  `machine: agents/generator/machine.yaml`, `tools: agents/generator/tools.yaml`,
  and `tools_declarations`.
- `agents/bench/builtin.yaml` configures `launch_eval` with
  `machine: agents/evaluator/machine.yaml`, `tools: agents/evaluator/tools.yaml`,
  and `tools_declarations`.
- `docs/specs/config-formats/runtime-contract.yaml` still describes child
  process boundaries as `--machine`/`--tools` subprocesses.

This weakens the pattern's boundary-actor rule because the canonical child
actor configuration is now an agent profile, not separate machine and tool
paths. The compatibility fields may remain, but child boundaries should accept
and prefer `profile`.

Follow-up: `agent-core-9iol.5`.

## Factory Registration Shape

The core engine is generic, but the composition root still contains hard-coded
tool-family knowledge:

- `cmd/agent/main.go` checks individual selected init names such as
  `file_read`, `invoke_llm`, `parse_response`, `validate`, and `self_invoke`.
- It registers domain families through repeated `anyInitSelected(...)` blocks
  for planner, evaluator, bench, and constitution-auditor factories.
- Adding a new domain tool family still requires editing `cmd/agent`, even when
  the selected tools are already declared in YAML.

This does not violate the current runtime contract, but it leaves part of the
lexicon assembly outside declarative configuration. A centralized factory
catalog keyed by init/category would better match "one binary serves N agents;
configuration provides specificity."

Follow-up: `agent-core-9iol.6`.

## Undo Strategy Vocabulary Gaps

`mage audit` succeeds, but warning-level `tool-undo-mismatch` findings remain
because `pkg/spec/checkToolUndoConsistency` only recognizes:

- reversible: `noop`, `reversible`, `snapshot_restore`
- compensatable: `compensatable`, `boundary_compensation`
- irreversible: `irreversible`

Active declarations use more specific strategies that are meaningful in the
rollback model, including:

- `workspace_restore`
- `session_state_restore`
- `conversation_truncate`
- `conversation_restore`
- `parse_retry_counter_restore`
- `child_command_undo`
- `compensating_action`

Those warnings are not currently blocking, but they reduce audit usefulness.
The audit rule should align with the structured undo vocabulary rather than
forcing declarations back to overly generic strategy names.

Follow-up: `agent-core-9iol.4`.

## Contract Enforcement Gap

`ValidateToolContracts` and `AuditToolContracts` exist in
`internal/tools/stl/tool_contracts.go`, but they are currently used by tests as
migration diagnostics rather than by the normal `mage audit` validation path.

Observed behavior:

- `mage audit` reports `OK` even while active selected tools have missing
  contract fields.
- `internal/tools/stl/configload_test.go` expects non-zero warn-only findings
  for current selected declarations.
- `pkg/spec.Validate` checks tool selection, side-effect vocabulary, emits, and
  undo consistency, but does not enforce the richer Grammar Machine word
  contract.

Follow-up: `agent-core-9iol.3` after `agent-core-9iol.2` fills or classifies
active declarations.

## Documentation Drift To Clean Up Last

Active docs should be refreshed after implementation changes so they describe
the final aligned design rather than today’s transitional state:

- `agents/evaluator/machine.yaml` still describes session iteration as
  `(harness, model, sample, gridpoint, rep)` and says evaluation is driven by
  `--machine agents/evaluator/machine.yaml`.
- `docs/specs/config-formats/runtime-contract.yaml` documents child processes
  in terms of machine/tool flags.
- `docs/ARCHITECTURE.yaml` and `docs/specs/semantic-models/tool-language.yaml`
  should mention strict selected-tool contract enforcement once audit wiring is
  implemented.

Follow-up: `agent-core-9iol.7`.
