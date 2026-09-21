<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Tool Contract

A Tool Contract defines each tool as a typed contract, encapsulating an atomic
operation with inputs, signals, side effects, and an Undo. By declaring these
elements, the contracts enable static validation and guarantee reversibility,
while boundary tools declare their variance when interacting with
non-deterministic actors.

## Intent

Specify each tool as a typed contract that lists inputs, emittable signals,
side effects, and an Undo. This specification lets the machine validate the
tool, the engine dispatch it, and the system roll back the operation from the
declaration and the tool's receipt.

## Motivation

An agent's capability is bounded by its tool set. A model can reason about any
task it can express in language, but it acts only through the tools the
harness provides. When tools are ambiguous in scope, silent on side effects,
or undeclared in failure modes, the model's planning becomes unreliable. No
amount of prompt engineering can compensate for a tool that sometimes writes
files without announcing it.

Frameworks usually define tools as function signatures accompanied by
docstrings. Signatures describe the model's arguments; docstrings describe the
tool's purpose. Neither representation details external state changes,
reversibility, tool sequencing, or failure modes. The model therefore infers
these aspects from experience, which introduces variability across contexts,
sessions, and versions.

One principle governs the fix: each tool does exactly one agent-visible thing.
"Cut" is a single operation; "cut the bread into slices and arrange them on
the plate" comprises several independently meaningful effects. Atomicity is
judged at selectable contract and rollback boundaries, not by counting
implementation helpers, subprocesses, validation branches, parsing loops, or
formatting stages that are required to construct one result. A mode flag that
selects distinct domain operations or a tool that hides separately selectable
effects therefore represents several tools in one interface. Multi-step
workflow behavior emerges later through composition by the machine—Chapter 2—;
the tool author defines atomic contracts.

## Applicability

Tool Contract fits any agent in which developers implement tools, a model
selects them, and a rollback engine potentially reverses them—three audiences
covered by one definition. The discipline pays off when tools compose into
multi-step sequences, when the tool set spans teams or model generations, or
when typed contracts let developers, models, and engines agree on a tool's
output without re-derivation. For a one-off script that uses a single
hard-coded tool and involves no selection, composition, or reversal, the
contract surface is unnecessary.

## Structure

Four tool categories emerge from the intersection of atomic vs. composed and
internal vs. boundary axes. Fig. 9's class diagram shows these categories as a
taxonomy under a common `Tool`. The composed category is an anti-pattern,
signaling incomplete decomposition.

![](figures/fig-10-tool-taxonomy.png)

| Figure 9. Class diagram. The tool taxonomy under the abstract `Tool`: `Atomic`, `StatefulInternal`, and `Boundary` are atomic categories; the `Composite` category is an anti-pattern to be decomposed into tools and machine transitions. |
|:---:|

### Participants

#### Well-formed tool

A well-formed tool does one thing, keeps inference out of the atomic
operation, takes structured input, returns structured output, declares its
side effects, knows its own reversibility, and signals failure with an error
that carries enough information for recovery. A bad tool performs multiple
actions based on a flag, requires an LLM to interpret its output, hides side
effects, or is named after its implementation—`run-python-script`— rather than
its function—`parse-csv`—.

#### Tool contract

The tool contract, distinct from a docstring, is a structured requirements
document with six mandatory sections. Each of its three consumers focuses on
specific subsets: the agent evaluates *Problem*, *Goals*, and *Non-goals* to
select. The developer consults *Requirements* and *Acceptance Criteria* to
implement; the rollback engine reviews *Reversibility* and side effects to
reverse. Fig. 10 illustrates the contract's composition and these reading
dependencies.

![](figures/fig-11-tool-contract.png)

| Figure 10. Class diagram. The tool contract is composed of six sections; each consumer (`Agent`, `Developer`, `RollbackEngine`) reads the sections relevant to its concern. |
|:---:|

## Collaborations

### The tool cycle

At runtime, a tool represents one iteration of the machine's dispatch cycle:
the machine dispatches a tool, the tool executes an effect and emits a signal.
And that signal drives the next transition. Fig. 11's activity diagram shows
the loop. A tool must not hide a workflow loop, branch among separately
selectable domain operations, or dispatch undeclared child tools. Internal
bounded iteration and conditionals that validate input, implement a protocol,
enforce policy, parse/format one result, or continue a single atomic recovery
operation are implementation details, provided they do not introduce another
contract or rollback boundary. A non-terminal boundary tool may also hide a
sub-machine behind its declared atomic interface.

![](figures/fig-12-tool-cycle.png)

