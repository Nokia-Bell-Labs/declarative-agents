<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 3. Model

## 3.1 Three levels

The language defines a three-level system, each with its own document family.
At the application level, an application-profile declares composition,
specifying agent-profile names and their respective agent-instance counts. The
agent level defines behavior, binding a machine-profile, tool selections, and
referenced documents into an executable unit via an agent-profile. The machine
level controls operation, with a machine-profile detailing states, signals,
and transitions for an agent-instance's loop.

Levels reference downward only. An application-profile names agent-profiles;
an agent-profile names a machine-profile and its associated documents; a
machine-profile names nothing above itself. The detailed grammar for these
levels is in Chapter 05 through Chapter 07.

## 3.2 From class to instance

Instantiation transforms a class into a running instance: an
application-profile produces an application-instance, creating agent-instances
from its seeded agent-profiles, each executing a machine-instance. An
agent-instance binds its identity and workspace during instantiation (section
2.1); lifecycle primitives for creating/destroying instances and workspaces
are in chapter 04. The class level remains unchanged as instances are created
or terminated — documents serve as immutable inputs to instantiation.

## 3.3 From class to class: expansion

Expansion is the language's sole mechanism for class-to-class construction. A
machine-profile document assembles from reusable units at load time using the
`expand:` keyname, with form determined by its target's body kind.

Splicing lets a machine-profile declare its body and expand stage-fragments,
adding their parameterized states and transitions to the declared lists. The
spliced machine-profile is then validated as a unified whole.

A machine-profile document can omit its body and expand a single
machine-template, where the template's `machine` body replaces the document's
body with substituted arguments. A machine-template can itself expand
stage-fragments, and nesting stops there.

Both forms finish before validation, ensuring processors encounter only
complete machine-profiles. Expansion stays within its designated scope,
avoiding interaction with the instance side, identity, workspace, or running
state.

{{statement R-MODEL-001}}

{{statement R-MODEL-002}}

R-MODEL-002 holds wherever an expansion entry appears, not only in a
machine-profile document; chapter 06 applies it to the agent-profile case.

Which form an entry takes is not visible in the document that carries it. A
splice and an application are written the same way, and they differ only in
the body kind of the unit they name, so the processor resolves the target
before it can act on either.

{{statement R-MODEL-003}}

The expanded machine-profile is validated like a hand-written one, with
diagnostics naming both the unit and the expanding document. Chapter 09 covers
processor obligations and fixture suites that exercise invalid expansions.
