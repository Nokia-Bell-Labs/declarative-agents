---
id: cfg-machine-format
title: Machine Configuration Format (machine.yaml)
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines the MachineSpec YAML schema used by all five agent modes
(generator, evaluator, planner, jurist, bench). Each machine.yaml
declares the finite state machine that drives an agent's execution
loop: its states, signals, transitions, actions, and budget constraints.

MachineSpec also carries optional semantic metadata — purpose,
invariants, lifecycle narrative, configuration cross-references, and
a pipeline diagram — that make the YAML file a self-contained
specification of agent behavior.

<!-- field: action_semantics -->
## Action semantics

<!-- action_semantic: Named tool -->
### Named tool

<!-- field: description -->
#### Description

A literal tool name (e.g. invoke_llm, prepare_workspace, build).
The tool is resolved from the tool registry at build time via
BuildTransitionTable. If the name is not found, the build fails.

<!-- field: example -->
#### Example

invoke_llm

<!-- action_semantic: $tool -->
### $tool

<!-- field: description -->
#### Description

Dynamic dispatch sentinel. At runtime, the actual tool name is
extracted from the parsed LLM output. This lets the LLM
choose one state-valid external word (e.g. read, write, edit,
build). Used in the generator's Parsing -> Composing transition.

<!-- field: example -->
#### Example

$tool

<!-- action_semantic: Empty/omitted -->
### Empty/omitted

<!-- field: description -->
#### Description

No command is executed. Used for terminal transitions where the
machine simply moves to a halting state (e.g. Succeeded, Failed,
BudgetExceeded).

<!-- field: schema -->
## Schema

<!-- schema_entry: name -->
### name

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Unique agent identifier. Used in logging, metrics, and
cross-references between agents.

<!-- schema_entry: purpose -->
### purpose

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Free-form prose describing the agent's role, architecture, and
high-level behavior. Intended for human readers and tooling that
presents machine descriptions. All production agents include a
multi-line purpose block.

<!-- schema_entry: invariants -->
### invariants

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Behavioral invariants the machine is expected to uphold. Each
entry is a single prose statement. These are documentation only
and are not enforced by the runtime, but specification tooling
may reference them for consistency checks.

<!-- schema_entry: lifecycle -->
### lifecycle

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

A numbered narrative describing the agent's typical execution
flow — the sequence of states, transitions, and actions from
startup to termination. Captures the "story" that the transition
table encodes declaratively.

<!-- schema_entry: configuration -->
### configuration

<!-- field: type -->
#### Type

map[string]any

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Cross-references to related configuration files. Common entries
include machine (self-reference), tools (tool selection file),
llm_configs (LLM config glob), tool_declarations (shared YAML
declaration files), and variants (alternative machine files).
Not consumed by the runtime; serves as a documentation index.

<!-- schema_entry: metric_labels -->
### metric_labels

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: status -->
#### Status

planned

<!-- field: description -->
#### Description

Planned monitor metadata for stable workflow labels. These labels come
from trusted MachineSpec YAML and may be attached to tool metric samples
by the runtime recorder. They are documentation only until monitor
validation and recorder code land.

Values must be low-cardinality. They should name workflow phases,
profile-owned use cases, or other bounded categories. They must not
include raw prompts, full outputs, secrets, unbounded paths, arbitrary
URLs, request IDs, timestamps, or user free text.

<!-- schema_entry: pipeline_diagram -->
### pipeline_diagram

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

A Mermaid diagram (or other markup) depicting the agent's state
flow. Embedded in the YAML as a multi-line string. Renderers
that support Mermaid can display it as a visual graph.

<!-- schema_entry: view_tags -->
### view_tags

<!-- field: type -->
#### Type

[]object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

Ordered presentation metadata for renderers that divide one large
machine into named views. The monitor declared_machines view preserves
this list as authored. The engine does not interpret it during
validation, transition lookup, dispatch, or run-state tracking.

<!-- field: sub_fields | form: "typed table" -->
#### Sub fields

