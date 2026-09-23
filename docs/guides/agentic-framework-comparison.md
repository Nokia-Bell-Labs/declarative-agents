<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Declarative control for autonomous networks

This guide asks why an autonomous network needs more than a broad agent SDK,
then compares the resulting requirements with agent frameworks and durable
workflow systems. It does not rank products. Existing systems can host many of
the mechanisms described here. The distinction is whether the network control
contract is a portable, enforceable program or application code assembled
anew around each framework.

The technical case is not that networks need another agent SDK. Autonomous
networks need an execution constitution between probabilistic planning and
privileged actuation. That constitution must join control flow, authority,
current-state evidence, external effects, recovery, and accountability. A
model may interpret intent or propose a plan; it must not acquire credentials,
select raw endpoints, bypass policy, or define what counts as a successful
rollback.

Declarative Agents is one implementation of that approach. Its strongest
shipped contribution is a profile-driven machine with declared tools and
outcomes, interpreted by a fixed runtime. The custom Go runtime is not
intrinsically required. A mature workflow backend could execute a conforming
machine if it preserves the same authority, effect, evidence, and reversal
semantics. This guide therefore separates three questions:

1. Which properties autonomous networks require.
2. Which portable contract would make those properties enforceable.
3. Which parts the current Go implementation actually demonstrates.

