# Agent-as-Data

This chapter presents the Agent-as-Data pattern, which packages the complete agent definition — machine, tool selection, and model binding — as three data files interpreted by a fixed binary. The chapter covers how the profile assembles these files, how the loader validates them before the first transition, and how the pattern enables agents to be versioned, composed, and swapped without touching code.

## Intent

Express the complete agent definition as data — machine, tools, and model binding — so the agent is a dataset the engine runs, not behaviour buried in code.


## Motivation

An agent built on the Machine Interpreter is not a program; it is a dataset interpreted by a fixed program. When agent definitions are scattered across code, config, and deployment scripts, behaviour cannot be reviewed, versioned, or governed as a unit. Three YAML layers instead define the agent, and one compiled binary interprets all of them:

- **Machine** (`machine.yaml`): the machine, with states, signals, transitions, and terminal states (Chapter 2). No logic, just a lookup table.
- **Tools** (`declarations/*.yaml`): each tool's name, type, parameters, emittable signals, contract, reversibility, side effects, and relationships (Chapter 4).
- **Profile** (`profile.yaml`): runtime binding for which model, temperature, budget, tool-declaration roots, and machine to run.

The interpreter loads the three layers, validates them against each other (every referenced tool has a declaration; every emittable signal is handled), builds the runtime objects, and enters the dispatch loop. It can run *any* agent whose YAML conforms to the schema. One binary, N agents. Deploying a new agent means writing YAML, not compiling code.


## Applicability

Agent-as-Data fits when multiple different agents — coding, evaluation, benchmarking, spec validation — should run from one binary, differing in configuration rather than code. The pattern becomes more valuable when agent behaviour needs to be reviewed, versioned, and governed as data; when models should be swappable to isolate their contribution during evaluation; and when pre-deployment validation of the whole agent matters. A single fixed binary that interprets configuration is also easier to operate than a collection of purpose-built programs.


## Structure

Three declarative layers feed one fixed interpreter, shown as a component diagram in Fig. 6. The compiled binary (Engine, Registry + Factories, Adapters) provides capability; the YAML provides specificity. Agent-specific behaviour never appears in the binary.

![](figures/fig-07-declarative-agent-components.png)

| **Figure 6.** Component diagram. The Profile, Machine, and Tool artifacts configure a fixed Go interpreter (Engine, Registry, Adapters); the data defines the agent, the binary never changes. |
|:---:|

### Participants

#### Machine (Artifact)

The machine artifact defines state, signal, transition, initial state, and terminal states as data. It contains no runtime logic; it is the control-flow table the engine interprets.

#### Tool Declarations (Artifacts)

Tool artifacts declare each tool's name, type, parameters, emittable signals, contract, side effects, reversibility, and relationships. They define what can be invoked and what outcomes must be handled.

#### Profile (Artifact)

The profile artifact binds runtime configuration: model/provider settings, budgets, machine path, and tool-declaration roots. Different profiles produce different agents over the same binary.

#### Engine (Runtime)

The engine is the fixed interpreter loop that reads machine transitions, dispatches tools, records execution, and routes returned signals. It is shared across all profiles.

#### Registry + Factories (Runtime)

The registry indexes tool declarations by name; factories instantiate live tools from those declarations by type (`exec`, `rest`, `builtin`, boundary). Validation ensures every referenced tool resolves before execution starts.

#### Adapters (Ports)

Adapters provide concrete implementations behind ports (LLM, telemetry, filesystem, HTTP). They isolate external systems so the declarative agent definition remains data-first and swappable.

A coding agent (`generator`), an evaluator (`evaluator`), a benchmark orchestrator (`bench`), and a deterministic spec validator (`jurist`) are four profile directories pointing at the same binary.


## Collaborations

Between YAML on disk and a dispatched tool sits a four-stage loading pipeline, shown as an activity diagram in Fig. 7. **Profile resolution** extracts the tool-declaration roots. **Catalog loading** parses every `.yaml` into a ToolDef and validates its schema, so malformed declarations fail at load, not dispatch. **Registry construction** registers ToolDefs by name and lets machine validation confirm every referenced tool exists and every emitted signal is handled. **Factory dispatch** hands each ToolDef to its type's factory, init-gated so shared resources (HTTP clients, file handles) are created once.

![](figures/fig-08-loading-pipeline.png)

| **Figure 7.** Activity diagram. The loading pipeline: profile → catalog → registry → factory → live tool, all before the first transition. {0.7} |
|:---:|

All four stages run at startup, so runtime dispatch is a lookup and a call, with no construction left to do.


## Consequences

### Benefits

#### Diffability

Agent changes are surface-level YAML diffs reviewers can read and approve, not edits hidden in function bodies.

#### Lintability

Schema, machine (reachability, signal completeness), and contract (four-question test) validation run in CI, catching errors before deployment without running the agent.

#### Versionability

Agent versions are tagged YAML snapshots; rolling back is a checkout, with field-by-field history.

#### Model swapping and evaluation isolation

