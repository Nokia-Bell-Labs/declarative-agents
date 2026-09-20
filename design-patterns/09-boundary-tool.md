<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Boundary Tool

Boundary Tool composes agents via non-terminal boundary tools. A parent
machine delegates to a child agent with one tool call; the child executes and
returns a signal. Each machine remains flat and independently validatable —
hierarchy arises from composition, not nesting.

## Intent

Delegate a complete child execution through one boundary tool, so every
machine stays flat, independently validatable, and composable without
structural coupling.


## Motivation

Chapter 2 distinguishes terminal tools (atomic) from non-terminal tools
(boundaries). A boundary tool's `Execute` interacts with an external actor,
returning only when that actor completes. To the parent machine, it appears as
any other tool (one tool, one signal), though an entire execution may occur
internally. The boundary type defines the actor it engages.

| Boundary kind | Actor | Example tool |
|---|---|---|
| None | local operation | `write_file`, `build` |
| Model | LLM provider | `invoke_llm` |
| Human | interactive UI | `rest_await_event` |
| Child process | subprocess agent | `run_agent`, `execute_task` |
| Nested machine | in-process FSM | `run_point` |

The composition primitive is uniformity. Imperative coupling requires the
parent to know the child's control flow. A boundary tool collapses the child's
execution into a single signal, keeping each machine flat while stacking
boundaries achieves hierarchical depth.


## Applicability

Boundary Tool applies when an agent coordinates work across sub-agents or
sub-machines instead of inlining their logic. Common cases include evaluation
stacks, where a harness runs subject agents, and task decomposition, where a
planner delegates sub-tasks. Its value increases when child executions require
independent auditability and rollbackability, and when resource authority
narrows with downward delegation.


## Structure

The canonical composition in the working implementation is a three-tier
process stack (bench, critic, executor), as shown in Fig. 24. The **Bench**,
started by a human, sends `self_invoke` to activate critics; each **Critic**
runs **Executors** as nested machines via `run_point`. Each tier runs on its
own dedicated machine.

![](figures/fig-25-composition-deployment.png)

| **Figure 24.** Deployment diagram. The bench/evaluator/generator process stack: Bench spawns Evaluator subprocesses; each Evaluator runs Generators as nested machines. |
|:---:|

### Participants

#### Boundary tool

The given paragraph adheres to the standard tool contract, encompassing
parameters, signal, side effects, and receipt, yet it transgresses into the
domain of an actor.

Adheres to the tool contract (parameters, signal, side effects, receipt) yet
crosses into an actor.

#### Profile reference

The boundary tool carries either `config.profile` (a child profile YAML) for
subprocess children or `config.point_machine` (a sub-machine) for nested
machines, so the parent sees only the profile path and parameter contract,
never the child's states or tools.

#### Parent

Routes on the returned signal.

#### Child

Produces its own execution, invisible above the boundary.


## Collaborations

### Delegation

A parent dispatching a boundary tool initiates a child execution; the child
runs to completion, and the tool collapses its execution into one signal. Fig.
25's sequence diagram shows `execute_task` delegation: the parent spawns the
child with a sub-task spec and trace context, the child runs its `agent.run`,
and a single `ToolDone`/`ToolFailed` returns. The child, seeing only its
sub-task, ensures independently auditable and rollbackable execution.

![](figures/fig-26-delegation-sequence.png)

| **Figure 25.** Sequence diagram. `execute_task` delegation: the parent spawns an isolated child, which runs its own execution and returns one signal. {wide} |
|:---:|

A single benchmark point flows through all three tiers: bench launches an
evaluator, which starts a nested generator that writes/builds/tests, returns
control, classifies the run, and adds the trace to the grid. At each boundary,
the parent sees one tool and one signal; the generator's 50-iteration
execution remains invisible to the evaluator.

### Trace propagation

The pattern requires trace context to cross each child-execution boundary
(Chapter 8). The shipped subprocess path meets this by serializing the active
span as W3C `traceparent`. The child roots its `agent.run` under the parent's
boundary span, generating a separate trace file with the same trace ID. Since
`$tool` dispatch stays within the current machine and span context, no
additional propagation is needed.

Nested evaluator machines reflect a reference-implementation constraint, not
the broader assertion. The `run_point` path provides `tracing.NoopTracer{}` to
the child loop, enabling bench-to-critic and critic-to-executor dispatch to
execute and yield structured results. The nested point lacks span generation
for parent attachment. The design aims to pass the active tracer and parent
context through `run_point`, verifying parent/child span IDs.


## Consequences

### Benefits

#### Isolation with flat machines

Each child maintains its own memory, machine, and execution, keeping
complexity local and machines independently validatable.

#### Authority attenuation