| field | type | required | description |
| --- | --- | --- | --- |
| tag | string | true | Stable tag referenced by StateSpec.tags. |
| label | string | true | Human-facing label for the tagged view. |

<!-- schema_entry: initial_state -->
### initial_state

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

The state the machine enters on startup. Must reference a state
present in the states list.

<!-- schema_entry: summary_signal -->
### summary_signal

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Signal whose Result.Output becomes RunResult.summary. Must name a declared
signal. When omitted, no signal implicitly supplies the run summary;
a transition may instead declare summary: true.

<!-- schema_entry: resume_signal -->
### resume_signal

<!-- field: type -->
#### Type

string

<!-- field: required -->
#### Required

when signals declares AwaitApproval

<!-- field: description -->
#### Description

Signal used when a checkpoint resumes without an operator
--resume-signal override. A resumable approval machine must declare it
and it must name a declared signal. Machines without approval suspension
have no implicit resume signal. The paired suspend signal is the engine
lifecycle signal AwaitApproval emitted by the suspend word; it is
protocol vocabulary rather than profile policy.

<!-- schema_entry: budget -->
### budget

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Resource limits that bound the agent's execution. MachineSpec is the sole
authority for run-level max_tokens and max_duration; invoke_llm word
configuration cannot replace either value.

<!-- field: sub_fields -->
#### Sub fields

<!-- sub_field_entry: max_iterations -->
##### max_iterations

<!-- field: type -->
###### Type

int

<!-- field: required | form: "scalar" -->
###### Required

true

<!-- field: description -->
###### Description

Maximum number of state transitions before halting. When
exceeded, the machine transitions via the BudgetExhausted
signal. Must be a positive integer.

<!-- sub_field_entry: max_tokens -->
##### max_tokens

<!-- field: type -->
###### Type

int

<!-- field: description -->
###### Description

Maximum total tokens consumed across all LLM calls. When
exceeded, transitions via BudgetExhausted.

<!-- sub_field_entry: max_duration -->
##### max_duration

<!-- field: type -->
###### Type

duration string

<!-- field: description -->
###### Description

Wall-clock time limit (e.g. "5m0s"). Enforced via
context.WithTimeout — when elapsed, the loop terminates
with StatusCancelled (not BudgetExhausted).

<!-- sub_field_entry: command_timeout -->
##### command_timeout

<!-- field: type -->
###### Type

duration string

<!-- field: required | form: "scalar" -->
###### Required

true

<!-- field: description -->
###### Description

Positive per-dispatch timeout applied to every command in this
machine. Runtime profiles fail validation when it is omitted.

The value must exceed the longest legitimate duration of every action
reachable in the machine, plus scheduling margin. Authorities include
ToolDef timeout config, REST limits and machine_request timeout,
model or child-process limits, and blocking await duration. A copied
repository-wide value is not calibration evidence when workloads
differ. Environment-overridable operation timeouts must have a finite
maximum no greater than this bound.

Examples:
- a request machine whose longest REST operation is 30s may use 1m;
- a host with a 10m blocking control await may use 11m;
- an evaluator with a 24h await or child operation may use 25h.

Startup validates this relationship from the executable profile
closure. The inspector follows declaration includes and override
precedence, selected actions, REST definitions and machine_request
profiles, child profiles, evaluator point machines, and compatibility
wrappers. It reports the machine, action, authority source, operation
duration, and command timeout for each failure.

Environment expansion happens before authority comparison. The active
machine command_timeout is the finite maximum for every expanded
operation timeout in that machine; an equal or larger value is
rejected before dispatch. A deployment schema may impose a tighter
input bound, but it is not the runtime safety boundary.

<!-- sub_field_entry: max_consecutive_parse_errors -->
##### max_consecutive_parse_errors

<!-- field: type -->
###### Type

int

<!-- field: description -->
###### Description

Maximum consecutive LLM parse failures before the machine
transitions via BudgetExhausted. The profile must select
report_parse_error, route ParseFailed through that word, and route
BudgetExhausted from the report transition's destination state.
Startup validation rejects the budget when any of that wiring is
absent; profiles that do not retry parse failures must omit it.

