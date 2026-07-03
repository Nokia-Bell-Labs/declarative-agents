# Introduction

Every agent has a bug that will surface only the first time a probabilistic tool like an large language model (LLM) returns an output no one expected. A problem like this is certain to occur, but impossible to find by reading the code. In an agent, the harness is code that wraps around the LLM tool. A reviewer can read it, but cannot trace every path through a loop that branches on every tool outcome, including from probabilistic tools. 

This monograph describes an alternative, where the harness's control flow is expressed as a finite-state machine in a data file and interpreted by a fixed engine at runtime. The machine defines what the agent is allowed to accept at each phase — which tools are available in a phase and which tool outcomes are valid — and rejects everything else. An unexpected tool output that would silently fall through imperative code hits a wall. If no transition exists, the engine stops or routes to an explicit error state. 

More practically, when the agent's behaviour needs to change — a new or updated tool, tighter constraints, a new check, a different routing — the adjustment is an edit to the data file, not a code change.
## Declarative agents

The recent idea of an "agent" arrived in stages, each adding capability and each adding a new class of failure:

1. **Chatbots.** An LLM is wrapped in an interactive chat application. It accumulates a transcript but takes no action in the world, so its mistakes stay in the chat. 
2. **Tool-connected models.** The wrapper wires an LLM model to tools — file writes, shell commands, API calls, other models — often through a protocol such as the Model Context Protocol [@anthropic-mcp-2024]. Now the model *acts*, and because models make mistakes, the errors have real-world consequences — consequences that compound when agents compose into multi-agent systems [@kim-scaling-agents-2025]. 
3. **Harnesses.** The software wrapper routes a model's responses to tool calls and practically improves the performance of code-generating agents with an explicit feedback to the model when things fail [@ning-code-as-harness-2026].

A harness does not fully solve the problem of real-world consequences, but it is a step in the right direction. Here we argue that the harness can be taken further if it is declarative.

A deployed agent's behaviour is the product of two factors: **harness × model.** The model (LLM)  supplies inference. The harness supplies everything else. It manages the environment, draws the boundary between the model and the system so that a bad action cannot reach the system unchecked, and makes the available actions explicit so the model knows what it may do and the operator knows what it might. Since we cannot edit the LLM, the harness is where risk can be managed [@meta-harness-2026] [@autoharness-2026] [@harbor-harness-optimization-2026].

Harnesses evolve constantly. The only way to know what a new model will do inside an existing harness is to run it — observe the results, find where the harness blocks what it should allow or allows what it should block, and adjust. A model swap may mean changing which tools are visible in a given phase, how many retries are allowed, or what validation runs before the agent may declare success. Adding a new capability — a new tool, a new workflow phase — triggers the same cycle: edit, run, observe.

The way to make a harness adjustable is to separate the control flow from the code that implements it. Every agent runs the same loop — observe, decide, act, validate — and that loop is a state machine whether or not it is written as one. Writing it explicitly, as a transition table, interpreted by a fixed engine, makes the control flow a data file: readable and changeable without touching the binary. An agent built this way — its machine, its tools, and its model binding all expressed as data — is a **declarative agent**.

Harness as data is easy to adjust. When the control flow is imperative code — buried in callbacks, tangled with the rest of the system — every adjustment is a code change that carries the risk of breaking unrelated behaviour, and that must pass through the full development cycle before it reaches production. When the control flow is a data file, the adjustment is a configuration change. The imperative logic is untouched, the full control flow is readable in the transition table, and the change can be deployed without rebuilding the binary.

Fig. 1 shows the architecture. The Engine is a fixed binary. It reads the Machine — a state machine transition table — and dispatches Tools by name. The tool Registry keeps track of tools, which are the agent's verbs: `read`, `write`, `validate`, `invoke_llm`. One tool, `invoke_llm`, crosses the model boundary and returns probabilistic results. Other tool are deterministic. The Model is external to the agent. The engine reaches it only through its tool, and swapping the model is a configuration change that touches neither the engine nor the other tools.

![](figures/fig-01-architecture.png)

| **Figure 1.** Component diagram. The Engine reads the Machine and dispatches Tools via the Registry. |
| :--------------------------------------------------------------------------------------------------: |

The data representation carries a second benefit. Each state corresponds to a phase of execution; in each execution phase, the agent runs a tool; each tool produces an outcome; and each outcome maps to the next state. The full set of (state, outcome) → next-state mappings is the transition table. Because the table is finite data, a loader can run static checks on it before the agent starts — reachability, terminal reachability, determinism, and completeness [@harel-statecharts-1987] [@pnueli-translation-validation-1998]. 
## Related work

