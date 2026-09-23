<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Examples

Two applications in this repository exercise the language end to end, and both
are built only from declarations — their topology, routing, fan-out,
delegation, and deployment are configuration, not orchestration code. They sit
at opposite ends of the ownership spectrum: the chatbot mesh implements most of
its agents locally, while the coding agent is composition-only, binding agents
it does not implement.

## A chatbot mesh

The chatbot mesh is a multi-RAG question-answering system. Its application-profile
composes a browser-facing `chatbot`, one or more `rag-server` agents, an ingest
and monitoring surface, and a control plane, together with canonical roots drawn
from the shared catalog. Every agent-instance in the running mesh enters through
one of these roots, which is exactly the closure property the population
invariants require.

The `chatbot` agent shows the two machine kinds working together. A persistent
machine, obtained by expanding a shared `serve-machine-template`, keeps the
service alive; a request machine bound to the chat endpoint runs one turn per
request. The turn embeds the incoming message once, declares the set of trusted
RAG sources, fans the embedding out to each, composes an answer from the
retrieved chunks, and ends in a terminal `LLMResponded` or `Failed` signal that
sets the HTTP status. Each `rag-server` answers with a much smaller request
machine — vector in, chunks out, with a distinct `QueryRejected` signal for the
embedding-space mismatch a bare error would hide. The same
`serve-machine-template` is expanded by four different agents, each supplying its
own launch, stop, and await words: expansion by machine-template, in practice.

The control plane makes the decider-and-actor split concrete. The
`provisioning-workflow-orchestrator` decides what the mesh should look like but
holds no deployment credential; the `creator` alone mounts the deployment-API
secret and realizes that decision, and it never decides. This is the authority
separation the population invariants state, realized as two agent-profiles with
disjoint capabilities rather than as a convention. When the mesh ingests a
corpus, the `creator` runs a `corpus-ingest` profile as a child under an explicit
environment contract — instantiation of one agent by another, kept on the
instance side.

## A coding agent

The coding agent is composition-only: it implements no agents of its own.
Its application-profile binds three thin wrappers — a planner, an executor, and a
critic — plus an applier, over canonical role profiles from the catalog. Each
wrapper supplies its own request machine and REST configuration but binds the
same `role-server` machine, a generic persistent host with a request surface and
a control surface. One machine definition, three services.

The planner's request machine carries the loop: `AwaitingRequest` → `Planning` →
`Parsing` → `Executing` → `Critiquing`, ending in `Succeeded`, `Rejected`, or
`Failed`. The planner invokes inference to produce a plan, delegates the work to
the executor service, and gates the outcome on the critic: a `CriticAccepted`
signal reaches `Succeeded`, a `CriticRejected` signal reaches `Rejected`. The
executor changes an isolated workspace and runs the validation its machine
declares; the critic reads that changed workspace and returns a verdict. The
three roles form an authority chain — plan, act, judge — each a distinct service,
with the application's result determined by the last.

## What the examples show

Both applications are microservices applications assembled from data. The same
machine model covers the probabilistic chatbot turn — embedding, source
selection, inference — and the deterministic retrieval of a `rag-server`, and it
covers the mixed plan–act–judge loop of the coding agent. Agents that differ in
almost every other respect still compose: agent-owning against composition-only,
locally implemented roots against canonical ones, a child agent instantiated in
process against remote services reached over REST. What keeps the declarations
honest across both is the mechanized checking described next.