<!-- schema_entry: states -->
### states

<!-- field: type -->
#### Type

StateSpecs ([]string | []StateSpec)

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

The set of valid machine states. Accepts two forms:

Scalar form (legacy): a plain list of state name strings.
  states:
  - Idle
  - Running
  - Done

Rich object form: a list of objects with name and optional meaning
metadata. The meaning field documents the state's semantic purpose.
The optional tags field is an ordered string list of presentation tags
declared by top-level view_tags. Renderers may use tags to select a
subgraph, but the engine does not interpret tags when validating or
executing the machine.
Every terminal state must declare run_status as succeeded, failed,
budget_exceeded, cancelled, or suspended.
  states:
  - name: Idle
    meaning: Initial state before any work begins.
    tags: [intake]
  - name: Running
    meaning: The main working state where tools execute.
    tags: [work]
  - name: Done
    meaning: Terminal. All work completed.
    run_status: succeeded
    tags: [work]

Both forms may be mixed in the same list. The parser
(StateSpecs.UnmarshalYAML) detects scalars vs mappings
per element.

Every state referenced in transitions, initial_state, and
terminal_states must appear here.
run_status is valid only on terminal states and is required for each name
listed by terminal_states.

StateSpec fields are name, meaning, run_status, and tags. tags is
presentation-only metadata. Scalar states have no tags.

<!-- schema_entry: terminal_states -->
### terminal_states

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Subset of states where the machine halts. Once the machine
enters a terminal state, no further transitions are processed.

<!-- schema_entry: signals -->
### signals

<!-- field: type -->
#### Type

SignalSpecs ([]string | []SignalSpec)

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

The set of valid signals. Accepts two forms:

Scalar form (legacy): a plain list of signal name strings.
  signals:
  - Seed
  - ToolDone
  - CommandError

Rich object form: a list of objects with name and optional
trigger metadata. The trigger field documents what event
causes the signal to be emitted.
  signals:
  - name: Seed
    trigger: Loop initialization.
  - name: ToolDone
    trigger: A tool executed successfully.
  - name: CommandError
    trigger: Infrastructure-level error.

Both forms may be mixed in the same list. The parser
(SignalSpecs.UnmarshalYAML) detects scalars vs mappings
per element.

<!-- schema_entry: transitions -->
### transitions

<!-- field: type -->
#### Type

[]object

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

The transition table. Each entry maps a (state, signal) pair to
a next state, an optional action, and an optional stable
command-state label for the completed action step.

<!-- field: sub_fields -->
#### Sub fields

<!-- sub_field_entry: state -->
##### state

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

Current state the machine must be in.

<!-- sub_field_entry: signal -->
##### signal

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

Signal that triggers this transition.

<!-- sub_field_entry: next -->
##### next

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

Target state after the transition.

<!-- sub_field_entry: action -->
##### action

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

Tool to execute during the transition. See action_semantics
for the three modes. May be omitted for terminal transitions.

<!-- sub_field_entry: label -->
##### label

<!-- field: type -->
###### Type

string

<!-- field: required | form: "scalar" -->
###### Required

false

<!-- field: description -->
###### Description

Stable authored address for the action's completed execution step.
A later word selects its output with $from(label).path. The label
belongs to this transition because MachineSpec owns workflow order
and step identity.

The label must be non-empty when present and must not contain dots,
parentheses, whitespace, or control characters. Duplicate labels are
valid; lookup resolves the most recent matching completed step.

When omitted, the step remains addressable by its executed command
name. On a $tool transition, label remains the stable authored
address while command_name records the tool selected at runtime.

<!-- sub_field_entry: report_output -->
##### report_output

<!-- field: type -->
###### Type

string

<!-- field: required | form: "scalar" -->
###### Required

false

<!-- field: description -->
###### Description

Operator-facing field selected from this action's Result.Output using
the canonical $.path grammar (for example $.address). Requires an
action. The CLI may report the selected scalar without hard-coding a
command name.

