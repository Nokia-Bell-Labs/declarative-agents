---
id: applications-shared-contract-specifications
title: Shared Applications Specification Index
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

This corpus defines the contract for runnable modules under applications/.
srd001 is the baseline for every runnable application. srd002 is conditional:
its requirement groups apply only to managed-service, packaged, Helm-managed,
or kind-demo capabilities. applications/catalog is indexed as a related
foundation but is not a runnable application.

srd003 implements the Release 14.0 consistency target: one
agents/application.yaml composition authority, application-owned actor and UI
grammar, manifest-derived packages, canonical promotion and shared-token
rules, common README and status vocabulary, and explicit catalog/application/
agent-core boundaries. Its capability gates do not require empty optional
directories.

The release 00.0 use case and suite trace the baseline SRDs reciprocally.
Release 14.0 adds a reciprocal cross-application use case and suite.
Release 15.0 adds srd004, the presentation contract and panel composition
model for application UIs, with its use case and suite.
Release 19.0 adds srd008, the persistent local/cloud platform contract:
separate platform, application-compute, and retained-data ownership; one
namespace, collector, and bucket per application; WAL-first immutable
telemetry; explicit purge and reset; and one reusable application-rig
boundary.
Structural
checks parse documents, verify module classification and root orchestration,
enforce ownership-aware stats, and invoke application-owned capability
evidence. The runnable set is chatbot-mesh, coding-agent,
and agent-architecture. All three expose implemented apprig lifecycle,
namespaced serving, retained telemetry, and shared-platform evidence. ENG01
remains the sole kind-rig contract; authenticated live GKE execution is
dependency-gated where project credentials are unavailable.

<!-- field: roadmap_summary -->
## Roadmap summary

<!-- roadmap_summary_entry: 00.0 -->
### 00.0

<!-- field: name -->
#### Name

Shared runnable-application contract

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

Root Go tests and application Mage gates are executable.

<!-- roadmap_summary_entry: 12.0 -->
### 12.0

<!-- field: name -->
#### Name

Agent role-realization alignment

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

Catalog migration, role governance, consumer closure, Helm package, tag-semantic, and scoped zero-hit tests are executable.

<!-- roadmap_summary_entry: 13.0 -->
### 13.0

<!-- field: name -->
#### Name

Multi-agent application vision

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

0

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

0

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

The shared vision, root guide, architecture index, and roadmap parse and pass the repository audit.

<!-- roadmap_summary_entry: 14.0 -->
### 14.0

<!-- field: name -->
#### Name

Application consistency and conformance

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

The SRD, cross-application use case, and eight-case test suite are
reciprocal and executable. Root conformance tests validate all three
manifests, ownership, package assets, canonical applier promotion, shared
UI tokens, and README/status classification.

<!-- roadmap_summary_entry: 15.0 -->
### 15.0

<!-- field: name -->
#### Name

Declarative UX

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

srd004, the use case, and the nine-case suite are reciprocal and executable; the kit, shell, embedded observer, and monorepo migrations landed in GH-2156 through GH-2160.

<!-- roadmap_summary_entry: 16.0 -->
### 16.0

<!-- field: name -->
#### Name

Chart conformance gate

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

ENG01 carries the numbered chart rules, srd005 owns them, and all twelve
suite cases are executable. The gate renders every chart under every
checked-in overlay; the baseline is empty, so the charts conform by
construction rather than by exception.

<!-- roadmap_summary_entry: 17.0 -->
### 17.0

<!-- field: name -->
#### Name

Pin currency report

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

ENG01 carries the pin currency section and the bump verb, srd006 owns
the two properties, and all nine suite cases are executable. The target
resolves eleven pins against their registries without a daemon, and its
own suite reaches no network.

<!-- roadmap_summary_entry: 18.0 -->
### 18.0

<!-- field: name -->
#### Name

Release annotation gate

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

ENG01 carries the release-annotation section and the tag:verify verb,
srd007 owns the rules, and all eight suite cases are executable. The
target passes the most recent real-summary tag and refuses the
degenerate one naming its rule; the runbook carries the step.

