<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Transition Spans

Transition Spans map executions to OpenTelemetry run and dispatch spans,
updating run attributes with transition outcomes and creating child spans for
tool and model dispatches. This yields the shipped topology, trace-context
propagation, and post-run evaluation metrics.

## Intent

Instrument run and dispatch boundaries via a tracer port, making execution
observable with standard tooling, decoupling the engine from any telemetry
backend.


## Motivation

Agent execution is opaque: without instrumentation, debugging and evaluation
rely on ad-hoc logging. The Machine Interpreter produces the **execution**
trace, a deterministic, replayable, and reversible sequence of $(state,
signal, tool, result)$ tuples (Chapter 2). This is what an *evaluator* uses to
classify a run. An *operator*, however, needs a timed, hierarchical,
cross-service view, visualizable in standard backends. An **OpenTelemetry
trace** [@otel-spec-2024], a DAG of timed spans with attributes and
propagation context, provides this view.

| Property | Execution | OTel trace |
|---|---|---|
| Structure | Flat tuple sequence | Hierarchical span tree |
| Timing | Ordering only | Wall-clock per span |
| Reversibility | Full (`Undo` per tool) | None (read-only) |
| Cross-service | None | W3C Trace Context |
| Primary consumer | Evaluator, auditor | Operator, SRE |

The semantic and operational records are distinct but share the same execution
origin. The pattern maps Machine Interpreter concepts to OTel's span model,
without integrating OTel into the engine.


## Applicability

Transition Spans support debuggability and auditability in any agent execution
without log scraping. Their value grows when evaluating tool duration, run
duration, token counts, and model latency. When correlating multi-agent
executions into a single distributed trace; and when attributing performance
differences to the model, not instrumentation variance.


## Structure

The engine uses a **Tracer port** with limited functionality (start/end child
span, record event, set attribute, expose context), while adapters manage
recording and export tasks. The port maintains semantic compatibility through
OpenTelemetry attribute and context types, and adapters handle provider,
processor, and exporter configuration. Fig. 21 shows three interchangeable
adapters.

![](figures/fig-22-tracer-port.png)

| **Figure 21.** Component diagram. The engine requires the Tracer port; the OTel, NDJSON-file, and no-op adapters provide it and route telemetry to their backends. |
|:---:|

### Participants

#### Tracer port

It specifies only the engine's required operations, excluding any provider,
processor, or exporter policy.

#### OTel adapter

The port uses the OpenTelemetry Go SDK, configuring exporters for OTLP, stdout, and file.

#### NDJSON file adapter

Writes spans as newline-delimited JSON, the CLI default, needing no collector.

#### No-op adapter

Discards telemetry for tests and benchmarks.

The generic loop generates one run span and one direct child per dispatched
command (Fig. 22). Model invocation is a specialized dispatch span.

#### `invoke_agent <name>`

The run root, one per loop invocation, carries the GenAI agent's attributes:
run identity, budget, final status, final state, iteration count, token
totals, and duration.

#### `chat <model>`

The `invoke_llm` dispatch replaces the generic tool span with the GenAI `chat`
operation, incorporating model, provider, and server attributes.

#### `execute_tool <name>`

Each dispatch span beyond the initial one names the tool, records the command
signal, duration, token usage, errors, and declared tool metrics.

![](figures/fig-23-span-tree.png)

| **Figure 22.** Object diagram of the shipped topology. `chat` and `execute_tool` dispatches are direct children of the `invoke_agent` run span; no per-iteration spans are created. |
|:---:|


## Collaborations

### Mapping concepts to spans

The tuple $(state, signal, tool, result)$ maps onto two levels. The run span
tracks `iteration`, `command`, `signal`, `from_state`, `to_state`, and
per-iteration token attributes; repeated attributes hold the latest
transition, and `run.iterations` stores the final count. Each dispatch child
logs `command.name`, `command.signal`, `command.duration_ms`, token usage,
errors, and optional `tool.metrics.{total,passed,failed}`. A `$tool` dispatch
resolves the command before tracing, using the real tool name in the child
span. The semantic execution log retains the complete transition sequence, not
the run span.

### Distributed tracing across boundaries

