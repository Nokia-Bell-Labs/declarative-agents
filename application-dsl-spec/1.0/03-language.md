<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# The language

The language has two grammars in a nesting relationship. The application grammar
composes agents into a microservices application; the agent and machine grammars
define each service as a state machine and its tools. Three document families
realize them — application-profile, agent-profile, and machine-profile — and
this section gives the grammar of each.

## The application-profile

An application-profile is the `application.yaml` in an application module's
agents root. It declares the composition — which agent-profiles the
application-instance runs — together with ownership, capability evidence, and
packaging. It is also the document the population's closure invariant closes
over: a root declared here is the only way an agent-instance enters the running
population.

{{keyname-table go:magefiles/appmanifest/manifest.go#Manifest}}

The `roots` sequence carries the composition; each entry seeds one agent-profile
the application-instance runs.

{{keyname-table go:magefiles/appmanifest/manifest.go#Root}}

A capability entry under `capabilities` declares a status and evidence commands;
the repository's magefiles hold the authoritative validation semantics beyond
keyname presence.

{{statement R-APP-001}}

The header fixes what the document is: the one schema version the repository
reads, a portable identity, and whether the application-profile owns
agent-profiles or only composes them. The schema is closed, so a misspelt
keyname is an error rather than a silent omission.

{{statement R-APP-002}}

A capability claim is checked against its evidence: a status that asserts
working behavior names the commands that prove it, and a claim that depends on
another, such as Helm management on packaging, holds only when that other claim
does.

{{statement R-APP-003}}

A root is `local` when its agent-profile lives in the application module, or
`canonical` when it refers to the shared catalog. An application-profile
concerned only with composition declares no local roots and introduces no
duplicate agent-profiles, which lets the repository's reuse accounting tell
implementations apart from compositions.

Ownership decides where a root's source is read from: a `local` root lives in
the module and is versioned with it, while a `catalog` root is pinned to the
catalog release it is compatible with.

{{statement R-APP-004}}

## The agent-profile

An agent-profile, written as `profile.yaml`, is the behavior unit an
agent-instance realizes. It binds a machine-profile to tool selections, tool
declarations, REST definitions, and inference configurations that dispatch the
machine-instance's actions. The agent-profile is the class side of
instantiation: the lifecycle primitives take one agent-profile per
agent-instance.

{{keyname-table yaml:agent-core/docs/specs/config-formats/agent-profile-format.md}}

{{statement R-PROF-001}}

An agent-profile is identified by where it lives and by the root that names it,
so `name` is a label the loader keeps but never requires.

Beyond its machine-profile, an agent-profile names the tool selections
it draws from and the declaration sources that supply those names. Every source
keyname is a list of paths, and at least one tool selection is required.

{{statement R-PROF-004}}

An agent-profile may declare named library roots under `libraries`. The
`agent-core` root is implicit and cannot be redeclared; every other root maps a
kebab name to the directory that imports under `/opt/<name>/` resolve to.

{{statement R-PROF-005}}

Path resolution is the processor's side of these keynames: relative paths are
read from the agent-profile's own directory, and install and library prefixes
map to the roots they name.

{{statement R-PROF-006}}

Reference keynames resolve relative to the agent-profile's directory unless the
runtime contract says otherwise. Tool selections must match declared entries in
the tool declarations; REST definitions bind those tools to their transports;
inference configurations name a model per state. Each referenced family has its
own format specification under `agent-core/docs/specs/config-formats/`, so the
agent-profile grammar governs only the binding keynames in the table above and
does not repeat those grammars.

An agent-profile is written out in full. Version 1.0 has no blueprints: no
document declares an agent-profile once with typed parameters for others to
expand. The profile loader decodes strictly and knows none of the keynames a
blueprint would need, so an agent-profile document can neither expand one nor
be one. Agent-profiles that share structure share it through the documents
they reference, such as tool selections, declarations, and machine-profiles
built from stage-fragments and machine-templates.

{{statement R-PROF-002}}

{{statement R-PROF-003}}

## Tools

A tool-declaration document supplies the vocabulary an agent-profile selects
from. It carries either tool entries or named types, never both, so a file is
one kind of unit.

{{statement R-TOOL-001}}

Two kinds of tool exist: a builtin is compiled into the runtime and named by
its factory, and an exec tool runs an external program. An entry that omits its
type is an exec tool.

{{statement R-TOOL-002}}

The contract fields the loader enforces are closed vocabularies, so a typo
cannot expose an internal tool or disable receipt validation. Prose obligations
from the tool-authoring standard, such as goals and non-goals, bind authors and
are not checked here.

{{statement R-TOOL-003}}

Exec fields describe how a program is launched — its environment, its standard
input drawn from a prior step's output, and how its output is captured — and
mean nothing to a builtin, so a builtin may not carry them.

{{statement R-TOOL-004}}

A signature states a tool's input and output by type reference and its outcomes
as signals. It replaces the older contract blocks rather than adding to them,
so a tool may not say the same thing twice.

{{statement R-TOOL-005}}

Declared types use a closed subset of JSON Schema. The subset is closed so that
a selector path into a typed value stays decidable; a type refers to another by
`$type` and adds nothing beside the reference.

{{statement R-TOOL-006}}

A fragment is the parameterized form of a declaration unit: its body holds
`$param(name)` references that an expansion entry fills, per R-MODEL-002. A
fragment does not expand another, and a unit that is not a fragment has no
parameters to reference.

{{statement R-TOOL-007}}

Units compose by import and by expansion. The loader resolves the closure once
and refuses any composition whose meaning would depend on load order: a cycle,
a name supplied twice, or a write into a library that its importers only read.

{{statement R-TOOL-008}}

A tool selection is separate from the declarations it draws on. It lists names
only, so the same declaration serves agent-profiles that select different
toolsets.

{{statement R-TOOL-009}}

Selection is resolved after every declaration source has loaded, in the order
the agent-profile lists them, so local declarations override shared ones and
every selected name must resolve.

{{statement R-TOOL-010}}

## REST definitions

A REST definition binds tools to HTTP: clients describe outbound operations and
servers describe inbound endpoints. One file holds one `rest` mapping, grouped
by family.

{{statement R-REST-001}}

REST units compose by the same import and fragment rules as tool declarations,
and the definition model is closed at every level.

{{statement R-REST-002}}

A client groups resource-shaped calls under resources and their four verbs, and
keeps RPC-shaped calls under its own `operations` map.

{{statement R-REST-003}}

Status mappings turn an HTTP response into the signal the operation emits. Each
status has exactly one owner, so the signal a response produces never depends
on the order of the mappings.

{{statement R-REST-004}}

An operation that changes a remote service carries the same contract a mutating
tool does: what it changes, and how that change is undone, compensated, or
confirmed before it runs.

{{statement R-REST-005}}

Authentication and redirect behavior are named profiles that clients and
servers reference; their kinds are closed sets the transport implements.

{{statement R-REST-006}}

A server endpoint's binding says what an accepted request does — emit a signal,
read state, run a request-scoped machine-instance, and so on — and a binding
the runtime cannot dispatch is a load error rather than a 501 at run time.

{{statement R-REST-007}}

A `machine_request` endpoint is how a request machine-profile is served. Its
document fixes what runs, how long it may run, and which terminal outcomes
become which responses; nothing in the request can choose them.

{{statement R-REST-008}}

## Inference configuration

Inference configuration is an ordinary tool declaration: an `invoke_llm`
builtin whose config names the model, the state whose external tools form the
manifest, and the chat dialect the call speaks. A dialect replaces the legacy
provider name, so naming both is an error.

{{statement R-LLM-001}}

The base URL is checked when the tool is built, not when its config is read:
decoding fills the Ollama default, and building refuses a call that would have
no address to send to.

{{statement R-LLM-006}}

A chat dialect, shipped as `chat-dialect.yaml` in a provider library, fills the
provider-specific steps of an inference call using the REST grammar for its
body, failure map, and auth profile. Its header names the library and the
endpoint the call posts to.

{{statement R-LLM-002}}

The request body is a template over the values the inference call supplies. The
conversation goes in whole, never spliced into text, so the provider receives
it as the structured list it is.

{{statement R-LLM-003}}

The reply side names where the text and token counts sit in the provider's
response, and maps provider error statuses onto a shared failure vocabulary, so
a machine-profile routes provider failures the same way whichever provider it
calls.

{{statement R-LLM-004}}

A dialect names its credentials and never carries them, so the library can be
shipped and reviewed without secrets.

{{statement R-LLM-005}}

## The machine-profile

A machine-profile is a `machine.yaml` that declaratively controls an
agent-instance's loop. Its states, signals, and transitions define all behavior
— a machine-instance only acts as its transitions declare. A machine-profile
either contains this body directly or is built through expansion, as the model
describes.

{{keyname-table yaml:agent-core/docs/specs/config-formats/machine-format.md}}

{{statement R-MACH-001}}

The keyname table is closed. The loader decodes strictly, so a misspelled
keyname is a load error rather than a silently ignored field.

{{statement R-MACH-003}}

A transition maps one (state, signal) pair to a next state, and may name the
action that runs on the way.

{{statement R-MACH-004}}

States and signals each have a short form, a bare name, and a long form that
carries documentation beside the name.

{{statement R-MACH-005}}

The transition table is closed over its own declarations: every name a
transition uses is one the machine-profile declares, and the table is a
function of (state, signal).

{{statement R-MACH-006}}

A terminal state says how a run that halts there ends, so the reported outcome
comes from the document rather than from the state's name. The parser checks
the value and its placement; the startup boundary rejects a terminal state that
leaves it out.

{{statement R-MACH-007}}

The budget bounds a run. The interpreter does not choose an iteration bound or
a per-command timeout on a machine-profile's behalf, so an executed
machine-profile declares both.

{{statement R-MACH-008}}

Two machine-level policy keynames name signals: the one whose output becomes
the run summary, and the one a suspended run resumes with.

{{statement R-MACH-009}}

A parse-retry budget is meaningful only when the transition table retries and
exhausts through it, so the startup boundary checks that wiring.

{{statement R-MACH-010}}

An action takes one of three forms: a named tool, the `$tool` sentinel whose
tool the model chooses at run time, or nothing at all, which moves the
machine-instance without running a command.

{{statement R-MACH-011}}

{{statement R-MACH-012}}

A label gives a completed step a stable address that a later word reads with
`$from(label).path`, independent of which tool ran. Runtime-seeded labels under
`external_labels` widen the same address space.

{{statement R-MACH-013}}

{{statement R-MACH-014}}

{{statement R-MACH-015}}

Data-driven iteration runs one named action over an array a prior step
produced. The document supplies the action and the binding name; the runtime
data supplies only the items, so it can neither choose an action nor introduce
an expression. The join folds the item outcomes back into one signal the
transition table routes.

{{statement R-MACH-016}}

Iteration is sequential and fails fast unless the document says otherwise;
concurrency is always bounded.

{{statement R-MACH-017}}

{{statement R-MACH-018}}

Signal exhaustiveness runs in both directions once the selected tools are
known. In the first, every signal a tool emits has a route from the state it
lands in.

{{statement R-MACH-019}}

In the second, every transition waits on a signal that can arrive.

{{statement R-MACH-020}}

A state is a grammar phase, and the transition table, not the tool
declarations, says which tools the model may choose in it.

{{statement R-MACH-021}}

{{statement R-MACH-022}}

A request machine binds a machine-profile to a REST endpoint: each accepted
request creates a request-scoped machine-instance whose terminal signals
determine the response status. The binding rules live in agent-core srd030
(machine request REST binding); the bound machine-profile is grammatically
ordinary under this grammar.

A machine-profile with a declared body splices stage-fragments; a bodiless one
applies a single machine-template. Both run under the expansion keyname at load
time, per R-MODEL-001 and R-MODEL-002. A bodiless document carries its header
and its expansion and nothing else: the template supplies the body, and a
keyname beside it would contend with what the template produces.

{{statement R-MACH-002}}

The expanded result follows every keyname and validation rule above, exactly as
a hand-written machine-profile does.
