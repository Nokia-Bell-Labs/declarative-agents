<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Introduction

Agents built today wrap a probabilistic tool — a language model — in imperative
control flow. The harness is code: it calls the model, reads the reply, and
branches on it. This works until the model returns something the branches did
not anticipate, and because the model is probabilistic that first surprise can
arrive in production, on a path no reviewer ever traced. The failure is not a
bug in the ordinary sense; it is control flow that was never written down where
it could be checked.

Much of a harness exists to suppress this non-determinism — to constrain the
model's output until the surrounding code can act on it. A model makes two kinds
of mistake, and the harness deals with both. The first is a mistake of form: the
answer is right but arrives in the wrong shape — malformed JSON, a tool call
that does not match the schema, prose where a field was expected — and the
harness parses, validates, and re-prompts until the shape is one the code can
read. The second is a mistake of substance: the shape is fine but the answer is
wrong, and the harness guards against it with checks, retries, and fallback
paths. But the non-determinism behind both is not uniform: each model has its
own failure modes, its own idea of how to format a tool call, its own ways of
drifting off the expected path. A harness tuned to one model rarely fits the
next, so suppressing non-determinism is not a fixed cost paid once but an
ongoing act of customization. The harness must be flexible enough to be fitted
to each model, which is precisely the flexibility that ad hoc control-flow code
makes expensive to change.

The coupling runs deeper than tuning. Models are increasingly post-trained
*inside* a harness rather than against it: the scaffolding that decides which
tools are exposed, how they are described, and what each observation carries is
part of the environment the model is optimized for. This harness is not an inert
engineering detail but a design dimension that interacts with post-training, and
a model taken out of the harness it was trained in can degrade sharply under
tool-environment shift [@kim-harness-posttraining-2026]. Taking a model out of
its harness, or evolving the harness underneath it, is therefore a real cost —
one more reason to make the harness an explicit, inspectable artifact rather
than control flow buried in code.

This paper takes a different route: the agent's control flow is written as data.
An agent is a finite-state machine whose transitions dispatch tools, and a fixed
engine interprets that machine at runtime. What the agent may do at each step,
and what it does with each outcome, is a table a reader can inspect and a
processor can check before the agent runs. An unhandled outcome becomes a
load-time error rather than a silent dead end, and fitting the agent to a
different model — new signals for its failure modes, new transitions for its
quirks — is an edit to the data, not a rewrite of the engine.

We develop this into a domain-specific language — an inner DSL in Fowler's
sense, hosted in YAML — for declaring an application, its agents, and their
machines. The language has three document families: the application document
(`application.yaml`), the agent profile and its referenced documents
(`profile.yaml`, tool declarations, tool selections, REST definitions), and the
machine document (`machine.yaml`). The rest of this section builds the idea from
a single agent up to the multi-agent application.

## Agents as state machines and tools

An agent is a state machine paired with a set of tools. The machine is the
control flow: states are the phases the agent moves through, signals are the
typed outcomes it can observe, and each transition — on a given signal in a
given state — dispatches one tool and names the next state. The tools are the
agent's actions; the machine decides what to do and when, and a tool does it.
Nothing in the agent's behavior lives outside these two parts, so the whole
loop — observe, decide, act — is data a processor reads, not control flow
buried in code.

The tools span a spectrum. At one end a tool is deterministic local code: it
runs, returns a fixed signal, and a workflow agent whose every transition fires
such a tool behaves the same way on every run. At the other end a tool leaves
the process and returns a result the machine cannot predict — an inference
request to a language model, a retrieval for RAG search, or an agentic tool
call whose signal the machine branches on. One machine model covers the range:
the difference between a deterministic workflow agent and an LLM-driven one is
which tools its transitions dispatch, not a different kind of agent.

Among these tools is the REST call. A tool backed by a REST definition binds to
a transport and reaches a service outside the agent: the agent
issues a request, waits for the response, and turns it into a signal the
machine transitions on. A REST call is therefore not a special primitive but
one tool among the others — the tool through which an agent reaches another
agent.

## Multi-agent systems as microservices applications

Because an agent reaches another agent through a REST call, and because each
agent likewise exposes an interface others call, the agents are services. A
multi-agent system is then a microservices application whose services are
agents: each agent is independently deployed, addressable, and bound by a
declared contract, and the application is the composition of those agents and
the messages between them.

This compounds the harness problem rather than sharing it away. The agents in an
application need not run the same model, and even when they do they play
different roles — a router, a summarizer, a coder — each with its own tools, its
own failure modes to suppress, and its own control flow. A multi-agent system
therefore wants a *different* harness per agent, not one harness stretched over
all of them. Writing each harness as data, in that agent's own profile, is what
lets a single application hold many differently-tuned agents without a
correspondingly tangled body of code.

The application-profile declares that composition: which
agent-profiles the application runs, how they are owned and packaged, and the
roots through which an agent enters the running population. An agent-profile in
turn declares one agent — its machine, its tools, its REST definitions, and its
inference configuration. The language thus has two grammars in a
nesting relationship: the application grammar composes agents into a
microservices application, and the agent grammar defines each service as a
state machine and its tools.

The rest of the paper develops this in order: the model and the vocabulary it
rests on, the grammar of the application, agent, and machine documents, the
lifecycle and the invariants that hold across a running population of agents,
worked examples drawn from this repository, and a note on how those examples
are checked mechanically.
