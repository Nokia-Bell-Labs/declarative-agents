<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Operator Port

An Operator Port couples an observation surface and a control surface with a
running engine, while preserving three elements: the shipped monitor profile,
runtime-only lifecycle-control conformance, and signal-injection plus rollback
design intent.

## Intent

The goal is to attach a control-plane server to the running engine so that
observers can query execution state and controllers can inject signals, all
within the declared state space.

## Reference implementation status

The shipped `applications/catalog/agents/runtime-state-reader` adapter
implements read routes for the root machine, declared machines, state, tools,
metrics, recent events, event SSE, and OpenAPI. It does not expose its own
control route; instead, `agent-core` injects the `POST /api/lifecycle/exit`
endpoint—GH-1264— into every agent, emitting `ExitRequested` and allowing the
profile's control logic to await this route. The listener binds to
`127.0.0.1:0`, and supervisors retrieve the address from the REST launch
output.

The REST runtime contains conformance-tested `lifecycle_control` and
`inject_signal` bindings, although no production profile selects them. Design
intent preserves arbitrary signal injection, pause/resume/rollback control,
PID-file discovery, coding-agent rollback, multi-agent polling, and checkpoint
restoration by a lifecycle agent.

## Motivation

Imperative agents currently lack transparency during runtime: developers
receive only the log output they choose to emit, and control is limited to
SIGSTOP—pause—, SIGKILL—abort—, or a restart that effectively "rolls back."
Consequently, there is no way to ask "what state is the agent in?" or to
command "roll back to the last checkpoint" without terminating the process.

Machine Interpreters change this picture because the state space is finite and
declared. At any instant the engine occupies a named state, holds a single
pending signal, and maintains a bounded history—properties that belong to the
design, not to debugging artifacts. Declared state becomes queryable;
enumerated signals become injectable; and because the machine defines a
response to every signal in every state, an injected valid signal yields a
predictable, machine-guaranteed reaction. Operator Port exploits these
guarantees through three modes—in-process recording, HTTP read access, and
HTTP signal injection— without touching the machine, tools, or business logic.

## Applicability

Operator Port fits agents that run for minutes to hours, because it delivers
live progress updates, not post-hoc logs. Its value grows when operators
intervene mid-execution—pausing before irreversible steps, rolling back to
checkpoints, or injecting termination—and when a parent supervises multiple
children, requiring per-child state monitoring without scraping logs. For
agents that finish in seconds, post-hoc trace analysis (Chapter 11) remains
sufficient. The control plane therefore handles administrative intervention,
while operational signals stay confined to it.

## Structure

External consumers interact with the engine via three attachment modes (Fig. 31).

![](figures/fig-32-runtime-probe-components.png)

| Figure 31. Component diagram. The MonitorRecorder feeds engine events into a bounded Store; the read plane exposes them, while the control plane enqueues signals and commands consumed by the next dispatch; persisted ops drive checkpointing. |
|:---:|

### Participants

#### MonitorRecorder

After each dispatch, the engine notifies an in-process observer by appending a
`RunEvent` to the Store; this step involves only serialization and performs no
computation.

#### Store

A bounded in-memory ring holds the most recent N events, serves REST reads,
SSE streaming, and OTel metric export, and discards the oldest events when
full.

#### RestServer

The system exposes HTTP routes declared in the profile; both paths and
bindings are defined inside the profile, avoiding any dependence on a fixed
server API.

#### EventQueue

A bounded channel links signal-producing endpoints to the dispatch loop. The
shipped monitor profile uses this channel solely for `ExitRequested` messages;
broader lifecycle control relies on runtime conformance tests.

#### LifecycleTool

A proposed separate agent would operate on persisted checkpoints after the
live process exits. No production lifecycle-tool profile ships today.

#### LoopHooks

Policy callbacks (before/after dispatch, on state change, on budget threshold)
observe but never alter the transition; this represents the lightest mode,
requiring no network or serialization.

## Collaborations

After each dispatch, the engine transfers a `RunEvent` to the recorder,
capturing state, signal, tool, result, iteration, timestamp, and remaining
budget once the transition is committed. The Store keeps a bounded recent
window of these events and offers snapshot and SSE bindings for downstream
processing.

The shipped monitor profile declares the following observability routes:

| Method and path | Binding and view |
|---|---|
| `GET /monitor/machine` | `read_state`: machine specification |
| `GET /monitor/machines` | `read_state`: root and request-machine declarations |
| `GET /monitor/state` | `read_state`: current state |
| `GET /monitor/tools` | `read_state`: tool inventory |
| `GET /monitor/metrics` | `read_state`: metric snapshot |
| `GET /monitor/events` | `read_state`: recent events |
| `GET /monitor/events/stream` | `stream_events`: event SSE |
| `GET /monitor/openapi` | `static_metadata`: generated route description |

The profile does not declare an exit route. `agent-core` therefore injects
`POST /api/lifecycle/exit` (`lifecycle_control`, `ExitRequested`) into every
served agent, enabling the Operator Port to expose shutdown control without
profile redundancy, while the monitor server remains focused on observability.
The REST runtime also includes `emit_signal` and `lifecycle_control` bindings.
Tests verify queueing, policy validation, and lifecycle action mapping, even
though binding names do not match endpoint paths and no production profile
employs arbitrary injection, pause, resume, or rollback.

