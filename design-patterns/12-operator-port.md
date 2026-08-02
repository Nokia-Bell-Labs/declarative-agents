# Operator Port

This chapter presents the Operator Port pattern, which attaches an observation and control surface to a running engine. It separates the shipped monitor profile, runtime-only lifecycle-control conformance, and the broader signal-injection and rollback design intent.

## Intent

Attach a control-plane server to the running engine so observers can query execution state and controllers can inject signals, all within the machine's declared state space.

## Reference implementation status

The shipped `applications/catalog/agents/runtime-state-reader` infrastructure adapter owns read routes for machine, state, tools, metrics, recent events, event SSE, and OpenAPI. It declares no control route of its own: agent-core injects the canonical `POST /api/lifecycle/exit` lifecycle-control endpoint into every served agent (GH-1264), which emits `ExitRequested`, and the profile's control await selects that injected route. The listener binds `127.0.0.1:0`; supervisors discover the selected address from the REST launch output.

The REST runtime has conformance-tested `lifecycle_control` and `inject_signal` bindings, but no production profile selects them. Arbitrary signal injection, pause/resume/rollback control, PID-file discovery, coding-agent rollback, multi-agent polling, and checkpoint restoration by a lifecycle agent remain design intent.


## Motivation

Imperative agents are opaque at runtime: the only visibility is log output the developer thought to emit, and the only control is crude (SIGSTOP to pause, SIGKILL to abort, restart to "roll back"). There is no way to ask "what state is the agent in?" or say "roll back to the last checkpoint" without killing the process.

Machine Interpreters change this because the state space is finite and declared. At any moment the engine occupies one named state, holds one pending signal, and keeps a bounded history. These are inherent properties of the design, not debug artifacts. Declared state is queryable; enumerated signals are injectable; and because the machine defines its response to every signal in every state, an injected valid signal produces a predictable, machine-guaranteed response. Operator Port exploits this through three modes (in-process recording, HTTP read access, and HTTP signal injection) without touching the machine, tools, or business logic.


## Applicability

The Operator Port fits agents that run for minutes to hours and where operators need live progress rather than post-hoc logs. It becomes more valuable when operators may intervene mid-execution — pausing before an irreversible step, rolling back to a checkpoint, or injecting termination — and when a parent supervises many children and needs per-child state without log scraping. For agents that finish in seconds, post-hoc trace analysis (Chapter 11) suffices. The control plane is for administrative intervention; signals that belong in the machine should stay there.


## Structure

External consumers reach the engine through three attachment modes, laid out in the component diagram of Fig. 31.

![](figures/fig-32-runtime-probe-components.png)

| **Figure 31.** Component diagram. The MonitorRecorder feeds engine events into a bounded Store the read plane exposes; the control plane enqueues signals and commands consumed by the next dispatch; persisted ops drive checkpointing. |
|:---:|

### Participants

#### MonitorRecorder

An in-process observer the engine notifies after every dispatch, appending a RunEvent to the Store (serialization only, no computation).

#### Store

A bounded in-memory ring of the most recent N events, serving REST reads, SSE streaming, and OTel metric export; oldest events drop when full.

#### RestServer

Exposes profile-declared routes over HTTP. Route paths and bindings belong to the profile, not to a fixed server API.

#### EventQueue

A bounded channel from signal-producing endpoints to the dispatch loop. The shipped monitor profile uses it only for `ExitRequested`; broader lifecycle control is covered by runtime conformance tests.

#### LifecycleTool

A proposed separate agent that would operate on persisted checkpoints after the live process exits. No production lifecycle-tool profile ships.

#### LoopHooks

In-process policy callbacks (before/after dispatch, on state change, on budget threshold) that observe but never alter the transition, the lightest mode, with no network or serialization.


## Collaborations

After each dispatch, once the transition is committed, the engine hands the recorder a RunEvent (state, signal, tool, result, iteration, timestamp, remaining budget). The store retains a bounded recent window and feeds snapshot and SSE bindings.

The shipped monitor profile declares these observability routes:

| Method and path | Binding and view |
|---|---|
| `GET /monitor/machine` | `read_state`: machine specification |
| `GET /monitor/state` | `read_state`: current state |
| `GET /monitor/tools` | `read_state`: tool inventory |
| `GET /monitor/metrics` | `read_state`: metric snapshot |
| `GET /monitor/events` | `read_state`: recent events |
| `GET /monitor/events/stream` | `stream_events`: event SSE |
| `GET /monitor/openapi` | `static_metadata`: generated route description |

