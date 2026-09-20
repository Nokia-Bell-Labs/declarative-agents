<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Machine Interpreter

Developers who've written agents have also written state machines, just not
explicitly. The Machine Interpreter pattern formalizes this: states, tools,
and transitions move into a data file; a fixed engine handles the rest.

## Intent

Separate control flow from execution by expressing the agent loop as a
transition table, so that behaviour is data, not code.

## Motivation

Agents follow the same loop: dispatch a tool, process the outcome, and repeat.
The LLM is a tool activated when needed. Some tools execute based on the LLM's
request; others are hardcoded, independent of its input.

A coding agent that reads, writes, and edits files using a standard imperative
loop provides a clear, efficient file-processing approach. Executing
operations sequentially ensures systematic file handling without unnecessary
complexity or ambiguity. This framework avoids convoluted logic, maintaining
clarity and efficiency.

```
while not done and steps < budget:
    response = invoke_llm(messages, tools)
    call = parse_response(response)
    if call.is_tool_call:
        if call.name == "read":
            result = read_file(call.args)
        elif call.name == "write":
            write_file(call.args)
            result = run_build()
        elif call.name == "edit":
            edit_file(call.args)
            result = run_lint(call.args)
        messages.append(result)
    elif call.is_completion:
        result = validate(call.output)
        if result.passed:
            done = True
        else:
            messages.append(result)
    steps += 1
```

The `if/elif` branches handle *read*, *write*, *edit*, and *completion*
operations, representing implicit states. Each branch's trigger and subsequent
control flow are scattered across conditions, requiring readers to trace every
loop path to reconstruct the state machine.

## Applicability


The Machine Interpreter suits workflows with discrete, branching operations,
like LLM agents, CI pipelines, evaluation harnesses, and migration scripts.
Its utility grows with more relevant conditions: recurring operations across
workflows, executions persisting across process boundaries, multi-actor
workflows (e.g., a machine author defining the flow and an LLM selecting tools
at runtime), and scenarios needing pre-run validation, reversibility, or audit
trails.

## Structure


The Machine Interpreter has three core components: Machine, Engine, and Tools,
interconnected via a dispatch cycle. Fig. 3 shows their roles and
relationships.

![](figures/fig-03-state-interpreter-class.png)

| **Figure 3.** Class diagram. The Engine consults the Machine and dispatches Tools; each Tool returns a signal that routes the next lookup. |
|:---:|

### Participants

#### Machine (Transition Table)

A pure data structure maps ` (state, signal)` to ` (next_state, tool)`,
lacking logic, conditionals, or loops.

A **state** is a node in the execution graph, identified by name and looked up
by it. States carry no behaviour. The machine declares each state
**non-terminal** (execution continues after the tool completes) or
**terminal** (execution halts), specifies the startup state, and defines
states signaling successful or failed completion.

A **signal** represents a tool's typed outcome, determining the next state
transition. Each tool declares its possible signals, and the machine must
handle all of them. Unhandled signals cause static load-time errors, detected
before execution begins, enabling static validation through a closed-world
assumption.

#### Engine (Loop, Interpreter, Runtime)

The engine is a fixed, generic state machine execution loop. It operates by
looking up the ` (current_state, last_signal)` pair, halting if the next state
is terminal, or resolving the tool name via the registry, calling `Execute`,
recording the result, and looping with the returned signal. Behaviour derives
entirely from the state machine and referenced tools, with no embedded engine
logic.

#### Tool (Command, Action)

A tool operates as an action: it takes parameters, executes tasks, and
produces a result with a signal. Every tool declares its **signature**,
detailing input types, emitted signals, output type, and modifiable external
state. Each tool implements `Execute` (performing the operation and returning
a signal) and `Undo` (reversing the operation using the recorded result).
Read-only tools make `Undo` a no-op; stateful tools ensure the result contains
enough information to undo changes.

Tools are classified by their result predictability, which directly affects
how the machine handles variability.

- A **deterministic tool** — `write_file`, `run_build`, `run_tests` — produces the same signal given identical inputs. The machine can rely on this and design transitions accordingly.
- A **boundary tool** — `invoke_llm`, `rest_await_event`, `run_agent` — crosses a boundary to an external actor: a language model, a human, or another system. Its response is not predictable from inputs alone. Boundary tools are the primary source of variance in execution. To handle this variance, the machine requires that every signal a boundary tool can emit appear explicitly in the transition table, giving the machine control over every possible outcome.

