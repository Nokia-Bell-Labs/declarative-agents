# Machine Interpreter

Every developer who has written an agent has also written a state machine — they just didn't write it that way. The Machine Interpreter pattern makes this explicit: states, tools, and transitions move into a data file; a fixed engine does the rest.
## Intent

Separate control flow from execution: express the agent loop as a transition table so that behaviour is data, not code.

## Motivation

Every agent runs the same loop: dispatch a tool, handle the result, repeat. The LLM is one tool among many, invoked when the loop calls for it. Some tools run because the LLM requested them; others are hardcoded into the loop regardless of what the model said. 

A coding agent that can read, write, and edit files, written as a typical imperative loop:

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

The `if/elif` branches are implicit states — they handle *read*, *write*, *edit*, and *completion*. What triggers each branch and where control goes afterward is scattered through the conditions. A reader has to trace every path through the loop to recover the state machine.
## Applicability

The Machine Interpreter fits workflows that are sequences of discrete operations branching on outcomes — LLM agents, CI pipelines, evaluation harnesses, migration scripts. The pattern becomes more valuable as the number of these conditions that apply grows: operations that recur across multiple workflows, executions that need to survive process boundaries, workflows where different actors contribute (a machine author defines the flow; an LLM chooses tools at runtime), and cases where pre-run validation, reversibility, or audit trails matter. 
## Structure

The Machine Interpreter has three primary structural elements (Machine, Engine, and Tools) connected by a dispatch cycle. The class diagram in Fig. 3 shows their roles and relationships.

![](figures/fig-03-state-interpreter-class.png)

| **Figure 3.** Class diagram. The Engine consults the Machine and dispatches Tools; each Tool returns a signal that routes the next lookup. |
|:---:|

### Participants

#### Machine (Transition Table)

A pure data structure mapping `(state, signal)` to `(next_state, tool)`, with no logic, conditionals, or loops. 

A **state** is a node in the execution graph, identified by name and used as a lookup key. States carry no behaviour — they are inert markers of position. The machine declares each state as either **non-terminal** (execution continues after the tool completes) or **terminal** (execution halts). The machine also specifies which state to enter on startup and which states signal successful or failed completion.

A **signal** is the typed outcome a tool returns, used to route the next state transition. Each tool declares the finite set of signals it can emit; the machine must handle every possible signal. An unhandled signal is a static load-time error, caught before execution begins. This closed-world assumption about outcomes is what allows static validation.

#### Engine (Loop, Interpreter, Runtime)

The engine is a fixed, generic state machine execution loop. It operates in steps: look up `(current_state, last_signal)` in the machine; if the next state is terminal, halt; otherwise resolve the tool name through the registry, call its `Execute` method, record the result, and loop with the returned signal. Behaviour comes entirely from the machine and the tools it references, never from engine logic. 

#### Tool (Command, Action)

A tool is an operation: it accepts parameters, performs work (possibly with side effects), and returns a result carrying a signal. Every tool declares its **signature**: the types of inputs it accepts, the set of signals it can emit, the type of its output, and what external state it may affect. Every tool implements two methods: `Execute` (perform the operation and return a signal) and `Undo` (reverse the operation given the recorded result). Read-only tools make `Undo` a no-op; stateful tools record enough information in the result to undo their mutations.

Tools are categorized by the predictability of their outcome, which shapes how the machine handles variance:

- A **deterministic tool** — `write_file`, `run_build`, `run_tests` — produces the same signal given identical inputs. The machine can rely on this and design transitions accordingly.
- A **boundary tool** — `invoke_llm`, `serve_ui`, `run_agent` — crosses a boundary to an external actor: a language model, a human, or another system. Its response is not predictable from inputs alone. Boundary tools are the primary source of variance in execution. To handle this variance, the machine requires that every signal a boundary tool can emit appear explicitly in the transition table, giving the machine control over every possible outcome.

