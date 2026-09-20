<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Operator Port

An Operator Port integrates an observation and control surface with a running
engine, maintaining three elements: the shipped monitor profile, runtime-only
lifecycle-control conformance, and signal-injection and rollback design
intent.

## Intent

Attach a control-plane server to the running engine so observers can query
the execution state and controllers can inject signals, all within the
declared state space.

## Reference implementation status

The shipped `applications/catalog/agents/runtime-state-reader` adapter handles
read routes for the root machine, declared machines, state, tools, metrics,
recent events, event SSE, and OpenAPI. It lacks its own control route;
agent-core injects the `POST /api/lifecycle/exit` endpoint (GH-1264) into
every agent, emitting `ExitRequested`, with the profile's control await
selecting this route. The listener binds to `127.0.0.1:0`; supervisors
retrieve the address from REST launch output.

The REST runtime includes conformance-tested `lifecycle_control` and
`inject_signal` bindings, though no production profile selects them. Design
intent retains arbitrary signal injection, pause/resume/rollback control,
PID-file discovery, coding-agent rollback, multi-agent polling, and checkpoint
restoration by a lifecycle agent.


## Motivation

Imperative agents lack transparency during runtime: the developer's only
insight is through log output they chose to emit, and control is limited to
basic commands (SIGSTOP to pause, SIGKILL to abort, or restart to "roll
back"). There is no mechanism to query "what state is the agent in?" or
instruct "roll back to the last checkpoint" without terminating the process.

Machine Interpreters change this because the state space is finite and
declared. At any moment the engine occupies one named state, holds one pending
signal, and keeps a bounded history. These are inherent properties of the
design, not debug artifacts. Declared state is queryable; enumerated signals
are injectable; and because the machine defines its response to every signal
in every state, an injected valid signal produces a predictable,
machine-guaranteed response. Operator Port exploits this through three modes
(in-process recording, HTTP read access, and HTTP signal injection) without
touching the machine, tools, or business logic.


## Applicability

The Operator Port suits agents operating over minutes to hours, providing live
progress updates instead of post-hoc logs. Its utility grows when operators
intervene mid-execution—pausing before irreversible steps, rolling back to
checkpoints, or injecting termination—and when a parent supervises multiple
children, requiring per-child state monitoring without log scraping. For
agents completing tasks in seconds, post-hoc trace analysis (Chapter 11)
suffices. The control plane handles administrative intervention, and
operational signals stay within it.


## Structure

External consumers interact with the engine via three attachment modes (Fig. 31).

![](figures/fig-32-runtime-probe-components.png)

| **Figure 31.** Component diagram. The MonitorRecorder feeds engine events into a bounded Store the read plane exposes; the control plane enqueues signals and commands consumed by the next dispatch; persisted ops drive checkpointing. |
|:---:|

### Participants

#### MonitorRecorder

The engine notifies an in-process observer after every dispatch by appending a
RunEvent to the Store, involving serialization only, with no computation
performed.

#### Store

A bounded in-memory ring of the most recent N events serves REST reads, SSE
streaming, and OTel metric export, dropping the oldest events when full.

#### RestServer

The system exposes HTTP routes declared in the profile, with paths and
bindings defined within the profile, not tied to a fixed server API.

#### EventQueue

A bounded channel links signal-producing endpoints to the dispatch loop. The
shipped monitor profile uses this channel solely for `ExitRequested` messages;
broader lifecycle control relies on runtime conformance tests.

#### LifecycleTool

A proposed separate agent would operate on persisted checkpoints after the
live process exits. No production lifecycle-tool profile ships.

#### LoopHooks

Policy callbacks (before/after dispatch, on state change, on budget threshold)
observe but never alter the transition, the lightest mode, with no network or
serialization.


## Collaborations

After each dispatch, the engine transfers a RunEvent to the recorder,
including state, signal, tool, result, iteration, timestamp, and remaining
budget, once the transition is committed. The store keeps a bounded recent
window of these events, offering snapshot and SSE bindings for further
processing.

The shipped monitor profile declares these observability routes:

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

The profile lacks an exit route. The agent-core injects `POST
/api/lifecycle/exit` (`lifecycle_control`, `ExitRequested`) into every served
agent, enabling the operator port to expose shutdown control without profile
redundancy, while the monitor server focuses on observability. The REST
runtime includes `emit_signal` and `lifecycle_control` bindings. Tests verify
queueing, policy validation, and lifecycle action mapping, though binding
names do not match endpoint paths, and no production profile uses arbitrary
injection, pause, resume, or rollback.

Fig. 32's signal injection represents the complete pattern and current
conformance behavior, distinct from the shipped monitor profile's HTTP
surface. A profile selecting the binding must explicitly declare the path,
allowed signal, and machine transition.

![](figures/fig-33-signal-injection.png)

| **Figure 32.** Sequence diagram of the generic injection binding. The shipped monitor profile relies on the injected exit route and selects no injection binding of its own. {wide} |
|:---:|

The lifecycle agent, responsible for browsing checkpoint history and restoring
terminated runs, adheres to the design intent. But the shipped monitor surface
lacks a production profile with this functionality.


## Consequences

### Benefits

#### Live inspection without stopping

Operators see current state, history, and resource use via non-blocking reads.

#### Control through declared transitions

For profiles selecting a control binding, injected signals follow the same
machine rules as internal ones; there is no backdoor, and the machine enforces
the authorization policy.

#### Machine-validated safety

The runtime rejects invalid signals in the current state. The
`RollbackRequested` signal exemplifies design intent, yet no shipped profile
declares it.

#### Independent of business logic

The probe does not interact with the machine, tools, or prompts; the agent
operates independently, executing identically regardless of its presence.

### Liabilities

#### Memory overhead

The ring's memory consumption scales with its capacity, which grows as tool
results increase, trading depth for memory usage.

#### Network attack surface

HTTP endpoints must use localhost binding or authentication middleware; the
pattern offers attachment points rather than security.

#### Observer effect

Per-dispatch recording adds bounded, non-zero latency, measurable when agents
dispatch hundreds of tools per second.


## Implementation

The monitor uses a profile-owned, opt-in model, activating the recorder and
listener when its machine, tools, and REST definition are selected. The
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

The `launch_rest_server` function outputs the bound `address`, which
supervisors like the CLI proof use to build the base URL, eliminating
PID/profile discovery files or fixed ports.

Monitor state reads access the live in-memory store, offering no durable
history. Declared-machines reads serve the trusted profile closure, including
the root and distinct `machine_request` MachineSpecs, without asserting those
machines are running. Checkpointing is handled separately via the typed
checkpoint port. The monitor functions independently of the bench: monitor
routes observe live runs or declarations, while bench evaluates completed
trace artifacts (Chapter 11).


## Relationships in the Pattern Language

Operator Port, part of the Machine Interpreter, depends on the Machine
Interpreter, Bidirectional Log, and Approval Gate for safe live control,
requiring explicit state, rollback, and suspend/resume decisions. It overlaps
with Transition Spans, both exposing execution state, but differs in its live,
bidirectional operation versus Transition Spans' telemetry role. The grammar
is defined in `pattern-language.yaml`.


## Known Uses

**Shipped runtime-state-reader profile.** The `agents/runtime-state-reader`
starts in the Connecting state, synchronizing with the network and
establishing pairwise links via control messages. Transitioning to the Waiting
state, it processes packets or timer events, moving to the Sending state on
timer events to transmit packets. Upon hardware confirmation, it returns to
Waiting, allowing packet reception even in Sending to handle errors. The
Disconnecting state releases resources. The runtime-state-reader serves routes
for profile-owned root-machine, declared-machines, state, metrics, event, SSE,
and OpenAPI, with shutdown controlled by `/api/lifecycle/exit` from
agent-core. The CLI proof extracts the ephemeral loopback address from launch
output, reads declarations, live state, and metrics, posts
`/api/lifecycle/exit`, and verifies a successful terminal state.

**Long-running coding-agent intervention (design intent).** The shipped
profile lacks functionality to monitor coding transitions, identify cycles, or
inject `RollbackRequested`.

**Multi-agent supervision (design intent).** Polling many child monitors,
issuing generic lifecycle-control actions, and restoring crashed children via
a lifecycle agent are not shipped orchestration behaviors.

**Control planes over running processes** recur in systems where live
processes expose declared inspection and control endpoints. **Kubernetes
liveness and readiness probes** [@k8s-probes] let a control plane query and
act on a workload without killing it. **Temporal signals and queries**
[@temporal-2024] expose handlers for inspection and external steering,
preserving workflow state and history. **Erlang/OTP system messages**
[@erlang-sys-2024] provide processes standardized debug, trace, suspend,
resume, and status operations without altering process logic.

**Disciplined runtime injection.** **Chaos Engineering** [@basiri-chaos-2016]
injects controlled signals into a running system, validating each against the
machine to observe and steer its behavior. For observation, the
**OpenTelemetry** [@otel-spec-2024] feed exports live gauges and counters,
offering real-time dashboards of in-progress runs and post-hoc traces from
Chapter 8.
