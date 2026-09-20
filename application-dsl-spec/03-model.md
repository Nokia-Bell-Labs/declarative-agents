<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 3. Model

## 3.1 Three levels

The language describes a system at three levels, each declared by its own
document family. The application level declares composition: an
application-profile names the agent-profiles that make up the application and
how many agent-instances each one seeds. The agent level declares behaviour:
an agent-profile binds a machine-profile, tool selections, and referenced
documents into one runnable unit. The machine level declares control: a
machine-profile gives the states, signals, and transitions that drive an
agent-instance's loop.

The levels reference downward only. An application-profile names
agent-profiles; an agent-profile names a machine-profile and its documents; a
machine-profile names nothing above itself. Chapter 05 through chapter 07
give each level's grammar.

## 3.2 From class to instance

Instantiation takes a class and produces a running instance: an
application-profile yields an application-instance, whose seeded
agent-profiles yield agent-instances, each executing a machine-instance. An
agent-instance binds its identity and workspace at instantiation (section
2.1); the lifecycle primitives that create and destroy instances and
workspaces are chapter 04's subject. Nothing at the class level changes when
instances come and go — documents are immutable inputs to instantiation.

## 3.3 From class to class: expansion

Expansion is the language's only class-to-class construction. A
machine-profile document may be built from reusable units at load time, under
the `expand:` keyname, in two forms that its target's body kind selects.

Splicing: a machine-profile that declares its own body may expand
stage-fragments, whose parameterized states and transitions are appended to
the declared lists. The spliced result is validated as one machine-profile.

Template application: a machine-profile document may instead carry no body at
all and expand exactly one machine-template, whose `machine` body becomes the
document's body with the entry's arguments substituted. A machine-template
may itself expand stage-fragments; it may not expand another
machine-template.

Both forms finish before validation, so a processor sees only complete
machine-profiles. Expansion never crosses to the instance side: no identity,
no workspace, no running state is involved.

{{statement R-MODEL-001}}

{{statement R-MODEL-002}}

The expanded machine-profile is validated exactly as a hand-written one, with
diagnostics that name both the unit and the expanding document; the processor
obligations, and the fixture suites that exercise invalid expansions, are
chapter 09's subject.