A tool may also be **non-terminal**: its `Execute` runs an entire sub-machine, and the parent machine receives a single signal when the sub-machine completes. This enables hierarchical composition (Chapter 9).

#### Registry (Tool Set)

The registry is the set of all available tool implementations. The machine references tools by symbolic name; the registry resolves names to implementations at load time. During validation, the engine checks that every tool name referenced in the machine has a corresponding implementation in the registry, and that every signal each tool declares to emit is handled by the machine. This mutual validation catches wiring errors before execution.


## Collaborations

### The engine cycle

The sequence diagram in Fig. 4 traces one dispatch cycle. The engine asks the machine to look up the current `(state, signal)`. If the next state is terminal, the engine stops and returns the execution — the recorded path of `(state, signal, tool, result)` tuples. Otherwise it resolves the tool name through the registry, calls `Execute`, records the result, and loops with the returned signal.

![](figures/fig-04-engine-cycle.png)

| **Figure 4.** Sequence diagram. One dispatch cycle: the Engine consults the Machine, resolves the tool through the Registry, calls Execute, and routes the returned signal back to the lookup. {wide 0.6} |
|:---:|

The tool never knows the machine's state; the machine never knows the tool's internals. The engine is the only participant that touches both, and it does so generically.

### Rollback

Rollback walks the execution backward, calling `Undo` on each tool with its recorded result. The tool decides what undoing means, whether deleting a created file, reverting a mutation, or issuing a compensating call; read-only tools make `Undo` a no-op. The engine only enforces reverse ordering.

### Resume

Because the engine's position reduces to two values (current state and last signal), execution can be serialized after any tool completes. Resumption restores the position and re-enters the loop at step 2. It requires the full engine stack (machine, registry, LLM adapter), since the next dispatch may invoke any tool.


## Consequences

### Benefits

#### Static validation

The machine is checkable before execution: every referenced state--signal pair exists, every tool name resolves, and every signal a tool emits is handled in every state where it can be dispatched, eliminating a class of runtime errors.

#### Auditable execution

The execution is a structured record: each entry names the tool, transition, and signal. The path reconstructs without re-running anything, serving as a compliance artifact and a deterministic replay log.

#### Reversibility

With `Undo` per tool and recorded order, the engine walks the execution backward; combined with environment checkpointing, this gives full rollback to any prior point.

#### Serializability

Machine, execution, and engine position are all data, so execution can be persisted and resumed in another process, machine, or time.

#### Composability

Machines share registries. A generation machine and an evaluation machine reuse `build`, `test`, `write`. New workflows are new machines, not new code, and they compose hierarchically through non-terminal tools.

#### Dual authorship

A machine can be human-authored for determinism or LLM-driven for adaptivity. The `$tool` slot lets the model choose tools at runtime while the machine still constrains the signals handled afterward. The model speaks the language; the machine enforces its syntax.

### Liabilities

#### Indirection

Understanding a workflow means reading machine and tools separately; the path is in no single file. The machine-as-data model is disorienting until internalized.

#### Signal explosion

Finer-grained tools mean more signals, and the machine must handle every state--signal combination, which is verbose for large machines. Mitigate by grouping signals, encapsulating sub-workflows in non-terminal tools, and generating machine skeletons from tool declarations.

#### Implicit data flow

The machine declares control flow but not data flow. Data passes through the result channel and the builder, untyped by the machine; type-checking it requires an analysis layer beyond the machine.


## Relationship to Known Patterns

The Machine Interpreter is a compound pattern that repurposes each referenced pattern's core mechanism while changing its dispatch model.

#### GoF [@gamma-gof-1994]

Each tool is a **Command** (`Execute`/`Undo`), but selected by a data-driven table rather than imperative code. The machine is the **State** pattern inverted, with states as inert labels and behaviour living in dispatched tools rather than a class hierarchy. The engine is an **Interpreter** over a flat transition table that executes side-effectful commands instead of evaluating expressions. Tools are interchangeable **Strategies** selected by the machine. The execution is a **Memento** capturing enough to reverse or replay without exposing tool internals.