The profile declares no exit route. agent-core injects `POST /api/lifecycle/exit` (`lifecycle_control`, `ExitRequested`) into every served agent, so the operator port exposes a shutdown control without each profile restating it, and the monitor server carries observability only. The REST runtime also implements generic `emit_signal` and `lifecycle_control` bindings. Tests prove queueing, policy validation, and lifecycle action mapping, but these binding names are not endpoint paths and no production profile selects arbitrary injection, pause, resume, or rollback.

Signal injection in Fig. 32 is the complete pattern and current conformance behavior, not the shipped monitor profile's HTTP surface. A profile that selects the binding must declare the path, allowed signal, and machine transition.

![](figures/fig-33-signal-injection.png)

| **Figure 32.** Sequence diagram of the generic injection binding. The shipped monitor profile relies on the injected exit route and selects no injection binding of its own. {wide} |
|:---:|

A lifecycle agent that browses checkpoint history and restores terminated runs remains design intent; no such production profile is part of the shipped monitor surface.


## Consequences

### Benefits

#### Live inspection without stopping

Operators see current state, history, and resource use via non-blocking reads.

#### Control through declared transitions

For profiles that select a control binding, injected signals obey the same machine rules as internal ones; there is no backdoor, and the machine is the authorization policy.

#### Machine-validated safety

The runtime rejects a signal invalid in the current state. `RollbackRequested` is illustrative design intent; no shipped profile declares that signal.

#### Independent of business logic

The probe touches neither machine, tools, nor prompts; an agent runs identically with or without it.

### Liabilities

#### Memory overhead

The ring consumes memory proportional to capacity, which grows with large tool results, a depth/memory trade-off.

#### Network attack surface

Open HTTP endpoints need localhost binding or auth middleware; the pattern provides attachment points, not a security model.

#### Observer effect

Per-dispatch recording adds bounded but non-zero latency, measurable for agents dispatching hundreds of tools per second.


## Implementation

The monitor is profile-owned and opt-in: selecting its machine, tools, and REST definition activates the recorder and listener. The checked-in server requests an ephemeral loopback port:

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

`launch_rest_server` returns structured output containing the bound `address`. Supervisors, including the CLI proof, construct the base URL from that output rather than using a PID/profile discovery file or a fixed port.

Monitor reads use the live in-memory store and do not provide durable history. Checkpointing is a separate runtime concern through the typed checkpoint port. The monitor is also distinct from the bench UI: monitor routes observe a live run, while bench evaluates completed trace artifacts (Chapter 11).


## Relationships in the Pattern Language

Operator Port sits within Machine Interpreter and requires Machine Interpreter, Bidirectional Log, and Approval Gate: live control is safe only when state, rollback, and suspend/resume decisions are declared. It overlaps Transition Spans because both expose execution state, but Operator Port is live and bidirectional while Transition Spans are telemetry records for observation and evaluation. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Shipped runtime-state-reader profile.** `agents/runtime-state-reader` serves profile-owned monitor state, metrics, event, SSE, and OpenAPI routes; its shutdown control is the agent-core-injected `/api/lifecycle/exit`. Its CLI proof discovers the ephemeral loopback address from launch output, reads live state and metrics, posts `/api/lifecycle/exit`, and observes a successful terminal state.

**Long-running coding-agent intervention (design intent).** Watching coding transitions, detecting cycles, and injecting `RollbackRequested` is not implemented by a shipped profile.

**Multi-agent supervision (design intent).** Polling many child monitors, issuing generic lifecycle-control actions, and restoring a crashed child through a lifecycle agent are not shipped orchestration behavior.

**Control planes over running processes.** The pattern recurs wherever a live process exposes declared inspection and control endpoints. **Kubernetes liveness and readiness probes** [@k8s-probes] let a control plane query and act on a running workload through declared endpoints without killing it; **Temporal signals and queries** [@temporal-2024] expose query handlers for inspection and signal handlers for external steering while preserving workflow state and history; and **Erlang/OTP system messages** [@erlang-sys-2024] give processes standardized debug, trace, suspend, resume, and status operations without changing process logic.

**Disciplined runtime injection.** **Chaos Engineering** [@basiri-chaos-2016] injects controlled signals into a running system to observe and steer its behaviour, the practice this pattern makes safe by validating every injected signal against the machine. For observation, the same **OpenTelemetry** [@otel-spec-2024] feed can be exported as live gauges and counters, giving real-time dashboards of an in-progress run alongside the post-hoc traces of Chapter 8.
