---
id: srd001-core-types
title: Core Types
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Every agentic loop needs a common vocabulary: executable units of work,
their outcomes, the signals those outcomes carry, and the cost of
execution. Without shared types, each agent reimplements these concepts
with incompatible interfaces. Agent-core defines these types once so
domain agents import them rather than redefine them.

<!-- field: goals -->
## Goals

- G1: Domain-agnostic types for commands, results, signals, and costs.
- G2: Interfaces for command construction (Builder) and lookup (CommandResolver).
- G3: Foundation types for typed checkpoints, execution history, and tool-owned receipts.

<!-- req: R1 -->
## R1 - State and Signal types

<!-- req: R1.1 -->
### R1.1

State is a named string type representing a position in the agentic loop lifecycle.

<!-- req: R1.2 -->
### R1.2

Signal is a named string type carrying the outcome of a Command back to the state machine.

<!-- req: R1.3 -->
### R1.3

Three generic Signal constants are defined: Seed, BudgetExhausted, CommandError.

<!-- req: R1.4 -->
### R1.4

Three standard tool Signal constants are defined: ToolDone, ToolFailed, EditDone.

<!-- req: R1.5 -->
### R1.5

Three LLM Signal constants are defined: LLMResponded, ParseFailed, TaskCompleted.

<!-- req: R2 -->
## R2 - Command interface

<!-- req: R2.1 -->
### R2.1

Command is an interface with three methods -- Name() string, Execute() Result, and Undo() Result.

<!-- req: R2.2 -->
### R2.2

Commands embed their parameters at creation time. Execute takes no domain parameters.

<!-- req: R2.3 -->
### R2.3

NoopUndo returns a successful ToolDone Result for commands whose rollback behavior is not yet specialized.

<!-- req: R3 -->
## R3 - Cost struct

<!-- req: R3.1 -->
### R3.1

Cost has four fields: Duration (time.Duration), TokensIn (int), TokensOut (int), Dollars (float64).

<!-- req: R3.2 -->
### R3.2

All Cost fields carry JSON tags for serialization.

<!-- req: R4 -->
## R4 - Result struct

<!-- req: R4.1 -->
### R4.1

Result has six fields: Output (string), Signal (Signal), Cost (Cost), Err (error), CommandName (string), Metrics (*ToolMetrics).

<!-- req: R4.2 -->
### R4.2

Err carries infrastructure errors. Signal carries domain outcomes.

<!-- req: R4.3 -->
### R4.3

Metrics is nil when the tool does not report structured metrics.

<!-- req: R7 -->
## R7 - ToolMetrics struct

<!-- req: R7.1 -->
### R7.1

ToolMetrics has four fields: Total (int), Passed (int), Failed (int), Details (map[string]any).

<!-- req: R7.2 -->
### R7.2

All ToolMetrics fields carry JSON tags for serialization. Details uses omitempty.

<!-- req: R7.3 -->
### R7.3

ToolMetrics enables structured progress tracking (e.g. test counts) without parsing free text.

<!-- req: R5 -->
## R5 - Builder and CommandResolver interfaces

<!-- req: R5.1 -->
### R5.1

Builder interface has Build(res Result) Command. It constructs a ready-to-execute Command from the previous Result.

<!-- req: R5.2 -->
### R5.2

CommandResolver interface has Resolve(name string) (Builder, bool). It looks up a Builder by command name.

<!-- req: R6 -->
## R6 - SpanOverride interface

<!-- req: R6.1 -->
### R6.1

SpanOverride allows Commands to customize the Dispatch span name and creation-time attributes.

<!-- req: R6.2 -->
### R6.2

SpanOverride has two methods: SpanName() string and SpanCreationAttrs() []attribute.KeyValue.

<!-- req: R6.3 -->
### R6.3

Without SpanOverride, Dispatch defaults to GenAI semconv execute_tool naming.

<!-- req: R8 -->
## R8 - Checkpoint foundation types

<!-- req: R8.1 -->
### R8.1

Checkpoint persists runtime state with Save(Position, Execution) and restores it with Load.

<!-- req: R8.2 -->
### R8.2

Position records CurrentState, LastSignal, and AgentSnapshot for resumable loop state.

<!-- req: R8.3 -->
### R8.3

Execution is an ordered slice of Entry values preserving dispatch history.