A **non-terminal** tool executes an entire sub-machine via its `Execute`
function, signaling the parent machine upon completion. This supports
hierarchical composition (Chapter 9).

#### Registry (Tool Set)

The registry contains all available tool implementations. The machine
references tools by symbolic name, and the registry resolves these names to
implementations at load time. During validation, the engine ensures each tool
name in the machine has a corresponding implementation in the registry and
that the machine handles every signal each tool declares to emit. This mutual
validation catches wiring errors before execution.


## Collaborations

### The engine cycle

Fig. 4 shows a single dispatch cycle. The engine retrieves the current `
(state, signal)` pair from the machine. If the next state is terminal, the
engine halts and returns the execution path of ` (state, signal, tool,
result)` tuples. Otherwise, it resolves the tool name via the registry,
invokes `Execute`, records the result, and continues with the returned signal.

![](figures/fig-04-engine-cycle.png)

| **Figure 4.** Sequence diagram. One dispatch cycle: the Engine consults the Machine, resolves the tool through the Registry, calls Execute, and routes the returned signal back to the lookup. {wide 0.6} |
|:---:|

The tool operates independently of the machine's state, the machine unaware of
the tool's internal mechanisms. The engine, as the sole intermediary,
interacts with both generically.

### Rollback

Rollback walks the execution backward, calling `Undo` on each tool with its
recorded result. The tool decides what undoing means, whether deleting a
created file, reverting a mutation, or issuing a compensating call; read-only
tools make `Undo` a no-op. The engine only enforces reverse ordering.

### Resume

The engine's position is defined by two values: the current state and the last
signal. This allows execution to be serialized after any tool completes.
Resumption restores the position and re-enters the loop at step 2. The full
engine stack (machine, registry, LLM adapter) is required, as the next
dispatch may invoke any tool.


## Consequences

### Benefits

#### Static validation

The machine ensures checkability before execution: every referenced
state-signal pair exists, every tool name resolves, and every emitted signal
is handled in every dispatchable state, eliminating a class of runtime errors.

#### Auditable execution

The execution is a detailed record, with each entry specifying the tool,
transition, and signal. This path reconstructs the sequence without
re-execution, serving as both a compliance artifact and a deterministic replay
log.

#### Reversibility

`Undo` per tool and recorded order let the engine walk execution backward;
with environment checkpointing, this enables full rollback to any prior point.

#### Serializability

Machine, execution, and engine position are data, enabling execution
persistence and resumption across processes, machines, or time.

#### Composability

Machines share registries, enabling reuse of `build`, `test`, `write` across
generation and evaluation machines. New workflows are instantiated as new
machines, not new code, and compose hierarchically via non-terminal tools.

#### Dual authorship

A machine can be designed with human-authored determinism or LLM-driven
adaptivity. The `$tool` slot lets the model choose tools at runtime, with the
machine controlling subsequent signals. The model works within the language
framework, and the machine enforces its syntax.

### Liabilities

#### Indirection

Understanding a workflow requires examining the machine and tools separately,
as the path is not contained in any single file. The machine-as-data model may
initially confuse, but repeated exposure clarifies it.

#### Signal explosion

Finer-grained tools increase signals, requiring the machine to handle every
state-signal combination, which becomes verbose for large machines. Mitigate
by grouping signals, encapsulating sub-workflows in non-terminal tools, and
generating machine skeletons from tool declarations.

#### Implicit data flow

The machine declares control flow, not data flow. Data traverses the result
channel and the builder, untyped by the machine; type-checking requires an
external analysis layer.


## Relationship to Known Patterns

The Machine Interpreter repurposes each referenced pattern's core mechanism,
changing its dispatch model.

#### GoF [@gamma-gof-1994]

**Command** (`Execute`/`Undo`) tools are selected via a data-driven table
rather than imperative code. The **State** pattern is inverted: states are
inert labels, behavior resides in dispatched tools rather than a class
hierarchy. The engine acts as an **Interpreter**, processing a flat transition
table to execute side-effectful commands rather than evaluate expressions.
Tools serve as interchangeable **Strategies** chosen by the machine. Execution
is captured as a **Memento**, retaining data to reverse or replay actions
without exposing tool internals.

