# Operator Port

This chapter presents the Operator Port pattern, which attaches a control-plane server to a running engine. Observers can query current state and dispatch history; controllers can inject signals into the next dispatch cycle. The machine's declared state space defines what queries and injections are valid; the engine and tools are unmodified.

## Intent

Attach a control-plane server to the running engine so observers can query execution state and controllers can inject signals, all within the machine's declared state space.


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

Exposes the store and queue over HTTP.

#### EventQueue

A bounded channel from the control endpoints to the dispatch loop; the engine dequeues at its next cycle, and a full queue returns backpressure.

#### LifecycleTool

A *separate* agent (its own machine and profile) that operates on persisted checkpoints rather than the live process, so it survives restarts and manages terminated agents.

#### LoopHooks

In-process policy callbacks (before/after dispatch, on state change, on budget threshold) that observe but never alter the transition, the lightest mode, with no network or serialization.


## Collaborations

After each dispatch, once the transition is committed so recording can neither block nor alter it, the engine hands the recorder a RunEvent (state, signal, tool, result, iteration, timestamp, remaining budget), which the store appends and pushes to connected SSE clients.

The **read plane** is two HTTP endpoints: `/read_state` returns a non-blocking snapshot (current state, last signal, iteration, budget, recent events), and `/stream_events` pushes each RunEvent over SSE (clients that fall behind get a gap indicator, not backlog). OTel metrics export the same data as gauges and counters for standard dashboards.

The **control plane** injects signals, traced by the sequence diagram in Fig. 32. `/emit_signal` enqueues a signal the engine dequeues and routes like any other, looking it up in the machine and dispatching the resulting tool, rejecting it with a machine-violation error if the current state has no such transition. `/lifecycle_control` accepts pause/resume/exit/rollback, which translate to internal signals (pause reuses the Approval Gate, Chapter 10; rollback the checkpoint mechanism, Chapter 7). Injection is safe because the machine validates it: an external controller cannot reach an undeclared state or fire an undefined transition. The machine is the authority; the probe is a messenger.

![](figures/fig-33-signal-injection.png)

| **Figure 32.** Sequence diagram. Control-plane signal injection: `/emit_signal` validates the signal against the machine, enqueues it, and the engine dequeues and routes it at its next cycle. {wide} |
|:---:|

When the live process is gone, a LifecycleTool (an ordinary agent whose machine browses checkpoint history, selects a restore point, and triggers restoration) operates on persisted state instead. This is the Machine Interpreter applied to its own operations: recursion, not special-casing.


## Consequences

### Benefits

#### Live inspection without stopping

Operators see current state, history, and resource use via non-blocking reads.

#### Control through declared transitions

Injected signals obey the same machine rules as internal ones; there is no backdoor, and the machine is the authorization policy.

#### Machine-validated safety

A signal invalid in the current state is rejected, so an operator cannot corrupt the machine (injecting `RollbackRequested` outside `Suspended` errors rather than breaking it).

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

The probe is opt-in: one flag gates the recorder, store, and REST server, so short-lived agents pay nothing:

```yaml
monitor: { enabled: true, ring_capacity: 500, rest_port: 8080, otel_export: true }
```

The HTTP interface is four endpoints: `/read_state` (GET snapshot), `/stream_events` (GET SSE), `/emit_signal` (POST, 400 if the machine rejects), and `/lifecycle_control` (POST administrative command). Agents publish their REST address to a well-known file derived from PID and profile, so orchestrators and lifecycle tools locate them without port scanning.

Checkpointing rides on the dispatch loop: after each step the engine saves the Position and the appended Execution entry through the typed checkpoint port — committed per step by the Dolt backend — as a side effect that produces no signal and does not affect the transition, which keeps checkpoint logic out of the machine. Runs that need no persistence bind `NoopCheckpoint` and pay nothing. Note the probe is not the bench UI (`serve_ui`): the probe observes live, in-memory, in-progress executions; the bench reads completed trace files and classifies them (Chapter 11). Same data formats, different temporal scope.


## Relationships in the Pattern Language

Operator Port sits within Machine Interpreter and requires Machine Interpreter, Bidirectional Log, and Approval Gate: live control is safe only when state, rollback, and suspend/resume decisions are declared. It overlaps Transition Spans because both expose execution state, but Operator Port is live and bidirectional while Transition Spans are telemetry records for observation and evaluation. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Long-running coding agents.** A coding agent on a large task exposes its probe on localhost; the operator watches transitions stream past (Composing, Validating, Composing-retry, …), spots three Stuck-pattern cycles, and injects `RollbackRequested` to restore the last checkpoint and let it try a different approach.

**Multi-agent orchestration.** A bench spawning 50 generators across a cluster polls each one's `/read_state` into a live dashboard (30 Composing, 12 Validating, 5 Succeeded, 3 Failed) and sends `lifecycle_control?action=exit` to gracefully stop any that overrun, rather than killing the process. When an agent crashes mid-run, a lifecycle agent loads its checkpoint history and restores the last stable point (say iteration 180), so a fresh process resumes without re-executing the first 180 iterations.

**Control planes over running processes.** The pattern recurs wherever a live process exposes declared inspection and control endpoints. **Kubernetes liveness and readiness probes** [@k8s-probes] let a control plane query and act on a running workload through declared endpoints without killing it; **Temporal signals and queries** [@temporal-2024] expose query handlers for inspection and signal handlers for external steering while preserving workflow state and history; and **Erlang/OTP system messages** [@erlang-sys-2024] give processes standardized debug, trace, suspend, resume, and status operations without changing process logic.

**Disciplined runtime injection.** **Chaos Engineering** [@basiri-chaos-2016] injects controlled signals into a running system to observe and steer its behaviour, the practice this pattern makes safe by validating every injected signal against the machine. For observation, the same **OpenTelemetry** [@otel-spec-2024] feed can be exported as live gauges and counters, giving real-time dashboards of an in-progress run alongside the post-hoc traces of Chapter 8.