A child inherits part of its parent's authority, reflecting structured
permission delegation. The bench's budget narrows progressively: first to an
evaluator's per-run allocation, then to a generator's per-point budget.
Authority never increases at boundaries, maintaining consistent constraints
across transitions.

#### Trace coherence and deterministic termination

Boundary adapters disseminating context naturally organize hierarchical traces
into tree structures, visualized with standard tools. Parents manage
termination independently by enforcing budgets and timeouts, surfacing
`BudgetExceeded` errors at the boundary.

### Liabilities

#### Coarse boundary compensation

A parent cannot invoke `Undo` on a child's individual tools; it must use the
child's `BoundaryCompensation` mechanism, which deletes the output directory
and reverts to the pre-child checkpoint. Fine-grained rollback within a child
requires re-entering its lifecycle machine, a feasible but costly process.

#### Hierarchical only

Composition flows strictly parent-to-child, with no peer-to-peer negotiation
allowed. Workflows needing peer interaction are refactored to have a parent
query its children and merge results, preserving authority, trace, and
termination guarantees lost in an unrooted peer-to-peer model.

#### Adapter-specific trace gaps

Structural composition alone doesn't guarantee trace composition. Each
boundary adapter needs its own propagation test, since a no-op child tracer
maintains dispatch semantics but silently breaks the span tree. The
`run_point` adapter has this limitation.


## Implementation

### Three composition mechanisms

**Subprocess child.** The parent initiates a separate OS process
(`execute_task`, `run_agent`, `self_invoke`) with dedicated memory and trace
file. Isolation is strong; cost is spawn plus serialization. **In-process
nested machine.** The parent creates a new engine within the same process,
running a sub-machine (`run_point`); near-zero overhead, weaker isolation,
ideal for tight inner loops. **`$tool` dispatch.** Not a boundary but a
composition tool: the engine resolves the tool from an LLM result, composing
tools *within* one machine without child execution.

| Mechanism | Isolation | Overhead | Child execution |
|---|---|---|---|
| Subprocess | Process boundary | High | Separate trace file |
| Nested machine | Shared memory | Near zero | Independent nested run; evaluator currently records no child spans |
| `$tool` dispatch | Same machine | None | None |

### Profile-driven declaration

Boundary tools reference child *configuration* rather than child code. The
parent carries only a profile path, so the child can change (faster model, new
tool, restructured machine) without altering the parent. The same parent
delegates to different children by parameterizing the profile path, and depth
is unbounded (the profile graph is a DAG).

### Rollback at boundaries

A boundary tool's receipt encodes coarse compensation for the entire child
execution. **Reversible** children's artifacts are deleted or reverted;
**compensatable** children's external calls are corrected using stored
resource identifiers; **irreversible** children are skipped and logged.
Classification propagates upward. If any child tool is irreversible, the
boundary tool is at least compensatable, as coarse compensation cannot undo
the irreversible step.


## Relationships in the Pattern Language

Boundary Tool, part of the Machine Interpreter, requires Machine Interpreter,
Tool Contract, and Transition Spans. Delegation is a single declared tool call
producing one signal, with a boundary adapter connecting child and parent
traces. It overlaps with the Inference Boundary in managing boundary crossings
but is more comprehensive for child actors and sub-machines. The full grammar
is in `pattern-language.yaml`.


## Known Uses

**Bench/critic/executor stack.** The evaluation harness comprises three tiers
(Figure 24). The bench tier delegates tasks to the critic tier via
`self_invoke`, and critics execute operations using `run_point`. This design
routes each benchmark point through all tiers, isolating complexity at each
level.

**Planner/generator delegation.** A planner uses LLM inference to divide a
task, then calls `execute_task` for each sub-task. Each generator handles only
its assigned sub-task, producing an independently rollbackable execution, so a
failure in sub-task 3 can be reverted without impacting sub-tasks 1 and 2.
Security-review and migration planners use this mechanism with unique child
profiles.

**Self-invocation.** The `self_invoke` mechanism re-executes the current
binary under an alternate profile, letting a single agent act as another
configuration of itself without requiring separate deployment.

**Supervisor trees (Erlang/OTP)** [@armstrong-2003] organize processes in a
hierarchy where a parent starts, monitors, and restarts its children. This
structure ensures reliability through parent-to-child delegation, embodying
hierarchical composition with attenuated responsibility.

**Actor model** [@hewitt-actor-1973]. This model conceptualizes computation as
actors creating and interacting with other actors via messages, enabling
isolated executions within message boundaries.

**MapReduce** [@dean-mapreduce-2004] is a framework where a coordinator
assigns isolated sub-computations to workers, merges their results. And
ensures parent-to-child composition returns results upward, mirroring how a
bench collapses child executions into one signal.