| Figure 11. Activity diagram. The tool cycle: each tool is a single dispatch-execute-emit step that the machine repeats. {0.7} |
|:---:|

### Evaluating a contract: the four-question test

A contract is complete when it answers four consumer-specific questions. If
any answer is "no," a gap exists, later causing runtime failure, misuse,
composition ambiguity, or rollback failure.

1. Implementable? Could a developer build it without asking questions?  
2. Selectable? Could the agent decide when to use it from *Problem*, *Goals*, and *Non-goals* alone?  
3. Composable? Is its output schema precise enough to feed the next tool without ambiguity?  
4. Failure-defined? If it fails mid-execution, does the contract say what state the world is in?

Fig. 12's activity diagram illustrates the gate: all four components must
pass, or the contract remains incomplete, with any failure route directing
back to revision. Partial passes hold no significance. An
implementable-but-not-composable tool will inevitably fail in production,
though this occurs later and at greater cost.

Composite tools violate the pattern's core rule: each tool should do one
thing. When a tool uses "and" to describe its operation—such as creating and
configuring a resource—the composition belongs in the machine's transition
table, not inside the tool. Hiding this sequence inside a tool prevents static
validation, hinders reuse, and obscures rollback boundaries. During a tool-set
audit, a composite tool indicates incomplete decomposition and should be split
unless it is explicitly a non-terminal boundary tool.

The audit must differentiate between contract-level composition and
implementation structure. `net/http`, `exec.Command`, directory walks,
parsers, or loops indicate inspection need but do not alone constitute
findings. Before suggesting external executables, the audit must demonstrate
behavioral equivalence in security, observability, receipts, errors,
portability, performance, runtime provisioning, and executable test paths.
Test-only orchestration is excluded unless the harness is the product under
review; using the system's own mechanisms for conformance verification can
introduce circularity.

![](figures/fig-13-contract-gate.png)

| Figure 12. Activity diagram. The four-question gate: a contract is complete only if all four questions yield "yes"; any failure routes to revision. |
|:---:|

## Consequences

### Benefits

#### A uniform, testable inventory

Each tool is defined by a single verb and its corresponding signal set, both
independently testable and reversible. Composition resides within the machine,
not within any tool.

#### Reliable planning

Precise output schemas and declared predecessors/successors let the agent plan
multi-step sequences, avoiding trial and error.

#### Safe reversal

Reversibility tiers guide the rollback walk, detailing undo capabilities for
each tool's receipt and marking actions for compensation or logging.

### Liabilities

#### A larger tool set

Decomposition creates more, smaller tools, moving the challenge from
understanding hidden internals to selecting from a richer set. Declared
relationships and identified non-goals aid selection.

#### Specification overhead

Each tool requires a full contract rather than just a docstring. The payoff is
a contract that serves three distinct audiences at once; the cost is real and
upfront.

## Implementation

### Validation boundaries

The complete contract serves as an audit standard rather than a
runtime-startup gate. The public specification audit checks every selected
declaration for problems, goals, requirements, non-goals, schemas,
reversibility, undo, and relationships, treating incomplete migrated contracts
as errors. It is the sole statement of this check: a signature discharges
blocks filled by load-time defaults, the audit reads the same discharge table
the loader applies, and no second authoring-time checker exists to agree with
it.

Agent startup enforces the subset needed for safe execution: parse-retry
budgets require retry and exhaustion routes, declared emitted signals must be
routable by the loaded machine, and state-mutating reversible or compensatable
tools must declare receipt-consuming undo. Startup does not reject
declarations due to incomplete descriptive contract sections. CI and profile
audits must run broader checks before release.

### Contract sections

Every contract contains six mandatory sections:

* Problem. One paragraph that explains why the tool exists and what gap it fills.  
* Goals. Numbered, measurable success conditions that form the acceptance boundary.  
* Requirements. Grouped "must" statements covering input formats, output structure, side effects, undo, and error signals.  
* Non-goals. Scope bounds that tell the agent when *not* to use the tool (e.g., "does not transform data").  
* Acceptance criteria. Specific input/output/side-effect scenarios that double as tests and agent examples.  
* Reversibility. One of three tiers, together with the undo or compensation mechanism.

Satisfying only one consumer is insufficient: a perfect schema without a
problem statement leaves the agent unable to select; detailed goals without an
undo spec leave the rollback engine blind.

### Reversibility tiers

Tools fall into three tiers, each defining dispatch prerequisites and rollback
capabilities. Fig. 13 shows these tiers as `Tool` subclasses, clarifying their
roles and interactions.

![](figures/fig-14-reversibility-tiers.png)

