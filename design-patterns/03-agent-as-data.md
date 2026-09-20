<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Agent-as-Data

Agent-as-Data packages the complete agent definition—machine, tool selection,
and model binding—as three data files interpreted by a fixed binary. The
profile assembles these files, and the loader validates them before the first
transition, transforming agents into versioned, composable, and
interchangeable components, eliminating underlying code modifications.

## Intent

Express the complete agent definition as data — machine, tools, and model
binding — making the agent a dataset the engine runs, not behaviour buried in
code.


## Motivation

The Machine Interpreter agent is a dataset interpreted by a fixed program, not
a program itself. Scattered agent definitions across code, config, and
deployment scripts prevent unified review, versioning, or governance. Instead,
three YAML layers define the agent, interpreted by a single compiled binary.

- **Machine** (`machine.yaml`): the machine, with states, signals, transitions, and terminal states (Chapter 2). No logic, just a lookup table.
- **Tools** (`declarations/*.yaml`): each tool's name, type, parameters, emittable signals, contract, reversibility, side effects, and relationships (Chapter 4).
- **Profile** (`profile.yaml`): runtime binding for which model, temperature, budget, tool-declaration roots, and machine to run.

The interpreter loads the three layers, validates them against each other
(every referenced tool has a declaration; every emittable signal is handled),
builds the runtime objects, and enters the dispatch loop. It can run *any*
agent whose YAML conforms to the schema. One binary, N agents. Deploying a new
agent means writing YAML rather than compiling code.


## Applicability

Agent-as-Data is applicable when various agents—coding, evaluation,
benchmarking, and spec validation—are designed to operate from a single
binary, differentiated by configuration rather than code. The approach pays
off when agent behaviour must be reviewed, versioned, and governed as data,
when models need to be interchangeable to isolate their impact during
evaluation, and when the whole agent must validate before deployment. A single
fixed binary that interprets configuration is also simpler to manage compared
to a set of specialized programs.


## Structure

Three declarative layers feed one fixed interpreter, as illustrated in the
component diagram of Fig. 6. The compiled binary (Engine, Registry +
Factories, Adapters) provides capability, while the YAML provides specificity.
Agent-specific behaviour never appears in the binary.

![](figures/fig-07-declarative-agent-components.png)

| **Figure 6.** Component diagram. The Profile, Machine, and Tool artifacts configure a fixed Go interpreter (Engine, Registry, Adapters); the data defines the agent, the binary never changes. |
|:---:|

### Participants

#### Machine (Artifact)

The machine artifact explicitly defines state, signal, transition, initial
state, and terminal states as data. It contains no runtime logic, serving as
the control-flow table that the engine interprets.

#### Tool Declarations (Artifacts)

Tool artifacts specify essential details for each tool, including its name,
type, parameters, the signals it can emit, associated contract, side effects,
reversibility, and relationships. These artifacts outline the invokable
elements and the outcomes that must be handled.

#### Profile (Artifact)

The profile artifact binds runtime configuration, including model and provider
settings, budgets, machine path, and tool-declaration roots. By altering these
parameters, different profiles can generate distinct agents from the same
underlying binary. This mechanism allows for flexible and context-specific
agent creation, tailored to the requirements of the task at hand.

#### Engine (Runtime)

The engine is the fixed interpreter loop that reads machine transitions,
dispatch tools, records execution, and routes returned signals. This engine is
shared across all profiles.

#### Registry + Factories (Runtime)

The registry indexes tool declarations by name; factories instantiate live
tools from those declarations by type (`exec`, `rest`, `builtin`, boundary).
Validation ensures every referenced tool resolves before execution starts.

#### Adapters (Ports)

Adapters deliver concrete implementations for ports, such as LLM, telemetry,
filesystem, and HTTP. By isolating external systems, they ensure the
declarative agent definition maintains a data-first approach and remains
swappable.