#### Post-GoF

The split mirrors **Functional Core, Imperative Shell**
[@bernhardt-fcis-2012]. The machine is the pure core, tools the effectful
shell. Non-terminal tools provide **Harel Statecharts**
[@harel-statecharts-1987] hierarchical composition without nesting inside one
machine. The transition table uses an informal **Action Language**
[@bultan-action-lang-2000], enabling reachability and invariant analysis.

#### Related

The execution-plus-`Undo` mechanism extends the **Saga** pattern
[@garcia-molina-sagas-1987], incorporating static validation. Compared to a
**Workflow Engine** [@van-der-aalst-workflow-1998], it uses signals instead of
explicit edges, prioritizing static validation and reversibility over built-in
parallelism. Unlike the **Blackboard** pattern, the machine determines the
next step rather than an opportunistic controller.


## Implementation

### The machine is data, not code

The machine must load, serialize, and validate without executing code. If it
requires code, it has absorbed logic that belongs in tools. The canonical
generator machine, used throughout this book, is represented as a YAML
transition table:

```yaml
name: generator
initial_state: Composing
terminal_states: [Succeeded, Failed]
transitions:
  - {state: Composing,   signal: Seed,             next: Composing,   action: invoke_llm}
  - {state: Composing,   signal: LLMResponded,     next: Parsing,     action: parse_response}
  - {state: Composing,   signal: BudgetExceeded,   next: Failed}
  - {state: Parsing,     signal: ToolCall,         next: Dispatching, action: $tool}
  - {state: Parsing,     signal: Completion,       next: ValidatingBuild, action: build}
  - {state: Dispatching, signal: ToolDone,         next: Composing,   action: invoke_llm}
  - {state: Dispatching, signal: ToolFailed,       next: Composing,   action: invoke_llm}
  - {state: ValidatingBuild, signal: ToolDone,      next: ValidatingLint, action: lint}
  - {state: ValidatingBuild, signal: ToolFailed,    next: Composing,   action: invoke_llm}
  - {state: ValidatingLint,  signal: ToolDone,      next: ValidatingTest, action: test}
  - {state: ValidatingLint,  signal: ToolFailed,    next: Composing,   action: invoke_llm}
  - {state: ValidatingTest,  signal: ToolDone,      next: Succeeded}
  - {state: ValidatingTest,  signal: ToolFailed,    next: Composing,   action: invoke_llm}
```

From the `Composing` state, the seed invokes the model, parses its response,
and dynamically dispatches a tool call via `$tool`, feeding it back into the
system. The completion routes through build, lint, and test states. If
validation checks pass, the process succeeds; otherwise, failed validation or
tool execution returns the system to `Composing`, allowing the model to react.
Budget exhaustion transitions the system to the `Failed` state. Every legal
run constitutes an execution in the language defined by this state machine.
Fig. 5 illustrates this process as a state machine diagram.

![](figures/fig-05-canonical-machine.png)

| **Figure 5.** State machine diagram. The canonical generator machine of the listing, with edge labels read as `signal / tool`. {wide} |
|:---:|

### Tools are opaque; signals are closed

The machine knows only a tool's name and emittable signals, never its
implementation. Tools can thus be swapped (mock, logging wrapper, remote
delegate), implemented in any language or process, and reused across machines
unchanged. Each tool's signal set is a closed set validated at load time. An
unhandled signal is a machine error caught before any tool runs.

### The engine is fixed

The engine loop, identical across all workflows and devoid of domain logic,
must avoid conditionals, which should reside in a tool or the machine. This
loop exposes four operations:

| Operation | Function | Dependencies |
|---|---|---|
| **Run** | Execute a machine from its initial state | Full stack: machine, registry, LLM adapter |
| **Resume** | Continue from a persisted checkpoint | Full stack + checkpoint |
| **Rollback** | Rewind persisted state with Dolt `Revert`, then reverse external effects through receipts | Checkpoint port, run ID, target step |
| **History** | Format a loaded run's execution log | Checkpoint port, run ID |

Resume re-enters the loop, requiring the full machine and registry. Rollback
rewinds persisted state, reverses external effects using receipts, and avoids
loading a machine, justifying separate entry points.