<!-- sub_field_entry: summary -->
##### summary

<!-- field: type -->
###### Type

bool

<!-- field: required | form: "scalar" -->
###### Required

false

<!-- field: description -->
###### Description

When true, this action's complete Result.Output becomes the run
summary and is not overwritten by later dispatches; when an earlier
summary transition already supplied it, the output is appended on its own
line, so a machine of several words reports each in the order they ran.
Use this for machines with more than one summary-producing signal. A machine-level
summary_signal remains the concise option when one signal owns every
summary.

<!-- sub_field_entry: metric_labels -->
##### metric_labels

<!-- field: type -->
###### Type

object

<!-- field: status -->
###### Status

planned

<!-- field: description -->
###### Description

Planned transition-level metric labels. These labels refine the
workflow context for samples emitted by the action on this
transition. Transition labels override or extend machine-level
labels by label name. They must follow the same low-cardinality safety
policy as top-level metric_labels.

<!-- sub_field_entry: for_each -->
##### for_each

<!-- field: type -->
###### Type

object

<!-- field: required | form: "scalar" -->
###### Required

false

<!-- field: description -->
###### Description

Expand this transition's named action over an array selected from a
prior command-state entry. Dispatch is sequential by default or
bounded parallel when max_concurrency is declared.
Each item is a normal registry-built command dispatch and therefore
gets its own trace, checkpoint Entry, receipt, cancellation boundary,
budget iteration, and rollback visibility.

<!-- field: sub_fields | form: "tree" -->
###### Sub fields

1. - field: items
   - type: string
   - description: A required $from(label).path selector resolving to a JSON array.
1. - field: as
   - type: string
   - description: Command-state binding label for the current item.
1. - field: mode
   - type: string
   - description: sequential (default) or parallel.
1. - field: max_concurrency
   - type: int
   - description: Required positive worker bound for parallel mode; invalid for sequential mode.
1. - field: failure
   - type: string
   - description: fail_fast (default) or collect_all.
1. - field: continue_on
   - type: "[]string"
   - description: Item result signals counted as successful.
1. - field: abort_on
   - type: "[]string"
   - description: Item signals that stop scheduling and cancel contextual siblings.
1. - field: join
   - type: object
   - description:
     Declares the receiving state, optional output label, and signals
     for all_success, partial (collect_all), failed, and empty.

<!-- field: data_driven_iteration -->
## Data driven iteration

<!-- field: trust_boundary -->
### Trust boundary

items must come from a $from(label).path command-state selector. MachineSpec
supplies the action and binding name; runtime data cannot choose an action
or introduce executable expressions.

<!-- field: item_binding -->
### Item binding

Object items are exposed directly at $from(as).field. Scalar items are
exposed as $from(as).value. The binding exists only while that item is
dispatched and does not create a fake Execution entry.

<!-- field: join -->
### Join

for_each.join emits one engine-owned for_each.join Entry containing ordered
item outcomes and succeeded/failed counts. The join signal is selected by
the declared outcome map and is then routed through the normal transition
table. Each outcome retains result.output as a string. When that
already-redacted ResultDigest output is valid JSON with recognized applied
redaction metadata, result.structured_output also exposes its parsed object,
array, or scalar shape for dotted-path traversal. Plain text and
output_omitted results have no structured_output.

<!-- field: checkpoint_resume -->
### Checkpoint resume

AgentSnapshot persists the selected items, next index, ordered outcomes, and
iterator grammar. Resume continues at the next undispatched item.
Parallel completions are committed in deterministic input order, so each
item retains its own checkpoint Entry and receipt.
structured_output is shaped only when the join payload is emitted and is not
added to IteratorSnapshot, preserving the persisted outcome format.

<!-- field: concurrency -->
### Concurrency

parallel requires a positive max_concurrency. Workers execute normal
registry-built commands concurrently, while outcomes, RunEvents, checkpoint
entries, and join output are reduced in input order. fail_fast cancels
contextual siblings and waits for all started work; legacy non-contextual
commands are allowed to finish before join so no work is orphaned.

