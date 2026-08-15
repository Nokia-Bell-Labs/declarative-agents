<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Applications

`applications/` contains runnable application modules and the repository's
reusable declarative catalog. These are different ownership classes.

The canonical [applications vision](docs/VISION.yaml) explains why we compose
multi-agent systems, when one actor is sufficient, how intent and assurance
flows cross the composition, and how a solution architecture becomes a runnable
module, making it the entry point for the shared application contract.

## Why multiple agents

We separate actors when responsibilities need independent authority, failure,
scaling, or lifecycle boundaries. The split must make those boundaries easier
to govern and observe. A single actor is preferable when another remote or
managed boundary would add cost without isolation or control.

Canonical roles are semantic interfaces. Profiles and runtime actors realize
one or more of them as defined by the
[role-realization model](docs/specs/semantic-models/agent-role-realizations.yaml).
The application owns relationships, sequencing, topology, configuration, and
human decision points among those actors. `agent-core` interprets each profile.

## How we build applications

1. Define the initiating scenario, constraints, human exceptions, expected
   outcome, and end-to-end evidence.
2. Allocate scenario responsibilities to canonical roles, then group roles into
   actors. Split actors only for a stated authority, failure, scaling, or
   lifecycle boundary.
3. Reuse independently useful profiles from `applications/catalog/` by
   canonical path. Keep application-specific behavior local.
4. Declare the application-owned composition: actor relationships, boundary
   tools, sequencing, topology, policy attachment, configuration, and operator
   surfaces.
5. For a packaged application, resolve the complete transitive profile closure
   before deployment and mount it into a profile-free runtime.
6. Prove the real path with structural checks and application-owned end-to-end
   evidence, then register the module in the applicable root Mage gates.

The [application pattern language](docs/pattern-language.yaml) gives construction
patterns for canonical reuse, explicit composition, package closure,
profile-free runtimes, wrappers, and role-scoped workloads. The shared vision
remains the authority for purpose, boundaries, and the ordered process.

## Directory classes

- A **runnable application** owns a composition that a developer or operator can
  execute. It owns application documentation, audit evidence, tests where code
  exists, and root orchestration participation.
- A **composition-only application** is runnable but contributes no canonical
  or application-local agent implementation. It may own thin serving, request,
  or applier composition wrappers and composition-specific bindings, but it
  references canonical implementations and reports reuse without adding those
  agents to repository totals. A local profile is agent-owning only when it
  owns application-specific machine behavior rather than wrapping a canonical
  realization.
- An **audit-only application** owns `agents/application.yaml`, the common
  documentation corpus, a local audit target, and root audit-only registration.
  It does not claim a runnable composition, tests, stats, packaging, deployment,
  or UI unless separate evidence supports those capabilities.
- `applications/catalog/` is a **reusable catalog module**. It owns canonical
  declarative blocks and conformance evidence. It is not a runnable application.

The shared contract follows the [vision](docs/VISION.yaml) under [`docs/`](docs/).
Application-local architecture and SRDs extend that contract with business
behavior, topology, and evidence. They cite the shared requirements instead of
copying them.

The application-composition pattern language is
[`docs/pattern-language.yaml`](docs/pattern-language.yaml). It covers catalog
consumption, application-owned composition, package-time closure, deployment
topology, and module governance. It extends the
[single-agent pattern language](../design-patterns/pattern-language.yaml)
without repeating agent internals.

## Common core and optional directories

Release 14.0 defines this common core for every runnable or audit-only
application:

- `README.md`: purpose, status, composition, capabilities, ownership boundaries,
  run or planned entry points, verification, and documentation.
- `agents/application.yaml`: schema-versioned composition authority for direct
  catalog and local profile roots, runtime paths, compatibility, deployment and
  UI entries when present, capability status, and evidence.
- `docs/VISION.yaml`, `docs/ARCHITECTURE.yaml`, `docs/road-map.yaml`, and
  `docs/SPECIFICATIONS.yaml`: the application-local design and traceability
  corpus.
- `magefiles/` or an equivalent local Mage entry: `audit` is required. Tests,
  stats, package, Helm, integration, doctor, and demo targets exist only when
  the application has those surfaces.

All other directories are capability- or content-dependent:

- `agents/<actor>/` exists only for an application-owned production actor or
  thin wrapper. Composition-only applications need no empty actor directory.
- `agents/<actor>/ui/` exists only when that actor serves a UI. A UI with no
  serving actor may use the application-level `ui/` root.
- `helm/` exists only for `helm_managed`.
- package output and package tests exist only for `packaged`; generated closure
  inventories derive from `agents/application.yaml` and are not maintained as a
  second list.
- kind fixtures and verbs exist only for `kind_demo`.
- `testdata/` and actor-local `tests/` exist only when tests require them and do
  not enter production profile roots or agent accounting.

