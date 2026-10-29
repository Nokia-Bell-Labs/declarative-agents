---
id: sm-agent-role-realizations
title: Agent Role Realizations and Bindings
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Separate reference-architecture roles from repository actors and profiles.
A canonical role is an interface or trait: it defines a reusable semantic
responsibility and may be assumed by many actors. A realization is a
configured actor or profile that binds one or more roles for a domain,
execution scope, deployment form, and configuration. Repeated bindings never
create another canonical role definition.

<!-- field: ontology -->
## Ontology

<!-- field: canonical_role -->
### Canonical role

A stable reference-architecture interface or trait. It is not a globally
unique deployed agent, process, profile, directory, or binary.

<!-- field: canonical_sub_role -->
### Canonical sub role

A semantic specialization whose parent is exactly one canonical role in the
pinned source. Sub-role bindings refine, but do not replace, a role binding.

<!-- field: actor -->
### Actor

A purposeful runtime identity. An actor may bind many roles, and one role may
be bound by many actors.

<!-- field: profile -->
### Profile

A loadable concrete role realization or a descriptive multi-role actor.
Profile names identify configured programs, not additions to the canonical
role catalog.

<!-- field: realization -->
### Realization

The binding of an actor/profile to roles in a stated domain and execution
scope. Deployment and configuration variants are facets of that realization.

<!-- field: wrapper -->
### Wrapper

A profile that selects a canonical machine or closure while adding
application-owned endpoints, configuration, serving, or packaging.

<!-- field: fixture -->
### Fixture

Test-only data or a bounded profile used to exercise another subject.
A fixture is classified but excluded from canonical role coverage.

<!-- field: adapter -->
### Adapter

Infrastructure-facing configuration that translates a runtime or protocol
boundary. An adapter is not a role merely because its name resembles one.

<!-- field: alias -->
### Alias

A historical or compatibility identity that resolves to a realization. An
alias cannot legitimize an incompatible use of a bare canonical role name.

<!-- field: naming_grammar -->
## Naming grammar

<!-- field: form -->
### Form

<context-or-purpose>-<primary-role>[-<deployment-or-configuration-form>]

<!-- field: identifier_style -->
### Identifier style

lower-kebab-case

<!-- field: rules -->
### Rules

- Context or purpose precedes the primary canonical role.
- Deployment, request, session, server, workspace, and model qualifiers follow the primary role.
- A generic reusable realization may use the bare canonical role name.
- A semantic specialization uses a canonical sub-role binding; it is not inferred from a deployment or model suffix.
- A descriptive multi-role actor keeps its actor name and declares role bindings separately.
- Scenario scope belongs in a scenario path or configuration unless it changes the reusable actor responsibility.

<!-- field: examples | form: "tree" -->
### Examples

- generic: critic
- contextual: coding-critic
- deployment: coding-critic-server
- configuration: executor-qwen27b
- scope: critic-workspace
- multi_role_actor: chatbot

<!-- field: canonical_source -->
## Canonical source

<!-- field: authority -->
### Authority

agentic-framework L4 reference architecture Agent View

<!-- field: repository -->
### Repository

https://gitlabe1.ext.net.nokia.com/architecture/self-evolving-networks.git

<!-- field: revision -->
### Revision

779a654fcfd3e5418991cb154a6cb3cc894be6ae

<!-- field: retrieved -->
### Retrieved

2026-07-27

<!-- field: paths | form: "tree" -->
### Paths

- role_catalog: agentic-framework/reference-architecture/data/agent-view-roles.yaml
- narrative:
  1. agentic-framework/reference-architecture/agent-view/01-introduction.md
  1. agentic-framework/reference-architecture/agent-view/02-agent-roles.md
  1. agentic-framework/reference-architecture/agent-view/03-role-catalog.md
  1. agentic-framework/reference-architecture/agent-view/04-activity-catalog.md

<!-- field: ci_policy -->
### CI policy

The role and sub-role IDs below are the pinned in-repository snapshot used by
validation. CI must not read a developer-local checkout or absolute path.

<!-- field: source_notes -->
### Source notes

- The machine-readable catalog defines 14 roles and 40 sub-roles.
- The source contains prose count and alias drift; IDs and parent links in the pinned role catalog take precedence.
- Malformed indentation folds service-composer's simulate-impact item into its parsed parent scalar; the surrounding role entry and narrative establish assembler as the intended parent.
- Malformed indentation omits the critic activities key; the role ID and Critic sub-role parents remain authoritative.
- Factory Manager is prose drift for canonical Creator; AgentFactory and AgentManager are deployment realizations, not role IDs.

<!-- field: canonical_roles -->
## Canonical roles

| id | name |
| --- | --- |
| planner | Planner |
| assembler | Assembler |
| connector | Connector |
| executor | Executor |
| knowledge-manager | Knowledge Manager |
| coordinator | Coordinator |
| monitor | Monitor |
| analyst | Analyst |
| guardian | Guardian |
| creator | Creator |
| critic | Critic |
| collaborator | Collaborator |
| agent-owner | Agent Owner |
| intent-owner | Intent Owner |

<!-- field: canonical_sub_roles -->
## Canonical sub roles