### Data flow and tool construction

The machine declares control flow but not data flow. A **builder** bridges
tools by creating a tool instance from the previous result, connecting output
to input. This design separates tool *selection* (machine's role) from
*construction* (data flow), keeping tools decoupled. Each tool accepts typed
parameters without knowing their origin.

### Dynamic dispatch and recursive composition

The `$tool` slot resolves the tool name at runtime from the preceding result,
the machine defining which signals are handled afterward. The model selects
any tool from the registry, the machine handling the returned signal. Once
resolved, dispatch mirrors the static case.

Machines stay flat and independently validatable; hierarchy comes from
non-terminal tools composing sub-machines. Rollback mirrors this—undoing a
non-terminal tool reverses the recorded child execution. For more on this
model, see the Boundary Tool section in Chapter 9.

### Boundary tools and side-effect declarations

Three types of boundary tools recur, each configured via data rather than compilation.

| Actor type | Example tool | Shaping mechanism |
|---|---|---|
| **Model** | `invoke_llm` | System prompt + tool manifest |
| **Human** | `rest_await_event` | The interface presented |
| **Agent** | `run_agent` | The child profile |


## Relationships in the Pattern Language

Machine Interpreter underpins the language, embedding Agent-as-Data, Tool
Contract, Bidirectional Log, Transition Spans, Boundary Tool, Approval Gate,
Convergence Taxonomy, and Operator Port as direct outcomes of explicit loop
design. It supports all downstream patterns: Agent-as-Data, Tool Contract,
Phase-Scoped Toolset, Inference Boundary, Bidirectional Log, Transition Spans,
Boundary Tool, Approval Gate, Convergence Taxonomy, and Operator Port. The
full grammar is in `pattern-language.yaml`.


## Known Uses

**LangGraph** [@langgraph-2024] represents agent workflows as directed graphs
of Python nodes with callable edge conditions. Developers favor declaring
transitions over nesting conditionals, as shown by its widespread adoption,
though its conditions, being code, require execution for static analysis.

**StateFlow** [@wu-stateflow-2024] models task solving as a state machine with
transitions driven by outcomes. It outperforms unstructured loops on
multi-step benchmarks, providing empirical evidence that explicit state
machines improve agent performance rather than just clarity.

**Donna** [@tiendil-donna-2025] shows that Markdown-defined workflows can
compile into finite-state machines, enabling coding agents. The runtime
validates reachability before execution, demonstrating that machines from
higher-level notations keep static validation. This preserves validation
checks when moving from abstract workflows to executable state machines.

**Jido** [@jido-2025] builds agent systems on explicit state transformations,
signal routing, directives, and FSM execution, with optional AI/LLM
integration. This design enables Jido to demonstrate dual authorship in a
single framework.

**Lean4Agent** [@lean4agent-2026] formally verifies agent workflows within
dependent type theory. Machine validation ensures structural well-formedness,
semantic soundness checks pre/post-conditions, and trajectory analysis
verifies executions, establishing the pattern's verifiability upper bound.

**XState** [@xstate-2024] is a widely adopted statechart interpreter for
managing application control flow, exemplifying the interpreter-over-data
structure in production systems.

**SCXML** [@w3c-scxml-2015], a W3C executable state-machine notation,
serializes reactive control flow as data for interpretation by a conforming
processor, avoiding hand-written code.

**BPMN engines** [@omg-bpmn-2011] model business processes as data, executed
by a fixed engine. This mirrors the separation of flow-as-data from a generic
runtime, a principle adopted in enterprise workflow tooling.

**AWS Step Functions** [@aws-step-functions-2024] are serverless workflows
defined in Amazon States Language, executed by a dedicated service,
exemplifying JSON state machines as operational control flow at production
scale.

**Redux** [@redux-2015] manages UI state via a reducer function, ` (state,
action) -> state`, applying its table-driven transition discipline to
front-end engineering.

**TLA+** [@lamport-tla-2002] and **translation validation**
[@pnueli-translation-validation-1998] are two formal pillars of pre-run
analysis. TLA+ models behavior as states and actions, enabling model-checking
of reachable behaviors, while translation validation ensures a generated
artifact matches its source-level intent, similar to load-time verification
confirming the machine accurately reflects declared tool outcomes.

