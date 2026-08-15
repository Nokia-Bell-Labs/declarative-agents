<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Transition Spans

This chapter presents the Transition Spans pattern, which projects an execution onto OpenTelemetry run and dispatch spans. Transition outcomes update run attributes; tool and model dispatches create child spans. The chapter covers the shipped topology, trace-context propagation, and post-run evaluation metrics.

## Intent

Instrument run and dispatch boundaries through a tracer port so execution is observable with standard tooling without coupling the engine to any telemetry backend.


## Motivation

Agent execution is opaque: without instrumentation, debugging and evaluation fall back on ad-hoc logging. The Machine Interpreter already produces one trace, the **execution**, a sequence of $(state, signal, tool, result)$ tuples that is deterministic, replayable, and reversible (Chapter 2). That is the artifact an *evaluator* reads to classify a run. But an *operator* needs a different view: timed, hierarchical, cross-service, and visualizable in standard backends. An **OpenTelemetry trace** [@otel-spec-2024], a DAG of timed spans with attributes and propagation context, is that view.

| Property | Execution | OTel trace |
|---|---|---|
| Structure | Flat tuple sequence | Hierarchical span tree |
| Timing | Ordering only | Wall-clock per span |
| Reversibility | Full (`Undo` per tool) | None (read-only) |
| Cross-service | None | W3C Trace Context |
| Primary consumer | Evaluator, auditor | Operator, SRE |

Neither replaces the other. One is the semantic record, the other the operational record, and both come from the same execution. The pattern maps Machine Interpreter concepts onto OTel's span model without importing OTel into the engine.


## Applicability

Transition Spans fits any agent execution that needs to be debuggable and auditable without log scraping. It becomes more valuable when tool duration, run duration, token counts, and model latency matter for evaluation; when multi-agent executions must correlate into a single distributed trace; and when comparative evaluation must attribute performance differences to the model rather than to instrumentation variance.


## Structure

The engine depends on a narrow **Tracer port** (start/end child span, record event, set attribute, expose context), and adapters implement recording/export. The port's OpenTelemetry attribute and context types preserve semantic compatibility while provider, processor, and exporter configuration stays in adapters. The component diagram in Fig. 21 shows three interchangeable adapters.

![](figures/fig-22-tracer-port.png)

| **Figure 21.** Component diagram. The engine requires the Tracer port; the OTel, NDJSON-file, and no-op adapters provide it and route telemetry to their backends. |
|:---:|

### Participants

#### Tracer port

Declares only the operations the engine needs and carries no provider, processor, or exporter policy.

#### OTel adapter

Implements the port via the OpenTelemetry Go SDK and configures OTLP/stdout/file exporters.

#### NDJSON file adapter

Writes spans as newline-delimited JSON, the CLI default, needing no collector.

#### No-op adapter

Discards telemetry for tests and benchmarks.

The generic loop produces one run span and one direct child per dispatched command, shown in Fig. 22. Model invocation is a specialized dispatch span.

#### `invoke_agent <name>`

The run root (one per loop invocation), carrying GenAI agent attributes, run identity, budget, final status, final state, iteration count, token totals, and duration.

#### `chat <model>`

The `invoke_llm` dispatch overrides the generic tool span with the GenAI `chat` operation and model/provider/server attributes.

#### `execute_tool <name>`

All other dispatch spans, each naming the tool and recording command signal, duration, token usage, errors, and declared tool metrics.

![](figures/fig-23-span-tree.png)

| **Figure 22.** Object diagram of the shipped topology. `chat` and `execute_tool` dispatches are direct children of the `invoke_agent` run span; no per-iteration spans are created. |
|:---:|


## Collaborations

### Mapping concepts to spans

The tuple $(state, signal, tool, result)$ maps onto two levels. The run span receives `iteration`, `command`, `signal`, `from_state`, `to_state`, and per-iteration token attributes as the loop advances; repeated attribute names hold the latest transition, while `run.iterations` records the final count. Each dispatch child records `command.name`, `command.signal`, `command.duration_ms`, token usage, errors, and optional `tool.metrics.{total,passed,failed}`. A `$tool` dispatch resolves the command before tracing, so the child span uses the real tool name. The semantic execution log, not the run span, remains the complete transition sequence.