Switching backends is a one-field edit in `profile.yaml`; because machine and tools are model-independent, evaluation measures the model's contribution alone, made quantitative by the fixed trace format (Chapter 8).

### Liabilities

#### An imperative floor

Engine internals, port adapters, type factories (`ExecBuilder`, REST), and builtins whose semantics no schema can capture (response parsing, git-worktree management, nested-machine orchestration) remain compiled Go. Adding a new tool *type* or provider means writing code, not YAML.

#### Schema expressiveness

Anything outside the declared field set cannot be configured; novel semantics require a new builtin or factory rather than a declaration.


## Implementation

### The tool declaration

Each tool is a YAML declaration the registry reads to build a live tool. The core fields:

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

`type` selects the factory; `emits` is checked against the machine; `contract`, `reversibility`, and `side_effects` feed planning, rollback (Chapter 7), and static analysis.

### CLI and REST tools

Most coding-agent tools are shell commands. An `exec` declaration maps parameters to argv via a `parameter_map` (`flag`, `positional`, `bool_flag`, `default`); the generic `ExecBuilder` constructs the command, runs it, and maps exit code to signal. Adding a CLI tool (`go test`, `git commit`, `make`) is a declaration, not Go code.

```yaml
name: git_commit
type: exec
exec: {binary: git, args: [commit],
       parameter_map: [{parameter: message, flag: "-m"},
                       {parameter: all, bool_flag: "-a"}]}
```

HTTP APIs use a `rest` type: a `rest.yaml` holds base URL, auth, and headers; per-endpoint declarations map method, path, body template, and response fields to a tool, bound to an `init` client (`rest_client_ollama`). Endpoint declarations can be generated from an OpenAPI spec [@openapi-spec-2024] and then edited. CLI and REST tools share the same lifecycle once built; only the factory differs.

### Profile composition

A profile references tool-declaration roots; multiple profiles share a root to inherit a common tool set, and a profile-specific root overrides shared declarations (later root wins). The package diagram in Fig. 8 shows the generator and evaluator profiles importing a shared `core/` library plus their own overrides.

![](figures/fig-09-profile-packages.png)

| **Figure 8.** Package diagram. Profiles import a shared `core/` tool library and add profile-specific override directories, reusing the common tool set without duplication. |
|:---:|

Composition operates at two levels: shared declarations avoid duplication (one `write_file.yaml` for all), while each profile binds its own model, machine, and budget. Swapping a backend edits one `llm.model` field. Machine, tools, and traces are untouched, isolating inference quality as the only variable.


## Relationships in the Pattern Language

Agent-as-Data sits within Machine Interpreter and requires both Machine Interpreter and Tool Contract: the profile can package an agent only because the loop and tools are already declarative. It contains and enables Phase-Scoped Toolset and Inference Boundary, because scoped manifests and model bindings become profile-level data rather than code changes. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**One binary, many agents.** A coding agent (`generator`), an evaluator (`evaluator`), a benchmark orchestrator (`bench`), and a deterministic spec validator (`jurist`) are four profile directories over the same binary; new agents and tools are written in YAML, and compiled-code changes are infrequent and confined to adapters and factories.

**Model-comparison grids.** Generator profiles share a machine and tools but bind different `llm` configs; the bench/evaluator stack (Chapter 9) runs the grid and attributes outcome differences to the model because the evaluator can confirm two runs used the same machine byte-for-byte.

**CI validation gates.** Schema, machine, and contract validation run on every change, so an agent with an unreachable state, an unhandled signal, or an incomplete tool contract fails the build before it ships.

**Orchestration as declared data.** **Kubernetes** [@burns-borg-omega-kubernetes-2016] establishes the same discipline outside the agent domain: an operator writes desired state as data and a fixed control loop reconciles reality to it, orchestration as data rather than scripts. The split mirrors **Functional Core, Imperative Shell** [@bernhardt-fcis-2012] — a fixed imperative shell (the interpreter) wrapped around declarative, data-first definitions.

**The harness as a first-class artifact.** A recent line of work treats the runtime around the model as an engineered object in its own right, corroborating the claim that an agent's definition is a governable artifact separate from its model. **Meta-Harness** [@meta-harness-2026] treats the scaffolding around LLM calls as an optimizable object; **AutoHarness** [@autoharness-2026] automatically generates harness code to improve agent performance; **Code as Agent Harness** [@ning-code-as-harness-2026] frames agent systems as executable, verifiable, stateful harnesses; **HARBOR** [@harbor-harness-optimization-2026] optimizes coding-agent harnesses directly; and a study toward a science of scaling agent systems [@kim-scaling-agents-2025] argues that agent systems introduce scaling failures beyond single-model behaviour. Each reinforces that harness structure materially shapes behaviour and must be managed as a system artifact.

**A contrast: durable execution as code.** **Temporal** [@temporal-2024] offers durable execution with human-in-the-loop waits, but its workflows are imperative code; the agent is therefore not reviewable as pure data, marking the boundary of what this pattern requires.
