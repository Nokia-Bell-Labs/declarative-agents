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

{{keyname-table yaml:agent-core/docs/specs/config-formats/agent-profile-format.yaml}}

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

{{keyname-table yaml:agent-core/docs/specs/config-formats/machine-format.yaml}}

{{statement R-MACH-001}}

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
