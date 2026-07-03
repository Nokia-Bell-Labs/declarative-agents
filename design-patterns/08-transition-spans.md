# Transition Spans

This chapter presents the Transition Spans pattern, which maps each engine transition onto an OpenTelemetry span. Each transition becomes a span; the execution becomes a trace. The chapter covers span structure, trace-context propagation across agent boundaries, and how observers decouple from the engine through the registry.

## Intent

Map each engine transition to an OpenTelemetry span so the execution is observable with standard tooling without coupling the engine to any telemetry backend.


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

Transition Spans fits any agent execution that needs to be debuggable and auditable without log scraping. It becomes more valuable when per-step timing, token cost, and latency matter for evaluation; when multi-agent executions must correlate into a single distributed trace; and when comparative evaluation must attribute performance differences to the model rather than to instrumentation variance.


## Structure

The engine must not import OpenTelemetry; doing so would force the OTel dependency tree on every consumer of the engine library. Instead the engine defines a **Tracer port** (start/end span, record event, set attribute, propagate context) and adapters implement it. The component diagram in Fig. 21 shows the port and three interchangeable adapters; adapter selection is configuration, not code.

![](figures/fig-22-tracer-port.png)

| **Figure 21.** Component diagram. The engine requires the Tracer port; the OTel, NDJSON-file, and no-op adapters provide it and route telemetry to their backends. |
|:---:|

### Participants

#### Tracer port

Declares the operations the engine needs, knowing nothing of OTel SDK types.

#### OTel adapter

Implements the port via the OpenTelemetry Go SDK and configures OTLP/stdout/file exporters.

#### NDJSON file adapter

Writes spans as newline-delimited JSON, the CLI default, needing no collector.

#### No-op adapter

Discards telemetry for tests and benchmarks.

An execution produces a tree of four span types whose hierarchy mirrors the machine's structure, shown as an object diagram in Fig. 22.

#### `agent.run`

The root span (one per invocation), carrying profile, machine, and budget attributes.

#### `invoke_agent`

Engine-cycle iteration spans.

#### `chat`

LLM invocation spans.

#### `execute_tool`

Tool dispatch spans, each naming the tool and recording its returned signal.

![](figures/fig-23-span-tree.png)

| **Figure 22.** Object diagram. The span tree of one execution: a root `agent.run` contains per-iteration `invoke_agent` spans, each parent to the `chat` and `execute_tool` spans of that iteration. |
|:---:|


## Collaborations

### Mapping concepts to spans

The tuple $(state, signal, tool, result)$ maps onto OTel primitives with minimal impedance. **States become span events.** Each is a timestamped annotation on the current `invoke_agent` span carrying the triggering signal and the tool to dispatch. **Signals become span attributes** on the `execute_tool` span (`machine.signal`, `machine.state.from`, `machine.state.to`), indexed and queryable. **Tools become `execute_tool` spans** whose duration captures wall-clock execution the execution cannot represent. For a `$tool` dispatch, the engine creates the span with a generic name and overrides it with the resolved tool name before `Execute` begins, so analysis sees the real tool.

### Distributed tracing across boundaries

When an agent spawns a child via `run_agent`, the child's execution must join the parent's trace. The sequence diagram in Fig. 23 shows the W3C Trace Context [@w3c-trace-context-2021] propagation: the parent extracts a `traceparent` (`00-{trace_id}-{span_id}-{flags}`) from its `execute_task` span and passes it to the child, which roots its own `agent.run` under the parent span. Both share a trace ID; each writes its own trace file (namespaced by profile and timestamp), and a collector ingesting both reconstructs the full distributed trace.

![](figures/fig-24-trace-propagation.png)

| **Figure 23.** Sequence diagram. Trace-context propagation across an agent boundary: the parent passes a W3C `traceparent` when spawning the child, linking the child's `agent.run` into the parent trace. {wide} |
|:---:|

When the child finishes, the parent's `execute_task` span records the child's terminal state and convergence class; the parent machine sees a single `ToolDone`/`ToolFailed` signal with the full child execution preserved underneath.


## Consequences

### Benefits

#### Backend independence

The engine depends only on the Tracer port; switching from file tracing to OTLP export is a YAML change, and library consumers inherit no OTel dependency.

#### Standard tooling and quantitative analysis

Traces are searchable and visualizable in Jaeger, Tempo, or Honeycomb, and yield metrics the execution cannot: wall-clock time per tool, LLM latency per iteration, token cost per run, recovery cost (the delta from first `ValidationFailed` to eventual `Succeeded`).

#### Model-attributable evaluation

With a fixed harness and standardized trace format, performance differences across model backends are attributable to the model, with instrumentation held constant.

### Liabilities

#### Span volume

Per-iteration `invoke_agent` spans preserve timing granularity but multiply span count for agents that run hundreds of iterations; the budget mechanism bounds the worst case.

#### Derived, not real-time, metrics

Metrics are computed from spans in the analysis layer rather than emitted as a primary signal, consistent by construction but available only after ingestion. Real-time workloads can add a metrics adapter alongside the trace adapter without changing the engine.

#### Legibility

NDJSON span records are machine-readable but less glanceable than log lines; `jq` and OTel viewers mitigate this.


## Implementation

LLM `chat` spans carry the OpenTelemetry GenAI semantic conventions [@otel-genai-semconv-2025], making inference telemetry queryable across vendors: `gen_ai.system`, `gen_ai.request.model`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, and `gen_ai.response.finish_reasons`. Finish reasons bridge the two models: `tool_calls` maps to the machine's `ToolCall` signal, `stop` to `Completion`, and `length` to a retry or `BudgetExceeded`. Token counts aggregate at the `agent.run` root for a single cost-per-run number.

`SpanOverride` relabels the `invoke_llm` span as `gen_ai.chat` and rewrites a `$tool` span with its resolved name; OTel spans are mutable until ended. Adapter selection lives in profile runtime settings; CLI runs default to the self-contained NDJSON file adapter, and production switches to OTLP for real-time dashboards.


## Relationships in the Pattern Language

Transition Spans sits within Machine Interpreter and requires Machine Interpreter: the trace exists because the engine has declared states, signals, tools, and iterations to project into spans. It enables Convergence Taxonomy, which reads structured execution traces, and Boundary Tool, which propagates trace context into child executions. It overlaps Operator Port because both expose running behaviour, but spans are observational and post-hoc while Operator Port also accepts controlled intervention. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Grid evaluation.** The bench/evaluator stack (Chapter 9) consumes trace files, not running agents; each is an NDJSON span sequence readable without an OTel SDK. Convergence classes map directly to span patterns (e.g. a `Validating`→`Composing` event sequence ending in `Succeeded` is Recovery), and per-run cost and latency are read from span attributes.

**Local debugging.** A developer running one agent gets a self-contained NDJSON trace file (no collector, no network) inspectable with `jq` or an OTel desktop viewer.

**Production monitoring.** Deployments switch the adapter to OTLP via profile config, gaining real-time dashboards and cross-service correlation while the engine and machine stay unchanged.

**Dapper** [@sigelman-dapper-2010]. Google's large-scale distributed tracing infrastructure established span trees and context propagation as the way to observe distributed executions, the lineage this pattern inherits when it maps transitions onto spans and propagates trace context into child agents.