<!-- field: non_goal -->
### Non goal

for_each is not a scripting language: it has no expressions, mutation,
dynamic action names, nested programs, or unbounded concurrency.

<!-- field: command_state_step_labels -->
## Command state step labels

<!-- field: purpose -->
### Purpose

Defines the stable authoring surface for command-state selectors. The
transition label identifies one workflow step independently of the tool
that implements it. This lets two transitions execute the same tool under
different addresses, and lets a $tool transition keep one address while
runtime dispatch chooses different commands.

<!-- field: addressing -->
### Addressing

- A completed labeled transition is addressable by both label and its executed command name.
- A completed unlabeled transition is addressable by its executed command name.
- A programmatic TransitionTable run without MachineSpec metadata has command-name addressing only.
- Lookup scans completed steps from newest to oldest and matches either address.
- If one step's authored label equals another step's command name, the same newest-match rule resolves the collision.
- Absolute step indices are persistence keys and are not part of selector syntax.

<!-- field: persistence -->
### Persistence

- Entry records label separately from command_name.
- Checkpoint adapters persist and restore both fields.
- Legacy persisted entries with no label remain addressable by command_name.
- A schema upgrade adds nullable label storage without invalidating existing rows.

<!-- field: relationship_to_metric_labels -->
### Relationship to metric labels

label is one command-state address. metric_labels is a map of bounded
observability attributes. A command-state selector never reads
metric_labels, and monitor grouping never changes command-state identity.

<!-- field: example -->
### Example

```yaml
transitions:
  - state: ReadingCorpus
    signal: Seed
    next: CorpusRead
    action: read
    label: source_document
  - state: ChoosingTool
    signal: LLMResponded
    next: RunningTool
    action: $tool
    label: selected_operation
```

<!-- field: state_scoped_dynamic_dispatch -->
## State scoped dynamic dispatch

<!-- field: purpose -->
### Purpose

Defines the availability contract for LLM-selected `$tool` dispatch.
A state is a grammar phase. The model may choose only tools that are valid
in the current phase.

<!-- field: source_of_truth -->
### Source of truth

MachineSpec transitions are the source of truth for dynamic tool
availability. A tool is available from the state that invokes the LLM when
the next `$tool` transition can dispatch that tool and every declared
signal emitted by that tool is accepted by the target state or the target
state is terminal.

Explicit ToolSpec phases may further narrow availability for compatibility
with registry manifests. They must not make a tool available where the
MachineSpec transition graph cannot route that tool's declared signals.

<!-- field: manifest_rule -->
### Manifest rule

The LLM manifest is built for the machine's current state. It includes
external tools that satisfy the state-scoped dynamic availability contract.
Internal tools and tools outside the current state are omitted from the
manifest.

<!-- field: validation_rule -->
### Validation rule

Runtime parsing and dispatch validate the parsed tool name against the same
current-state availability set used for the manifest. Unknown tools,
internal tools, and tools not valid in the current state are rejected before
command execution.

Before registration, the invoke_llm selector and parse_response word on an
actual selector-to-$tool machine path must each declare the `$tool` target
as manifest_state. Unrelated invoke words are not selectors for that path
and do not participate. Every external word must derive at least one target
phase. A mismatch, empty explicit-phase intersection, missing emitted-signal
declaration, or unroutable emitted signal rejects startup with the word and
cause.

<!-- field: rejection_signal -->
### Rejection signal

A parsed tool call that names a registered external tool outside the
current state's availability set emits ParseFailed. The diagnostic should
distinguish the out-of-state tool from an unknown tool so the machine can
route recovery deterministically through its ParseFailed transition.

<!-- field: ownership_rules -->
### Ownership rules

- MachineSpec owns phase-specific availability because transitions own workflow order.
- ToolDef owns vocabulary metadata such as emitted signals, visibility, parameters, and side effects.
- ToolSpec phases are derived runtime metadata or an additional restriction; they are not a second workflow definition.

