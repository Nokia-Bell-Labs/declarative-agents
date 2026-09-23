<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Model

Every document in the language is a single YAML mapping, and a loader that reads
anything else rejects it. This is the well-formedness floor the grammar builds
on, and we keep it as two checked invariants — one on documents, one on the
processors that read them:

{{statement R-INTRO-001}}

{{statement R-INTRO-002}}

## Classes and instances

The language separates what is declared from what runs. A *class* is a document
kept in git; an *instance* is a live entity in the running mesh. Because the two
sides are easy to conflate, every concept carries a suffix naming its side, so
the prose never leans on a bare noun that could mean either:

| Class (declared, in git) | Instance (running, in the mesh) |
|---|---|
| application-profile | application-instance |
| agent-profile | agent-instance |
| machine-profile | machine-instance |

An application-profile declares a composition: the agent-profiles the
application-instance runs, how many of each, and the documents each loads. An
agent-profile specifies one agent-instance's behavior — the machine-profile it
executes, the tools it selects, and the documents it references. A
machine-profile defines the states, signals, and transitions a machine-instance
steps through.

An agent-instance is the triple (agent-profile, identity, workspace). The
identity names the agent-instance; the workspace is its operational environment.
Workspaces outlive instances and keep their identity: an agent-instance binds to
exactly one workspace when it starts and releases it when it stops, and a later
agent-instance binding the same workspace inherits the same identity. A
workspace with no bound agent-instance is an orphan workspace — a state the
population invariants explicitly permit.

The distinction between a declared property and a reported attribute follows the
same line. A property is written in a profile, versioned in git, and identical
across every instance of that class. An attribute is observed on one running
instance and exists only for its lifetime. Profiles declare only properties;
attributes reach a reader through the runtime's reporting surface. Keeping the
two apart is what makes a document checkable without a running system:
document-level checks read properties, and population-level checks read
attributes.

## Three levels

The language has three levels, each with its own document family. At the
application level, an application-profile declares the composition: which
agent-profiles run and how many agent-instances of each. At the agent level, an
agent-profile binds a machine-profile, a set of tools, and referenced documents
into one executable unit. At the machine level, a machine-profile gives the
states, signals, and transitions of an agent-instance's loop.

Levels reference downward only. An application-profile names agent-profiles; an
agent-profile names a machine-profile and its documents; a machine-profile names
nothing above itself. The grammar of each family is the subject of the next
section.

## From class to instance

Instantiation turns a class into a running instance: an application-profile
produces an application-instance, which creates agent-instances from the
agent-profiles it seeds, each running a machine-instance. An agent-instance
binds its identity and workspace as it is instantiated; the primitives that
create and destroy instances and workspaces belong to the lifecycle. The class
level does not change as instances come and go — documents are immutable inputs
to instantiation.

## Construction is not instantiation

Two operations yield new entities, and the language keeps their names apart.
*Instantiation* creates an instance from a class, as above. *Expansion*
constructs a class from other classes: a template or fragment, combined with
arguments, produces a machine-profile at load time, entirely on the document
side. Two compound terms name the expandable units. A *stage-fragment*, whose
body is `stage`, bundles states and transitions to be spliced into a
machine-profile that declares its own body. A *machine-template*, whose body is
`machine`, replaces the body of a machine-profile document that carries an
expansion and nothing else.

Expansion is the language's only mechanism for class-to-class construction. A
machine-profile document assembles from reusable units at load time through the
`expand:` keyname, and the form it takes depends on the body kind of the unit it
names.

Splicing lets a machine-profile declare its body and expand stage-fragments,
adding their parameterized states and transitions to the declared lists; the
result is then validated as one whole. Alternatively, a machine-profile document
can omit its body and expand a single machine-template, whose `machine` body
replaces the document's body with substituted arguments. A machine-template may
itself expand stage-fragments, and nesting stops there. Both forms finish before
validation, so a processor only ever sees a complete machine-profile, and
neither touches the instance side — identity, workspace, or running state.

{{statement R-MODEL-001}}

{{statement R-MODEL-002}}

R-MODEL-002 holds wherever an expansion entry appears, not only in a
machine-profile document; the agent grammar applies it to the agent-profile
case.

Which form an entry takes is not visible in the document that carries it. A
splice and an application are written the same way and differ only in the body
kind of the unit they name, so the processor resolves the target before it can
act on either.

{{statement R-MODEL-003}}

The expanded machine-profile is validated like a hand-written one, with
diagnostics naming both the unit and the expanding document. Processor
obligations, and the fixture suites that exercise invalid expansions, are the
subject of the closing section on mechanized checking.