<!-- roadmap_summary_entry: 19.0 -->
### 19.0

<!-- field: name -->
#### Name

Persistent application platform

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

srd008, its use case, and the eight-case suite define the ownership,
routing, lifecycle, storage, query, threat, and reusable-runner contract.
TC1 through TC8 are implemented by GH-2474: three concurrent application
migrations, durable collector/query/WAL evidence, explicit purge,
platform failure separation, external-module apprig use, and cloud IAM
policy fixtures.

<!-- roadmap_summary_entry: 20.0 -->
### 20.0

<!-- field: name -->
#### Name

Agent operator

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

srd009, its use case, and the five-case suite define the Agent resource,
reconciliation, status, artifacts, platform install, and the collector
proof; TC1 through TC5 are implemented under GH-2681.

<!-- roadmap_summary_entry: 20.1 -->
### 20.1

<!-- field: name -->
#### Name

Profile-derived Agents

<!-- field: use_cases_done | form: "scalar" -->
#### Use cases done

1

<!-- field: use_cases_total | form: "scalar" -->
#### Use cases total

1

<!-- field: status -->
#### Status

implemented

<!-- field: evidence -->
#### Evidence

srd009's revised R1.1, R1.2, R2.1, R2.2, R2.4, R2.5, and R5.1 with AC5 and
AC6, the use case, and the four-case suite define derivation from the
profile, reconcile-time validation, the ConfigMap watch, and the shrunk
collector proof; TC1 through TC4 are implemented under GH-2688.

<!-- field: foundation_document_index -->
## Foundation document index

<!-- foundation_document_index_entry: application-composition-pattern-language -->
### application-composition-pattern-language

<!-- field: title -->
#### Title

A Pattern Language for Declarative Agent Applications

<!-- field: summary -->
#### Summary

Recurring application-composition, package-time closure, deployment
topology, and module-governance patterns linked to the single-agent
language and the role-realization semantic model.

<!-- field: path -->
#### Path

docs/pattern-language.md

<!-- foundation_document_index_entry: applications-shared-contract -->
### applications-shared-contract

<!-- field: title -->
#### Title

Vision for Declarative Multi-Agent Applications

<!-- field: summary -->
#### Summary

Multi-agent rationale, intent and assurance flows, role realization,
human governance, construction sequence, runnable-module contract, and
capability-aware success criteria.

<!-- field: path -->
#### Path

docs/VISION.md

<!-- foundation_document_index_entry: architecture-applications-shared-contract -->
### architecture-applications-shared-contract

<!-- field: title -->
#### Title

Shared Applications Architecture

<!-- field: summary -->
#### Summary

Ownership layers, module classes, capability layers, and Mage interfaces.

<!-- field: path -->
#### Path

docs/ARCHITECTURE.md

<!-- foundation_document_index_entry: applications-shared-contract-roadmap -->
### applications-shared-contract-roadmap

<!-- field: title -->
#### Title

Shared Applications Contract Road Map

<!-- field: summary -->
#### Summary

Release status tied to executable structural evidence.

<!-- field: path -->
#### Path

docs/road-map.md

<!-- foundation_document_index_entry: applications-readme -->
### applications-readme

<!-- field: title -->
#### Title

Applications

<!-- field: summary -->
#### Summary

Human entry point, terminology, and current classifications.

<!-- field: path -->
#### Path

README.md

<!-- foundation_document_index_entry: eng01-kind-test-demo-rig -->
### eng01-kind-test-demo-rig

<!-- field: title -->
#### Title

Standard kind Rig for Agent Integration Tests and Demos

<!-- field: summary -->
#### Summary

External engineering contract used only by kind-demo applications.

<!-- field: path -->
#### Path

../docs/engineering/eng01-kind-test-demo-rig.md

<!-- foundation_document_index_entry: architecture-application-catalog -->
### architecture-application-catalog

<!-- field: title -->
#### Title

