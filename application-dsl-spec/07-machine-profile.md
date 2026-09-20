<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 7. Machine-profile grammar

## 7.1 The document

A machine-profile is a `machine.yaml`: the declarative control of an
agent-instance's loop. Its states, signals, and transitions are the whole of
the behaviour — a machine-instance does nothing a transition does not
declare. A machine-profile document either carries this body itself or is
constructed by expansion (chapter 03).

## 7.2 Keynames

{{keyname-table yaml:agent-core/docs/specs/config-formats/machine-format.yaml}}

{{statement R-MACH-001}}

## 7.3 Request machines

A request machine is a machine-profile bound to a REST endpoint: each
accepted request seeds one request-scoped machine-instance whose terminal
signals map to the response status. The binding grammar and its obligations
are specified by agent-core srd030 (machine request REST binding); the
machine-profile it binds is grammatically an ordinary machine-profile under
this chapter.

## 7.4 Fragments and templates

A machine-profile that declares its own body may splice stage-fragments, and
a bodiless machine-profile document applies exactly one machine-template;
both run under the expansion keyname at load time, governed by R-MODEL-001
and R-MODEL-002. The expanded result is subject to every keyname and
validation rule in this chapter, exactly as a hand-written machine-profile.