#### Post-GoF

The split mirrors **Functional Core, Imperative Shell** [@bernhardt-fcis-2012]. The machine is the pure core, tools the effectful shell. Non-terminal tools give the hierarchical composition of **Harel Statecharts** [@harel-statecharts-1987] without nesting inside one machine. The transition table is an informal **Action Language** [@bultan-action-lang-2000], open to reachability and invariant analysis.

#### Related

The execution-plus-`Undo` generalizes the **Saga** pattern [@garcia-molina-sagas-1987], adding static validation. Versus a BPMN-style **Workflow Engine** [@van-der-aalst-workflow-1998], signals replace explicit edges, trading built-in parallelism for static validation and reversibility. Unlike the **Blackboard** pattern, the machine, not an opportunistic controller, decides what runs next.


## Implementation

### The machine is data, not code

The machine must load, serialize, and validate without executing code. If it requires code to express, it has absorbed logic that belongs in tools. The canonical generator machine, the same one used throughout this book, as a YAML transition table:

```yaml
name: generator
initial_state: Composing
terminal_states: [Succeeded, Failed]
transitions:
  - {state: Composing,   signal: Seed,             next: Composing,   action: invoke_llm}
  - {state: Composing,   signal: LLMResponded,     next: Parsing,     action: parse_response}
  - {state: Composing,   signal: BudgetExceeded,   next: Failed}
  - {state: Parsing,     signal: ToolCall,         next: Dispatching, action: $tool}
  - {state: Parsing,     signal: Completion,       next: Validating,  action: validate}
  - {state: Dispatching, signal: ToolDone,         next: Composing,   action: invoke_llm}
  - {state: Dispatching, signal: ToolFailed,       next: Composing,   action: invoke_llm}
  - {state: Validating,  signal: ValidationPassed, next: Succeeded}
  - {state: Validating,  signal: ValidationFailed, next: Composing,   action: invoke_llm}
```

From `Composing`, the seed invokes the model; the response is parsed; a tool call is dispatched dynamically through `$tool` and fed back; a completion routes to validation; passing validation succeeds, while a failed validation or tool returns to `Composing` so the model can react, and an exhausted budget routes to `Failed`. Every legal run is an execution in the language this machine defines. Fig. 5 renders the same table as a state machine.

![](figures/fig-05-canonical-machine.png)

| **Figure 5.** State machine diagram. The canonical generator machine of the listing, with edge labels read as `signal / tool`. {wide} |
|:---:|

### Tools are opaque; signals are closed

The machine knows only a tool's name and emittable signals, never its implementation. So tools can be swapped (mock, logging wrapper, remote delegate), implemented in any language or process, and reused across machines unchanged. Each tool's signal set is a closed tool set the machine is validated against at load time. An unhandled signal is a machine error caught before any tool runs.

### The engine is fixed

The engine loop is identical for all workflows and holds no domain logic; if conditionals accumulate there, they belong in a tool or the machine. It exposes four operations:

| Operation | Function | Dependencies |
|---|---|---|
| **Run** | Execute a machine from its initial state | Full stack: machine, registry, LLM adapter |
| **Resume** | Continue from a persisted checkpoint | Full stack + checkpoint |
| **Rollback** | Rewind persisted state with Dolt `Revert`, then reverse external effects through receipts | Checkpoint port, run ID, target step |
| **History** | Format a loaded run's execution log | Checkpoint port, run ID |

Resume re-enters the loop and needs the full machine and registry; Rollback rewinds persisted state and reverses external effects through receipts without loading a machine, which justifies separate entry points.

### Data flow and tool construction

The machine declares control flow but says nothing about data flow. A **builder** bridges one tool's output to the next tool's input: it constructs a tool instance from the previous tool's result, separating tool *selection* (the machine's job) from tool *construction* (data flow). This keeps tools decoupled — each tool takes typed parameters without knowing which tool produced them.