Fig. 32's signal injection illustrates the complete pattern and current
conformance behavior, which differs from the shipped monitor profile's HTTP
surface. A profile that selects the binding must explicitly declare the path,
allowed signal, and machine transition.

![](figures/fig-33-signal-injection.png)

| Figure 32. Sequence diagram of the generic injection binding. The shipped monitor profile relies on the injected exit route and selects no injection binding of its own. |
|:---:|

The lifecycle agent, responsible for browsing checkpoint history and restoring
terminated runs, adheres to the design intent. But the shipped monitor surface
lacks a production profile that provides this functionality.

## Consequences

### Benefits

* Live inspection without stopping -- Operators can view current state, recent history, and resource usage through non-blocking reads.  
* Control through declared transitions -- For profiles that select a control binding, injected signals follow the same machine rules as internal ones; there is no backdoor, and the machine enforces the authorization policy.  
* Machine-validated safety -- The runtime rejects invalid signals in the current state; the `RollbackRequested` signal exemplifies design intent, even though no shipped profile declares it.  
* Independence from business logic -- The probe does not interact with the machine, tools, or prompts; the agent operates identically whether the probe is present or not.

### Liabilities

* Memory overhead -- The ring's memory consumption grows with its capacity, which in turn expands as tool results increase; this trades depth for memory usage.  
* Network attack surface -- HTTP endpoints must bind to localhost or be protected by authentication middleware; the pattern supplies attachment points rather than security guarantees.  
* Observer effect -- Per-dispatch recording adds bounded, non-zero latency, which becomes measurable when agents dispatch hundreds of tools per second.

## Implementation

The monitor follows a profile-owned, opt-in model, activating the recorder and
listener only when its machine, tools, and REST definition are selected. The
checked-in server requests an ephemeral loopback port:

```yaml
servers:
  monitor:
    address: 127.0.0.1:0
    endpoints:
      current_state:
        method: GET
        path: /monitor/state
        binding: read_state
        monitor_view: current_state
      # No exit route is declared: agent-core injects POST /api/lifecycle/exit
      # (lifecycle_control, ExitRequested) into every served agent.
```

The `launch_rest_server` function prints the bound `address`; supervisors such
as the CLI proof read this output to build the base URL, thereby eliminating
PID/profile discovery files or fixed ports.

Monitor state reads access the live in-memory store and provide no durable
history. Declared-machines reads serve the trusted profile closure, exposing
the root and distinct `machine_request` `MachineSpecs` without asserting that
those machines are currently running. Checkpointing is handled separately via
the typed checkpoint port. Thus the monitor operates independently of the
bench: monitor routes observe live runs or declarations, while the bench
evaluates completed trace artifacts (Chapter 11).

## Relationships in the Pattern Language

Operator Port, as part of the Machine Interpreter, depends on the Machine
Interpreter, Bidirectional Log, and Approval Gate to achieve safe live
control. It requires explicit state, rollback, and suspend/resume decisions.
It overlaps with Transition Spans, because both expose execution state, yet
Operator Port differs by providing live, bidirectional operation whereas
Transition Spans focus on telemetry. The grammar for this pattern resides in
`pattern-language.yaml`.

## Known Uses

Shipped runtime-state-reader profile. The `agents/runtime-state-reader` starts
in the *Connecting* state, synchronizes with the network, and establishes
pairwise links via control messages. It moves to *Waiting*, processes packets
or timer events, and transitions to *Sending* on timer events to transmit
packets. After hardware confirmation, it returns to *Waiting*, allowing packet
reception even while in *Sending* to handle errors. The *Disconnecting* state
releases resources. The profile serves routes for the root-machine,
declared-machines, state, metrics, events, SSE, and OpenAPI; shutdown is
controlled by `/api/lifecycle/exit` injected by `agent-core`. The CLI proof
extracts the ephemeral loopback address from launch output, reads
declarations, live state, and metrics, posts `/api/lifecycle/exit`, and
verifies a successful terminal state.

Long-running coding-agent intervention (design intent). The shipped profile
lacks functionality to monitor coding transitions, identify cycles, or inject
`RollbackRequested`.

Multi-agent supervision (design intent). Polling many child monitors, issuing
generic lifecycle-control actions, and restoring crashed children via a
lifecycle agent are not shipped orchestration behaviors.

Control planes over running processes recur in systems that expose declared
inspection and control endpoints. *Kubernetes* liveness and readiness probes
[@k8s-probes] let a control plane query and act on a workload without killing
it. *Temporal* signals and queries [@temporal-2024] expose handlers for
inspection and external steering, preserving workflow state and history.
*Erlang/OTP* system messages [@erlang-sys-2024] provide standardized debug,
trace, suspend, resume, and status operations without altering process logic.

Disciplined runtime injection. *Chaos Engineering* [@basiri-chaos-2016]
injects controlled signals into a running system, validates each against the
machine, and observes the resulting behavior. For observation, *OpenTelemetry*
[@otel-spec-2024] exports live gauges and counters, offering real-time
dashboards of in-progress runs and post-hoc traces (see Chapter 8).