Do not add an empty actor, service, package, chart, kind, request, or UI
directory to imply conformance. An absent undeclared capability is
`not_applicable`, not implemented.

## Normalized agent program names

An actor's primary files are `profile.yaml`, `machine.yaml`, `tools.yaml`,
`declarations.yaml`, and `rest.yaml` when those surfaces exist. A distinct
operation uses one lower-kebab prefix across related files, such as
`apply-profile.yaml`, `apply-machine.yaml`, `apply-tools.yaml`, and
`apply-declarations.yaml`. `request-profile.yaml`, `request-machine.yaml`,
`request-tools.yaml`, and `request-declarations.yaml` are the reserved request
variant names. A variant does not need every file.

`tools.yaml` and `<operation>-tools.yaml` select tool names.
`declarations.yaml` and `<operation>-declarations.yaml` own profile ToolDefs.
Actor-served UI stays under `agents/<actor>/ui/` and is declared with its REST
`static_assets` boundary in `agents/application.yaml`.

Canonical reusable implementations stay under `catalog/agents/`. Applications
own local actors, wrappers, endpoints, deployment, credentials, UI, and
end-to-end evidence. `agent-core` owns profile, machine, tool, REST, lifecycle,
telemetry, checkpoint, and execution semantics.

## Promotion and shared UI tokens

Promote an application asset to `catalog/` only when it is independently useful
or has at least two real consumers. Promotion moves reusable behavior and
conformance evidence, migrates every consumer to the canonical path, and removes
copies. Application-specific topology and bindings remain local.

A UI claims shared design-token conformance only when it consumes
[`catalog/ui/design-tokens.css`](catalog/ui/design-tokens.css) through a build
import or a deterministic generated copy. A checked-in generated token block
must name the canonical source and have a byte-for-byte drift test. Visual
similarity or a handwritten copy is not consumption.

## Capability classes

Every runnable application has the baseline `runnable_module` capability.
Additional obligations apply only when the application declares or implements
the corresponding capability:

- `managed_service`: long-running application-owned processes, lifecycle,
  health, control, telemetry, configuration, and graceful shutdown.
- `packaged`: a deterministic distributable application artifact.
- `helm_managed`: a Helm chart and chart-specific operator surfaces.
- `kind_demo`: a kind integration or demo rig governed by
  [`ENG01`](../docs/engineering/eng01-kind-test-demo-rig.yaml).

The baseline does not require packaging, Helm, Kubernetes, kind, or a managed
service. ENG01 governs applications that provide a kind demo; it does not make a
kind demo mandatory for every application.

## Status vocabulary

Application manifests and conformance summaries use:

- `planned`: specified work has no executable evidence yet.
- `audit_only`: manifest and documentation audit participate in the root gate;
  no runnable composition is claimed.
- `partial`: some required evidence exists and the missing evidence is named.
- `dependency_gated`: the named evidence requires an unavailable external
  prerequisite and is not counted as passing.
- `implemented`: named executable evidence satisfies the applicable contract.
- `not_applicable`: the application does not declare or ship that capability.

Statuses are capability-specific. An implemented `runnable_module` does not
imply implemented `managed_service`, `packaged`, `helm_managed`, `kind_demo`, or
UI evidence. A prose-only directory is not `audit_only` until its manifest,
local audit target, and root audit-only registration exist.

## Current classification

- `chatbot-mesh/`: agent-owning runnable application. It ships managed-service,
  packaged, Helm-managed, kind-demo, and actor-served UI surfaces. Its five
  local implementations enter agent totals; corpus-ingest and applier are
  composition wrappers. The manifest-derived package and canonical token
  imports are executable Release 14 evidence.
- `coding-agent/`: runnable application that consumes canonical planner,
  executor, critic, collector, and applier implementations. Its local planner,
  executor, critic, and applier profiles are composition wrappers, so the
  application is composition-only and contributes zero agents. Its
  managed-service, packaged, Helm-managed, kind-demo, and catalog UI surfaces
  have executable evidence.
- `agent-architecture/`: runnable application that consumes the canonical
  documentation-curator, collector, lifecycle-exit, and applier. Its local
  applier is a composition wrapper, so it remains composition-only and
  contributes zero agents. `runnable_module`, package, Helm, kind, and catalog
  UI evidence are implemented. `managed_service` remains partial because live
  lifecycle observations are dependency-gated.
- `catalog/`: reusable catalog module, not a runnable application.

Prose Editor moved downstream to
[`petar-djukic/declarative-agents`](https://github.com/petar-djukic/declarative-agents),
which adopted the module at upstream commit `9cb87d97` (GH-1578). It is no
longer built, tested, counted, or released by this repository.

Root Mage orchestration is the source of truth for participating modules.
`magefiles/build.go` separates reusable submodules from runnable applications;
`mage audit`, `mage test`, and `mage stats` dispatch through that classification.
Application-local Mage targets provide the evidence behind each claimed
capability.