Application Catalog Architecture

<!-- field: summary -->
#### Summary

Canonical reusable catalog ownership and consumer boundary.

<!-- field: path -->
#### Path

catalog/docs/ARCHITECTURE.md

<!-- foundation_document_index_entry: architecture-agent-architecture -->
### architecture-agent-architecture

<!-- field: title -->
#### Title

Agent Architecture Application Architecture

<!-- field: summary -->
#### Summary

Composition-only ownership, capability status, and local evidence boundary.

<!-- field: path -->
#### Path

agent-architecture/docs/ARCHITECTURE.md

<!-- field: srd_index -->
## SRD index

<!-- srd_index_entry: srd001-application-module-contract -->
### srd001-application-module-contract

<!-- field: title -->
#### Title

Runnable Application Module Contract

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

Every runnable application; excludes applications/catalog.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd001-application-module-contract.md

<!-- field: use_cases -->
#### Use cases

- [rel00.0-uc001-application-contract-conformance](specs/use-cases/rel00.0-uc001-application-contract-conformance.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel00.0-application-contract-conformance](specs/test-suites/test-rel00.0-application-contract-conformance.md)

<!-- srd_index_entry: srd002-managed-application-services -->
### srd002-managed-application-services

<!-- field: title -->
#### Title

Capability-Dependent Application Services

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

Requirement groups apply only to declared managed_service, packaged, helm_managed, or kind_demo capabilities.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd002-managed-application-services.md

<!-- field: use_cases -->
#### Use cases

- [rel00.0-uc001-application-contract-conformance](specs/use-cases/rel00.0-uc001-application-contract-conformance.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel00.0-application-contract-conformance](specs/test-suites/test-rel00.0-application-contract-conformance.md)

<!-- srd_index_entry: srd003-application-consistency -->
### srd003-application-consistency

<!-- field: title -->
#### Title

Application Composition Consistency and Conformance

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

Manifest, actor, naming, README, status, and ownership requirements apply
to runnable or audit-only applications; package, deployment, kind, and UI
requirements apply only when the matching capability or asset exists.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd003-application-consistency.md

<!-- field: use_cases -->
#### Use cases

- [rel14.0-uc001-application-consistency](specs/use-cases/rel14.0-uc001-application-consistency.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel14.0-application-consistency](specs/test-suites/test-rel14.0-application-consistency.md)

<!-- srd_index_entry: srd004-declarative-ux -->
### srd004-declarative-ux

<!-- field: title -->
#### Title

Presentation Contract and Panel Composition

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

Applications and actors that ship UI assets; R9 applies only to REST
definitions that select an embedded bundle.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd004-declarative-ux.md

<!-- field: use_cases -->
#### Use cases

- [rel15.0-uc001-declarative-ux](specs/use-cases/rel15.0-uc001-declarative-ux.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel15.0-declarative-ux](specs/test-suites/test-rel15.0-declarative-ux.md)

<!-- srd_index_entry: srd005-chart-conformance -->
### srd005-chart-conformance

<!-- field: title -->
#### Title

Chart Conformance Against the Rig Guideline

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

R1 through R5, R8, and R11 apply to every application that ships a chart; R6
applies to every checked-in kind configuration; R7 governs the
exception baseline.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd005-chart-conformance.md

<!-- field: use_cases -->
#### Use cases

- [rel16.0-uc001-chart-conformance](specs/use-cases/rel16.0-uc001-chart-conformance.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel16.0-chart-conformance](specs/test-suites/test-rel16.0-chart-conformance.md)

<!-- srd_index_entry: srd006-pin-currency -->
### srd006-pin-currency

<!-- field: title -->
#### Title

Pin Integrity and Currency

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

Applies to every pinned upstream dependency this repository resolves
from a registry; Go module versions stay outside it.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd006-pin-currency.md

<!-- field: use_cases -->
#### Use cases

- [rel17.0-uc001-pin-currency](specs/use-cases/rel17.0-uc001-pin-currency.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel17.0-pin-currency](specs/test-suites/test-rel17.0-pin-currency.md)

