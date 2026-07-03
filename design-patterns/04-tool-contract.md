# Tool Contract

This chapter presents the Tool Contract pattern, which specifies each tool as a typed contract: one atomic operation with declared inputs, emittable signals, side effects, and an Undo operation. The chapter covers how contracts enable static validation and reversibility, and how boundary tools — those that cross into non-deterministic actors — declare their variance explicitly.

## Intent

Specify each tool as a typed contract — inputs, emittable signals, side effects, and an Undo — so the machine can validate it, the engine can dispatch it, and the rollback engine can reverse it from the declaration alone.


## Motivation

An agent's capability is bounded by its tool set. A model can reason about any task it can express in language, but it can act only through the tools the harness provides. If the tools are ambiguous in scope, silent about side effects, or undeclared in their failure modes, the model's planning is built on sand. No prompt engineering compensates for a tool that sometimes writes files without saying so.

Current frameworks define tools as function signatures with docstrings. Signatures tell the model what arguments to pass; docstrings describe, in prose, what the tool does. Neither states how the tool changes the world, whether that change can be undone, which tools precede or follow it, or how it fails mid-execution. The model infers all of this from experience, and infers differently across contexts, sessions, and model versions.

One principle governs the fix: **each tool does exactly one thing.** "Cut" is one operation; "cut the bread into slices and arrange them on the plate" is several. If a tool needs "and" to describe it, it is two tools; if its behaviour changes on a mode flag, it is several tools in one interface. Multi-step behaviour emerges later, through composition by the machine (Chapter 2); the tool author's job is to define atomic tools.


## Applicability

Tool Contract fits any agent where tools are implemented by developers, selected by a model, and potentially reversed by a rollback engine — three audiences served by one definition. The discipline pays off when tools compose into multi-step sequences, when the tool set spans teams or model generations, or when typed contracts let developers, models, and engines agree on what a tool produces without re-derivation. For a one-off script with a single hard-coded tool and no selection, composition, or reversal, the contract surface is unnecessary.


## Structure

Four tool categories emerge from two axes, atomic vs. composed and internal vs. boundary. The class diagram in Fig. 9 shows them as a taxonomy under a common `Tool`. The composed category is always an anti-pattern: its presence signals incomplete decomposition.

![](figures/fig-10-tool-taxonomy.png)

| **Figure 9.** Class diagram. The tool taxonomy under the abstract `Tool`: `Atomic`, `StatefulInternal`, and `Boundary` are atomic categories; the `Composite` category is an anti-pattern to be decomposed into tools and machine transitions. |
|:---:|

### Participants

#### Well-formed tool

A well-formed tool does one thing, is deterministic (no inference inside an atomic operation), has structured input and output, declares its side effects, knows its own reversibility, and fails loudly with an error signal carrying enough information to route recovery. A bad tool does multiple things on a flag, needs an LLM to interpret its output, hides side effects, or is named for its implementation (`run-python-script`) rather than its function (`parse-csv`).

#### Tool contract

The tool contract is the structured requirements document behind each tool, not a docstring but a specification with six mandatory parts. Its three consumers each read a different subset: the agent reads Problem, Goals, and Non-goals to *select*; the developer reads Requirements and Acceptance Criteria to *implement*; the rollback engine reads Reversibility and side effects to *reverse*. The class diagram in Fig. 10 shows the contract's composition and these reading dependencies.

![](figures/fig-11-tool-contract.png)

| **Figure 10.** Class diagram. The tool contract is composed of six sections; each consumer (`Agent`, `Developer`, `RollbackEngine`) reads the sections relevant to its concern. |
|:---:|


## Collaborations

### The tool cycle

At runtime a tool is one iteration of the machine's dispatch cycle: the machine dispatches a tool, the tool executes an effect and emits a signal, and that signal drives the next transition. The activity diagram in Fig. 11 shows the loop. A tool should therefore never contain a loop, a conditional over multiple outcomes, or an embedded state machine. Those are execution-level constructs that belong in the machine. (The sole exception is a non-terminal tool, which hides a sub-machine behind an atomic interface.)

![](figures/fig-12-tool-cycle.png)

| **Figure 11.** Activity diagram. The tool cycle: each tool is a single dispatch--execute--emit step that the machine repeats. {0.7} |
|:---:|

### Evaluating a contract: the four-question test

A completed contract passes four questions, each targeting a different consumer. Any "no" is a gap that surfaces later, as a runtime failure, a misuse, a composition ambiguity, or a rollback failure.

1. **Implementable?** Could a developer build it without asking questions?
2. **Selectable?** Could the agent decide when to use it from Problem, Goals, and Non-goals alone?
3. **Composable?** Is its output schema precise enough to feed the next tool without ambiguity?
4. **Failure-defined?** If it fails mid-execution, does the contract say what state the world is in?

The activity diagram in Fig. 12 shows the gate: pass all four or the contract is incomplete, and any failure routes back to revision. Partial passes are meaningless. An implementable-but-not-composable tool fails in production just as surely, only later and more expensively.

Composite tools violate the pattern's core rule: each tool should do exactly one thing. When a tool needs "and" to describe its operation, such as create a resource and configure it, the composition belongs in the machine's transition table rather than inside the tool. Hiding that sequence inside a tool prevents static validation, makes reuse harder, and obscures rollback boundaries. During a tool-set audit, a composite tool is evidence of incomplete decomposition and should be split unless it is explicitly a non-terminal boundary tool.