The move from imperative code to declarative data is the path most systems orchestration already took. Cluster management was once workflow scripts that issued commands in a sequence. It is now declarative. An operator writes the desired state as data and a fixed control loop reconciles reality to it. Kubernetes is the familiar case — a data object describes what should be true, and the system, not a script, decides the steps to make it so [@burns-borg-omega-kubernetes-2016]. 

In the agentic space, there are two general approaches. Agentic frameworks such as LangGraph and Temporal, use an imperative approach. LangGraph nodes and edge conditions are Python functions [@langgraph-2024]. Temporal gives durable execution and human-in-the-loop waits, but workflows are impertaive workflows [@temporal-2024]. Declarative approaches with generic state machine tools such as XState and BPMN know nothing about agents. XState implements Harel statecharts for application control flow [@xstate-2024] [@harel-statecharts-1987]; BPMN models business processes [@omg-bpmn-2011]. 

Table I summarizes the differences.

| **Table I.** Capability comparison across related tools. |
|:---|

| Capability                       | LangGraph | Temporal |   XState   |    BPMN    |   This book   |
| -------------------------------- | :-------: | :------: | :--------: | :--------: | :-----------: |
| Behaviour is data, not code      |  partial  |    no    |    yes     |    yes     |      yes      |
| Outcomes checked before running  |    no     |    no    | structural | structural |    **yes**    |
| Model boundary + per-phase tools |    no     |    no    |     no     |     no     |    **yes**    |
| Built-in undo / rollback         |    no     |  coded   |     no     |   coded    |    **yes**    |
| Suspend/resume across restarts   |  add-on   |   yes    |     no     |    yes     |      yes      |
| Outcome classification           |    no     |    no    |     no     |     no     |    **yes**    |
| Runs non-model pipelines too     |    ---    |   yes    |    yes     |    yes     |      yes      |
| Footprint                        |  library  | cluster  |  library   |   server   | binary + YAML |
## A declarative agent

We use an example of a code **generator** agent throughout the monograph. It can read files, write code, run the tests, and loop until validation passes. It can be defined in three parts.
### The machine

The **machine** is the transition table, the agent's whole control flow as data:

```yaml
# machine.yaml
name: generator
initial_state: Composing
terminal_states: [Succeeded, Failed]
transitions:
  - { state: Composing,   signal: Seed,             next: Composing,   action: invoke_llm }
  - { state: Composing,   signal: LLMResponded,     next: Parsing,     action: parse_response }
  - { state: Parsing,     signal: ToolCall,         next: Dispatching, action: $tool }
  - { state: Parsing,     signal: Completion,       next: Validating,  action: validate }
  - { state: Dispatching, signal: ToolDone,         next: Composing,   action: invoke_llm }
  - { state: Validating,  signal: ValidationPassed, next: Succeeded }
  - { state: Validating,  signal: ValidationFailed, next: Composing,   action: invoke_llm }
  # ... ToolFailed retries via Composing; BudgetExceeded routes to Failed
```

The same control flow as a diagram appears in Fig. 2.

![](figures/fig-02-generator-state-machine.png)

| **Figure 2.** State machine diagram. The generator agent's control flow from `machine.yaml`: states are phases, edges are signals with their actions. The model speaks only at the `$tool` boundary out of `Parsing`; every other transition is fixed by the table. `ToolFailed`, elided from the YAML above, retries via `Composing`; `BudgetExceeded` routes to `Failed`. {wide} |
|:---:|
### The Tools
The **tool selection** names what the machine may dispatch; full contracts (parameters, emitted signals, reversibility) live in the referenced declarations:

```yaml
# tools.yaml
tools:
  - read            # file tools the model
  - write           #   calls via $tool dispatch
  - edit            #
  - invoke_llm      # the model boundary (Composing)
  - parse_response  # extract the tool call (Parsing)
  - done            # the model declares the task complete
  - validate        # run the build and tests
```
### The profile
The **profile** binds the machine to its tools and names the model to call:

```yaml
# profile.yaml
name: generator
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [llm/default.yaml]   # provider, model, system prompt
tool_config_dirs:
  - /opt/agent-core/tools/builtin/filesystem
  - /opt/agent-core/tools/exec/go
```