The framework comparison was reviewed on 2026-09-21 against the official
documentation linked under [Sources](#sources). Framework behavior and
hosted-product features change; the links, rather than a remembered version,
are the evidence for each framework claim.

## Evidence and claim vocabulary

Framework capabilities use four terms:

- **Native** means the documented runtime or SDK supplies the capability.
- **Extension** means an officially documented integration supplies it.
- **Application** means the application can implement it, but the framework
  does not define the common contract being compared.
- **Not core** means the capability is outside the framework's documented
  center. It does not mean that an integration is impossible.

Claims about this repository use seven evidence statuses:

- **Shipped runtime** means agent-core implements the behavior and focused
  executable tests cover the reusable contract.
- **Shipped application** means an application-owned integration exercises the
  behavior through its production composition path.
- **Conformance fixture** proves a reusable boundary or lifecycle contract but
  is not a deployed operational service.
- **Design intent** describes a documented target without a complete shipped
  path and representative evidence.
- **External requirement** is necessary for network autonomy but belongs to a
  controller, policy system, inventory, protocol adapter, identity platform,
  digital twin, or other system outside this repository.
- **Not demonstrated** means the repository has no representative evidence for
  the claim.
- **Research horizon** names a capability whose underlying method remains an
  open research problem rather than a planned product feature.

Persistence is not rollback. Persistence reconstructs execution state after a
pause or failure. Rollback restores prior persisted state and reverses or
compensates for effects in external systems. A framework can provide the first
without the second.

## What an autonomous network means here

An autonomous network is a governed closed-loop system. It receives intent or
detects deviation, evaluates policy and current state, plans bounded work,
obtains exceptional approval where required, actuates through limited
authority, verifies service and forwarding outcomes, and then succeeds,
compensates, enters a safe state, or escalates. It repeats without exceeding
declared risk or allowing interacting loops to oscillate without bound.

This definition excludes unrestricted model-to-device access. It also excludes
the claim that a chatbot, a finite machine, or a multi-agent topology is by
itself an autonomous network. Humans remain accountable for objectives,
policy, exceptional decisions, and the boundaries of autonomous authority.
Self-evolving does not mean self-authorizing.

## Requirements from the network and its stakeholders

The requirements below are independent system properties, not different names
for state machines. Each names the stakeholders who depend on it, the network
condition that creates it, and what a general agent or workflow framework
leaves to the application.

### Operations and service continuity

#### AN-1: Failure-contained actuation

**Customers, service operators, and NOC operators** need service continuity
while the managing system changes. A route, policy, sleep-state, or rollout
decision can affect many services at once. The model may propose a change, but
a deterministic path must resolve its targets, run pre-checks, apply a bounded
increment, and verify service and forwarding outcomes.

Agent SDK guardrails can reject model inputs or tool calls. They do not derive
network invariants or prove that the forwarding plane still satisfies them.
Those checks are **external requirements** supplied by inventory, topology,
controller, simulator, and assurance systems. The machine's responsibility is
to make their order and failure routes non-bypassable.

#### AN-2: Topology-aware blast radius

**Network planners, engineers, and approval authorities** need impact expressed
in services, shared-risk groups, redundancy, and failure domains, not only a
device count. A request must not widen its own target set after approval.
Candidate targets therefore need a trusted, versioned inventory basis,
dependency-aware selection, canary waves, and a scope decision recorded with
the plan.

Budgets and tool lists do not calculate this blast radius. A topology service
or digital twin does. The declarative contract can require and carry the
result, but the current repository does not implement network topology
analysis. A digital twin is a gate, not an oracle: its fidelity, calibration,
coverage, and allowed gap from production need evidence, followed by canary and
post-change assurance.

#### AN-3: Partial, uncertain, and delayed effects

**NOC operators and tool implementers** must handle more than success and
failure. A device can accept a command while the caller times out; one domain
can commit while another rejects; telemetry can arrive after the workflow
decides. Network operation contracts need outcomes such as `partial`,
`unknown`, `unverifiable`, `already_compliant`, and `compensated`, with
explicit routes to reconcile, isolate, roll forward, reverse, or escalate.

Durable workflow engines are well suited to waiting and recovery, but they do
not supply this outcome alphabet. Declarative Agents can check routes for the
signals a tool declares. It cannot prove that a tool reports reality
truthfully.

#### AN-4: Effect reversal rather than state rewind

**Operators, customers, and change authorities** need the network restored,
not merely the workflow resumed from an older checkpoint. Every mutating
operation needs a prior revision or snapshot, effect evidence, an idempotency
rule, and a reversal classification: exact, compensating, or irreversible.

Temporal Sagas and Step Functions branches can run compensations; network
automation can supply vendor-specific rollback. This repository's
**shipped runtime** reverts persisted state and then walks receipts in reverse.
That two-part mechanism is best effort rather than atomic: current evidence
does not show that a crash during the receipt walk durably records and resumes
every outstanding reversal. Irreversible entries are skipped, a compensatable
entry may require operator work, and exact recovery remains dependent on each
tool, concurrent external changes, device semantics, and post-rollback
verification.

#### AN-5: Closed-loop convergence

**Service operators and control-system architects** need assurance that
autonomous loops converge rather than fight. Energy optimization can wake and
sleep the same capacity that quality optimization tries to retain. Fault
remediation and traffic engineering can reroute load back and forth. Stable
operation requires ownership, hysteresis, cooldowns, stale-observation rules,
bounded remediation, and a safe terminal or escalation state.

An execution graph can show one loop's path. It does not prove stability among
several loops acting on a changing network. Application-level convergence
analysis and long-duration experiments are **not demonstrated** here.

### Distributed operation and platform reliability

#### AN-6: Delivery identity, deduplication, and ordering

**Platform SREs and adapter implementers** must assume retries, duplicate
delivery, and local rather than global ordering. Messages need stable identity,
intent correlation, idempotency keys, deduplication, and an explicit ordering
scope.

Agent conversation history is not a delivery contract. Durable engines provide
much of the transport and replay machinery. Device-side operation identity
remains an application or controller concern; AN-3 covers reconciliation when
delivery succeeds but the effect remains ambiguous.

#### AN-7: Partition modes and sovereign domains

**Solution and domain architects, platform SREs, and federation owners** need a
decision for each partition: fail safe, hold last state, or continue locally
under a lease. Edge and regional loops cannot assume cloud inference, a global
inventory, or a shared mutable workspace remains reachable.

Federated domains also keep their own credentials, policies, telemetry, and
assurance. Peering exposes negotiated capabilities and evidence; it does not
collapse domains into one controller. Agent-worker distribution alone does not
define quorum, fencing, split-brain prevention, or reconciliation after
reconnection.

#### AN-8: Several timescales and placement constraints

**Network architects and capacity planners** operate loops with different
deadlines. Protection-grade control belongs in network elements or
deterministic domain controllers. Assurance may run in seconds, planning in
minutes, and approvals over hours. Placement also follows latency, residency,
data gravity, authority, energy, and availability.

One model-driven loop is inappropriate for all of these classes. LLM-free
machines must remain first-class, and model unavailability must not prevent a
safe hold state, rollback, or emergency intervention.

#### AN-9: High availability, fencing, and disaster recovery

**Platform SREs and operators** need safe behavior across process, node,
checkpoint-store, site, and control-plane failure. Durable state is only one
part. Production control also requires replicated persistence, ownership
fencing, duplicate-dispatch prevention, backup and restore, RPO/RTO targets,
and chaos evidence.

The repository ships checkpoint adapters and Kubernetes packaging. It does not
demonstrate a highly available network controller or disaster-recovery design.

#### AN-10: Fleet scale, contention, and backpressure

**Platform owners and network operators** need per-resource serialization,
sharding, queue and fairness policy, reserved emergency capacity, and explicit
degradation under overload. A fault storm can create agents and inference work
at the moment assurance capacity is most valuable.

Budgets, bounded queues, and request-scoped machines are useful primitives.
Representative device-fleet throughput, conflict behavior, and overload
benchmarks are **not demonstrated**.

### Authority, governance, and accountability

#### AN-11: Orchestrate-Act separation

**Security owners, planners, and NOC operators** need decision authority
separate from deployment authority. A planner, model, Critic, or Guardian
should not hold southbound credentials. The Actuator receives narrowly scoped
authority for a domain, target class, and operation.

Role names and model-visible tool filtering do not enforce this split.
Workload identity, credential brokering, mTLS, network policy, admission, and
negative authorization tests must realize it. Framework middleware can
participate, but deployment identity is the control that matters.

#### AN-12: Critic and Guardian are different gates

**Service designers, policy owners, and approval authorities** need two
questions answered independently. The Critic evaluates whether a candidate
would satisfy acceptance criteria. The Guardian evaluates whether the action
is allowed under business, security, scope, rate, outcome, authority, and
budget policy.

A single model verdict or callback obscures why a plan was rejected. A complete
system needs versioned policy, deny-by-default evaluation, conflict semantics,
an explainable decision receipt, and binding between approval and the exact
policy-checked plan. This repository does not ship that complete network policy
subsystem.

#### AN-13: Human intervention independent of the agent plane

**Operators and accountable authorities** need authenticated inspection,
approval, rejection, pause, guardrail tightening, autonomy reduction, and
emergency withdrawal even when the agent plane is overloaded or compromised.
Approval also needs expiry, escalation, delegation, decider identity, rationale,
and safe behavior when no person responds.

The runtime has a **conformance fixture** for suspend and resume. It does not
ship the complete authority service, decision record, out-of-band stop path, or
rejection-triggered production rollback. A domain cannot advance to bounded
autonomy while the human and organizational owners of guardrails,
authorization, escalation, and accountability are unassigned.

#### AN-14: Attention is a managed capacity

**Operations leaders and NOC operators** cannot review every autonomous
decision. Escalation storms can exceed human capacity during a widespread
fault. Risk classification, queue priority, evidence packaging, delegation,
expiry, and intervention-rate metrics determine whether human-on-the-loop
governance remains available.

Human approval support in an SDK provides the wait. It does not size or govern
the organization that must answer.

#### AN-15: Intent, policy, and decision lineage

**Business stakeholders, Intent Owners, auditors, service designers, and data
or model engineers** need a low-level effect justified by the objective and
evidence that caused it. Business intent decomposes into service and resource
intents, requirements, plans, actions, and receipts. Stable parent identity,
world-model revision, model version, and policy version must survive every hop.

Generic traces show execution. They do not automatically preserve this
semantic lineage. The repository records machine execution and profile
identity but does not ship the shared intent, policy, and decision-record model
described by the reference architecture.

#### AN-16: Tamper-evident and residency-aware evidence

**Auditors, security teams, and data owners** need attributable evidence with
retention, redaction, integrity, and residency controls. Telemetry, customer
data, model context, credentials, and decision records cannot all cross the
same boundaries or enter traces and checkpoints.

Transition spans and execution history are useful evidence primitives. They
are not by themselves non-repudiable audit storage. Signed identity,
append-only retention, legal hold, integrity verification, and classified-data
handling remain external requirements.

### Integration, lifecycle, and self-evolution

#### AN-17: Multi-vendor semantic integration

**Vendors, systems integrators, and tool implementers** face different commit
models, error codes, eventual-consistency behavior, rollback support,
transaction scope, rate limits, and idempotency even when APIs share a
transport. Generic REST declarations reduce repeated HTTP machinery; they do
not remove protocol, semantic, identity, deployment, or operational
integration.

The repository does not ship NETCONF, RESTCONF, gNMI, OpenConfig, Batfish,
Ansible, Nornir, vendor capability matrices, or physical-device evidence.

#### AN-18: Truthful tool contracts

**Tool implementers and assurance engineers** need evidence that an
implementation matches its declaration. Static signal coverage proves only
that declared outcomes have routes. It does not prove that a REST endpoint or
Go implementation emits the right signal, applies atomically, records enough
state to reverse, or observes the resulting network effect.

Executable contract tests, protocol simulators, fault injection,
declaration-to-observed-effect checks, compensation idempotency tests, and
partial-rollback escalation are required. The engine cannot infer contract
truthfulness.

#### AN-19: Safe program and supply-chain evolution

**Platform owners, release engineers, and security teams** need signed profiles
and images, trusted signer policy, SBOM and provenance admission, semantic
compatibility, checkpoint migration, mixed-version tests, vulnerability
response, and anti-rollback policy. A small configuration diff is easier to
review than a binary change, but it still needs testing, admission, canary
rollout, and recovery.

This repository ships deterministic closures, content references, read-only
profile mounts, and release gates. It does not provide the complete signed
supply-chain and in-flight upgrade contract above.

#### AN-20: Self-change without self-authorization

**Agent-factory designers, formal-assurance engineers, and governance owners**
need generated roles, policies, tools, and machines to enter as versioned
candidates. Small programmed components validate, authorize, apply, and record
them. No agent expands its own authority, rewrites its own guardrails, or
creates a descendant with greater privilege.

The fixed interpreter and data-defined profile are a useful foundation.
Runtime role and specification synthesis, sound acceptance criteria for
capabilities novel in kind, and formal bounds on emergent multi-agent behavior
remain a **research horizon**.

#### AN-21: Agent lifecycle ownership

**Platform owners, capability-catalog operators, and governance owners** need
agent creation and retirement to preserve stable identity, an accountable
owner, health and resource bounds, capability registration and withdrawal,
authority revocation, cleanup, and a rule for in-flight work. Starting or
stopping an SDK agent or process covers only one part of that lifecycle.

The repository can package and run profile-defined workloads, but it does not
ship the complete autonomous Agent Owner and capability-catalog lifecycle
described by the reference architecture.

## What an agent means here

In this repository an agent is a profile interpreted by one Go runtime. The
profile selects a finite-state machine and a vocabulary of declared tools.
States name phases, tools perform actions, and signals are the closed set of
outcomes that route transitions. The engine, not a prompt or model, owns the
next transition.

An LLM is optional. `invoke_llm` is one boundary word, alongside REST calls,
human input, child processes, and deterministic local operations. The catalog
ships profiles with no model word; for example, the host-side deploy profile
validates, applies, verifies, and compensates without consuming model tokens.
Collectors, reconcilers, policy checks, deployment actuators, and data
pipelines fit the same definition.

This differs from SDKs in which an agent is primarily a model configured with
instructions and tools. Those SDKs may also provide deterministic graph nodes
or workflow functions. The distinction is the center of authority:

- a code-first SDK executes control flow assembled from language objects,
  callbacks, decorators, and functions;
- Declarative Agents executes a data-defined transition system whose actions
  resolve against declared tool contracts;
- a durable workflow system executes application code or a service-specific
  state language and does not itself define an LLM agent.

## An application is a workflow topology

A single profile can own the whole workflow in one process. A larger
application can distribute the workflow across profiles and service
boundaries. The same capability profile has three supported consumption
forms:

| Form | Boundary | Runtime shape |
|---|---|---|
| Child agent | `self_invoke` or `run_agent` | Child process using the same runtime image |
| Service endpoint | `machine_request` | One request-scoped machine run |
| Standalone workload | Serving wrapper | Independent Deployment |

The parent sees each boundary as one declared tool that returns one of its
declared signals. The child keeps its own machine, budget, trace, and failure
boundary. An application can therefore centralize sequencing in one machine,
distribute actors as microservices, or combine both. Kubernetes is a
deployment substrate, not the workflow definition.

This model does not imply that more services are better. Split an actor when
authority, failure isolation, scaling, or lifecycle needs an independent
boundary. Keep tightly coupled steps in one machine when a remote boundary
adds no control benefit.

## Capability comparison

Four system classes contribute different parts of the solution:

- **Agent SDKs** supply models, tools, delegation, memory, evaluation, and
  developer experience. They generally leave network authority, effect,
  topology, rollback, and intent-lineage contracts to the application.
- **Durable workflow engines** supply scheduling, recovery, timers, signals,
  workers, and compensation execution. They generally leave agent semantics
  and network effect contracts to authored workflow and activity code.
- **Statecharts and service state languages** make control flow inspectable and
  structurally valid. They do not by themselves define model boundaries,
  network tools, evidence, or reversal.
- **Network automation and controllers** supply inventory, protocols, device
  semantics, and proven operations. They do not generally admit
  runtime-generated agent programs under one portable model/non-model
  contract.

The table summarizes the architectural center of each compared system.
“Application” does not mean incapable; it means the common contract must be
designed and maintained by the adopter. Details and qualifications follow.

| System | Authored control flow | Non-LLM workflow | Static checks | Durable pause/resume | Approval | External-effect rollback |
|---|---|---|---|---|---|---|
| Declarative Agents | YAML finite-state machine and tool contracts | Native | Shipped runtime: reference and transition-key checks plus declared tool-outcome coverage; reachability diagnostics do not constitute general formal verification | Shipped runtime with a persistent backend such as Dolt; no-op or in-memory state is not restart-durable | Conformance fixture; deployed authority service is not shipped | Shipped non-atomic mechanism: persisted-state revert plus receipt walk; exact recovery is tool- and environment-dependent |
| Google ADK 2.0 | SDK graph of agents, tools, and functions; experimental YAML Agent Config for a supported subset | Native graph nodes; `LlmAgent` remains model-centered | Agent Config schema and runtime node schemas; no general static graph checker is documented | Native workflow/session facilities; Restate adds journaled recovery | Native workflow HITL; Restate adds durable external waits | Application |
| LangGraph | Code-defined graph or decorated functions | Native | Graph compilation; function outcomes remain code | Native checkpointers | Native interrupts | Application-defined compensation; native error-handler routing can direct Saga branches |
| Microsoft Agent Framework | Code-defined typed graph or functional workflow | Native function executors | Connectivity, executor binding, and typed routing | Native checkpoints | Native request/response HITL | Application |
| OpenAI Agents SDK | Model-agent loop plus Python orchestration | Not core | Runtime tool/output-schema validation and guardrails, not static workflow-graph validation | Native serializable run state for HITL; Restate and DBOS provide durable-execution extensions | Native tool approval | Application |
| CrewAI | Python crews and decorated Flows | Native Flow methods; crews remain model-centered | Python typing and application tests; Flow routes remain code | Native Flow persistence and event-driven checkpoints | Native human-feedback decorator | Application |
| Temporal | Deterministic workflow code and activities | Native | No general static workflow validation; replay tests check recorded-history compatibility | Native durable execution | Native signal-and-wait pattern | Application-defined Saga compensations with durable execution |
| AWS Step Functions | Amazon States Language state machine | Native | State-language definition validation; task semantics remain external | Native Standard Workflow execution | Native callback task-token pattern | Application-defined branches and compensating tasks |

“Application” in the last column is material. A callback that retries a failed
device call is not rollback. Reversal needs a prior-state snapshot or
compensation receipt, ordering rules, idempotency, and an explicit result when
an effect is irreversible.

The operational surface differs too:

| System | Composition and deployment | Authority boundary | Observability and model scope |
|---|---|---|---|
| Declarative Agents | Process child, request endpoint, or standalone Kubernetes workload using one runtime image | Machine phase, selected tools, declared signals, budgets, and tool side-effect contract; deployment identity must separately enforce credential scope | Transition and GenAI spans; provider dialects are profile data |
| Google ADK 2.0 | Graph nodes in an application deployed to Agent Runtime, Cloud Run, GKE, or containers | Agent tools, callbacks, plugins, and graph routing | Events, evaluation, telemetry, and Google or other model integrations |
| LangGraph | In-process graph or Agent Server/Platform deployment | Node and edge code, interrupts, and application policy | LangSmith integration; model use is optional and supplied through the application stack |
| Microsoft Agent Framework | In-process workflow runtime or Durable Extension over Azure Functions or self-hosted workers | Typed executors, middleware, tools, and request information | Telemetry and broad model-client support inherited from its agent layer |
| OpenAI Agents SDK | Application-hosted SDK with documented Restate and DBOS durability integrations | Guardrails, tool schemas, handoffs, and tool approval | Built-in tracing; OpenAI models plus custom-provider points and beta Any-LLM/LiteLLM adapters |
| CrewAI | Application-hosted Flows or CrewAI Enterprise | Flow routing, task/tool configuration, and human feedback | CrewAI tracing and multiple model-provider integrations |
| Temporal | Workers coordinated by a Temporal service; child workflows and activities cross boundaries | Workflow code, activity contracts, signals, and namespaces | Event history and platform visibility; no model abstraction |
| AWS Step Functions | Managed state machine coordinating AWS services and callback workers | Amazon States Language, IAM, service integrations, and task tokens | Execution history and service metrics; no model abstraction |

### Google Agent Development Kit

ADK is a broad agent SDK. Its `LlmAgent` combines a model, instructions, and
tools; ADK also defines non-model workflow and custom agents. ADK 2.0 supplies
a Workflow Runtime in which agents, tools, and functions become graph nodes.
That graph supports deterministic routing, retries, and human-input pauses, so
an application need not delegate every step to a model.

ADK also has an experimental YAML Agent Config format. Its schema covers a
supported subset of agents, tools, and sub-agents, while custom functionality
still enters through Python or Java. At the review date Agent Config supports
only Gemini models and does not support every agent type. It is therefore a
real declarative authoring option, but not the same closed machine-and-tool
contract as this runtime.

ADK provides sessions, state, memory, artifacts, events, plugins, evaluation
facilities, a development UI, and several deployment paths. It can run on
Google Agent Runtime, Cloud Run, GKE, or other container infrastructure. The
official Restate plugin adds journaled LLM and tool execution, durable
cross-agent HTTP calls, and long-lived human approval.

ADK's breadth is useful when model integrations, conversational state,
multimodal artifacts, managed agents, and Google Cloud deployment are primary
requirements. It does not replace a network transaction contract. Device
snapshots, compensating actions, policy gates, and post-change verification
remain application concerns unless supplied by domain tools.

### LangGraph

LangGraph is a low-level orchestration runtime for long-running stateful
agents. Developers define graph nodes and routing in Python or JavaScript, or
use a functional API. It explicitly mixes deterministic code with LLM-driven
steps.

Checkpointers persist graph state at super-step boundaries and support
recovery, time travel, and human interrupts. On resume, a node may run again.
The Functional API requires non-determinism and side effects to be placed in
tasks; Graph API nodes may perform effects directly, but rerun semantics still
require idempotent design or checkpointed task isolation. Native error-handler
routing can direct a Saga or compensation branch, while the compensating
operation remains application code.

The graph is inspectable, but node bodies and conditional routes are code.
Framework checks cannot derive the complete outcome alphabet of an arbitrary
Python function in the way this repository cross-checks a tool's declared
signals against every receiving state.

### Microsoft Agent Framework

Microsoft Agent Framework is the successor to AutoGen and Semantic Kernel. It
separates model-backed agents from graph-based workflows that connect agents
and ordinary functions. The graph API validates connectivity, executor
binding, and typed message routing. Its experimental Python functional API
uses native Python control flow for less fixed topologies.

Graph workflows checkpoint executor state, messages, pending requests, and
workflow metadata at super-step boundaries. Python functional workflows can
checkpoint after each completed `@step`. Request executors support
human-in-the-loop continuation. Checkpoint storage alone does not distribute
execution; the separate Durable Extension supports Azure Functions or
self-hosted distributed workers over Durable Task infrastructure.

These checks are stronger than an unvalidated callback chain, but application
code still defines executor behavior and conditional logic. The framework
does not define a common receipt-and-undo contract for external effects.

### OpenAI Agents SDK

The OpenAI Agents SDK centers on an LLM agent with instructions, tools,
guardrails, handoffs, sessions, and a built-in turn loop. Python code composes
manager and handoff patterns. The SDK supplies custom model-provider points
and beta Any-LLM and LiteLLM adapters whose feature support varies by provider,
but a workflow without model participation is outside its core agent
abstraction.

Tools can declare approval requirements. A paused run exposes interruptions,
serializes `RunState`, records approve or reject decisions, and resumes the
original top-level run. Sessions retain conversation history. Restate and DBOS
are documented durable-execution integrations for recovery across failures
and restarts.

Guardrails and approvals constrain calls at important boundaries. They do not
by themselves prove that every possible tool outcome has a route or reverse a
completed external mutation.

### CrewAI

CrewAI distinguishes Crews, which coordinate model-backed agents, from Flows,
which are Python methods connected by decorators. Flows provide explicit
branches, loops, typed state, persistence, and human-feedback pauses; a Flow
can wrap one or more Crews only where inference is useful. Explicit routing
does not mean deterministic replay: arbitrary Flow methods can be
non-deterministic, and human-feedback outcome routing can itself invoke an LLM.

`@persist` saves Flow state and supports continuation after restart. CrewAI
also documents best-effort event-driven checkpoints for Crews, Flows, and
Agents. The default `@human_feedback` provider blocks for console input. With
an asynchronous provider, raising `HumanFeedbackPending` persists state before
returning control. CrewAI Enterprise supplies a managed deployment path and
tracing.

Flow structure improves operational control around a Crew, but methods remain
application code. Compensation for an already completed network mutation is
also application code.

## Adjacent workflow systems

Temporal and AWS Step Functions are relevant because network automation is
often closer to a durable transaction workflow than to an open-ended chat
agent.

Temporal executes durable workflow functions and records event history for
deterministic replay. Signals and durable waits implement approval. Activities
cross into external systems. The documented Saga pattern registers
idempotent compensations and executes them in reverse order after failure.
The workflow and compensation logic are code, while the Temporal service
provides the durability that ensures they continue after worker failure.

AWS Step Functions executes Amazon States Language, a declarative state
machine. Service integrations call tasks and microservices. Callback task
tokens pause Standard Workflows for external work or human approval, up to the
one-year execution limit; the token must be returned by a principal in the
same AWS account. Compensation is modeled with `Catch`, `Choice`, and explicit
compensating tasks; Step Functions does not infer an inverse operation.

These systems are alternatives when durable distributed execution is the
dominant requirement. They can also host LLM calls as ordinary activities or
tasks. They do not supply this repository's agent-specific model boundary,
tool vocabulary, per-phase model toolset, or tool-contract validation.

## The portable Network Action Machine contract

The requirements above justify a portable control-and-effect contract, not a
particular programming language or scheduler. We call that contract a
**Network Action Machine** here. A conforming implementation must provide all
of these properties together:

1. Control flow is a versioned data artifact with closed state/signal routing.
2. Every outcome declared by a dispatchable tool has a route before admission.
3. Model output cannot select raw endpoints, credentials, policy, or an
   undeclared capability.
4. Tool visibility and dispatch authority are phase-scoped.
5. Trusted services resolve targets and current-state revisions.
6. Mutating tools declare authority, preconditions, effects, evidence,
   uncertain outcomes, idempotency, and reversal classification.
7. Policy, approval, snapshot, apply, verify, and recovery ordering is
   structurally non-bypassable for the applicable change class.
8. Decision and deployment authority are assigned to different principals.
9. Profiles, contracts, decisions, and effects are content-addressed or
   versioned and correlated to the originating intent.
10. A backend conformance suite tests observable compliance with the
    engine-independent structural and lifecycle contract. It does not prove
    tool truthfulness, external authority enforcement, atomic effects, or
    network behavior.

One common change shape is:

```text
request or event
      |
      v
normalize intent -- optional LLM, no device authority
      |
      v
resolve inventory and policy -- trusted data and deterministic rules
      |
      v
build plan and bounded target set
      |
      v
pre-check --> approval when policy requires it
      |
      v
snapshot --> apply --> post-check
                  |         |
                  +---- failure ----> compensate or restore
```

The machine fixes the admissible order. The model can propose parameters or
choose among declared actions where a profile permits `$tool`, but it cannot
invent a transition or bypass a required gate. A request-supplied or
model-authored endpoint never becomes a device target directly; a trusted
inventory and policy step resolves the target from an identifier and binds the
result to the reviewed plan.

Network tools need contracts beyond a protocol client:

- **Authority:** device, tenant, command family, and credential scope.
- **Preconditions:** inventory version, observed state, maintenance window,
  redundancy, and blast-radius limit.
- **Effect:** the exact resource and state that may change.
- **Evidence:** request, response, before/after state, and correlation IDs.
- **Outcome signals:** changed, already compliant, rejected, timed out,
  partially applied, unverifiable, and failed.
- **Reversal:** exact restore, compensating operation, or an explicit
  irreversible classification.
- **Idempotency:** the key and behavior for retries after an uncertain result.

The Go runtime is the reference interpreter for much of this contract. It is
useful where a small self-hosted binary, profile-only deployment, offline or
edge placement, and one portable model/non-model execution path matter. It is
not the only possible backend. Temporal, Step Functions, or another durable
engine could execute a compiled form and provide stronger scheduling,
availability, and operational tooling. If compilation preserves the ten
properties above, backend portability strengthens the approach.

The strongest counterargument to this project is therefore that it has defined
a useful intermediate representation and unnecessarily coupled it to a custom
executor. The project justifies the custom runtime only when it demonstrates
that the same guarantees cannot be preserved with acceptable cost on an
existing backend, or when footprint, offline operation, placement, and
portability requirements exclude that backend.

The runtime claim is falsified if an existing platform can provide the same
machine, outcome, authority, effect, evidence, and reversal contracts without
bespoke scaffolding for each application. A comparative implementation should
measure configuration and code volume, validation coverage, partial-failure
behavior, audit burden, latency, operating cost, change success, and recovery
time rather than relying on feature names.

## Structural validation and formal-analysis limits

The shipped loader and validator perform useful structural checks before
dispatch:

- referenced states, signals, transitions, and selected tools must resolve;
- duplicate or conflicting state/signal transition keys are rejected;
- every signal declared by a dispatchable tool must be handled in the state
  that receives its result;
- diagnostics can report unreachable and dead grammar.

The first three checks reject malformed harnesses. Reachability diagnostics do
not establish that every non-terminal state reaches a terminal as a universal
admission condition. Tool-outcome coverage also assumes that the declaration
truthfully describes what the implementation emits.

A finite state set and declared signal alphabet make the abstract control
relation finite and inspectable. They do not make the set of executions finite.
Cycles permit long traces, iterators consume runtime-sized data, external
actors produce values outside the control abstraction, and child composition
expands the state space. Budgets and timeouts bound configured attempts under
stated cancellation and scheduler assumptions; terminal reachability does not
prove successful termination or termination of an external effect.

The representation is suitable for further formal analysis. A future
translation to a model checker could verify abstract invariants such as:

- no mutation state is reachable before policy and approval states;
- a model-selected action never reaches a tool outside the phase toolset;
- every partial-apply signal reaches compensation or an explicit
  operator-intervention terminal;
- the number of selected devices never exceeds a declared blast radius;
- a terminal is eventually reached under explicit fairness, timeout,
  cancellation, and boundary-response assumptions.

No such general model-checker export is shipped today. Per-profile validation
also does not prove an application's global distributed behavior merely
because each service machine passes independently.

Formal analysis of the harness cannot prove:

- that a tool implementation matches its declaration;
- that a device applies an operation atomically;
- that telemetry reflects the actual forwarding plane;
- that an LLM response is true or useful;
- that a receipt contains enough information to compensate;
- that an irreversible external action can be undone.

Those obligations require implementation tests, protocol simulators, device
or digital-twin evidence, fault injection, and operational controls. The
runtime's receipt-presence checks and tool round-trip tests strengthen the
claim but do not turn an external network into a closed formal system. LLM
inference is one explicit source of variance; humans, REST services,
subprocesses, clocks, retries, and mutable external state are also
non-deterministic boundaries.

## Current implementation status

The architecture argument is broader than the product evidence. The statuses
below prevent a useful direction from being presented as a completed
autonomous-network platform.

| Capability | Evidence status | Current boundary |
|---|---|---|
| Profile-driven machine and tool vocabulary | Shipped runtime | The [agent-core runtime](../../agent-core/README.md) loads profile, machine, tool selection, declarations, and optional REST bindings. |
| Reference and declared-outcome validation | Shipped runtime | Startup validates references, transition keys, selected words, and declared emitted-signal routes; [`emits_from_stl_test.go`](../../agent-core/internal/tools/catalog/emits_from_stl_test.go) covers the outcome route. |
| Optional model boundary | Shipped runtime | Profiles can omit model words; the [host-side deploy use case](../../applications/catalog/docs/specs/use-cases/rel18.0-uc001-host-side-deploy.yaml) exercises a no-model workflow. |
| Process, endpoint, and workload composition | Shipped runtime; Shipped application | The [composition model](composition-model.md), [`selfinvoke_test.go`](../../agent-core/internal/tools/control/selfinvoke_test.go), and [`server_machine_request_behavior_test.go`](../../agent-core/internal/tools/rest/server_machine_request_behavior_test.go) cover runtime forms. The coding application's [`test-rel02.0-serving-contract.yaml`](../../applications/coding-agent/docs/specs/test-suites/test-rel02.0-serving-contract.yaml) binds the production serving paths to `mage integration:servingHealth` and `mage integration:servingRemote`. |
| Checkpoint history and receipt reversal | Shipped runtime | [`checkpoint_rollback_test.go`](../../agent-core/internal/tools/lifecycle/checkpoint_rollback_test.go) and [`receipt_rollback_test.go`](../../agent-core/internal/tools/lifecycle/receipt_rollback_test.go) cover orchestration and receipts; [`dolt_checkpoint_revert_test.go`](../../agent-core/internal/runtime/checkpoint/dolt/dolt_checkpoint_revert_test.go) and the catalog's [`lifecycle_history_rollback_test.go`](../../applications/catalog/conformance/lifecycle_history_rollback_test.go) exercise persisted-state reversion. The two-part path is non-atomic, irreversible entries are skipped, and exact restoration depends on each tool and external state. |
| Approval | Conformance fixture | The [approval fixture](../../applications/catalog/testdata/conformance/lifecycle/approval/profile.yaml), [`main_lifecycle_test.go`](../../agent-core/cmd/agent/main_lifecycle_test.go), and [`checkpoint_lifecycle_test.go`](../../agent-core/cmd/agent/checkpoint_lifecycle_test.go) cover suspend and resume routing; authority notification, decider identity, rationale, discovery, and deployed approval APIs are not shipped. |
| Trusted transport configuration | Shipped runtime | REST client [target](../../agent-core/internal/tools/rest/client/client_target_test.go) and [request](../../agent-core/internal/tools/rest/client/client_request_test.go) tests cover configured authority surfaces; this is not a complete network authorization plane. |
| Network protocol capability library | Not demonstrated | No NETCONF, RESTCONF, gNMI, OpenConfig, Batfish, Ansible, Nornir, vendor matrix, or device-lab evidence is shipped. |
| Network Guardian and policy plane | Design intent; External requirement | No complete scope, rate, authority, outcome, policy-conflict, and decision-receipt subsystem is shipped. |
| Shared world model and intent lineage | Design intent; External requirement | No complete historized topology, validity, intent decomposition, policy basis, or decision-record service is shipped. |
| Application-level distributed verification | Not demonstrated | Per-machine checks do not prove concurrency, retries, partitions, fencing, global invariants, timing, or convergence. |
| High availability and disaster recovery | Not demonstrated | Checkpoints and Kubernetes packaging do not establish replicated persistence, ownership fencing, RPO/RTO, or regional failover. |
| Fleet scale and overload behavior | Not demonstrated | No representative device/event benchmark establishes serialization, fairness, backpressure, emergency capacity, or safe degradation. |
| Deterministic application closure | Shipped application | The coding application's [`test-rel03.0-deployment-profile-package.yaml`](../../applications/coding-agent/docs/specs/test-suites/test-rel03.0-deployment-profile-package.yaml), [`profiles_closure_test.go`](../../applications/coding-agent/magefiles/profiles_closure_test.go), and [`profiles_deployment_test.go`](../../applications/coding-agent/magefiles/profiles_deployment_test.go) cover exact role closures, reproducibility, and bounded package partitions. |
| Signed profile and tool admission | Not demonstrated | Complete signer, SBOM, vulnerability, and anti-rollback admission is not one shipped contract. |
| Proven device rollback | Not demonstrated | Receipt semantics are available; no physical or simulated network-device campaign proves exact or compensating restoration. |
| Runtime role or specification synthesis | Research horizon | The repository does not generate and admit novel network roles or sound acceptance criteria for capabilities novel in kind. |

Configuration-only behavior changes avoid a binary rebuild. They do not avoid
review, testing, admission, rollout, compatibility, or recovery. Likewise,
agent-level sequencing is data, while engine code, tool implementations,
adapters, policy systems, and bounded logic inside tools remain code.

## Conceptual provenance and qualifications

The autonomous-network requirement model draws on the
`self-evolving-networks` reference architecture. This repository records its
conceptual provenance in
[`applications/docs/VISION.yaml`](../../applications/docs/VISION.yaml):

- repository:
  `https://gitlabe1.ext.net.nokia.com/architecture/self-evolving-networks.git`
- pinned revision: `779a654fcfd3e5418991cb154a6cb3cc894be6ae`
- exact source paths recorded for that revision:
  - `agentic-framework/reference-architecture/overview/01-introduction.md`
  - `agentic-framework/reference-architecture/overview/02-design-principles.md`
  - `agentic-framework/reference-architecture/agent-view/01-introduction.md`
  - `agentic-framework/reference-architecture/agent-view/02-agent-roles.md`
  - `agentic-framework/reference-architecture/agent-view/03-role-catalog.md`
  - `agentic-framework/reference-architecture/agent-view/04-activity-catalog.md`
  - `agentic-framework/reference-architecture/functional-view/02-functional-architecture.md`
  - `agentic-framework/self-evolving-vision/from-code-to-configuration/01-introduction.md`
  - `agentic-framework/self-evolving-vision/from-code-to-configuration/04-integration-without-integration.md`

A later uncommitted architecture working tree was used to discover candidate
requirements and open questions. It is not cited or treated as reproducible
evidence. Only the pinned paths above constitute the recorded conceptual
provenance.

The stakeholder requirements, Network Action Machine contract, implementation
status assessment, and runtime-neutral backend strategy are deductions in this
guide, not claims imported from that architecture.
Reference roles are conceptual interfaces, not one-agent-per-role deployment
requirements. Logical intent-to-actuation and observation-to-feedback flows do
not prove real-time behavior, liveness, convergence, or distributed
consistency. The source is conceptual provenance, while this repository's
specifications and executable evidence remain authoritative for implementation
claims. Repository builds do not depend on a developer-local architecture
checkout.

## Choosing and combining systems

Use an agent SDK such as ADK when the main problem is conversational or
multimodal interaction, dynamic delegation, broad model and tool integration,
or alignment with its managed platform.

Use a durable workflow system when the main problem is long-lived distributed
transactions, high-scale scheduling, or recovery across many independently
deployed workers, and writing workflow and compensation code is acceptable.

Use statecharts or a service state language when inspectable deterministic
control flow is sufficient and agent-specific tool authority, model boundaries,
effect receipts, and intent provenance are unnecessary.

Use conventional network automation when the workflow is known, the device and
controller integrations already exist, and runtime generation adds no value.
Inventory, protocol semantics, and tested network operations remain necessary
whichever orchestration layer is selected.

Use the Declarative Agents contract when the machine itself must be a
reviewable deployment artifact, LLM and non-LLM components must share one
operational model, declared outcomes must be checked before execution, and
tool effects need common evidence and reversal semantics. Use the Go reference
runtime when its profile-only deployment, footprint, self-hosting, or placement
properties matter. Prefer a mature durable backend when distributed scheduling,
high availability, and operational tooling dominate and the contract can be
preserved through compilation.

The systems can be layered. An ADK or OpenAI agent can provide a northbound
assistant that turns a conversation into a typed request. A declarative
machine can admit and execute that request through policy, approval, actuation,
verification, and compensation. Temporal or Step Functions can host that
machine or provide a wider durable business process. The boundary must preserve
one rule: the conversational layer proposes; the authoritative workflow admits
and executes.

This guide is related to
[GH-2469](https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2469),
which reframes the application DSL as a paper. It does not set that paper's
outline or related-work claims.

## Sources

Repository evidence:

- [Composition model](composition-model.md)
- [Machine Interpreter](../../design-patterns/02-machine-interpreter.md)
- [Phase-Scoped Toolset](../../design-patterns/05-phase-scoped-toolset.md)
- [Bidirectional Log](../../design-patterns/07-bidirectional-log.md)
- [Boundary Tool](../../design-patterns/09-boundary-tool.md)
- [Approval Gate](../../design-patterns/10-approval-gate.md)
- [Host-side deploy use case](../../applications/catalog/docs/specs/use-cases/rel18.0-uc001-host-side-deploy.yaml)

Framework and workflow-system documentation:

- Google ADK: [agents](https://google.github.io/adk-docs/agents/),
  [ADK 2.0](https://google.github.io/adk-docs/2.0/),
  [agent configuration](https://google.github.io/adk-docs/agents/config/),
  [runtime](https://google.github.io/adk-docs/runtime/),
  [deployment](https://google.github.io/adk-docs/deploy/), and
  [Restate integration](https://google.github.io/adk-docs/integrations/restate/)
- LangGraph: [overview](https://docs.langchain.com/oss/python/langgraph/overview),
  [persistence](https://docs.langchain.com/oss/python/langgraph/persistence),
  [functional API](https://docs.langchain.com/oss/python/langgraph/functional-api),
  [fault tolerance](https://docs.langchain.com/oss/python/langgraph/fault-tolerance),
  and [interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts)
- Microsoft Agent Framework:
  [overview](https://learn.microsoft.com/en-us/agent-framework/overview/),
  [workflows](https://learn.microsoft.com/en-us/agent-framework/workflows/),
  [checkpoints](https://learn.microsoft.com/en-us/agent-framework/workflows/checkpoints),
  [functional workflows](https://learn.microsoft.com/en-us/agent-framework/concepts/workflows/functional),
  [Durable Extension](https://learn.microsoft.com/en-us/agent-framework/integrations/durable-extension),
  and [AutoGen migration](https://learn.microsoft.com/en-us/agent-framework/migration-guide/from-autogen/)
- OpenAI Agents SDK:
  [overview](https://openai.github.io/openai-agents-python/),
  [models](https://openai.github.io/openai-agents-python/models/),
  [sessions](https://openai.github.io/openai-agents-python/sessions/),
  [human in the loop](https://openai.github.io/openai-agents-python/human_in_the_loop/),
  and [running agents](https://openai.github.io/openai-agents-python/running_agents/)
- CrewAI: [Flows](https://docs.crewai.com/edge/en/concepts/flows),
  [production architecture](https://docs.crewai.com/edge/en/concepts/production-architecture),
  [checkpointing](https://docs.crewai.com/edge/en/concepts/checkpointing),
  and [human feedback](https://docs.crewai.com/edge/en/learn/human-feedback-in-flows)
- Temporal: [Workflow execution](https://docs.temporal.io/workflow-execution),
  [approval pattern](https://docs.temporal.io/design-patterns/approval), and
  [Saga pattern](https://docs.temporal.io/design-patterns/saga-pattern), plus
  [safe deployments](https://docs.temporal.io/develop/safe-deployments)
- AWS Step Functions:
  [workflow development and definition validation](https://docs.aws.amazon.com/step-functions/latest/dg/developing-workflows.html),
  [error handling](https://docs.aws.amazon.com/step-functions/latest/dg/concepts-error-handling.html),
  [service integration patterns](https://docs.aws.amazon.com/step-functions/latest/dg/connect-to-resource.html)
  and [human approval](https://docs.aws.amazon.com/step-functions/latest/dg/tutorial-human-approval.html)