<!-- field: workflow_metric_labels -->
## Workflow metric labels

<!-- field: status -->
### Status

planned

<!-- field: purpose -->
### Purpose

Defines planned MachineSpec metadata for grouping embedded monitor metrics
by workflow context. Tool declarations define metric instruments.
MachineSpec labels define where in the grammar those instruments ran.

<!-- field: ownership_rules -->
### Ownership rules

- MachineSpec owns workflow label values because it owns sequencing.
- ToolDef owns metric instrument names and specialized metric meaning.
- Runtime dispatch may attach machine and transition labels to samples.
- Labels are trusted config values, not runtime input chosen by the model.

<!-- field: allowed_label_examples | form: "typed table" -->
### Allowed label examples

| phase | use_case | boundary | checkpoint_policy |
| --- | --- | --- | --- |
| validation |  |  |  |
|  | rel04.0-monitor |  |  |
|  |  | rest_server |  |
|  |  |  | disabled |

<!-- field: prohibited_label_sources -->
### Prohibited label sources

- raw prompts
- full tool outputs
- secrets
- absolute paths
- arbitrary URLs
- request IDs
- timestamps
- user free text

<!-- field: example -->
### Example

```yaml
metric_labels:
  use_case: rel04.0-monitor
  phase: main_loop

transitions:
  - state: WaitingForRequest
    signal: ServerLaunched
    next: ReadingMonitor
    action: monitor_snapshot
    metric_labels:
      phase: monitor_read
```

<!-- field: machine_presentation_views -->
## Machine presentation views

<!-- field: status -->
### Status

implemented

<!-- field: purpose -->
### Purpose

view_tags and StateSpec.tags let a renderer present named subgraphs from
one authored machine declaration. They do not create nested machines,
alter state membership, or constrain transitions.

<!-- field: monitor_contract -->
### Monitor contract

The srd033 declared_machines view preserves top-level view_tags and each
state's tags. The existing machine_spec view remains a names-only root
projection and does not define this presentation contract.

<!-- field: authored_shape -->
### Authored shape

view_tags:
  - {tag: intake, label: Intake}
  - {tag: answer, label: Answer composition}
states:
  - {name: AwaitingRequest, meaning: Waiting for input., tags: [intake]}
  - {name: Composing, meaning: Building the response., tags: [answer]}
  - {name: Done, run_status: succeeded, tags: [answer]}

<!-- field: ownership_rules -->
### Ownership rules

- Machine authors own view tag names, labels, ordering, and state membership.
- Renderers decide how to draw a tagged view.
- The engine preserves these named fields but does not interpret them.
- Unknown MachineSpec fields remain outside this named presentation contract.

<!-- field: validation_rules -->
## Validation rules

Validation occurs in two phases at startup:

Phase 1 — validateSpec() during ParseMachineSpec():
- name is required (non-empty)
- initial_state is required
- At least one state, one terminal state, and one transition must exist
- Every state in transitions must exist in the states list
- Every signal in transitions must exist in the signals list
- initial_state must be a member of states
- terminal_states must be a subset of states
- Each present transition label satisfies the canonical selector-label grammar
- Duplicate valid transition labels are allowed
- for_each requires a named action, a valid $from selector and binding label,
  declared continue/join signals, and a declared join state
- mode is sequential or parallel; parallel requires positive max_concurrency
  and sequential forbids it; failure is fail_fast or collect_all
- collect_all requires a partial join signal

Phase 2 — runtime tool wiring and BuildTransitionTable():
- Named actions (not $tool, not empty) must resolve to a registered tool
- $tool requires a non-nil ToolAction callback
- Dynamic $tool availability is derived from the current state, the next
  $tool transition, and selected external ToolDef emits
- Explicit ToolSpec phases may only narrow the transition-derived set
- The invoke_llm selector and parse_response word linked to a $tool path must
  each declare that $tool target as manifest_state
- Every selected external word must derive at least one dynamic phase

