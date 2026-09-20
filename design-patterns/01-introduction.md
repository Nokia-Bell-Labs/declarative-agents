<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Introduction

Agents have a bug that surfaces only when a probabilistic tool like an LLM
returns an unexpected output for the first time. This problem is inevitable
but undetectable by code review. In an agent, the harness is code wrapping the
LLM tool. A reviewer can read it but cannot trace every path through loops
branching on tool outcomes, including probabilistic ones.

This monograph describes an alternative where the harness's control flow is
expressed as a finite-state machine in a data file and interpreted by a fixed
engine at runtime. The machine defines what the agent can accept at each
phase—which tools are available and which outcomes are valid—rejecting
everything else. Unexpected tool outputs that would silently fall through
imperative code are blocked. If no transition exists, the engine stops or
routes to an explicit error state.

When the agent's behaviour needs to change ( a new or updated tool, tighter
constraints, a new check, or different routing ) the adjustment edits the data
file, not the code.

## Declarative agents


The recent idea of an "agent" arrived in stages, each adding capability and
new failure classes.

1. **Chatbots.** An LLM is wrapped in an interactive chat application. It accumulates a transcript but takes no action in the world, so its mistakes stay in the chat. 
2. **Tool-connected models.** The wrapper wires an LLM model to tools — file writes, shell commands, API calls, other models — often through a protocol such as the Model Context Protocol [@anthropic-mcp-2024]. Now the model *acts*, and because models make mistakes, the errors have real-world consequences — consequences that compound when agents compose into multi-agent systems [@kim-scaling-agents-2025]. 
3. **Harnesses.** The software wrapper routes a model's responses to tool calls and practically improves the performance of code-generating agents with an explicit feedback to the model when things fail [@ning-code-as-harness-2026].

A harness does not fully solve real-world consequences, but it is a step
forward. We argue that a declarative harness can be taken further.

A deployed agent's behaviour is **harness × model.** The model (LLM) supplies
inference; the harness supplies everything else. It manages the environment,
draws the boundary between the model and the system to prevent unchecked bad
actions, and makes available actions explicit so the model knows what it may
do and the operator knows what it might. As we cannot edit the LLM, the
harness is where risk can be managed [@meta-harness-2026] [@autoharness-2026]
[@harbor-harness-optimization-2026].

Harnesses evolve constantly. To know what a new model will do inside an
existing harness, run it, observe results, identify where the harness blocks
or allows incorrectly, and adjust. A model swap may change visible tools,
retry limits, or validation steps. Adding a new capability triggers the same
cycle: edit, run, observe.

Making a harness adjustable requires separating control flow from its
implementation code. Each agent runs the same loop (observe, decide, act,
validate)which functions as a state machine, regardless of its explicit
implementation. Writing it as a transition table, interpreted by a fixed
engine, turns the control flow into a data file, enabling readability and
modification without altering the binary. An agent structured this way (with
its machine, tools, and model binding expressed as data)is a **declarative
agent**.

Harness data is easy to adjust. Imperative control flow (buried in callbacks,
tangled with the system)requires code changes that risk breaking unrelated
behavior and must pass the full development cycle before reaching production.
Data-file control flow allows configuration changes without altering
imperative logic, keeps the full flow readable in the transition table, and
deploys without rebuilding the binary.

Fig. 1 shows the architecture. The Engine, a fixed binary, reads the Machine
(a state machine transition table) and dispatches Tools by name. The Registry
tracks tools (`read`, `write`, `test`, `invoke_llm`), which are the agent's
verbs. `invoke_llm` crosses the model boundary, returning probabilistic
results; other tools are deterministic. The Model is external, accessed only
via its tool. Swapping the model is a configuration change that does not
affect the Engine or other tools.

![](figures/fig-01-architecture.png)

| **Figure 1.** Component diagram. The Engine reads the Machine and dispatches Tools via the Registry. |
| :--------------------------------------------------------------------------------------------------: |

The data representation has a second benefit: each state corresponds to an
execution phase, where the agent runs a tool, producing an outcome that maps
to the next state. The full set of (state, outcome) → next-state mappings
forms the transition table. As the table is finite, a loader can perform
static checks before the agent starts—reachability, terminal reachability,
determinism, and completeness [@harel-statecharts-1987]
[@pnueli-translation-validation-1998].

## Related work


Most systems orchestration has already shifted from imperative code to
declarative data. Cluster management evolved from sequential command scripts
to declarative configurations. Now, operators define the desired state as
data, and a control loop enforces it. Kubernetes exemplifies this: a data
object specifies the desired state, and the system determines the steps to
achieve it [@burns-borg-omega-kubernetes-2016].