An agent spawning a child via `run_agent` requires the child's execution to
join the parent's trace. Fig. 23 shows W3C Trace Context
[@w3c-trace-context-2021] propagation: the parent extracts a `traceparent`
(`00-{trace_id}-{span_id}-{flags}`) from its `execute_task` span and passes it
to the child, anchoring the child's `agent.run` span under the parent's span.
Both share a trace ID, generate their own trace files (namespaced by profile
and timestamp), allowing a collector to reconstruct the full distributed trace
from both files.

![](figures/fig-24-trace-propagation.png)

| **Figure 23.** Sequence diagram. Trace-context propagation across an agent boundary: the parent passes a W3C `traceparent` when spawning the child, linking the child's `agent.run` into the parent trace. {wide} |
|:---:|

The parent boundary span records the child's command outcome and duration upon
completion. The child writes its linked trace. Convergence is later computed
from evaluation artifacts rather than stamped onto the boundary span.


## Consequences

### Benefits

#### Backend independence

Runtime behavior depends solely on the tracer port. Switching from file
tracing to OTLP export preserves the machine and dispatch loop.

#### Standard tooling and quantitative analysis

Traces are searchable and visualizable in Jaeger, Tempo, or Honeycomb. Shipped
evaluation reports include runs, successes, success/clean/recovery/stuck
rates, mean iterations, mean input/output tokens, and mean duration. Tool
metric snapshots expose total/passed/failed counts for selected test, build,
and edit tools. Monetary cost and recovery-cost duration are not reported.

#### Model-attributable evaluation

With a fixed harness and standardized trace format, performance differences
across model backends stem from the model itself, assuming constant
instrumentation.

### Liabilities

#### Span volume

One child span is emitted per dispatch; long runs still increase span volume
even without per-iteration spans. The budget mechanism bounds the worst case.

#### Derived, not real-time, metrics

Evaluation metrics derive from run metadata, span token attributes, and
structured tool metric snapshots post-artifact ingestion. Real-time workloads
can add a metrics adapter alongside the trace adapter, leaving the engine
unchanged.

#### Legibility

NDJSON span records, though machine-readable, are less glanceable than
traditional log lines. Tools like `jq` and OTel viewers address this
limitation.


## Implementation

The runtime uses OpenTelemetry GenAI semantic-convention operation names
[@otel-genai-semconv-2025], specifically `invoke_agent`, `execute_tool`, and
`chat`. Creation attributes include `gen_ai.operation.name`, provider, agent
or tool name, model, tool type, and server address where applicable. On
completion, the span records `gen_ai.usage.input_tokens`,
`gen_ai.usage.output_tokens`, and `gen_ai.response.finish_reasons` from the
terminal status. Token counts measure usage rather than monetary cost.

`SpanOverride` renames `invoke_llm` to `chat <model>` during initialization.
Adapter selection is managed in profile runtime settings. CLI runs produce
self-contained NDJSON, while deployments can export data via OTLP.


## Relationships in the Pattern Language

Transition Spans reside within the Machine Interpreter, relying on its
functionality to generate traces when the engine instruments declared runs and
dispatches. This component supports the Convergence Taxonomy, which processes
structured trace artifacts and tool snapshots, and the Boundary Tool, whose
subprocess adapters propagate trace context. Transition Spans overlap with the
Operator Port, both exposing running behavior; however, spans are
observational and post-hoc, while the Operator Port allows controlled
intervention. The grammar is defined in `pattern-language.yaml`.


## Known Uses

**Grid evaluation.** The bench/evaluator stack (Chapter 9) processes trace
files rather than running agents. Reports summarize iterations, tokens,
duration, success, and progression rates; convergence relies on tool-metric
snapshots rather than iteration-span sequences.

**Local debugging.** Running a single agent generates a standalone NDJSON
trace file, bypassing the collector and network. Inspect this file with `jq`
or an OTel desktop viewer.

**Production monitoring** transitions the adapter to OTLP via profile
configuration, enabling real-time dashboards and cross-service correlation
without changing the engine or machine.

**Dapper** [@sigelman-dapper-2010], Google's large-scale distributed tracing
infrastructure, established span trees and context propagation as the primary
method for observing distributed executions. This approach maps transitions
onto spans and propagates trace context into child agents, maintaining the
lineage of the execution pattern.