| id | parent |
| --- | --- |
| intent-handler | planner |
| intent-decomposer | planner |
| resource-planner | planner |
| service-composer | assembler |
| resource-integrator | assembler |
| domain-binder | connector |
| relationship-manager | connector |
| protocol-adapter | connector |
| workflow-orchestrator | executor |
| change-manager | executor |
| actuation-agent | executor |
| ontology-manager | knowledge-manager |
| policy-architect | knowledge-manager |
| recommendation-generator | knowledge-manager |
| workflow-synchronizer | coordinator |
| conflict-resolver | coordinator |
| consensus-facilitator | coordinator |
| telemetry-collector | monitor |
| fleet-observer | monitor |
| anomaly-detector | monitor |
| assurance-reporter | monitor |
| predictive-modeler | analyst |
| root-cause-analyst | analyst |
| policy-enforcer | guardian |
| threat-analyst | guardian |
| incident-responder | guardian |
| template-selector | creator |
| instance-manager | creator |
| resource-allocator | creator |
| output-evaluator | critic |
| feedback-generator | critic |
| sandbox-validator | critic |
| knowledge-discovery-facilitator | collaborator |
| negotiator | collaborator |
| collective-intelligence-enabler | collaborator |
| decentralized-operations-coordination | collaborator |
| data-and-telemetry-steward | collaborator |
| agent-lifecycle-manager | agent-owner |
| agent-registrar | agent-owner |
| agent-custodian | agent-owner |
| agent-retirer | agent-owner |

<!-- field: classifications | form: "keyed table" -->
## Classifications

| id | counts_toward_role_coverage | description |
| --- | --- | --- |
| role_realization | true | A production or reusable actor with justified canonical bindings. |
| descriptive_multi_role_actor | true | An actor whose stable identity is clearer than any one primary role. |
| wrapper | false | An application or serving wrapper that inherits a canonical realization. |
| configuration_variant | false | A model or configuration choice over one realization. |
| human_interface | false | A human boundary that launches or observes role realizations. |
| test_harness | false | A scenario composer or test runner. |
| mock | false | A scripted test double; the source architecture has no equivalent role. |
| fixture | false | Test-only profile or data. |
| infrastructure_adapter | false | Runtime, protocol, serving, or lifecycle boundary adapter. |
| workflow_machine | false | A workflow machine of the scrum-master application, whose states are a process skill's steps, whose tools are its words, and whose roles live in the application's own corpus rather than this role model. |

<!-- field: bindings -->
## Bindings

<!-- field: catalog -->
### Catalog

<!-- catalog_entry: critic -->
#### critic

<!-- field: profile -->
##### Profile

applications/catalog/agents/critic/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

critic

<!-- field: sub_roles -->
##### Sub roles

- output-evaluator
- feedback-generator

<!-- field: domain -->
##### Domain

evaluation

<!-- field: execution_scope -->
##### Execution scope

session

<!-- field: components -->
##### Components

| name | scope | inherits |
| --- | --- | --- |
| critic-session | session | critic |
| critic-point | point | critic |

<!-- catalog_entry: critic-workspace -->
#### critic-workspace

<!-- field: profile -->
##### Profile

applications/catalog/agents/critic/profile-workspace.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

critic

<!-- field: domain -->
##### Domain

evaluation

<!-- field: execution_scope -->
##### Execution scope

workspace

<!-- catalog_entry: rig-doctor -->
#### rig-doctor

<!-- field: profile -->
##### Profile

applications/catalog/agents/rig-doctor/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

analyst

<!-- field: sub_roles -->
##### Sub roles

- root-cause-analyst

<!-- field: domain -->
##### Domain

operations

<!-- field: execution_scope -->
##### Execution scope

workspace

<!-- catalog_entry: executor -->
#### executor

<!-- field: profile -->
##### Profile

applications/catalog/agents/executor/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

executor

<!-- field: sub_roles -->
##### Sub roles

- change-manager
- actuation-agent

<!-- field: domain -->
##### Domain

coding

<!-- field: execution_scope -->
##### Execution scope

workspace

<!-- catalog_entry: executor-qwen35b -->
#### executor-qwen35b

<!-- field: profile -->
##### Profile

applications/catalog/agents/executor/profile-qwen35b.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

executor

<!-- field: configuration | form: "tree" -->
##### Configuration

- model: qwen35b

<!-- catalog_entry: executor-qwen27b -->
#### executor-qwen27b

<!-- field: profile -->
##### Profile

applications/catalog/agents/executor/profile-qwen27b.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

executor

<!-- field: configuration | form: "tree" -->
##### Configuration

- model: qwen27b

<!-- catalog_entry: planner -->
#### planner

<!-- field: profile -->
##### Profile

applications/catalog/agents/planner/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

planner

<!-- field: sub_roles -->
##### Sub roles

- intent-decomposer
- resource-planner

<!-- field: domain -->
##### Domain

coding

<!-- field: execution_scope -->
##### Execution scope

requirement-graph

<!-- catalog_entry: specification-critic -->
#### specification-critic

<!-- field: profile -->
##### Profile

applications/catalog/agents/specification-critic/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

critic

<!-- field: sub_roles -->
##### Sub roles

- output-evaluator

<!-- field: domain -->
##### Domain

specification

<!-- field: execution_scope -->
##### Execution scope

corpus

<!-- field: responsibility_justification -->
##### Responsibility justification

The profile evaluates specification corpus output and reports findings.
It does not enforce policy or control access, so it has no Guardian /
Policy Enforcer binding.

<!-- catalog_entry: specification-critic-audit -->
#### specification-critic-audit

<!-- field: profile -->
##### Profile

applications/catalog/agents/specification-critic/audit-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

specification-critic

<!-- field: execution_scope -->
##### Execution scope

repository-audit

<!-- catalog_entry: specification-critic-corpus-audit -->
#### specification-critic-corpus-audit

<!-- field: profile -->
##### Profile

applications/catalog/agents/specification-critic/corpus-audit-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

specification-critic

<!-- field: execution_scope -->
##### Execution scope

corpus-audit

<!-- catalog_entry: jurist -->
#### jurist

<!-- field: profile -->
##### Profile

applications/catalog/agents/jurist/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

specification-critic

<!-- field: domain -->
##### Domain

specification

<!-- field: execution_scope -->
##### Execution scope

corpus

<!-- catalog_entry: documentation-curator -->
#### documentation-curator

<!-- field: profile -->
##### Profile

applications/catalog/agents/knowledge-manager/documentation-curator/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

knowledge-manager

<!-- field: sub_roles -->
##### Sub roles

