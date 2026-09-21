<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Agentic framework comparison for network applications

This guide compares Declarative Agents with agent frameworks and workflow
systems that can participate in a network application. It does not rank the
projects. They choose different units of authorship and different places to
enforce control.

The comparison was reviewed on 2026-09-21 against the official documentation
linked under [Sources](#sources). Framework behavior and hosted-product
features change; the links, rather than a remembered version, are the evidence
for each claim.

## Claim vocabulary

The comparison uses four terms:

- **Native** means the documented runtime or SDK supplies the capability.
- **Extension** means an officially documented integration supplies it.
- **Application** means the application can implement it, but the framework
  does not define the common contract being compared.
- **Not core** means the capability is outside the framework's documented
  center. It does not mean that an integration is impossible.

Persistence is not rollback. Persistence reconstructs execution state after a
pause or failure. Rollback restores prior persisted state and reverses or
compensates for effects in external systems. A framework can provide the first
without the second.

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

The table summarizes the architectural center of each system. Details and
qualifications follow it.

| System | Authored control flow | Non-LLM workflow | Static checks | Durable pause/resume | Approval | External-effect rollback |
|---|---|---|---|---|---|---|
| Declarative Agents | YAML finite-state machine and tool contracts | Native | Machine reachability and declared tool-outcome coverage | Native when a checkpoint backend is configured | Native suspend/resume fixture; deployed authority service is not shipped | Native receipt walk and persisted-state revert, limited by each tool's declared tier |
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
| Declarative Agents | Process child, request endpoint, or standalone Kubernetes workload using one runtime image | Machine phase, selected tools, declared signals, budgets, and tool side-effect contract | Transition and GenAI spans; provider dialects are profile data |
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

## Why the distinction matters in a network

A network change has a control-plane consequence even when an LLM proposed
it. The harness must treat generated text as untrusted input and retain
authority over target selection and mutation.

A controlled change can use the following phases:

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

The machine fixes that order. The model cannot skip pre-check or approval
because it cannot choose the next state. It can select only a tool visible in
the current phase, and `$tool` dispatch still resolves through the declared
registry. A request-supplied or model-authored endpoint should never become a
device target; a deterministic inventory and policy step must resolve the
target from trusted identifiers.

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

The runtime provides the grammar for sequencing these tools, but this
repository does not currently ship NETCONF, RESTCONF, gNMI, OpenConfig,
Batfish, Ansible, or Nornir integrations. Architectural suitability is not a
shipped network automation product. Adding a protocol adapter without the
contracts above would not close that gap.

## Static checks and formal verification

The shipped validator performs useful static checks before dispatch:

- every referenced state and tool resolves;
- every non-terminal state can reach a terminal state;
- the transition table is deterministic for its state/signal keys;
- every signal declared by a dispatchable tool is handled in the state that
  receives its result.

These checks reject malformed harnesses. They are not full formal verification.

A finite machine with a closed signal alphabet is suitable input to a model
checker. A future translation could verify invariants such as:

- no mutation state is reachable before policy and approval states;
- a model-selected action never reaches a tool outside the phase toolset;
- every partial-apply signal reaches compensation or an explicit
  operator-intervention terminal;
- the number of selected devices never exceeds a declared blast radius;
- every non-terminal execution reaches a terminal under stated fairness and
  boundary-response assumptions.

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
claim but do not turn an external network into a closed formal system.

## Choosing and combining systems

Use an agent SDK such as ADK when the main problem is conversational or
multimodal interaction, dynamic delegation, broad model and tool integration,
or alignment with its managed platform.

Use a durable workflow system when the main problem is long-lived distributed
transactions, high-scale scheduling, or recovery across many independently
deployed workers, and writing workflow and compensation code is acceptable.

Use Declarative Agents when the machine itself must be a reviewable deployment
artifact, LLM and non-LLM components must share one grammar, declared outcomes
must be checked before execution, and tool effects need a common reversal
contract.

The systems can be layered. An ADK or OpenAI agent can provide a northbound
assistant that turns a conversation into a typed request. A declarative
service can validate and execute that request through policy, approval,
actuation, verification, and compensation. Temporal or Step Functions can
provide a wider durable business process around either service. The boundary
must preserve one rule: the conversational layer proposes; the authoritative
workflow admits and executes.

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
  [service integration patterns](https://docs.aws.amazon.com/step-functions/latest/dg/connect-to-resource.html)
  and [human approval](https://docs.aws.amazon.com/step-functions/latest/dg/tutorial-human-approval.html)
