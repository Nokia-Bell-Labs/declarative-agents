<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Transition Spans

Transition Spans map executions to OpenTelemetry run and dispatch spans,
updating run attributes with transition outcomes and creating child spans for
tool and model dispatches. This yields the shipped topology, trace-context
propagation, and post-run evaluation metrics.

## Intent

We instrument run and dispatch boundaries through a tracer port, thereby
making execution observable with standard tooling while decoupling the engine
from any telemetry backend.

## Motivation

Agent execution remains opaque; without instrumentation, debugging and
evaluation depend on ad-hoc logging. The Machine Interpreter produces the
execution trace—a deterministic, replayable, and reversible sequence of
$(state, signal, tool, result)$ tuples—Chapter 2—. Evaluators use this trace
to classify a run, yet operators require a timed, hierarchical, cross-service
view that can be visualized in standard backends. An OpenTelemetry trace
[@otel-spec-2024]—a directed-acyclic graph of timed spans with attributes and
propagation context—delivers precisely that view.

| Property | Execution | OTel trace |
|---|---|---|
| Structure | Flat tuple sequence | Hierarchical span tree |
| Timing | Ordering only | Wall-clock per span |
| Reversibility | Full (`Undo` per tool) | None (read-only) |
| Cross-service | None | W3C Trace Context |
| Primary consumer | Evaluator, auditor | Operator, SRE |

The semantic and operational records differ, yet they share the same execution
origin. The pattern maps Machine Interpreter concepts to OTel's span model
without embedding OTel directly into the engine.

## Applicability

Transition Spans improve debuggability and auditability for any agent
execution, eliminating the need for log scraping. Their value increases when
evaluating tool duration, run duration, token counts, and model latency. When
correlating multi-agent executions into a single distributed trace; and when
attributing performance differences to the model, not to instrumentation
variance.

## Structure

The engine relies on a Tracer port that offers a limited set of
operations—start/end child span, record event, set attribute, expose context—.
Adapters manage recording and export tasks, preserving semantic compatibility
through OpenTelemetry attribute and context types. Adapters also handle
provider, processor, and exporter configuration. Fig. 21 illustrates three
interchangeable adapters.

![](figures/fig-22-tracer-port.png)

| Figure 21. Component diagram. The engine requires the Tracer port; the OTel, NDJSON-file, and no-op adapters provide it and route telemetry to their backends. |
|:---:|

### Participants

#### Tracer port

It defines only the operations the engine needs, deliberately excluding any
provider, processor, or exporter policy.

#### OTel adapter

The port uses the OpenTelemetry Go SDK and configures exporters for OTLP,
stdout, and file.

#### NDJSON file adapter

It writes spans as newline-delimited JSON, which is the CLI default and
requires no collector.

#### No-op adapter

It discards telemetry, supporting tests and benchmarks.

The generic loop generates one run span and one direct child per dispatched
command—Fig. 22—. Model invocation appears as a specialized dispatch span.

#### `invoke_agent <name>`

The run root, created once per loop invocation, carries the GenAI agent's
attributes: run identity, budget, final status, final state, iteration count,
token totals, and duration.

#### `chat <model>`

The `invoke_llm` dispatch replaces the generic tool span with the GenAI `chat`
operation, adding model, provider, and server attributes.

#### `execute_tool <name>`

Each subsequent dispatch span names the tool, records the command signal,
duration, token usage, errors, and any declared tool metrics.

![](figures/fig-23-span-tree.png)

| Figure 22. Object diagram of the shipped topology. `chat` and `execute_tool` dispatches are direct children of the `invoke_agent` run span; no per-iteration spans are created. |
|:---:|

## Collaborations

### Mapping concepts to spans

The tuple $(state, signal, tool, result)$ maps onto two span levels. The run
span tracks `iteration`, `command`, `signal`, `from_state`, `to_state`, and
per-iteration token attributes; repeated attributes hold the latest
transition, while `run.iterations` stores the final count. Each dispatch child
logs `command.name`, `command.signal`, `command.duration_ms`, token usage,
errors, and optional `tool.metrics.{total,passed,failed}`. A `$tool` dispatch
resolves the command before tracing, using the real tool name in the child
span. The semantic execution log therefore retains the complete transition
sequence, separate from the run span.

### Distributed tracing across boundaries

When an agent spawns a child via `run_agent`, the child's execution must join
the parent's trace. Fig. 23 shows W3C Trace Context [@w3c-trace-context-2021]
propagation: the parent extracts a `traceparent`
(`00-{trace_id}-{span_id}-{flags}`) from its `execute_task` span and passes it
to the child, anchoring the child's `agent.run` span under the parent's span.
Both share a trace ID, generate their own trace files (namespaced by profile
and timestamp), and allow a collector to reconstruct the full distributed
trace from the combined files.

![](figures/fig-24-trace-propagation.png)

| Figure 23. Sequence diagram. Trace-context propagation across an agent boundary: the parent passes a W3C `traceparent` when spawning the child, linking the child's `agent.run` into the parent trace. {wide} |
|:---:|

The parent boundary span records the child's command outcome and duration
after completion. The child writes its linked trace; convergence is later
computed from evaluation artifacts, not stamped onto the boundary span.

## Consequences

### Benefits

#### Backend independence

Runtime behavior depends solely on the tracer port. Switching from file
tracing to OTLP export preserves the machine and dispatch loop.

#### Standard tooling and quantitative analysis

Traces become searchable and visualizable in Jaeger, Tempo, or Honeycomb.
Shipped evaluation reports include runs, successes,
success/clean/recovery/stuck rates, mean iterations, mean input/output tokens,
and mean duration. Tool metric snapshots expose total/passed/failed counts for
selected test, build, and edit tools. Monetary cost and recovery-cost duration
are not reported.

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
structured tool metric snapshots after artifact ingestion. Real-time workloads
can add a metrics adapter alongside the trace adapter, leaving the engine
unchanged.

#### Legibility

NDJSON span records, while machine-readable, are less glanceable than
traditional log lines. Tools such as `jq` and OTel viewers mitigate this
limitation.

## Implementation

The runtime adopts OpenTelemetry GenAI semantic-convention operation names
[@otel-genai-semconv-2025], specifically `invoke_agent`, `execute_tool`, and
`chat`. Creation attributes include `gen_ai.operation.name`, provider, agent
or tool name, model, tool type, and server address where applicable. On
completion, the span records `gen_ai.usage.input_tokens`,
`gen_ai.usage.output_tokens`, and `gen_ai.response.finish_reasons` from the
terminal status. Token counts therefore measure usage rather than monetary
cost.

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
observational and post-hoc, whereas the Operator Port enables controlled
intervention. The grammar is defined in `pattern-language.yaml`.

## Known Uses

Grid evaluation. The bench/evaluator stack (Chapter 9) processes trace files
rather than running agents. Reports summarize iterations, tokens, duration,
success, and progression rates; convergence relies on tool-metric snapshots
rather than iteration-span sequences.

Local debugging. Running a single agent generates a standalone NDJSON trace
file, bypassing the collector and network. Inspect this file with `jq` or an
OTel desktop viewer.

Production monitoring. Switching the adapter to OTLP via profile configuration
enables real-time dashboards and cross-service correlation without altering
the engine or machine.

Dapper [@sigelman-dapper-2010], Google's large-scale distributed tracing
infrastructure, established span trees and context propagation as the primary
method for observing distributed executions. This approach maps transitions
onto spans and propagates trace context into child agents, maintaining the
lineage of the execution pattern.