<!-- srd_index_entry: srd007-release-annotation -->
### srd007-release-annotation

<!-- field: title -->
#### Title

Release Annotation Verification

<!-- field: status -->
#### Status

implemented

<!-- field: applicability -->
#### Applicability

Applies to every tag on the v0 release line, on demand at the push
boundary; gates no build and joins no audit.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd007-release-annotation.md

<!-- field: use_cases -->
#### Use cases

- [rel18.0-uc001-release-annotation](specs/use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel18.0-release-annotation](specs/test-suites/test-rel18.0-release-annotation.md)

<!-- srd_index_entry: srd008-persistent-application-platform -->
### srd008-persistent-application-platform

<!-- field: title -->
#### Title

Persistent Platform and Namespaced Application Contract

<!-- field: status -->
#### Status

planned

<!-- field: applicability -->
#### Applicability

Helm-managed applications on the shared platform; cluster-mutating
integration scenarios retain ENG01's owned-cluster exception.

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd008-persistent-application-platform.md

<!-- field: use_cases -->
#### Use cases

- [rel19.0-uc001-persistent-application-platform](specs/use-cases/rel19.0-uc001-persistent-application-platform.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel19.0-persistent-application-platform](specs/test-suites/test-rel19.0-persistent-application-platform.md)

<!-- srd_index_entry: srd009-agent-operator -->
### srd009-agent-operator

<!-- field: title -->
#### Title

The Agent Operator and the Agent Resource

<!-- field: summary -->
#### Summary

One namespaced Agent resource reconciled into the Deployment and Service
the charts write by hand, with status, CRD-on-boot, platform install, and
a collector proof; from 20.1 the operator derives ports, probe, monitor
label, and bind host from the profile itself.

<!-- field: status -->
#### Status

implemented

<!-- field: path -->
#### Path

docs/specs/software-requirements/srd009-agent-operator.md

<!-- field: use_cases -->
#### Use cases

- [rel20.0-uc001-agent-operator](specs/use-cases/rel20.0-uc001-agent-operator.md)
- [rel20.1-uc001-profile-derived-agent](specs/use-cases/rel20.1-uc001-profile-derived-agent.md)

<!-- field: test_suites -->
#### Test suites

- [test-rel20.0-agent-operator](specs/test-suites/test-rel20.0-agent-operator.md)
- [test-rel20.1-profile-derived-agent](specs/test-suites/test-rel20.1-profile-derived-agent.md)

<!-- field: config_format_index -->
## Config format index

<!-- field: semantic_model_index -->
## Semantic model index

<!-- semantic_model_index_entry: sm-agent-role-realizations -->
### sm-agent-role-realizations

<!-- field: title -->
#### Title

Agent Role Realizations and Bindings

<!-- field: summary -->
#### Summary

Pinned canonical role and sub-role authority, realization naming grammar,
actor-role bindings, wrapper inheritance, classifications, and migration
aliases shared by the catalog and runnable applications.

<!-- field: status -->
#### Status

implemented

<!-- field: path -->
#### Path

docs/specs/semantic-models/agent-role-realizations.md

<!-- field: reciprocal_indexes -->
#### Reciprocal indexes

- catalog/docs/SPECIFICATIONS.md#semantic_model_index
- catalog/docs/specs/semantic-models/agent-role-taxonomy.md#shared_authority

<!-- field: use_case_index | form: "typed table" -->
## Use case index