- ontology-manager
- recommendation-generator

<!-- field: domain -->
##### Domain

documentation

<!-- field: execution_scope -->
##### Execution scope

server

<!-- catalog_entry: documentation-curator-request -->
#### documentation-curator-request

<!-- field: profile -->
##### Profile

applications/catalog/agents/knowledge-manager/documentation-curator/request-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

documentation-curator

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: corpus-ingest -->
#### corpus-ingest

<!-- field: profile -->
##### Profile

applications/catalog/agents/knowledge-manager/corpus-ingest/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

knowledge-manager

<!-- field: sub_roles -->
##### Sub roles

- ontology-manager

<!-- field: domain -->
##### Domain

retrieval-corpus

<!-- field: execution_scope -->
##### Execution scope

batch

<!-- catalog_entry: corpus-reader -->
#### corpus-reader

<!-- field: profile -->
##### Profile

applications/catalog/agents/knowledge-manager/corpus-reader/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

knowledge-manager

<!-- field: sub_roles -->
##### Sub roles

- recommendation-generator

<!-- field: domain -->
##### Domain

retrieval-corpus

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: scenario-critic -->
#### scenario-critic

<!-- field: profile -->
##### Profile

applications/catalog/agents/scenario-critic/profile.yaml

<!-- field: classification -->
##### Classification

test_harness

<!-- field: primary_role -->
##### Primary role

critic

<!-- field: sub_roles -->
##### Sub roles

- sandbox-validator
- output-evaluator

<!-- field: domain -->
##### Domain

conformance

<!-- field: execution_scope -->
##### Execution scope

scenario

<!-- catalog_entry: assembler -->
#### assembler

<!-- field: profile -->
##### Profile

applications/catalog/agents/assembler/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

scenario-critic

<!-- field: domain -->
##### Domain

conformance

<!-- field: execution_scope -->
##### Execution scope

scenario

<!-- field: naming_status -->
##### Naming status

migration_required_collision

<!-- catalog_entry: bench -->
#### bench

<!-- field: profile -->
##### Profile

applications/catalog/agents/bench/profile.yaml

<!-- field: classification -->
##### Classification

human_interface

<!-- field: domain -->
##### Domain

evaluation

<!-- field: execution_scope -->
##### Execution scope

server

<!-- field: launches -->
##### Launches

critic

<!-- catalog_entry: bench-request -->
#### bench-request

<!-- field: profile -->
##### Profile

applications/catalog/agents/bench/request-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

bench

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: bench-experiments -->
#### bench-experiments

<!-- field: profile -->
##### Profile

applications/catalog/agents/bench/experiments-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

bench

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: mock -->
#### mock

<!-- field: profile -->
##### Profile

applications/catalog/agents/mock/profile.yaml

<!-- field: classification -->
##### Classification

mock

<!-- field: domain -->
##### Domain

conformance

<!-- field: execution_scope -->
##### Execution scope

scenario

<!-- catalog_entry: runtime-state-reader -->
#### runtime-state-reader

<!-- field: profile -->
##### Profile

applications/catalog/agents/runtime-state-reader/profile.yaml

<!-- field: classification -->
##### Classification

infrastructure_adapter

<!-- field: domain -->
##### Domain

runtime-state

<!-- field: execution_scope -->
##### Execution scope

server

<!-- field: adjacent_role -->
##### Adjacent role

monitor

<!-- catalog_entry: monitor -->
#### monitor

<!-- field: profile -->
##### Profile

applications/catalog/agents/monitor/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

runtime-state-reader

<!-- field: domain -->
##### Domain

runtime-state

<!-- field: execution_scope -->
##### Execution scope

server

<!-- field: naming_status -->
##### Naming status

migration_required_collision

<!-- catalog_entry: collector -->
#### collector

<!-- field: profile -->
##### Profile

applications/catalog/agents/collector/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

monitor

<!-- field: sub_roles -->
##### Sub roles

- telemetry-collector

<!-- field: domain -->
##### Domain

observability

<!-- field: execution_scope -->
##### Execution scope

server

<!-- catalog_entry: deployment-applier -->
#### deployment-applier

<!-- field: profile -->
##### Profile

applications/catalog/agents/applier/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

executor

<!-- field: sub_roles -->
##### Sub roles

- actuation-agent
- change-manager

<!-- field: domain -->
##### Domain

deployment

<!-- field: execution_scope -->
##### Execution scope

service-and-request

<!-- catalog_entry: applier-apply-request -->
#### applier-apply-request

<!-- field: profile -->
##### Profile

applications/catalog/agents/applier/apply-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: applier-rollout-request -->
#### applier-rollout-request

<!-- field: profile -->
##### Profile

applications/catalog/agents/applier/rollout-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: execution_scope -->
##### Execution scope

rollout-read

<!-- catalog_entry: applier-deploy-request -->
#### applier-deploy-request

<!-- field: profile -->
##### Profile

applications/catalog/agents/applier/deploy-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: applier-undeploy-request -->
#### applier-undeploy-request

<!-- field: profile -->
##### Profile

applications/catalog/agents/applier/undeploy-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: execution_scope -->
##### Execution scope

request

<!-- catalog_entry: lifecycle-exit -->
#### lifecycle-exit

<!-- field: profile -->
##### Profile

applications/catalog/agents/lifecycle-exit/profile.yaml

<!-- field: classification -->
##### Classification

infrastructure_adapter

<!-- field: domain -->
##### Domain

lifecycle

<!-- field: execution_scope -->
##### Execution scope

client

<!-- catalog_entry: application-purge -->
#### application-purge

<!-- field: profile -->
##### Profile

applications/catalog/agents/application-purge/profile.yaml

<!-- field: classification -->
##### Classification

infrastructure_adapter

<!-- field: domain -->
##### Domain

retention

<!-- field: execution_scope -->
##### Execution scope

host-request

