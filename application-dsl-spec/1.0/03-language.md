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

A root is `local` when its agent-profile lives in the application module, or
`canonical` when it refers to the shared catalog. An application-profile
concerned only with composition declares no local roots and introduces no
duplicate agent-profiles, which lets the repository's reuse accounting tell
implementations apart from compositions.

## The agent-profile

An agent-profile, written as `profile.yaml`, is the behavior unit an
agent-instance realizes. It binds a machine-profile to tool selections, tool
declarations, REST definitions, and inference configurations that dispatch the
machine-instance's actions. The agent-profile is the class side of
instantiation: the lifecycle primitives take one agent-profile per
agent-instance.

{{keyname-table yaml:agent-core/docs/specs/config-formats/agent-profile-format.md}}

{{statement R-PROF-001}}

Reference keynames resolve relative to the agent-profile's directory unless the
runtime contract says otherwise. Tool selections must match declared entries in
the tool declarations; REST definitions bind those tools to their transports;
inference configurations name a model per state. Each referenced family has its
own format specification under `agent-core/docs/specs/config-formats/`, so the
agent-profile grammar governs only the binding keynames in the table above and
does not repeat those grammars.

A blueprint declares an agent-profile once and leaves what varies as typed
parameters. It carries the `params` and `profile` keynames: `params` declares
the parameters, and `profile` holds the body — an agent-profile whose scalar
values may reference `$param(name)`. A blueprint is not itself an agent-profile
document, because its references stay unfilled until arguments arrive. An
agent-profile document expands a blueprint under the expansion keyname,
supplying the arguments and any fields it overrides. We call it the expanding
document rather than an instance: the model reserves *instance* for the mesh
side, and expansion constructs a class from a class.

{{statement R-PROF-002}}

{{statement R-PROF-003}}

R-MODEL-002 governs the entry itself, in an agent-profile document as in a
machine-profile document: the entry names its blueprint under `fragment` and its
argument bindings under `args`.

This is the document-target subset of `srd055-agent-blueprints`, which also
settles what the grammar leaves alone — fields present on the expanding document
replace the blueprint's wholesale, paths in the blueprint body resolve against
the blueprint's own directory, and the expanded result satisfies the rules a
hand-written agent-profile does. No processor enforces any of it today: the
loader work is deferred under gh-2123, so these statements bind documents alone
until it lands.

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