| id | title | release | status | srds | test_suite | path |
| --- | --- | --- | --- | --- | --- | --- |
| [rel00.0-uc001-application-contract-conformance](specs/use-cases/rel00.0-uc001-application-contract-conformance.md) | Verify a Runnable Application Against the Shared Contract | "00.0" | implemented | [[srd001-application-module-contract](specs/software-requirements/srd001-application-module-contract.md), [srd002-managed-application-services](specs/software-requirements/srd002-managed-application-services.md)] | [test-rel00.0-application-contract-conformance](specs/test-suites/test-rel00.0-application-contract-conformance.md) | docs/specs/use-cases/rel00.0-uc001-application-contract-conformance.md |
| [rel14.0-uc001-application-consistency](specs/use-cases/rel14.0-uc001-application-consistency.md) | Normalize and Verify a Showcase Application Composition | "14.0" | implemented | [[srd003-application-consistency](specs/software-requirements/srd003-application-consistency.md)] | [test-rel14.0-application-consistency](specs/test-suites/test-rel14.0-application-consistency.md) | docs/specs/use-cases/rel14.0-uc001-application-consistency.md |
| [rel15.0-uc001-declarative-ux](specs/use-cases/rel15.0-uc001-declarative-ux.md) | Compose an Application UI from Declared Panels | "15.0" | implemented | [[srd004-declarative-ux](specs/software-requirements/srd004-declarative-ux.md)] | [test-rel15.0-declarative-ux](specs/test-suites/test-rel15.0-declarative-ux.md) | docs/specs/use-cases/rel15.0-uc001-declarative-ux.md |
| [rel16.0-uc001-chart-conformance](specs/use-cases/rel16.0-uc001-chart-conformance.md) | Add a Workload and Learn at Audit Time That Its Image Floats | "16.0" | implemented | [[srd005-chart-conformance](specs/software-requirements/srd005-chart-conformance.md)] | [test-rel16.0-chart-conformance](specs/test-suites/test-rel16.0-chart-conformance.md) | docs/specs/use-cases/rel16.0-uc001-chart-conformance.md |
| [rel17.0-uc001-pin-currency](specs/use-cases/rel17.0-uc001-pin-currency.md) | Read Which Pins Moved and Which Aged, Then Decide | "17.0" | implemented | [[srd006-pin-currency](specs/software-requirements/srd006-pin-currency.md)] | [test-rel17.0-pin-currency](specs/test-suites/test-rel17.0-pin-currency.md) | docs/specs/use-cases/rel17.0-uc001-pin-currency.md |
| [rel18.0-uc001-release-annotation](specs/use-cases/rel18.0-uc001-release-annotation.md) | A Degenerate Release Annotation Is Refused Before the Push | "18.0" | implemented | [[srd007-release-annotation](specs/software-requirements/srd007-release-annotation.md)] | [test-rel18.0-release-annotation](specs/test-suites/test-rel18.0-release-annotation.md) | docs/specs/use-cases/rel18.0-uc001-release-annotation.md |
| [rel19.0-uc001-persistent-application-platform](specs/use-cases/rel19.0-uc001-persistent-application-platform.md) | Operate Two Namespaced Applications Without Losing Retained Telemetry | "19.0" | implemented | [[srd008-persistent-application-platform](specs/software-requirements/srd008-persistent-application-platform.md)] | [test-rel19.0-persistent-application-platform](specs/test-suites/test-rel19.0-persistent-application-platform.md) | docs/specs/use-cases/rel19.0-uc001-persistent-application-platform.md |
| [rel20.0-uc001-agent-operator](specs/use-cases/rel20.0-uc001-agent-operator.md) | Apply an Agent for the Catalog Collector and Watch It Become Ready | "20.0" | implemented | [[srd009-agent-operator](specs/software-requirements/srd009-agent-operator.md)] | [test-rel20.0-agent-operator](specs/test-suites/test-rel20.0-agent-operator.md) | docs/specs/use-cases/rel20.0-uc001-agent-operator.md |
| [rel20.1-uc001-profile-derived-agent](specs/use-cases/rel20.1-uc001-profile-derived-agent.md) | Declare an Agent from Its Profile Alone | "20.1" | implemented | [[srd009-agent-operator](specs/software-requirements/srd009-agent-operator.md)] | [test-rel20.1-profile-derived-agent](specs/test-suites/test-rel20.1-profile-derived-agent.md) | docs/specs/use-cases/rel20.1-uc001-profile-derived-agent.md |