<!-- field: authority_boundary -->
##### Authority boundary

Deletes only the application bucket prefix rendered by apprig after exact application-specific confirmation; it chooses no storage identity and cannot mutate collector query behavior.

<!-- field: coding_agent -->
### Coding agent

<!-- coding_agent_entry: coding-planner-server -->
#### coding-planner-server

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/planner/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

planner

<!-- field: deployment_form -->
##### Deployment form

server

<!-- coding_agent_entry: coding-planner-request -->
#### coding-planner-request

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/planner/request-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

coding-planner-server

<!-- field: execution_scope -->
##### Execution scope

request

<!-- coding_agent_entry: coding-executor-server -->
#### coding-executor-server

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/executor/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

executor

<!-- field: deployment_form -->
##### Deployment form

server

<!-- coding_agent_entry: coding-critic-server -->
#### coding-critic-server

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/critic/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

critic-workspace

<!-- field: deployment_form -->
##### Deployment form

server

<!-- coding_agent_entry: applier -->
#### applier

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/applier/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/profile.yaml

<!-- field: domain -->
##### Domain

deployment

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

applier

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Validate a decided values patch against the coding chart schema, apply it, verify the planner, executor, and critic rollouts, and roll the release back on a stall.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Executor / Actuation Agent and Change Manager.

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain applier as a descriptive actor and Helm workload name; Applier is not a canonical role.

<!-- field: authority_boundary -->
###### Authority boundary

An authorized operator or CI caller supplies the decided patch; the applier authors no values and receives no runtime transport authority, and the serving roles hold no deployment CLI.

<!-- field: evidence -->
###### Evidence

- applications/catalog/agents/applier/apply-machine.yaml
- applications/coding-agent/agents/applier/exec-declarations.yaml
- applications/coding-agent/helm/templates/applier.yaml

<!-- field: migration_impact -->
###### Migration impact

GH-1420 replaced the local machine and shared ToolDef copies with a wrapper over the canonical catalog applier while preserving local rollout commands and authority.

<!-- coding_agent_entry: coding-applier-apply-request -->
#### coding-applier-apply-request

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/applier/apply-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

applier-apply-request

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/apply-profile.yaml

<!-- field: execution_scope -->
##### Execution scope

request

<!-- coding_agent_entry: coding-applier-rollout-request -->
#### coding-applier-rollout-request

<!-- field: profile -->
##### Profile

applications/coding-agent/agents/applier/rollout-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

applier-rollout-request

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/rollout-profile.yaml

<!-- field: execution_scope -->
##### Execution scope

rollout-read

<!-- field: chatbot_mesh -->
### Chatbot mesh

<!-- chatbot_mesh_entry: chatbot -->
#### chatbot

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/chatbot/profile.yaml

<!-- field: classification -->
##### Classification

descriptive_multi_role_actor

<!-- field: roles | form: "typed table" -->
##### Roles

| role | sub_roles |
| --- | --- |
| planner | [intent-handler] |
| executor | [workflow-orchestrator] |
| knowledge-manager | [recommendation-generator] |

<!-- field: domain -->
##### Domain

conversational-rag

<!-- field: execution_scope -->
##### Execution scope

request

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

chatbot

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Select sources and a model tier, embed once, retrieve, compose a grounded prompt, answer, and host the browser-facing request boundary.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

- Planner / Intent Handler
- Executor / Workflow Orchestrator
- Knowledge Manager / Recommendation Generator

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain chatbot as a descriptive multi-role actor identity.

<!-- field: authority_boundary -->
###### Authority boundary

Selects only declared model words and trusted RAG topology entries; it has no deployment or lifecycle authority.

<!-- field: evidence -->
###### Evidence

- applications/chatbot-mesh/agents/chatbot/machine.yaml
- applications/chatbot-mesh/agents/chatbot/request-machine.yaml
- applications/chatbot-mesh/docs/specs/software-requirements/srd002-chatbot-agent.md

<!-- field: migration_impact -->
###### Migration impact

None; role bindings remain separate from the actor and Helm workload name.

<!-- chatbot_mesh_entry: chatbot-chat-request -->
#### chatbot-chat-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/chatbot/request-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

chatbot

<!-- field: execution_scope -->
##### Execution scope

request

<!-- chatbot_mesh_entry: rag-server -->
#### rag-server

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/rag-server/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

knowledge-manager

<!-- field: sub_roles -->
##### Sub roles

- recommendation-generator

<!-- field: domain -->
##### Domain

retrieval

<!-- field: execution_scope -->
##### Execution scope

server

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

rag-server

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Serve one configured corpus through a persistent vector-in retrieval boundary and return ranked chunks with embedding metadata.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Knowledge Manager / Recommendation Generator

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain rag-server as a descriptive serving identity.

<!-- field: authority_boundary -->
###### Authority boundary

Reads one declared Chroma collection and does not ingest, answer, choose transport authority, or manage another agent.

<!-- field: evidence -->
###### Evidence

- applications/chatbot-mesh/agents/rag-server/profile.yaml
- applications/chatbot-mesh/agents/rag-server/request-machine.yaml
- applications/chatbot-mesh/docs/specs/software-requirements/srd001-rag-server-agent.md

<!-- field: migration_impact -->
###### Migration impact

None; each values-named Helm RAG unit is a deployment instance of this realization, not a canonical role name.

<!-- chatbot_mesh_entry: corpus-ingest -->
#### corpus-ingest

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/corpus-ingest/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

corpus-ingest

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/knowledge-manager/corpus-ingest/profile.yaml

<!-- field: domain -->
##### Domain

chatbot-mesh

<!-- field: execution_scope -->
##### Execution scope

child-run

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

corpus-ingest

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Parameterize the catalog corpus-ingest machine with mesh-owned Chroma REST configuration and package its resolved closure.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Inherits Knowledge Manager / Ontology Manager from the catalog corpus-ingest realization.

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain corpus-ingest as a descriptive wrapper identity.