Profile closure timeout inspection occurs after profile-owned paths,
declaration includes and overrides, REST definitions, and environment
references resolve, but before any command dispatch:
- Every reachable named action and every selected $tool candidate resolves to
  a finite operation authority or to a known non-blocking word.
- ToolDef, REST, model, child/service, evaluator-point, and await authorities
  are compared with the MachineSpec that dispatches them.
- command_timeout must be strictly greater than each resolved operation.
- Compatibility aliases are ordinary profile references and are inspected
  through their canonical closure.
- Agent Core inspects only the profile supplied by its caller. Each owning
  module discovers and supplies its shipped profile inventory in owner tests.

Validation failures cause the agent to abort before processing any input.

<!-- field: diagnostics -->
## Diagnostics

After structural validation passes, DiagnoseMachineSpec() performs
optional static analysis that reports non-fatal warnings. Callers
decide which warnings to treat as policy violations.

Diagnostic codes:

unreachable_state — A state is declared but not reachable from
initial_state via the transition graph. May indicate dead grammar.

unreachable_transition — A transition starts from a state that is
itself unreachable from initial_state.

terminal_transition — A transition starts from a terminal state.
Terminal states should have no outgoing transitions.

unused_signal — A signal is declared but no transition references it.
May indicate a removed feature or a typo.

Diagnostics are returned as []MachineDiagnostic with severity,
code, message, and the affected state/signal/transition index.

<!-- field: examples -->
## Examples

<!-- example: ../applications/catalog/agents/executor/machine.yaml -->
### ../applications/catalog/agents/executor/machine.yaml

<!-- field: description -->
#### Description

7 states, 11 signals, rich state/signal metadata. LLM-driven
loop with $tool dispatch. The generator iterates between
Composing and Parsing, using dynamic tool dispatch to execute
LLM-chosen tools. Includes purpose, invariants, lifecycle,
configuration cross-references, and a Mermaid pipeline diagram.

<!-- example: ../applications/catalog/agents/critic/machine.yaml -->
### ../applications/catalog/agents/critic/machine.yaml

<!-- field: description -->
#### Description

11 states, session-level orchestration with nested core.Loop.
Drives grid iteration over (harness, model, sample, rep) points.
Each point runs the per-point pipeline via a nested core.Loop
using ../applications/catalog/agents/critic/point.yaml. Rich state/signal metadata
throughout.

<!-- example: ../applications/catalog/agents/critic/point.yaml -->
### ../applications/catalog/agents/critic/point.yaml

<!-- field: description -->
#### Description

15 states, 17 signals, linear pipeline with no LLM interaction.
Walks through workspace preparation, config snapshot, agent
execution, oracle check, trace collection, and metrics in a
fixed sequence with error handling. All states carry meaning
metadata and all signals carry trigger metadata.

<!-- example: ../applications/catalog/agents/planner/machine.yaml -->
### ../applications/catalog/agents/planner/machine.yaml

<!-- field: description -->
#### Description

16 states, complex branching with retry logic. Orchestrates
multi-step planning with LLM reasoning, sub-task decomposition,
issue creation, and error recovery paths. Rich metadata
including a Mermaid pipeline diagram.

<!-- example: ../applications/catalog/agents/bench/machine.yaml -->
### ../applications/catalog/agents/bench/machine.yaml

<!-- field: description -->
#### Description

Human-in-the-loop host machine. It launches the generic REST server,
awaits declared operator events, validates launch input, invokes the
critic through self_invoke, and drains the server on shutdown.

<!-- example: ../applications/catalog/agents/specification-critic/machine.yaml -->
### ../applications/catalog/agents/specification-critic/machine.yaml

<!-- field: description -->
#### Description

6 states, linear 3-tool pipeline. The jurist
loads the specification corpus, runs consistency checks across
SRDs, machine grammars, and tool declarations, and formats the
report. No LLM calls. Rich state/signal metadata with purpose,
invariants, and lifecycle narrative.

<!-- field: stage_fragments -->
## Stage fragments

