<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Lifecycle and population

The lifecycle governs how a single instance comes and goes; the population
invariants govern the set of instances that exist at any moment. This section
takes them in that order.

## The lifecycle square

The lifecycle has two columns — one for persistent workspaces, one for transient
instances — and each column has a creating and a destroying primitive, four
operations in all.

| | Workspace column | Instance column |
|---|---|---|
| create | provision | instantiate |
| destroy | decommission | terminate |

Provision creates a workspace and its identity; decommission destroys both.
Instantiate combines an agent-profile, an identity, and a provisioned workspace
into a running agent-instance; terminate ends that instance. The columns operate
independently: terminating an instance does not touch the provisioned workspace,
and a workspace can stay provisioned with no instance running. Expansion sits
outside the square — the model keeps class-to-class construction separate from
these operations.

Binding connects the columns. An agent-instance binds one workspace for its
lifetime, adopts its identity, and releases it on termination. The workspace
keeps that identity and its contents and stays inactive until the next
agent-instance takes it.

{{statement R-LCM-001}}

{{statement R-LCM-002}}

Resumption is how a later instantiation continues a run rather than starting
one. The runtime loads the persisted position and execution log, re-enters the
same machine-instance where it stopped, and lets the resume signal drive the
first transition; the request that seeded the original run is not replayed.

{{statement R-LCM-003}}

Suspension is the deliberate pause that makes resumption possible. A suspended
run promises a resumable checkpoint, so a run whose suspending save did not
land is a failed run.

{{statement R-LCM-004}}

Rollback has two halves. The checkpoint backend rewinds what it persisted, and
each rewound entry's receipt, which only the tool that wrote it interprets,
reverses the effect that entry had outside the runtime.

{{statement R-LCM-005}}

History and rollback are not a second programming model. They are
agent-profiles like any other, whose machine-profiles dispatch the lifecycle
tools.

{{statement R-LCM-006}}

Runtime-target statements bind running systems, so their acceptance entries are
rig references: each names a test in this repository's integration suites that
exercises the obligation against a real workspace or deployment. The language
gate checks that every reference resolves to a declared test, so a renamed or
deleted rig test fails `mage audit`. The referenced suites run under the
repository's test and integration gates, which own the clusters and rigs the
audit itself does not.

## The run

A run is one pass of an agent-instance through its machine-instance, from the
seed signal to a terminal state. It starts only after the runtime has checked
the whole wiring between the machine-instance and the tools its agent-profile
selects; a fault found there stops the process before any tool runs.

{{statement R-RUN-001}}

The check covers the closure, not only the top-level machine-instance. A child
agent-profile or a nested in-process run that would fail its own start fails
the parent's start instead, before the parent does any work.

{{statement R-RUN-002}}

The request is the run's one data channel. The runtime does not interpret it;
it hands the bytes to the first transition, and the selected tools decide what
they mean.

{{statement R-RUN-003}}

The runtime interprets; the machine-instance decides. Each step looks up the
current state and signal, and the outcome of a run comes from the terminal
state's declaration, not from its name.

{{statement R-RUN-004}}

Budgets are the runtime's universal safety. An exhausted iteration or token
budget becomes a signal the machine-instance routes like any other; an
exhausted clock cancels the run outright, because a cancelled process may not
be in a state where another transition can safely run.

{{statement R-RUN-005}}

Dispatch protection keeps a misbehaving tool inside the grammar. A panic or a
hung command does not take the process down; it becomes a `CommandError` the
machine-instance can recover from.

{{statement R-RUN-006}}

The step log is the run's history, and the checkpoint is that history made
durable. With persistence disabled the save is a no-op; with it enabled, the
runtime does not report progress it could not persist.

{{statement R-RUN-007}}

A caller that runs an agent-instance as a child process reads the outcome from
the exit status. A clean domain failure and a process that could not complete a
run are different facts, and the exit codes keep them apart.

{{statement R-RUN-008}}

Standard output carries the run's answer and nothing else. The machine-profile
names which result is the answer, and the runtime does not invent one when it
does not.

{{statement R-RUN-009}}

Termination is cooperative once the run has started. A signal unwinds the run
the same way cancellation does, so the evidence of an interrupted run survives
it.

{{statement R-RUN-010}}

A child agent-instance is another process with its own agent-profile. What
crosses the boundary is the agent-profile's path and the run's data; what the
child executes stays in the child's agent-profile, so a parent cannot reshape
the child's program from outside it.

{{statement R-RUN-011}}

The runtime safety boundary separates universal mechanics from domain behavior.
Budgets, dispatch protection, and checkpointing belong to the runtime; every
decision that depends on a particular workflow belongs to the declarations.

{{statement R-RUN-012}}

## The population

A population is the set of active instances in a mesh at a given moment —
application-instances, their agent-instances, and the workspaces those agents
bind or leave waiting. The invariants below apply to the whole set, which is
what separates the population target from the runtime target: a conforming
runtime can host a non-conforming population if some external entity starts an
agent-instance no application-profile declares.

The population is closed under the declarations. Every running agent-instance
traces to an agent-profile named among the roots of a governing
application-profile; there is no way into the population except through a
declared root.

{{statement R-POP-001}}

Deciding and acting are distinct roles. One agent-profile's instances determine
the desired state while another's hold the exclusive authority to change it. The
provisioning-workflow-orchestrator shows the separation: it makes the
operational decisions, and the creator alone reaches the deployment interface and
never decides (chatbot-mesh srd004 and srd005).

{{statement R-POP-002}}

Two invariants give workspaces a durable history. Exclusivity limits a workspace
to one bound agent-instance at a time, so no two locations act under the same
identity at once. Orphan legality makes a workspace with no bound agent-instance
a normal state — it is how an identity survives between instantiations, and only
an explicit decommission removes it.

{{statement R-POP-003}}

{{statement R-POP-004}}

An application-profile declares how many agent-instances each root seeds, and the
population count follows that declaration. The grammar for declaring cardinality
lives with the application-profile; the population statement is aligned with it
so the two cannot drift.

In this repository the operator is the population authority. One `Agent`
resource declares one agent-profile and an instance count, and the operator
realizes that declaration as running agent-instances it owns.

{{statement R-POP-005}}

The population authority applies the same startup checks the runtime applies,
before scheduling. An agent-profile that would refuse to start never enters the
population.

{{statement R-POP-006}}

The declaration stays the authority after scheduling. What a running
agent-instance exposes comes from its agent-profile, and an edited
agent-profile reaches the instances that realize it.

{{statement R-POP-007}}