### Distributed tracing across boundaries

When an agent spawns a child via `run_agent`, the child's execution must join the parent's trace. The sequence diagram in Fig. 23 shows the W3C Trace Context [@w3c-trace-context-2021] propagation: the parent extracts a `traceparent` (`00-{trace_id}-{span_id}-{flags}`) from its `execute_task` span and passes it to the child, which roots its own `agent.run` under the parent span. Both share a trace ID; each writes its own trace file (namespaced by profile and timestamp), and a collector ingesting both reconstructs the full distributed trace.

![](figures/fig-24-trace-propagation.png)

| **Figure 23.** Sequence diagram. Trace-context propagation across an agent boundary: the parent passes a W3C `traceparent` when spawning the child, linking the child's `agent.run` into the parent trace. {wide} |
|:---:|

When the child finishes, the parent boundary span records its command outcome and duration. The child writes its own linked trace. Convergence is computed later from evaluation artifacts, not stamped onto the boundary span.


## Consequences

### Benefits

#### Backend independence

Runtime behavior depends only on the Tracer port; switching from file tracing to OTLP export does not change the machine or dispatch loop.

#### Standard tooling and quantitative analysis

Traces are searchable and visualizable in Jaeger, Tempo, or Honeycomb. Shipped evaluation reports runs, successes, success/clean/recovery/stuck rates, mean iterations, mean input/output tokens, and mean duration. Tool metric snapshots expose total/passed/failed counts for selected test, build, and edit tools. Monetary cost and a computed recovery-cost duration are not reported.

#### Model-attributable evaluation

With a fixed harness and standardized trace format, performance differences across model backends are attributable to the model, with instrumentation held constant.

### Liabilities

#### Span volume

One child span is emitted per dispatch, so long runs still increase span volume even without per-iteration spans; the budget mechanism bounds the worst case.

#### Derived, not real-time, metrics

Evaluation metrics are computed after artifact ingestion from run metadata, span token attributes, and structured tool metric snapshots. Real-time workloads can add a metrics adapter alongside the trace adapter without changing the engine.

#### Legibility

NDJSON span records are machine-readable but less glanceable than log lines; `jq` and OTel viewers mitigate this.


## Implementation

The runtime uses OpenTelemetry GenAI semantic-convention operation names [@otel-genai-semconv-2025]: `invoke_agent`, `execute_tool`, and `chat`. Creation attributes include `gen_ai.operation.name`, provider, agent/tool name, model, tool type, and server address where applicable. Completion stamps `gen_ai.usage.input_tokens` and `gen_ai.usage.output_tokens`; the run span also records `gen_ai.response.finish_reasons` from terminal status. Token counts measure usage and do not represent monetary cost.

`SpanOverride` relabels the `invoke_llm` dispatch as `chat <model>` at creation. Adapter selection lives in profile runtime settings; CLI runs can write self-contained NDJSON, while deployments may export through OTLP.


## Relationships in the Pattern Language

Transition Spans sits within Machine Interpreter and requires Machine Interpreter: the trace exists because the engine has declared runs and dispatches to instrument. It enables Convergence Taxonomy, which reads structured trace artifacts and tool snapshots, and Boundary Tool, whose subprocess adapters propagate trace context. It overlaps Operator Port because both expose running behaviour, but spans are observational and post-hoc while Operator Port can add controlled intervention. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Grid evaluation.** The bench/evaluator stack (Chapter 9) consumes trace files, not running agents. Reports aggregate iterations, tokens, duration, success, and progression rates; convergence reads tool-metric snapshots rather than mapping a nonexistent iteration-span sequence.

**Local debugging.** A developer running one agent gets a self-contained NDJSON trace file (no collector, no network) inspectable with `jq` or an OTel desktop viewer.

**Production monitoring.** Deployments switch the adapter to OTLP via profile config, gaining real-time dashboards and cross-service correlation while the engine and machine stay unchanged.

**Dapper** [@sigelman-dapper-2010]. Google's large-scale distributed tracing infrastructure established span trees and context propagation as the way to observe distributed executions, the lineage this pattern inherits when it maps transitions onto spans and propagates trace context into child agents.