<!-- field: test_suite_index | form: "typed table" -->
## Test suite index

| id | title | release | test_case_count | traces | status | srds | path |
| --- | --- | --- | --- | --- | --- | --- | --- |
| [test-rel00.0-application-contract-conformance](specs/test-suites/test-rel00.0-application-contract-conformance.md) | Shared Application Contract Conformance Suite | "00.0" | 8 | [[rel00.0-uc001-application-contract-conformance](specs/use-cases/rel00.0-uc001-application-contract-conformance.md)] | implemented | [[srd001-application-module-contract](specs/software-requirements/srd001-application-module-contract.md), [srd002-managed-application-services](specs/software-requirements/srd002-managed-application-services.md)] | docs/specs/test-suites/test-rel00.0-application-contract-conformance.md |
| [test-rel14.0-application-consistency](specs/test-suites/test-rel14.0-application-consistency.md) | Release 14.0 Application Consistency Test Suite | "14.0" | 8 | [[rel14.0-uc001-application-consistency](specs/use-cases/rel14.0-uc001-application-consistency.md)] | implemented | [[srd003-application-consistency](specs/software-requirements/srd003-application-consistency.md)] | docs/specs/test-suites/test-rel14.0-application-consistency.md |
| [test-rel15.0-declarative-ux](specs/test-suites/test-rel15.0-declarative-ux.md) | Release 15.0 Declarative UX Test Suite | "15.0" | 9 | [[rel15.0-uc001-declarative-ux](specs/use-cases/rel15.0-uc001-declarative-ux.md)] | implemented | [[srd004-declarative-ux](specs/software-requirements/srd004-declarative-ux.md)] | docs/specs/test-suites/test-rel15.0-declarative-ux.md |
| [test-rel16.0-chart-conformance](specs/test-suites/test-rel16.0-chart-conformance.md) | Release 16.0 Chart Conformance Test Suite | "16.0" | 12 | [[rel16.0-uc001-chart-conformance](specs/use-cases/rel16.0-uc001-chart-conformance.md)] | implemented | [[srd005-chart-conformance](specs/software-requirements/srd005-chart-conformance.md)] | docs/specs/test-suites/test-rel16.0-chart-conformance.md |
| [test-rel17.0-pin-currency](specs/test-suites/test-rel17.0-pin-currency.md) | Release 17.0 Pin Currency Test Suite | "17.0" | 9 | [[rel17.0-uc001-pin-currency](specs/use-cases/rel17.0-uc001-pin-currency.md)] | implemented | [[srd006-pin-currency](specs/software-requirements/srd006-pin-currency.md)] | docs/specs/test-suites/test-rel17.0-pin-currency.md |
| [test-rel18.0-release-annotation](specs/test-suites/test-rel18.0-release-annotation.md) | Release 18.0 Release Annotation Test Suite | "18.0" | 6 | [[rel18.0-uc001-release-annotation](specs/use-cases/rel18.0-uc001-release-annotation.md)] | implemented | [[srd007-release-annotation](specs/software-requirements/srd007-release-annotation.md)] | docs/specs/test-suites/test-rel18.0-release-annotation.md |
| [test-rel19.0-persistent-application-platform](specs/test-suites/test-rel19.0-persistent-application-platform.md) | Release 19.0 Persistent Application Platform Test Suite | "19.0" | 8 | [[rel19.0-uc001-persistent-application-platform](specs/use-cases/rel19.0-uc001-persistent-application-platform.md)] | implemented | [[srd008-persistent-application-platform](specs/software-requirements/srd008-persistent-application-platform.md)] | docs/specs/test-suites/test-rel19.0-persistent-application-platform.md |
| [test-rel20.0-agent-operator](specs/test-suites/test-rel20.0-agent-operator.md) | Release 20.0 Agent Operator Test Suite | "20.0" | 5 | [[rel20.0-uc001-agent-operator](specs/use-cases/rel20.0-uc001-agent-operator.md)] | implemented | [[srd009-agent-operator](specs/software-requirements/srd009-agent-operator.md)] | docs/specs/test-suites/test-rel20.0-agent-operator.md |
| [test-rel20.1-profile-derived-agent](specs/test-suites/test-rel20.1-profile-derived-agent.md) | Release 20.1 Profile-Derived Agent Test Suite | "20.1" | 4 | [[rel20.1-uc001-profile-derived-agent](specs/use-cases/rel20.1-uc001-profile-derived-agent.md)] | implemented | [[srd009-agent-operator](specs/software-requirements/srd009-agent-operator.md)] | docs/specs/test-suites/test-rel20.1-profile-derived-agent.md |