<!-- req: R8.4 -->
### R8.4

Each Entry records command, state transition, signal, result digest, and opaque Receipt.

<!-- req: R8.5 -->
### R8.5

NoopCheckpoint implements disabled-mode persistence without recording state.

<!-- req: R8.6 -->
### R8.6

CheckpointReverter extends persistent backends that can Revert(run_id, step_index), such as DoltCheckpoint.

<!-- field: non_goals -->
## Non-goals

- Domain-specific state or signal constants.
- Command implementations (those live in domain agents).
- JSON serialization of Result (Result.Err is not serializable by default).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

State and Signal are named string types. Generic signals (Seed, BudgetExhausted, CommandError), tool signals (ToolDone, ToolFailed, EditDone), and LLM signals (LLMResponded, ParseFailed, TaskCompleted) are exported.

<!-- field: traces -->
#### Traces

- R1.1
- R1.2
- R1.3
- R1.4
- R1.5

<!-- ac: AC2 -->
### AC2

Command interface has Name(), Execute(), and Undo() methods. Execute and Undo return Result.

<!-- field: traces -->
#### Traces

- R2.1
- R2.2
- R2.3

<!-- ac: AC3 -->
### AC3

Cost struct has Duration, TokensIn, TokensOut, and Dollars fields with JSON tags.

<!-- field: traces -->
#### Traces

- R3.1
- R3.2

<!-- ac: AC4 -->
### AC4

Result struct has Output, Signal, Cost, Err, CommandName, and Metrics (*ToolMetrics) fields. Metrics is nil when not reported.

<!-- field: traces -->
#### Traces

- R4.1
- R4.2
- R4.3

<!-- ac: AC7 -->
### AC7

ToolMetrics struct has Total, Passed, Failed (int) and Details (map[string]any) with JSON tags. Details uses omitempty.

<!-- field: traces -->
#### Traces

- R7.1
- R7.2
- R7.3

<!-- ac: AC5 -->
### AC5

Builder has Build(Result) Command. CommandResolver has Resolve(string) (Builder, bool).

<!-- field: traces -->
#### Traces

- R5.1
- R5.2

<!-- ac: AC6 -->
### AC6

SpanOverride interface has SpanName() and SpanCreationAttrs() for customizing Dispatch span naming.

<!-- field: traces -->
#### Traces

- R6.1
- R6.2
- R6.3

<!-- ac: AC8 -->
### AC8

Checkpoint foundation types model resumable position, ordered execution history, opaque receipts, no-op persistence, and revert-capable persistent backends.

<!-- field: traces -->
#### Traces

- R8.1
- R8.2
- R8.3
- R8.4
- R8.5
- R8.6

<!-- field: diagrams -->
## Diagrams

````yaml
requirements_trace: |
    ```mermaid
    flowchart LR
      Problem["Problem: Every agentic loop needs a common vocabulary: executa..."] --> Goals["Goals: Core Types"]
      Goals --> R1["R1: State and Signal types"]
      Goals --> R2["R2: Command interface"]
      Goals --> R3["R3: Cost struct"]
      Goals --> R4["R4: Result struct"]
      Goals --> R7["R7: ToolMetrics struct"]
      Goals --> R5["R5: Builder and CommandResolver interfaces"]
      Goals --> R6["R6: SpanOverride interface"]
      Goals --> R8["R8: Rollback foundation types"]
      AC1["AC1: State and Signal are named string types. Generic signals..."]
      R1 --> AC1
      AC2["AC2: Command interface has Name(), Execute(), and Undo() metho..."]
      R2 --> AC2
      AC3["AC3: Cost struct has Duration, TokensIn, TokensOut, and Dollar..."]
      R3 --> AC3
      AC4["AC4: Result struct has Output, Signal, Cost, Err, CommandName,..."]
      R4 --> AC4
      AC7["AC7: ToolMetrics struct has Total, Passed, Failed (int) and De..."]
      R7 --> AC7
      AC5["AC5: Builder has Build(Result) Command. CommandResolver has Re..."]
      R5 --> AC5
      AC6["AC6: SpanOverride interface has SpanName() and SpanCreationAtt..."]
      R6 --> AC6
      AC8["AC8: Checkpoint foundation types, checkpoint structs, and G..."]
      R8 --> AC8
    ```
````