| Figure 13. Class diagram. Three reversibility tiers as subtypes of `Tool`, each with its own undo behaviour. |
|:---:|

Reversible tools undo their effects automatically upon receipt, as when a file
write restores prior content. This enables speculative execution.
Compensatable tools lack direct undo capability but can issue corrective
actions that restore an equivalent state, such as deleting a created resource.
The contract must explicitly define compensation and any semantic differences.
Irreversible tools—like sending an email or publishing a deployment—cannot be
undone. The agent must confirm before dispatch, the machine routes through a
confirmation state, and rollback skips and logs these actions. Omitting the
reversibility section does *not* imply irreversibility; omission leaves the
engine without guidance, while explicit irreversibility instructs it to skip
and log the action.

Reversibility constrains planning: speculative execution and rollback work
only with reversible tools, whereas irreversible steps require commitment.
Machines that separate reversible exploration from irreversible commitment
maximize agent flexibility.

### Tool relationships

No tool operates in isolation. Each contract explicitly declares three relationship types:

* Predecessors -- tools that typically come before it in a workflow.  
* Successors -- tools that usually follow it.  
* Overlaps -- tools performing similar functions but differing in specifics (`write` vs. `patch`), which helps avoid using a less specialized tool.

Fig. 14 illustrates these connections in an object diagram.

![](figures/fig-15-tool-relationships.png)

| Figure 14. Object diagram. Declared predecessor/successor links form well-tested composition paths; an `«overlaps»` link relates tools with similar capabilities. |
|:---:|

Relationships advise the agent and inform static analysis. A validator warns
when a tool lacks a reachable declared predecessor or has an unreachable
successor—a dead path—. Audits use predecessor/successor chains to test
*coherence*, ensuring every potential gap is filled by a tool or explicitly
excluded as a non-goal. Treating the tool set as a designed system—typed
tools, explicit machine, declared composition rules, coherence
audits—systematizes tool design.

### Tools are configured, not coded

A tool implementation is a generic interpreter, with its YAML declaration as
the program it interprets. For example, a shell executor can run `build`,
`test`, or `lint` based on its configured command. This separation principle
holds across the architecture: the engine interprets `machine.yaml`, the
machine interprets its transition table, each tool interprets its declaration,
and boundary tools interpret actor configuration (LLM config, UI config, child
profile). Compiled code provides capability (serve HTTP, invoke an LLM, run a
shell command), while configuration provides specificity (what to serve, which
model, what command). A single binary can serve N agents. The test is simple:
can the same code serve a different agent by modifying only the YAML? If not,
the tool has absorbed policy that belongs in its declaration.

## Relationships in the Pattern Language

The Tool Contract, residing in the Machine Interpreter, requires its presence
to enable the machine to validate and dispatch contracts effectively. This
mechanism supports Agent-as-Data, Phase-Scoped Toolset, Inference Boundary,
Bidirectional Log, Boundary Tool, and Approval Gate by explicitly defining
tool inputs, emittable signals, visibility, boundary kind, and reversibility.
The complete grammar lives in `pattern-language.yaml`.

## Known Uses

The working implementation verifies machine/tool signal wiring, parse-retry
routes, and receipt compatibility at runtime startup before execution.
Contract completeness is enforced via authoring and specification-audit checks
rather than ordinary startup. Reversibility tiers declared in contracts drive
receipt-driven undo (Chapter 7): reversible tools decode receipts for Undo,
compensatable tools issue corrective calls, irreversible tools are skipped and
logged, all per declaration, with `checkpoint_rollback` handling the process
and no engine special-casing.

Design by Contract [@meyer-dbc-1997] introduces preconditions, postconditions,
and invariants as first-class specifications, shifting behaviour from informal
descriptions to a checkable interface. This provides a verifiable framework
for clarity and enforceability.

Command pattern [@gamma-gof-1994] encapsulates an operation as an object with
`execute` and `undo`, forming the structural core of a reversible tool. The
reversibility tier directs rollback to the correct undo, which consumes the
receipt encoded by the tool during `execute`.

OpenAPI [@openapi-spec-2024] and the Model Context Protocol
[@anthropic-mcp-2024] supply tools with typed request/response contracts and
declared behaviour. OpenAPI enables generating and editing tool declarations
from a spec, while MCP standardises how tools are exposed to models, ensuring
typed inputs.

SWE-agent [@yang-swe-agent-2024] and ReAct [@yao-react-2023] provide empirical
evidence: SWE-agent shows that tool and environment interfaces are decisive
for software-engineering agents. ReAct establishes the reasoning-and-acting
loop in which a model chooses actions from observations—the same actions a
Tool Contract makes explicit and machine-checkable.