<!-- field: authority_boundary -->
###### Authority boundary

The mesh wrapper owns endpoints, model, collection, and packaging parameters; the catalog profile remains implementation owner.

<!-- field: evidence -->
###### Evidence

- applications/chatbot-mesh/agents/corpus-ingest/profile.yaml
- applications/catalog/agents/knowledge-manager/corpus-ingest/profile.yaml
- applications/chatbot-mesh/agents/creator/request-declarations.yaml

<!-- field: migration_impact -->
###### Migration impact

None; the wrapper has no independent Helm workload and runs as a creator-launched child profile.

<!-- chatbot_mesh_entry: provisioning-workflow-orchestrator -->
#### provisioning-workflow-orchestrator

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/provisioning-workflow-orchestrator/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

executor

<!-- field: sub_roles -->
##### Sub roles

- workflow-orchestrator

<!-- field: domain -->
##### Domain

provisioning

<!-- field: naming_status -->
##### Naming status

aligned_concrete_name

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

provisioning-workflow-orchestrator

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Sequence optional corpus ingestion followed by a values rollout through declared creator requests and aggregate the terminal result.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Executor / Workflow Orchestrator; explicitly not Coordinator.

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Use provisioning-workflow-orchestrator; implemented in GH-1077.

<!-- field: authority_boundary -->
###### Authority boundary

Holds no deployment API, child-process, conflict-resolution, negotiation, or consensus authority; the operator supplies desired values and the creator performs instance operations.

<!-- field: evidence -->
###### Evidence

- applications/chatbot-mesh/agents/provisioning-workflow-orchestrator/request-machine.yaml
- applications/chatbot-mesh/agents/provisioning-workflow-orchestrator/rest.yaml
- applications/chatbot-mesh/helm/templates/provisioning-workflow-orchestrator.yaml

<!-- field: migration_impact -->
###### Migration impact

GH-1077 migrated the profile directory/name, Helm workloads and policies, SRD004 and references, integrations, packaging closure, release artifacts, and compatibility metadata together.

<!-- chatbot_mesh_entry: creator -->
#### creator

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/creator/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: roles | form: "typed table" -->
##### Roles

| role | sub_roles |
| --- | --- |
| creator | [instance-manager] |
| agent-owner | [agent-lifecycle-manager] |

<!-- field: domain -->
##### Domain

agent-lifecycle

<!-- field: naming_status -->
##### Naming status

retained_compatible_canonical_name

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

creator

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Resolve and launch a corpus-ingest child, drive decided rollout operations, read resulting health and state, and own the deployment-API client boundary.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

- Creator / Instance Manager
- Agent Owner / Agent Lifecycle Manager

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain creator because instance realization is an implemented primary responsibility; record Agent Owner as an additional trait.

<!-- field: authority_boundary -->
###### Authority boundary

Acts on a decided operation and alone reaches the deployment API; it does not decide desired values or orchestrate the end-to-end provisioning workflow.

<!-- field: evidence -->
###### Evidence

- applications/chatbot-mesh/agents/creator/request-machine.yaml
- applications/chatbot-mesh/agents/creator/request-declarations.yaml
- applications/chatbot-mesh/helm/templates/creator.yaml

<!-- field: migration_impact -->
###### Migration impact

None; the actor and workload name remain semantically compatible with canonical Creator.

<!-- chatbot_mesh_entry: creator-request -->
#### creator-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/creator/request-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

creator

<!-- field: execution_scope -->
##### Execution scope

request

<!-- chatbot_mesh_entry: provisioning-workflow-orchestrator-request -->
#### provisioning-workflow-orchestrator-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/provisioning-workflow-orchestrator/request-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

provisioning-workflow-orchestrator

<!-- field: execution_scope -->
##### Execution scope

request

<!-- chatbot_mesh_entry: rag-server-request -->
#### rag-server-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/rag-server/request-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

rag-server

<!-- field: execution_scope -->
##### Execution scope

request

<!-- chatbot_mesh_entry: applier -->
#### applier

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/applier/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/profile.yaml

<!-- field: domain -->
##### Domain

deployment

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

applier

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Validate a decided values document, execute Helm and kubectl actuation, verify rollout state, and roll back failure.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Executor / Actuation Agent and Change Manager.

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain applier as a descriptive actor and Helm workload name; Applier is not a canonical role.

<!-- field: authority_boundary -->
###### Authority boundary

The creator is the only apply caller; the applier authors no desired state and receives no runtime transport authority.

<!-- field: evidence -->
###### Evidence

- applications/catalog/agents/applier/apply-machine.yaml
- applications/chatbot-mesh/agents/applier/exec-declarations.yaml
- applications/chatbot-mesh/helm/templates/applier.yaml

<!-- field: migration_impact -->
###### Migration impact

GH-1420 replaced the local machine and shared ToolDef copies with a wrapper over the canonical catalog applier while preserving the mesh state endpoint, rollout commands, and creator-only authority.

<!-- chatbot_mesh_entry: mesh-applier-apply-request -->
#### mesh-applier-apply-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/applier/apply-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

applier-apply-request

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/apply-profile.yaml

<!-- field: execution_scope -->
##### Execution scope

request

<!-- chatbot_mesh_entry: mesh-applier-rollout-request -->
#### mesh-applier-rollout-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/applier/rollout-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

applier-rollout-request

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/rollout-profile.yaml

<!-- field: execution_scope -->
##### Execution scope

rollout-read

<!-- chatbot_mesh_entry: mesh-applier-state-request -->
#### mesh-applier-state-request

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/applier/state-profile.yaml

<!-- field: classification -->
##### Classification

configuration_variant

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/profile.yaml

<!-- field: execution_scope -->
##### Execution scope

mesh-state-read

<!-- chatbot_mesh_entry: observer -->
#### observer