In agentic space, two general approaches exist. Agentic frameworks like
LangGraph and Temporal use an imperative approach. LangGraph nodes and edge
conditions are Python functions [@langgraph-2024]. Temporal provides durable
execution and human-in-the-loop waits, but workflows remain imperative
[@temporal-2024]. Declarative approaches, such as XState and BPMN, are generic
state machine tools unaware of agents. XState implements Harel statecharts for
application control flow [@xstate-2024] [@harel-statecharts-1987]; BPMN models
business processes [@omg-bpmn-2011].

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

We use a **generator** agent example throughout the monograph. It reads files,
writes code, runs tests, and loops until validation passes. It comprises three
parts.

### The machine


The **machine** is the transition table, the agent's control flow as data.

```yaml
# machine.yaml
name: generator
initial_state: Composing
terminal_states: [Succeeded, Failed]
transitions:
  - { state: Composing,   signal: Seed,             next: Composing,   action: invoke_llm }
  - { state: Composing,   signal: LLMResponded,     next: Parsing,     action: parse_response }
  - { state: Parsing,     signal: ToolCall,         next: Dispatching, action: $tool }
  - { state: Parsing,     signal: Completion,       next: ValidatingBuild, action: build }
  - { state: Dispatching, signal: ToolDone,         next: Composing,   action: invoke_llm }
  - { state: ValidatingBuild, signal: ToolDone,     next: ValidatingLint, action: lint }
  - { state: ValidatingBuild, signal: ToolFailed,   next: Composing,   action: invoke_llm }
  - { state: ValidatingLint,  signal: ToolDone,     next: ValidatingTest, action: test }
  - { state: ValidatingLint,  signal: ToolFailed,   next: Composing,   action: invoke_llm }
  - { state: ValidatingTest,  signal: ToolDone,     next: Succeeded }
  - { state: ValidatingTest,  signal: ToolFailed,   next: Composing,   action: invoke_llm }
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
  - build           # explicit validation pipeline
  - lint
  - test
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

Introduce a bug: delete the `ValidatingTest` transition for `ToolFailed`.
Though the happy path (`ToolDone` to `Succeeded`) appears complete, the agent
never starts. The engine loads machine and tool declarations, checks that each
tool's signals have corresponding transitions in dispatchable states. And
finds a gap: `test` can emit `ToolFailed`, but the machine lacks a transition
for it. The engine rejects the machine before the model is called. The bug is
a load-time error, not a silent dead end found when a test fails in
production.

## Design patterns for declarative agents


The following chapters describe eleven design patterns for building
declarative agents, organized like the Gang of Four [@gamma-gof-1994]. Each
pattern isolates a recurring problem in agent construction — expressing the
loop, scoping tools per phase, swapping models, rolling back effects,
delegating to sub-agents, classifying outcomes — and names a transferable
solution across teams, frameworks, and model generations. These patterns
derive from a working implementation, demonstrated by the reference harness's
coding agent throughout these chapters.

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

**The core idea** (Chapters 2--4). **Machine Interpreter** (Chapter 2) makes
the agent loop an explicit state machine interpreted by a fixed engine.
**Agent-as-Data** (Chapter 3) extends that to the whole agent — machine,
tools, and model binding as a single dataset. **Tool Contract** (Chapter 4)
specifies the typed interface each tool exposes to the machine: parameters,
emittable signals, and reversibility.

**Operational patterns** (Chapters 5--8). **Phase-Scoped Toolset** declares
which tools the model may call in each phase, making agent-computer interfaces
explicit while allowing the harness to verify tool reachability before the
agent runs [@yang-swe-agent-2024]. **Inference Boundary** isolates model
inference behind a single tool. **Bidirectional Log** adds bidirectional
traversal for undo and recovery. **Transition Spans** map execution onto
OpenTelemetry spans.

**Composition and oversight** (Chapters 9--10). **Boundary Tool** composes
agents hierarchically through non-terminal tools. **Approval Gate** makes
human oversight a first-class machine transition.

**Diagnostics** (Chapters 11--12). **Convergence Taxonomy** turns execution
traces into an actionable diagnosis, classifying how each run converged rather
than reporting a bare pass or fail, so every outcome points to a distinct root
cause and remedy. **Operator Port** attaches live observers to a running
machine, making agent state queryable and signals injectable without modifying
the machine or tools.

### How to read each chapter


Every pattern chapter follows the Gang of Four structure: **Intent** states
the purpose in one sentence, **Motivation** presents the problem scenario,
**Applicability** lists when to use and when not to use the pattern,
**Structure** names the participants and shows their relationships,
**Collaborations** describes the runtime interactions, **Consequences** lists
benefits and liabilities, **Implementation** provides specific guidance and
code examples, and **Known Uses** grounds the pattern in working deployments.