A machine can expand stage fragments (srd052 R4, srd006 R1.9). A stage
fragment is a unit whose body is `stage` rather than a whole machine:

  unit: control-stage-fragment
  params:
    - {name: prefix, type: string}
    - {name: from, type: string}
    - {name: enter, type: string}
    - {name: launch, type: string}
    - {name: await, type: string}
  stage:
    states:
      - {name: $param(prefix)Launching, meaning: Starting the control server.}
      - {name: $param(prefix)Awaiting, meaning: Waiting for an exit request.}
    transitions:
      - {state: $param(from), signal: $param(enter), next: $param(prefix)Launching, action: $param(launch)}
      - {state: $param(prefix)Launching, signal: ServerLaunched, next: $param(prefix)Awaiting, action: $param(await)}
      - {state: $param(prefix)Awaiting, signal: ExitRequested, next: Exiting, action: exit_agent}
      - {state: $param(prefix)Awaiting, signal: CommandError, next: Failed}

The importing machine lists it beside its own transitions:

  expand:
    - fragment: ../../units/control-stage-fragment.yaml
      args: {prefix: Patient, from: Awaiting, enter: AwaitTimedOut,
             launch: patient_launch_agent_control, await: patient_await_agent_control}

The loader fills the arguments into scalar values and appends the produced
states, signals, and transitions to the machine's lists. The stage's
entry (`from`, `enter`) and every state it hands to (`Exiting`, `Failed`
above) belong to the importer, which must declare them; a stage that names
a state or signal nothing declares fails as a hand-written transition
would, with the fragment named. The prefix is a parameter the fragment
composes into its state names, so one stage can be spliced twice under two
prefixes; splicing the same prefix twice is the existing duplicate-state
error. The spliced machine is then validated as one machine by every
machine check -- referential integrity, labels (srd038), exhaustiveness
(srd006 R2.10) -- and the dump renders the spliced result under `machine`
with the fragment, arguments, and produced names under `expansions`.
A machine declares no `imports`; a stage is expanded, and a machine
has no other unit kind to import.

<!-- field: machine_templates -->
## Machine templates

A whole machine can be a template (srd054-machine-templates). A template is
a fragment whose body is `machine` rather than `stage`, holding the fields
a machine file holds apart from `unit` and `imports`:

  unit: control-machine-template
  params:
    - {name: server, type: string}
    - {name: exit_signal, type: string}
  machine:
    name: control
    purpose: Serve a control endpoint until an exit request arrives.
    initial_state: Idle
    states: [...]
    transitions:
      - {state: Idle, signal: Seed, next: Launching, action: launch_$param(server)_control}
      ...

A machine file becomes an instance of it by carrying a unit, exactly one
expansion entry, and optionally its own name and purpose:

  unit: control-template
  name: control-template
  expand:
    - fragment: ../../units/control-machine-template.yaml
      args: {server: agent, exit_signal: ExitRequested}

Any other field on an instance is a load error naming the field, as is an
`as` on the entry (srd054 R1.2). The target's body kind selects the
behavior: a `machine` body replaces the instance's machine and a `stage`
body splices into it. A template body can itself expand stages, whose
paths resolve against the template's directory; a template cannot
expand a template, and an instance cannot splice a stage beside its
template (srd054 R1.3). The expanded machine is validated by every check
a hand-written machine passes, and a diagnostic names both the template and
the instance. The template and its stages are closure files and program
identity assets; the dump renders the expanded machine under `machine`
and lists expansions with `kind: machine` or `kind: stage`. A profile or
a `machine_request` binding that names a template file directly is a load
error, because a template is not a machine until its arguments arrive
(srd054 R1.4). Landed with GH-2114; testdata/integration/profiles/
control-template is the reference instance.

<!-- field: related_documents -->
## Related documents

- srd006
- [srd052-declaration-fragments](../software-requirements/srd052-declaration-fragments.md)
- [srd054-machine-templates](../software-requirements/srd054-machine-templates.md)
- [cfg-agent-profile-format](agent-profile-format.md)

<!-- field: implementation -->
## Implementation

- internal/runtime/core/machine.go