<!-- field: profile -->
##### Profile

applications/chatbot-mesh/agents/observer/profile.yaml

<!-- field: classification -->
##### Classification

role_realization

<!-- field: primary_role -->
##### Primary role

monitor

<!-- field: sub_roles -->
##### Sub roles

- fleet-observer

<!-- field: domain -->
##### Domain

observability

<!-- field: execution_scope -->
##### Execution scope

server

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

observer

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Discover mesh agent pods from the Kubernetes API and poll each agent's monitor surface to serve a fleet-level view.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Monitor / Fleet Observer.

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain observer as a descriptive actor name.

<!-- field: authority_boundary -->
###### Authority boundary

Read-only over kube pod listing and agent monitor surfaces; no lifecycle, deployment, configuration, or trace-intake authority.

<!-- field: evidence -->
###### Evidence

- applications/chatbot-mesh/agents/observer/machine.yaml
- applications/chatbot-mesh/agents/observer/declarations.yaml
- applications/chatbot-mesh/helm/templates/observer.yaml

<!-- field: migration_impact -->
###### Migration impact

None.

<!-- field: agent_architecture -->
### Agent architecture

<!-- agent_architecture_entry: applier -->
#### applier

<!-- field: profile -->
##### Profile

applications/agent-architecture/agents/applier/profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

deployment-applier

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/profile.yaml

<!-- field: domain -->
##### Domain

deployment

<!-- field: decision -->
##### Decision

<!-- field: current_actor -->
###### Current actor

applier

<!-- field: implemented_responsibility -->
###### Implemented responsibility