<!-- field: coverage_gaps -->
## Coverage gaps

<!-- coverage_gap: release-19-persistent-application-platform -->
### release-19-persistent-application-platform

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

GH-2474 implements the contract and all eight cases. Retained objects are
intentionally not a live query service while the application collector
is absent; a global query service and authenticated hostile local
multi-tenancy remain outside this release. The approved GH-2481 decision
retains application-owned vector stores.

<!-- coverage_gap: release-18-release-annotation -->
### release-18-release-annotation

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

The check runs and the runbook carries it between annotation and push.
The degenerate tags already pushed stay as they are; they are the
evidence, not the workload. The skill half lives in the coding-skills
repository as its issue 395 and lands there.

<!-- coverage_gap: release-17-pin-currency -->
### release-17-pin-currency

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

The survey runs and reports. On its first run no pin had moved and
seven were behind, among them three nobody had counted: busybox, chroma
and the kubectl proxy image. Taking those bumps is separate work, and
the report exists so the decision is informed rather than overdue.
Classifying an update from its release notes stays out of this release
and remains gated in GH-2049.

<!-- coverage_gap: release-16-chart-conformance -->
### release-16-chart-conformance

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

The gate found fifty violations on its first run, across five upstream
images and the init containers that stated no pull policy. All fifty
are closed and the baseline is empty. Rules C4 and C5 found nothing
when the gate first ran and remain ratchets rather than cleanups.
Classifying an application template as an un-migrated library candidate
stays outside this release and is tracked in GH-2410.

<!-- coverage_gap: release-15-declarative-ux -->
### release-15-declarative-ux

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

TC1 through TC9 are executable. Open kit gaps (same-origin trace backend,
URL-driven TraceView, span status, shell styles) are tracked in GH-2282.
The cohere-demo and agentic-wiki-mesh migrations are owned by those
repositories and consume release ui-kit/v1.0.0.

<!-- coverage_gap: release-14-application-normalization -->
### release-14-application-normalization

<!-- field: status -->
#### Status

implemented

<!-- field: description -->
#### Description

The three showcase applications have validated manifests and normalized
ownership. Coding Agent and Agent Architecture are composition-only with
local composition wrappers; Chatbot Mesh is agent-owning while reporting
wrappers separately. TC1 through TC8 are executable.

<!-- coverage_gap: knowledge-manager-live-managed-service-evidence -->
### knowledge-manager-live-managed-service-evidence

<!-- field: status -->
#### Status

dependency_gated

<!-- field: description -->
#### Description

The demo has implemented deterministic command, configuration, audit,
test, stats, and root-dispatch evidence. Live lifecycle, readiness,
liveness, failure, diagnostics, telemetry, shutdown, in-flight, and
listener-release observations remain required before it satisfies srd002
AC1 through AC3.

<!-- coverage_gap: future-application-registration -->
### future-application-registration

<!-- field: status -->
#### Status

by_design

<!-- field: description -->
#### Description

The shared suite cannot name future application paths. A future runnable
module must add itself to root orchestration and add path-specific evidence
while retaining the generic shared requirement IDs.

<!-- coverage_gap: optional-capability-behavior -->
### optional-capability-behavior

<!-- field: status -->
#### Status

delegated

<!-- field: description -->
#### Description

This suite verifies that capability evidence exists and is executable.
Application-local SRDs and suites own business behavior, topology, and
runtime outcomes.