The shipped `executor`, `critic`, `bench`, and `jurist` profile families share
a common binary reference. In other parts of this monograph, the terms
*generator* and *evaluator* describe conceptual roles. But they do
actually not correspond to any existing profile family names in the current
catalog.


## Collaborations

Between YAML on disk and a dispatched tool lies a four-stage loading pipeline,
illustrated in the activity diagram of Fig. 7. **Profile resolution** extracts
the tool-declaration roots, identifying the foundational elements. **Catalog
loading** parses each `.yaml` file into a ToolDef and validates its schema,
ensuring malformed declarations are caught at load time rather than during
dispatch. **Registry construction** registers ToolDefs by name, enabling
machine validation to confirm that every referenced tool exists and every
emitted signal is properly handled. **Factory dispatch** passes each ToolDef
to its corresponding factory, init-gated to ensure shared resources, such as
HTTP clients and file handles, are instantiated only once.

![](figures/fig-08-loading-pipeline.png)

| **Figure 7.** Activity diagram. The loading pipeline: profile → catalog → registry → factory → live tool, all before the first transition. {0.7} |
|:---:|

At startup, all four stages are executed, ensuring that runtime dispatch
consists solely of a lookup and a call, with no additional construction
required.


## Consequences

### Benefits

#### Diffability

Agent changes are presented as straightforward YAML diffs, allowing reviewers
to easily read and approve them, rather than edits buried within function
bodies.

#### Lintability

Schema, machine (reachability, signal completeness), and contract
(four-question test) validation are executed within the continuous integration
(CI) pipeline, enabling the detection of errors prior to deployment without
the need to run the agent.

#### Versionability

Agent versions are captured as tagged YAML snapshots, allowing rollbacks
through a checkout process that preserves field-by-field history.

#### Model swapping and evaluation isolation

Switching backends requires only a one-field edit in `profile.yaml`; since the
machine and tools are model-independent, evaluation measures the model's
contribution alone, made quantitative by the fixed trace format (Chapter 8).

### Liabilities

#### An imperative floor

Engine internals, port adapters, type factories (`ExecBuilder`, REST), and
builtins whose semantics no schema can capture (response parsing, git-worktree
management, nested-machine orchestration) remain compiled Go. Adding a new
tool *type* or provider requires writing code rather than YAML.

#### Schema expressiveness

Anything outside the declared field set cannot be configured; novel semantics
necessitate the creation of a new builtin or factory instead of relying on a
declaration.


## Implementation

### The tool declaration

Each tool is a YAML declaration that the registry reads to build a live tool.
The core fields:

```yaml
name: build_project
type: exec            # exec | rest | builtin | boundary — selects the factory
parameters:
  - {name: working_directory, type: string, required: true}
  - {name: target, type: string, required: false, default: "./..."}
emits: [ToolDone, ToolFailed, BudgetExceeded]
contract:             # full Chapter 4 contract: problem, goals, non_goals, ...
  goals: ["Compile the project at the specified path."]
reversibility: reversible
side_effects: [{filesystem_write: "Build artifacts in output directory."}]
relationships: {precedes: [run_tests], follows: [write_file]}
```

The `type` parameter selects the factory, while the `emits` value is verified
against the machine. The `contract`, `reversibility`, and `side_effects`
attributes are integral to planning, rollback mechanisms (Chapter 7), and
static analysis processes.

### CLI and REST tools

Most coding-agent tools rely on shell commands. An `exec` declaration maps
parameters to argv using a `parameter_map` (including `flag`, `positional`,
`bool_flag`, and `default` types). The generic `ExecBuilder` constructs the
command, runs it, and maps the exit code to a signal. Adding a CLI tool (`go
test`, `git commit`, `make`) is handled through a declaration rather than by
writing Go code.

```yaml
name: git_commit
type: exec
exec: {binary: git, args: [commit],
       parameter_map: [{parameter: message, flag: "-m"},
                       {parameter: all, bool_flag: "-a"}]}
```