### Dynamic dispatch and recursive composition

The `$tool` slot resolves the tool name at runtime from the preceding result; the machine still defines which signals are handled afterward. The model picks any tool in the registry, but the machine must handle whatever signal it returns. Dispatch is identical to the static case once resolved.

Each machine stays flat and independently validatable; hierarchy emerges from non-terminal tools composing sub-machines. Rollback composes the same way — undoing a non-terminal tool walks the recorded child execution backward. Boundary Tool (Chapter 9) covers the composition model in detail.

### Boundary tools and side-effect declarations

Three types of boundary tool recur, each configured through data rather than compiled:

| Actor type | Example tool | Shaping mechanism |
|---|---|---|
| **Model** | `invoke_llm` | System prompt + tool manifest |
| **Human** | `serve_ui` | The interface presented |
| **Agent** | `run_agent` | The child profile |


## Relationships in the Pattern Language

Machine Interpreter is the root of the language. It contains Agent-as-Data, Tool Contract, Bidirectional Log, Transition Spans, Boundary Tool, Approval Gate, Convergence Taxonomy, and Operator Port as specialized consequences of making the loop explicit. It enables the full set of downstream patterns: Agent-as-Data, Tool Contract, Phase-Scoped Toolset, Inference Boundary, Bidirectional Log, Transition Spans, Boundary Tool, Approval Gate, Convergence Taxonomy, and Operator Port. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**LangGraph** [@langgraph-2024]. Represents agent workflows as directed graphs of Python nodes with callable edge conditions. Its wide adoption shows developers prefer declaring transitions over nesting conditionals, though its conditions, being code, cannot be statically analyzed without execution.

**StateFlow** [@wu-stateflow-2024]. Models task solving as an explicit state machine with outcome-driven transitions, and reports measurable gains over unstructured loops on multi-step benchmarks. That is empirical evidence that making the machine explicit improves agent performance, not just clarity.

**Donna** [@tiendil-donna-2025]. Markdown-defined workflows compiled into finite-state machines for coding agents; the runtime validates reachability before execution, showing machines can be compiled from higher-level notations while preserving static validation.

**Jido** [@jido-2025]. Builds agent systems around explicit state transformations, signal routing, directives, and an FSM execution strategy, while keeping AI/LLM integration optional; it demonstrates dual authorship within one framework.

**Lean4Agent** [@lean4agent-2026]. Formal verification of agent workflows in dependent type theory: structural well-formedness is machine validation, semantic soundness checks tool pre/post-conditions, and trajectory analysis verifies executions, the upper bound of the pattern's verifiability.

**XState** [@xstate-2024]. A widely adopted statechart interpreter for application control flow, demonstrating the interpreter-over-data structure in production front-end and back-end systems.

**SCXML** [@w3c-scxml-2015]. A W3C executable state-machine notation, a direct precedent for serializing reactive control flow as data interpreted by a conforming processor rather than hand-written code.

**BPMN engines** [@omg-bpmn-2011]. Business processes modelled as data and executed by a fixed engine, the same separation of flow-as-data from a generic runtime carried into enterprise workflow tooling.

**AWS Step Functions** [@aws-step-functions-2024]. Serverless workflows declared in the Amazon States Language and executed by a fixed service, a production-scale example of JSON state machines as operational control flow.

**Redux** [@redux-2015]. UI state managed as a reducer over `(state, action) -> state`, the same table-driven transition discipline the pattern applies, here in front-end engineering.

**TLA+** [@lamport-tla-2002] **and translation validation** [@pnueli-translation-validation-1998]. Two formal bookends of pre-run analysis: TLA+ specifies behaviour as states and actions and model-checks over reachable behaviours, while translation validation checks a generated artifact against source-level intent, analogous to load-time verification that the machine faithfully covers every declared tool outcome.