Now introduce a bug: delete the `Validating` transition for `ValidationFailed`. It is tempting, because the happy path (`ValidationPassed` to `Succeeded`) still looks complete. The agent never starts. The engine loads the machine and the tool declarations, checks that every signal each tool can emit has a corresponding transition in every state where that tool can be dispatched, and finds a gap: `validate` declares it can emit `ValidationFailed`, but the machine has no transition for it. The engine rejects the machine before the model is called even once. The bug is a load-time error, not a silent dead end found the first time a test fails in production.
## Design patterns for declarative agents

The chapters that follow describe eleven design patterns for building declarative agents, organized in the style of the Gang of Four [@gamma-gof-1994]. Each pattern isolates one recurring problem in agent construction — expressing the loop, scoping tools per phase, swapping models, rolling back effects, delegating to sub-agents, classifying outcomes — and names a solution that transfers across teams, frameworks, and model generations. The patterns come from a working implementation; the reference harness whose coding agent appears throughout these chapters.
### Pattern catalog


| **Table II.** Pattern catalog. |
|:---|

| Ch. | Pattern               | GoF Category | Problem                                                                                                                      | GoF Relatives                       |
| --- | --------------------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| 2   | Machine Interpreter   | Behavioral   | The agent loop is reimplemented ad hoc in every agent, behaviour entangled with control flow that only code edits can change | Interpreter, State, Template Method |
| 3   | Agent-as-Data         | Creational   | Agent definitions are scattered across code, config, and deployment scripts                                                  | Abstract Factory, Prototype         |
| 4   | Tool Contract         | Structural   | Tool interfaces are implicit, undocumented, and untestable in isolation                                                      | Adapter, Facade                     |
| 5   | Phase-Scoped Toolset  | Behavioral   | Models receive tools irrelevant to the current phase, inflating prompts and inviting misuse                                  | Strategy, Flyweight                 |
| 6   | Inference Boundary    | Structural   | Model-specific API calls are scattered throughout the harness                                                                | Adapter, Bridge                     |
| 7   | Bidirectional Log     | Behavioral   | Agent mistakes compound because no undo mechanism exists                                                                     | Memento, Command                    |
| 8   | Transition Spans      | Behavioral   | Agent execution is opaque; debugging and evaluation rely on ad-hoc logging                                                   | Observer, Visitor                   |
| 9   | Boundary Tool         | Structural   | Multi-agent coordination is wired imperatively, tightly coupling parent and child                                            | Composite, Proxy                    |
| 10  | Approval Gate         | Behavioral   | Human oversight requires ad-hoc blocking or polling, losing execution state across process boundaries                        | Memento, Chain of Responsibility    |
| 11  | Convergence Taxonomy  | Behavioral   | Agent success or failure is a binary verdict with no actionable diagnosis                                                    | Strategy, Interpreter               |
| 12  | Operator Port         | Behavioral   | Running agents can only be observed through logs and controlled by killing the process                                       | Observer, Mediator                  |
### How patterns are organized

The catalogue has four groups.

**The core idea** (Chapters 2--4). **Machine Interpreter** (Chapter 2) makes the agent loop an explicit state machine interpreted by a fixed engine. **Agent-as-Data** (Chapter 3) extends that to the whole agent — machine, tools, and model binding as a single dataset. **Tool Contract** (Chapter 4) specifies the typed interface each tool exposes to the machine: parameters, emittable signals, and reversibility.

**Operational patterns** (Chapters 5--8). **Phase-Scoped Toolset** declares which tools the model may call in each phase, making agent-computer interfaces explicit while allowing the harness to verify tool reachability before the agent runs [@yang-swe-agent-2024]. **Inference Boundary** isolates model inference behind a single tool. **Bidirectional Log** adds bidirectional traversal for undo and recovery. **Transition Spans** map execution onto OpenTelemetry spans.

**Composition and oversight** (Chapters 9--10). **Boundary Tool** composes agents hierarchically through non-terminal tools. **Approval Gate** makes human oversight a first-class machine transition.

**Diagnostics** (Chapters 11--12). **Convergence Taxonomy** turns execution traces into an actionable diagnosis, classifying how each run converged rather than reporting a bare pass or fail, so every outcome points to a distinct root cause and remedy. **Operator Port** attaches live observers to a running machine, making agent state queryable and signals injectable without modifying the machine or tools.
### How to read each chapter

Every pattern chapter follows the Gang of Four structure: **Intent** states the purpose in one sentence, **Motivation** presents the problem scenario, **Applicability** lists when to use and when not to use the pattern, **Structure** names the participants and shows their relationships, **Collaborations** describes the runtime interactions, **Consequences** lists benefits and liabilities, **Implementation** provides specific guidance and code examples, and **Known Uses** grounds the pattern in working deployments. 