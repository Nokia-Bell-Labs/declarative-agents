<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 7. Machine-profile grammar

## 7.1 The document

A machine-profile is a `machine.yaml` file that declaratively controls an
agent-instance's loop. Its states, signals, and transitions define all
behavior — a machine-instance only acts as declared by transitions. A
machine-profile either contains this body directly or is built through
expansion (chapter 03).

## 7.2 Keynames

{{keyname-table yaml:agent-core/docs/specs/config-formats/machine-format.yaml}}

{{statement R-MACH-001}}

## 7.3 Request machines

A request machine binds a machine-profile to a REST endpoint: each accepted
request creates a request-scoped machine-instance, whose terminal signals
determine the response status. The binding rules and requirements are defined
in agent-core srd030 (machine request REST binding); the bound machine-profile
is grammatically standard under this chapter.

## 7.4 Fragments and templates

A machine-profile with a declared body splices stage-fragments; a bodiless one
applies one machine-template. Both execute under the expansion keyname during
load time, per R-MODEL-001 and R-MODEL-002. The expanded result follows all
keynames and validation rules in this chapter, like a hand-written
machine-profile.
