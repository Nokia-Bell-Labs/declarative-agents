# Boundary Tool

This chapter presents the Boundary Tool pattern, which composes agents through non-terminal boundary tools. A parent machine delegates to a child agent through one tool call; the child runs its own execution and returns one signal. Each machine stays flat and independently validatable; hierarchy emerges from composition, not from nesting.

## Intent

Delegate a complete child execution through one boundary tool so every machine stays flat, independently validatable, and composable without structural coupling.


## Motivation

Chapter 2 distinguished terminal tools (atomic) from non-terminal tools (boundaries). A boundary tool's `Execute` crosses into an external actor and returns only when that actor finishes; to the parent machine it is indistinguishable from any other tool (one tool, one signal) even though an entire execution may unfold inside. Boundary kind determines the actor:

| Boundary kind | Actor | Example tool |
|---|---|---|
| None | local operation | `write_file`, `build` |
| Model | LLM provider | `invoke_llm` |
| Human | interactive UI | `serve_ui` |
| Child process | subprocess agent | `run_agent`, `execute_task` |
| Nested machine | in-process FSM | `run_point` |

This uniformity is the composition primitive. Wiring multi-agent coordination imperatively couples parent and child. The parent must know the child's control flow. A boundary tool instead collapses the child's whole execution into a single signal, so hierarchical depth comes from stacking boundaries while each machine stays flat.


## Applicability

Boundary Tool fits when an agent must coordinate work across sub-agents or sub-machines rather than inlining their logic. Common cases are evaluation stacks (a harness running subject agents) and task decomposition (a planner delegating sub-tasks). The pattern becomes more valuable when child executions need to be independently auditable and rollbackable, and when resource authority should narrow as work is delegated downward.


## Structure

In the working implementation the canonical composition is a three-tier process stack (bench, evaluator, generator) shown as a deployment diagram in Fig. 24. **Bench** (human-launched) dispatches `launch_eval` subprocess tools to start evaluators; each **Evaluator** runs **Generators** as nested machines via `run_point`. Each tier is a separate profile with its own machine.

![](figures/fig-25-composition-deployment.png)

| **Figure 24.** Deployment diagram. The bench/evaluator/generator process stack: Bench spawns Evaluator subprocesses; each Evaluator runs Generators as nested machines. |
|:---:|

### Participants

#### Boundary tool

Obeys the ordinary tool contract (parameters, signal, side effects, UndoMemento) but crosses into an actor.

#### Profile reference

Carried by the boundary tool rather than a code dependency, either `config.profile` (a child profile YAML) for subprocess children or `config.point_machine` (a sub-machine) for nested machines, so the parent sees only the profile path and parameter contract, never the child's states or tools.

#### Parent

Routes on the returned signal.

#### Child

Produces its own execution, invisible above the boundary.


## Collaborations

### Delegation

When a parent dispatches a boundary tool it starts a child execution; the child runs to completion and the boundary tool collapses its execution into one signal. The sequence diagram in Fig. 25 shows `execute_task` delegation: the parent spawns the child with a sub-task spec and a trace context, the child runs its own `agent.run`, and a single `ToolDone`/`ToolFailed` returns. The child sees only its sub-task, not the parent's context, and its execution is independently auditable and rollbackable.

![](figures/fig-26-delegation-sequence.png)

| **Figure 25.** Sequence diagram. `execute_task` delegation: the parent spawns an isolated child, which runs its own execution and returns one signal. {wide} |
|:---:|

A single benchmark point flows through all three tiers: bench launches an evaluator, the evaluator starts a nested generator that writes/builds/tests, control returns up, the evaluator classifies the run, and bench adds the trace to the grid. At every boundary the parent sees one tool and one signal; a generator's 50-iteration execution is invisible to the evaluator.

### Trace propagation