Validate a decided values patch against the agent-architecture chart schema, apply it, verify the collector rollout (the application's stable, k8s-ready server; the bounded documentation-curator recycles every control window and is not a durable rollout target), and roll the release back on a stall.

<!-- field: canonical_role_sub_role -->
###### Canonical role sub role

Executor / Actuation Agent and Change Manager.

<!-- field: retained_or_proposed_name -->
###### Retained or proposed name

Retain applier as a descriptive actor and Helm workload name; Applier is not a canonical role.

<!-- field: authority_boundary -->
###### Authority boundary

A standalone actuator; an authorized operator or CI caller supplies the decided patch, the applier authors no values and receives no runtime transport authority, and the curator and collector hold no deployment CLI.

<!-- field: evidence -->
###### Evidence

- applications/catalog/agents/applier/apply-machine.yaml
- applications/agent-architecture/agents/applier/exec-declarations.yaml
- applications/agent-architecture/helm/templates/applier.yaml

<!-- field: migration_impact -->
###### Migration impact

GH-1420 replaced the local machine and shared ToolDef copies with a wrapper over the canonical catalog applier while preserving collector-only rollout verification and local authority.

<!-- agent_architecture_entry: agent-architecture-applier-apply-request -->
#### agent-architecture-applier-apply-request

<!-- field: profile -->
##### Profile

applications/agent-architecture/agents/applier/apply-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

applier-apply-request

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/apply-profile.yaml

<!-- field: execution_scope -->
##### Execution scope

request

<!-- agent_architecture_entry: agent-architecture-applier-rollout-request -->
#### agent-architecture-applier-rollout-request

<!-- field: profile -->
##### Profile

applications/agent-architecture/agents/applier/rollout-profile.yaml

<!-- field: classification -->
##### Classification

wrapper

<!-- field: inherits -->
##### Inherits

applier-rollout-request

<!-- field: inheritance_target -->
##### Inheritance target

applications/catalog/agents/applier/rollout-profile.yaml

<!-- field: execution_scope -->
##### Execution scope

rollout-read

<!-- field: scrum_master -->
### Scrum master

<!-- scrum_master_entry: scrum-master-implement -->
#### scrum-master-implement

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/implement/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-merge -->
#### scrum-master-merge

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/merge/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-pop -->
#### scrum-master-pop

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/pop/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-plan-push -->
#### scrum-master-plan-push

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/plan-push/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-plan -->
#### scrum-master-plan

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/plan/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-reports-init -->
#### scrum-master-reports-init

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/reports-init/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-site -->
#### scrum-master-site

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/site/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-specify -->
#### scrum-master-specify

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/specify/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-tasks -->
#### scrum-master-tasks

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/tasks/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-sprint-end -->
#### scrum-master-sprint-end

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/sprint-end/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-draft -->
#### scrum-master-draft

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/draft/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-admit -->
#### scrum-master-admit

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/admit/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-aggregate -->
#### scrum-master-aggregate

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/aggregate/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-approve -->
#### scrum-master-approve

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/approve/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-comment -->
#### scrum-master-comment

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/comment/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-close -->
#### scrum-master-close

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/close/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-labels -->
#### scrum-master-labels

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/labels/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-peer-read -->
#### scrum-master-peer-read

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/peer-read/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-recur -->
#### scrum-master-recur

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/recur/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-release -->
#### scrum-master-release

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/release/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-status -->
#### scrum-master-status

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/status/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-status-issues -->
#### scrum-master-status-issues

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/status-issues/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-architecture-status -->
#### scrum-master-architecture-status

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/architecture-status/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-backlog-check -->
#### scrum-master-backlog-check

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/backlog-check/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-backlog-components -->
#### scrum-master-backlog-components

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/backlog-components/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-backlog-guide -->
#### scrum-master-backlog-guide

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/backlog-guide/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-backlog-list -->
#### scrum-master-backlog-list

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/backlog-list/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-backlog-template -->
#### scrum-master-backlog-template

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/backlog-template/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-design -->
#### scrum-master-design

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/design/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-doctor -->
#### scrum-master-doctor

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/doctor/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-goals-plan -->
#### scrum-master-goals-plan

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/goals-plan/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-goals-push -->
#### scrum-master-goals-push

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/goals-push/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-issue-file -->
#### scrum-master-issue-file

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/issue-file/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-issue-lint -->
#### scrum-master-issue-lint

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/issue-lint/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-issue-list -->
#### scrum-master-issue-list

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/issue-list/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-plan-backlog -->
#### scrum-master-plan-backlog

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/plan-backlog/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-plan-candidates -->
#### scrum-master-plan-candidates

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/plan-candidates/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-plan-write -->
#### scrum-master-plan-write

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/plan-write/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-release-gate -->
#### scrum-master-release-gate

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/release-gate/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-release-tag -->
#### scrum-master-release-tag

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/release-tag/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-reports-validate -->
#### scrum-master-reports-validate

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/reports-validate/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-sprint-end-facts -->
#### scrum-master-sprint-end-facts

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/sprint-end-facts/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-sprint-end-stories -->
#### scrum-master-sprint-end-stories

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/sprint-end-stories/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-tracker-export -->
#### scrum-master-tracker-export

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/tracker-export/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-tracker-migrate -->
#### scrum-master-tracker-migrate

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/tracker-migrate/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-tracker-sync -->
#### scrum-master-tracker-sync

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/tracker-sync/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-check -->
#### scrum-master-check

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/check/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-init -->
#### scrum-master-init

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/init/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-onboard -->
#### scrum-master-onboard

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/onboard/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-checkouts -->
#### scrum-master-checkouts

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/checkouts/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-clean-up -->
#### scrum-master-clean-up

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/clean-up/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-reports-collect -->
#### scrum-master-reports-collect

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/reports-collect/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-reports-path -->
#### scrum-master-reports-path

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/reports-path/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-reports-previous -->
#### scrum-master-reports-previous

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/reports-previous/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-status-report -->
#### scrum-master-status-report

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/status-report/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-status-rollup -->
#### scrum-master-status-rollup

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/status-rollup/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-upgrade -->
#### scrum-master-upgrade

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/upgrade/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-roll -->
#### scrum-master-roll

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/roll/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- scrum_master_entry: scrum-master-reconcile -->
#### scrum-master-reconcile

<!-- field: profile -->
##### Profile

applications/scrum-master/plugin/agents/reconcile/profile.yaml

<!-- field: classification -->
##### Classification

workflow_machine

<!-- field: deployment_realizations -->
## Deployment realizations

<!-- field: chatbot_mesh -->
### Chatbot mesh

<!-- field: naming_rule -->
#### Naming rule

Helm metadata, container, Service, component, telemetry, values, and
values-selected RAG unit names identify workloads or deployment instances.
They inherit the mounted profile's actor-role bindings but never define or
rename a canonical role.

<!-- field: profile_backed | form: "typed table" -->
#### Profile backed

| workload | actor | realizes | note | migration | condition |
| --- | --- | --- | --- | --- | --- |
| <release>-chatbot | chatbot | descriptive_multi_role_actor |  |  |  |
| <release>-<rag-unit-name> | rag-server | knowledge-manager/recommendation-generator | The values-owned unit name identifies a corpus instance, not a role. |  |  |
| <release>-provisioning-workflow-orchestrator | provisioning-workflow-orchestrator | executor/workflow-orchestrator |  | implemented-by-GH-1077 |  |
| <release>-creator | creator | creator/instance-manager and agent-owner/agent-lifecycle-manager |  |  |  |
| <release>-applier | applier | executor/actuation-agent |  |  |  |
| <release>-collector | collector | monitor/telemetry-collector |  |  | collector.implementation=agent |
| <release>-observer | observer | monitor/fleet-observer |  |  |  |

<!-- field: infrastructure_and_adapters | form: "typed table" -->
#### Infrastructure and adapters

| workload | classification | condition | purpose |
| --- | --- | --- | --- |
| <release>-collector | infrastructure_adapter | collector.implementation=contrib |  |
| <release>-collector-metrics | infrastructure_adapter |  | retained contrib metrics and Dolt scrape path |
| <release>-<rag-unit-name>-chroma | infrastructure_adapter |  | vector-store StatefulSet |
| <release>-ollama | infrastructure_adapter |  | model-serving StatefulSet |
| <release>-ollama-preload | infrastructure_adapter |  | model preload Job |
| <release>-dolt | infrastructure_adapter |  | checkpoint-store StatefulSet |

<!-- field: wrapper_only -->
#### Wrapper only

| actor | deployment | note |
| --- | --- | --- |
| corpus-ingest | creator-launched child run | No independent Helm workload; inherits the catalog Knowledge Manager binding. |

<!-- field: non_role_inventory | form: "tree" -->
## Non role inventory

- catalog_fixtures:
  - inheritance_rule: All listed profiles are fixtures and do not create role bindings.
  - profiles:
    1. applications/catalog/testdata/conformance/control/profile.yaml
    1. applications/catalog/testdata/conformance/lifecycle/approval/profile.yaml
    1. applications/catalog/testdata/conformance/lifecycle/history/profile.yaml
    1. applications/catalog/testdata/conformance/lifecycle/rollback/profile.yaml
    1. applications/catalog/testdata/conformance/rest/profile.yaml
    1. applications/catalog/testdata/conformance/rest/ollama-profile.yaml
    1. applications/catalog/testdata/conformance/rig-subject/profile.yaml
    1. applications/catalog/testdata/conformance/rig-subject/tests/broken/profile.yaml
    1. applications/catalog/testdata/conformance/rig-subject/tests/dep-failure/profile.yaml
    1. applications/catalog/testdata/conformance/rig-subject/tests/happy-path/profile.yaml
- mesh_fixtures:
  - inheritance_rule: Scenario critics bind Critic for their configured scope; the rig wrapper itself is a fixture.
  - profiles:
    1. - path: applications/chatbot-mesh/agents/chatbot/tests/single-turn/profile.yaml
       - actor: chatbot-turn-critic
       - role: critic
       - sub_roles:
         1. output-evaluator
         1. sandbox-validator
       - domain: chatbot-turn
       - execution_scope: single-turn-scenario
       - responsibility_justification: The critic verifies that an undeclared URL-shaped source cannot escape the isolated mock topology.
    1. - path: applications/chatbot-mesh/agents/chatbot/tests/degraded-rag/profile.yaml
       - actor: chatbot-turn-critic
       - role: critic
       - sub_roles:
         1. output-evaluator
       - domain: chatbot-turn
       - execution_scope: degraded-rag-scenario
    1. - path: applications/chatbot-mesh/agents/rag-server/tests/query/profile.yaml
       - actor: rag-query-critic
       - role: critic
       - sub_roles:
         1. output-evaluator
       - domain: rag-query
       - execution_scope: query-scenario
    1. - path: applications/chatbot-mesh/testdata/rig/profile.yaml
       - classification: fixture
       - inherits: scenario-critic

<!-- field: inheritance_and_override_rules -->
## Inheritance and override rules

- An inheriting wrapper or variant receives all role and sub-role bindings from its target.
- A variant may override domain, execution_scope, deployment_form, or configuration without redefining a role.
- An override of role or sub-role bindings must replace the inherited list explicitly and provide a responsibility justification.
- An inheritance chain must terminate at a non-inheriting realization and must not contain a cycle.
- A wrapper around a catalog profile keeps the catalog profile as canonical implementation owner.
- Same-named actors in different applications are distinct bindings; inheritance_target disambiguates cross-module reuse.

<!-- field: validation_rules -->
## Validation rules

- canonical_roles contains exactly 14 unique lower-kebab IDs.
- canonical_sub_roles contains exactly 40 unique IDs and every parent names a canonical role.
- Every role and sub-role binding must resolve against the pinned snapshot.
- Every shipped catalog profile and application-local production profile must be listed directly or inherit through a documented rule.
- Repeated role bindings and many-to-many actor-role bindings are valid.
- Bare canonical names are valid only for generic reusable realizations with compatible responsibility.
- Fixtures, mocks, human interfaces, wrappers, and infrastructure adapters are excluded from canonical role-coverage counts.
- A migration-required alias cannot be normalized as a valid canonical realization.
- Profile paths are repository-relative and must not depend on a developer-local absolute path.

<!-- field: migration_aliases -->
## Migration aliases

<!-- migration_alias: assembler -->
### assembler

<!-- field: path -->
#### Path

applications/catalog/agents/assembler/profile.yaml

<!-- field: status -->
#### Status

migration_compatibility

<!-- field: collision_with -->
#### Collision with

assembler

<!-- field: target_name -->
#### Target name

scenario-critic

<!-- field: binding | form: "tree" -->
#### Binding

- role: critic
- sub_roles:
  1. sandbox-validator
  1. output-evaluator

<!-- field: canonical_path -->
#### Canonical path

applications/catalog/agents/scenario-critic/profile.yaml

<!-- field: reason -->
#### Reason

The test rig performs safe testing and verdict reduction, not production service composition and resource integration.

<!-- field: supported_through -->
#### Supported through

applications/catalog/v0.*

<!-- field: removal -->
#### Removal

applications/catalog/v1

<!-- migration_alias: monitor -->
### monitor

<!-- field: path -->
#### Path

applications/catalog/agents/monitor/profile.yaml

<!-- field: status -->
#### Status

migration_compatibility

<!-- field: collision_with -->
#### Collision with

monitor

<!-- field: target_name -->
#### Target name

runtime-state-reader

<!-- field: classification -->
#### Classification

infrastructure_adapter

<!-- field: canonical_path -->
#### Canonical path

applications/catalog/agents/runtime-state-reader/profile.yaml

<!-- field: reason -->
#### Reason

The profile reads cached runtime state; it does not collect telemetry, detect anomalies, or report assurance.

<!-- field: supported_through -->
#### Supported through

applications/catalog/v0.*

<!-- field: removal -->
#### Removal

applications/catalog/v1

<!-- migration_alias: jurist -->
### jurist

<!-- field: path -->
#### Path

applications/catalog/agents/jurist/profile.yaml

<!-- field: status -->
#### Status

migration_compatibility

<!-- field: target_name -->
#### Target name

specification-critic

<!-- field: binding | form: "tree" -->
#### Binding

- role: critic
- sub_roles:
  1. output-evaluator

<!-- field: canonical_path -->
#### Canonical path

applications/catalog/agents/specification-critic/profile.yaml

<!-- field: reason -->
#### Reason

Historical profile identity; the implementation evaluates output and has no Guardian enforcement authority.

<!-- field: supported_through -->
#### Supported through

applications/catalog/v0.*

<!-- field: removal -->
#### Removal

applications/catalog/v1

<!-- migration_alias: coordinator -->
### coordinator

<!-- field: path -->
#### Path

applications/chatbot-mesh/agents/coordinator/profile.yaml

<!-- field: status -->
#### Status

migration_removed

<!-- field: collision_with -->
#### Collision with

coordinator

<!-- field: target_name -->
#### Target name

provisioning-workflow-orchestrator

<!-- field: binding | form: "tree" -->
#### Binding

- role: executor
- sub_roles:
  1. workflow-orchestrator

<!-- field: canonical_path -->
#### Canonical path

applications/chatbot-mesh/agents/provisioning-workflow-orchestrator/profile.yaml

<!-- field: reason -->
#### Reason

Historical chatbot-mesh actor identity removed by GH-1077; no runtime profile, workload, Service, values-key, or ConfigMap alias is shipped.

<!-- field: non_goals -->
## Non goals

- Change the agent-core profile schema or add role fields to profile.yaml.
- Rename tools, runtime monitor APIs, or historical release IDs in this issue.
- Claim every canonical role has an implementation.
- Invent empty profiles to increase role coverage.
- Treat model, request, session, server, or workspace qualifiers as semantic sub-roles.