![](figures/fig-13-contract-gate.png)

| **Figure 12.** Activity diagram. The four-question gate: a contract is complete only if all four questions yield "yes"; any failure routes to revision. |
|:---:|


## Consequences

### Benefits

#### A uniform, testable inventory

Every tool is one verb with one signal set, independently testable and independently reversible. Composition lives in the machine, never inside a tool.

#### Reliable planning

Precise output schemas and declared predecessors/successors let the agent plan multi-step sequences without trial and error.

#### Safe reversal

Declared reversibility tiers tell the rollback engine exactly what it can undo, compensate, or only log.

### Liabilities

#### A larger tool set

Decomposition produces more, smaller tools; the burden shifts from understanding hidden internals to selecting among a richer set, mitigated by declared relationships and non-goals.

#### Specification overhead

Each tool needs a full contract, not a docstring. The payoff is that the contract serves three audiences at once; the cost is real up front.


## Implementation

### Contract sections

Every contract has six mandatory sections. **Problem.** Why the tool exists; the gap it fills; one paragraph. **Goals.** Numbered, measurable success conditions forming the acceptance boundary. **Requirements.** Grouped "must" statements covering input formats, output structure, side effects, undo, and error signals. **Non-goals.** Scope bounds that tell the agent when *not* to use it ("does not transform data"). **Acceptance criteria.** Specific input/output/side-effect scenarios that double as tests and as examples the agent reads. **Reversibility.** One of three tiers (below), with the undo or compensation mechanism. Satisfying only one consumer is insufficient: perfect schema but no problem statement leaves the agent unable to select; detailed goals but no undo spec leaves the rollback engine blind.

### Reversibility tiers

Every tool falls into one of three tiers, which govern what the agent must do before dispatch and what the rollback engine can do after. The class diagram in Fig. 13 shows them as subclasses of `Tool`.

![](figures/fig-14-reversibility-tiers.png)

| **Figure 13.** Class diagram. Three reversibility tiers as subtypes of `Tool`, each with its own undo behaviour. |
|:---:|

**Reversible** tools undo their own effects from a record (a file write that restores prior content); the engine cleans up automatically, so they can be executed speculatively. **Compensatable** tools cannot literally undo but can issue a corrective action restoring equivalent state (delete a created resource); the contract must specify the compensation and any semantic differences. **Irreversible** tools cannot be undone (sending email, publishing a deployment); the agent must confirm before dispatch, the machine should route through a confirmation state, and rollback skips and logs them. Omitting the reversibility section is *not* the same as declaring irreversibility. Omission leaves the engine not knowing what to do; explicit irreversibility tells it to skip and log.

Reversibility is a planning constraint: plans of only reversible tools can be executed speculatively and rolled back, while plans including irreversible steps require commitment. Machines that separate reversible exploration from irreversible commitment give the agent maximum flexibility.

### Tool relationships

No tool exists alone; each contract declares three relationship types. **Predecessors.** Which tools typically precede it. **Successors.** Which typically follow. **Overlaps.** Which tools do similar things and how they differ (`write` vs. `patch`), preventing misuse where a more specific tool exists. The object diagram in Fig. 14 shows a relationship graph over specific tool instances.

![](figures/fig-15-tool-relationships.png)

| **Figure 14.** Object diagram. Declared predecessor/successor links form well-tested composition paths; an `«overlaps»` link relates tools with similar capabilities. |
|:---:|

Relationships are advisory for the agent but informative for static analysis: a machine validator can warn when a tool is dispatched with no declared predecessor reachable, or when a declared successor is unreachable (a dead path). Tool set audits use predecessor/successor chains to test *coherence*, checking that every gap the agent might hit is either filled by a tool or explicitly excluded by a non-goal. Treating the tool set as a designed system (typed tools, an explicit machine, declared composition rules, and coherence audits) turns tool design from an ad hoc activity into a systematic one.

### Tools are configured, not coded

A tool implementation is a generic interpreter; its declaration (YAML) is the program it interprets. The same shell executor runs `build`, `test`, or `lint` depending on its configured command. This separation holds at every layer of the architecture: the engine interprets `machine.yaml`, the machine interprets its transition table, each tool interprets its declaration, and boundary tools interpret actor configuration (LLM config, UI config, child profile). Compiled code provides capability (serve HTTP, invoke an LLM, run a shell command); configuration provides specificity (what to serve, which model, what command). One binary serves N agents. The test: can the same code serve a different agent by changing only YAML? If not, the tool has absorbed policy that belongs in its declaration.


## Relationships in the Pattern Language

Tool Contract sits within Machine Interpreter and requires Machine Interpreter: contracts are useful because a machine can validate and dispatch them. It enables Agent-as-Data, Phase-Scoped Toolset, Inference Boundary, Bidirectional Log, Boundary Tool, and Approval Gate by making tool inputs, emittable signals, visibility, boundary kind, and reversibility explicit enough for those later patterns to read. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

In the working implementation, tool declarations are YAML contracts validated at load time (Chapter 3); a tool whose contract is incomplete fails before the agent runs. Tool set audits apply the four-question test at authoring time and when reviewing an existing tool set, flagging execution-shaped tools for decomposition. Reversibility tiers declared in each contract drive the rollback engine (Chapter 7): reversible tools undo automatically, compensatable tools issue corrective calls, irreversible tools are skipped and logged, all from the declaration, with no special-casing in the engine.