HTTP APIs utilize the `rest` type, where a `rest.yaml` file specifies the base
URL, authentication details, and headers. Per-endpoint declarations within
this file map the method, path, body template, and response fields to a tool,
bound to an `init` client (`rest_client_ollama`). Endpoint declarations can be
generated from an OpenAPI spec [@openapi-spec-2024] and then edited. Once
built, CLI and REST tools share the same lifecycle, with the only difference
being the factory used for their creation.

### Profile composition

A profile references tool-declaration roots, where multiple profiles share a
root to inherit a common tool set, and a profile-specific root overrides
shared declarations, with the later root taking precedence. The package
diagram in Fig. 8 demonstrates the executor and critic profiles importing a
shared `core/` library in addition to their own overrides.

![](figures/fig-09-profile-packages.png)

| **Figure 8.** Package diagram. Profiles import a shared `core/` tool library and add profile-specific override directories, reusing the common tool set without duplication. |
|:---:|

Composition operates at two levels: shared declarations eliminate redundancy
(a single `write_file.yaml` for all), while each profile binds its own model,
machine, and budget. Swapping a backend requires an edit to the `llm.model`
field. The machine, tools, and traces remain unchanged, ensuring that
inference quality is the sole variable under consideration.


## Relationships in the Pattern Language

Agent-as-Data resides within the Machine Interpreter framework, relying on
both the Machine Interpreter and Tool Contract to function. This integration
allows the profile to package an agent, leveraging the inherently declarative
nature of the loop and associated tools. It contains and enables the
Phase-Scoped Toolset and Inference Boundary, as scoped manifests and model
bindings are elevated to profile-level data, eliminating the need for code
modifications. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**One binary, many agents.** The `executor`, `critic`, `bench`, and `jurist`
families are profile directories over the same binary. New agents and tools
are written in YAML, and compiled-code changes are infrequent and confined to
adapters and factories.

**Model-comparison grids.** Executor profiles, sharing a machine and tools,
bind distinct `llm` configurations. The bench/critic stack (Chapter 9)
executes the grid, attributing outcome variations to the model, as the critic
verifies byte-for-byte consistency across runs on the same machine.

**CI validation gates.** Schema, machine, and contract validation are executed
for every change, ensuring that an agent with an unreachable state, an
unhandled signal, or an incomplete tool contract fails the build before
deployment.

**Orchestration through declared data.** **Kubernetes**
[@burns-borg-omega-kubernetes-2016] implements this principle beyond the agent
domain: an operator specifies the desired state as data, and a fixed control
loop reconciles reality to it, achieving orchestration via data rather than
scripts. This separation aligns with **Functional Core, Imperative Shell**
[@bernhardt-fcis-2012], where a fixed imperative shell (the interpreter)
encapsulates declarative, data-first definitions.

**The harness as a first-class artifact.** Recent research increasingly treats
the runtime environment surrounding a model as an engineered object in its own
right, corroborating the claim that an agent's definition is a governable
artifact separate from its model. **Meta-Harness** [@meta-harness-2026]
approaches the scaffolding around LLM calls as an optimizable object.
**AutoHarness** [@autoharness-2026] automates the generation of harness code
to enhance agent performance. **Code as Agent Harness**
[@ning-code-as-harness-2026] conceptualizes agent systems as executable,
verifiable, stateful harnesses. **HARBOR** [@harbor-harness-optimization-2026]
directly optimizes coding-agent harnesses; and a study toward a science of
scaling agent systems [@kim-scaling-agents-2025] highlights scaling failures
beyond single-model behaviour. Each of these works reinforces that harness
structure materially shapes behaviour and must be managed as a system
artifact.

**A contrast: durable execution as code.** **Temporal** [@temporal-2024]
provides durable execution with human-in-the-loop waits, yet its workflows are
written in imperative code: the agent cannot be reviewed as pure data, thereby
marking the boundary of what this pattern necessitates.