Composition requires traces to cross boundaries (Chapter 8). Subprocess children receive a W3C `traceparent` and root their `agent.run` under the parent's boundary span, writing their own trace file with a shared trace ID. Nested machines share the tracer instance, so their spans attach directly to the parent span. With `$tool` dispatch no propagation is needed; it stays in the current machine and span context.


## Consequences

### Benefits

#### Isolation with flat machines

Each child has its own memory, machine, and execution; complexity stays local and every machine remains independently validatable.

#### Authority attenuation

A child inherits a subset of its parent's authority. Bench's full budget narrows to an evaluator's per-run budget, then to a generator's per-point budget. Authority never increases at a boundary.

#### Trace coherence and deterministic termination

Hierarchical traces form trees that standard tools visualize; parents enforce termination on children through budgets and timeouts, surfacing `BudgetExceeded` at the boundary.

### Liabilities

#### Coarse boundary compensation

A parent cannot call `Undo` on a child's individual tools; it can only run the child's `BoundaryCompensation` (delete the output directory, revert to the pre-child checkpoint). Fine-grained rollback inside a child requires re-entering the child's lifecycle machine, which is possible but expensive.

#### Hierarchical only

All composition flows parent-to-child; there is no peer-to-peer negotiation. Workflows that seem to need it are restructured as a parent querying both children and merging results, preserving authority, trace, and termination guarantees that unrooted peers would lack.


## Implementation

### Three composition mechanisms

**Subprocess child.** The parent spawns a separate OS process (`execute_task`, `run_agent`, `self_invoke`, `launch_eval`) with its own memory and trace file. Isolation is strong; cost is spawn plus serialization. **In-process nested machine.** The parent creates a new engine in the same process and runs a sub-machine (`run_point`); near-zero overhead, weaker isolation, suited to tight inner loops. **`$tool` dispatch.** Not a boundary but a composition tool: the engine resolves the tool from an LLM result, composing tools *within* one machine with no child execution.

| Mechanism | Isolation | Overhead | Child execution |
|---|---|---|---|
| Subprocess | Process boundary | High | Separate trace file |
| Nested machine | Shared memory | Near zero | Embedded in parent entry |
| `$tool` dispatch | Same machine | None | None |

### Profile-driven declaration

Boundary tools reference child *configuration*, not child code. Because the parent carries only a profile path, the child can change (faster model, new tool, restructured machine) without touching the parent; the same parent machine delegates to different children by parameterizing the profile path; and depth is unbounded (the profile graph is a DAG).

### Rollback at boundaries

A boundary tool's UndoMemento carries a coarse `BoundaryCompensation` for the whole child execution. A **reversible** child's artifacts are deleted or reverted; a **compensatable** child's external calls are corrected via stored resource identifiers; an **irreversible** child is skipped and logged. Classification propagates upward. If any child tool is irreversible, the boundary tool is at least compensatable, since coarse compensation cannot undo the irreversible step.


## Relationships in the Pattern Language

Boundary Tool sits within Machine Interpreter and requires Machine Interpreter, Tool Contract, and Transition Spans: delegation is one declared tool call, its outcome is one signal, and trace context links the child execution to the parent. It overlaps Inference Boundary because both encapsulate boundary crossings, but Boundary Tool is the broader composition mechanism for child actors and sub-machines. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Bench/evaluator/generator stack.** The evaluation harness composes three tiers (Fig. 24): bench delegates to evaluators via subprocess `launch_eval`, evaluators run generators via nested `run_point`, and a single benchmark point traverses all three with complexity isolated at each level.

**Planner/generator delegation.** A planner uses LLM inference to decompose a task, then dispatches `execute_task` per sub-task; each generator sees only its sub-task and produces an independently rollbackable execution, so a failed sub-task 3 reverts without disturbing sub-tasks 1 and 2. Security-review and migration planners reuse the mechanism with different child profiles.

**Self-invocation.** `self_invoke` re-runs the current binary under a different profile, letting one agent act as another configuration of itself without a separate deployment.
