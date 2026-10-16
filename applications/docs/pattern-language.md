---
title: A Pattern Language for Declarative Agent Applications
version: 1
based_on: >
  Christopher Alexander, Sara Ishikawa, Murray Silverstein, "A Pattern Language: Towns, Buildings, Construction" (Oxford University Press, 1977).

extends: >
  "A Pattern Language for Declarative Agents", version 1 (../../design-patterns/pattern-language.yaml), referenced with the "da:" prefix.
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: declarative_agent_application -->
## Declarative agent application

```yaml
summary: >
    A declarative agent application is a runnable, governed composition of one or more agent realizations, application-owned configuration and interfaces, and any package or deployment artifacts needed to operate them. It consumes canonical agent programs by reference and owns the topology that turns those programs into an end-to-end capability.

parts:
    catalog: Canonical reusable agent programs and their conformance evidence.
    composition: Application-owned references, sequencing, wrappers, and interfaces.
    package: A resolved closure with provenance when a distributable artifact is required.
    deployment: Role-scoped workloads, mounts, endpoints, authority, and lifecycle.
    governance: Capability-dependent requirements, local evidence, and root orchestration.
canonical_example: >
    The coding-agent application references planner, executor, and critic in the catalog, resolves one closed profile tree at package time, mounts the appropriate closure into separate role workloads, and keeps its shared runtime image free of profiles.
```

<!-- field: role_realization_conventions -->
## Role realization conventions

```yaml
authority: applications/docs/specs/semantic-models/agent-role-realizations.yaml
naming_form: <context-or-purpose>-<primary-role>[-<deployment-or-configuration-form>]
identifier_style: lower-kebab-case
directory_rules:
    - Canonical reusable realizations live under applications/catalog/agents/.
    - Application wrappers and compositions live under applications/<application>/agents/.
    - Deployment, UX, test, and documentation artifacts remain with the application that owns them.
    - Directory and profile names describe ownership and realization context; they do not define canonical roles.
```

<!-- field: conventions -->
## Conventions

```yaml
confidence_scale:
    "2": Deep invariant; the same force is resolved the same way across many independent systems and prior literature.
    "1": Well supported by prior art, but the specific form presented here is more recent.
    "0": Tentative; corroboration is thin and the structure is largely from the single reference implementation.
relationship_types:
    within: Larger patterns whose context this pattern helps complete.
    requires: Patterns that must already be in place for this one to hold.
    contains: Smaller patterns that complete or refine this one.
    enables: Patterns that this one makes possible.
    overlaps: Patterns that address a neighbouring force by a different means.
```

<!-- field: patterns -->
## Patterns

<!-- pattern: capability-ladder | number: 1 -->
### Capability Ladder

<!-- field: also_known_as -->
#### Also known as

- Progressive Contract
- Capability-Gated Governance

<!-- field: confidence -->
#### Confidence

```yaml
1
```

<!-- field: intent -->
#### Intent

Assign obligations by declared capability so every runnable application has a common baseline without inheriting packaging or deployment duties it does not need.

<!-- field: context -->
#### Context

A repository contains runnable applications ranging from a local presentation to packaged, cluster-managed multi-agent systems. They need one governance vocabulary, but their operating surfaces differ.

<!-- field: problem -->
#### Problem

How do you enforce a useful application-wide contract when imposing the richest application's requirements on every module would create empty artifacts and false conformance claims?

<!-- field: applicability -->
#### Applicability

Use this when applications share a baseline but expose optional service, package, chart, or integration surfaces. A single homogeneous product with one fixed delivery form can use one undifferentiated contract.

<!-- field: forces -->
#### Forces

- Every runnable module needs ownership, documentation, audit, and evidence.
- Optional operational surfaces create real obligations only when present.
- Conformance claims must be testable rather than inferred from directories.
- New delivery mechanisms must be addable without weakening the baseline.

<!-- field: solution -->
#### Solution

Define one mandatory runnable-module capability, then add ordered, independently declared capabilities for managed service, package, Helm, and kind-demo behavior. Bind each requirement and acceptance check to the capability that makes it applicable. Require structural evidence for every declaration and permit explicit non-applicability for higher rungs.

<!-- field: implementation -->
#### Implementation

Keep the capability vocabulary in shared application documents. Each application records its declared capabilities, status, evidence, and exclusions. Root orchestration verifies baseline participation; local tests verify optional surfaces. A higher rung may depend on a lower rung, but no empty chart, package, or service stub may satisfy a declaration.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Small applications retain a meaningful baseline without ceremonial deployment files.
    - Reviewers can identify exactly which checks apply to a module.
    - New capability classes can extend the contract without changing existing declarations.
liabilities:
    - Applicability logic adds another dimension to audits and traceability.
    - Teams may under-declare capabilities unless evidence checks inspect actual surfaces.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: []
requires: []
contains: [one-canonical-home, composition-manifest, package-time-closure, profile-free-runtime, parameterization-wrapper, role-sharded-workload, orchestrate-act-split, persistent-application-platform]
enables: [one-canonical-home, composition-manifest]
overlaps: []
```

<!-- field: syntax -->
#### Syntax

Establish the Capability Ladder first; use its declared rungs to decide which composition, packaging, and deployment patterns an application must complete.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Kubernetes Operator Capability Levels | operator-framework-capabilities | external | Operators are classified by progressively richer lifecycle capabilities instead of requiring every operator to implement the highest level. |
| Progressive enhancement | champeon-progressive-enhancement-2003 | external | A dependable baseline is preserved while optional layers add obligations only when supported. |
| Shared applications contract | reference-implementation | internal | runnable_module is universal; managed_service, packaged, helm_managed, and kind_demo are evidence-gated. |

<!-- pattern: one-canonical-home | number: 2 -->
### One Canonical Home

<!-- field: also_known_as -->
#### Also known as

- Single Source of Truth
- Reference Not Fork

<!-- field: confidence -->
#### Confidence

```yaml
2
```

<!-- field: intent -->
#### Intent

Give reusable agent behavior one owning catalog location and make applications consume it by reference instead of copying it.

<!-- field: context -->
#### Context

Several applications need the same Agent-as-Data realization while also owning different interfaces, topology, and configuration. A reusable catalog and application directories already have distinct ownership.

<!-- field: problem -->
#### Problem

How do you let applications reuse and adapt an agent without allowing fixes, compatibility, conformance, and repository counts to diverge among copied profile trees?

<!-- field: applicability -->
#### Applicability

Use this when a profile is independently useful or has multiple consumers. Keep application-only behavior local when promotion would create an unstable abstraction with no second use.

<!-- field: forces -->
#### Forces

- Reusable behavior needs one maintenance and conformance owner.
- Applications must control their composition and deployment independently.
- Consumer upgrades must be explicit and compatible.
- Reused agents must not be counted as new implementations.

<!-- field: solution -->
#### Solution

Place each reusable profile family, its requirements, and conformance evidence in one canonical catalog directory. Have applications record versioned references to those paths. Keep application-only sequencing, endpoints, wrappers, and UX outside the catalog. Report reuse separately from application-owned agent implementations.

<!-- field: implementation -->
#### Implementation

Admit a catalog member only when it is independently useful or has multiple consumers. Version compatibility-sensitive paths and contracts. Reject copied canonical machines or declarations in application audits. Track source path and release or revision in each application manifest, and count only the canonical owner in repository statistics.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Fixes and conformance changes have one upstream location.
    - Ownership is visible in paths and statistics.
    - Applications can upgrade a known catalog revision on their own schedule.
liabilities:
    - Consumers need reference resolution and compatibility policy.
    - A catalog change may require coordinated migration across several applications.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [capability-ladder]
requires: ['da:agent-as-data']
contains: [composition-manifest, parameterization-wrapper]
enables: [composition-manifest, package-time-closure, parameterization-wrapper]
overlaps: []
```

<!-- field: syntax -->
#### Syntax

Under the Capability Ladder, keep every reusable Agent-as-Data in One Canonical Home, then let Composition Manifests reference it.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Bazel external repositories | bazel-external-dependencies | external | Consumers pin and reference one dependency source rather than copying its implementation into each target. |
| Git submodules | git-submodules | external | A repository records an exact external revision while preserving the external project as canonical owner. |
| Don't Repeat Yourself | hunt-thomas-pragmatic-1999 | external | Each piece of knowledge has one authoritative representation, matching the ownership force here. |
| Catalog and three runnable applications | reference-implementation | internal | coding-agent and agent-architecture reference canonical profiles; chatbot-mesh wraps canonical corpus-ingest while owning local mesh agents. |

<!-- pattern: composition-manifest | number: 3 -->
### Composition Manifest

<!-- field: also_known_as -->
#### Also known as

- Application Bill of Materials
- Assembly Manifest

<!-- field: confidence -->
#### Confidence

```yaml
1
```

<!-- field: intent -->
#### Intent

Record direct agent references, compatibility, runtime mounts, and application-owned bindings in one parseable composition root.

<!-- field: context -->
#### Context

One Canonical Home prevents copying, but references spread among prose, scripts, charts, and profiles leave no complete account of what the application assembles or which configuration it owns.

<!-- field: problem -->
#### Problem

How do you make an application composition reviewable and resolvable when its agents remain owned elsewhere and deployment-specific bindings stay local?

<!-- field: applicability -->
#### Applicability

Use this for any application that assembles catalog profiles, local wrappers, or several serving roles. A one-profile local demo may use a small manifest, but still benefits when it participates in governance.

<!-- field: forces -->
#### Forces

- Direct dependencies and compatible versions must be explicit.
- Application-owned bindings must not mutate canonical source files.
- Packaging needs a deterministic set of closure roots.
- Reviewers need one place to distinguish composition from implementation.

<!-- field: solution -->
#### Solution

Create one application-owned manifest that names every direct canonical and local profile root, its source path, runtime path, compatible catalog release, runtime mount, and configurable surfaces. Treat this manifest as the root of packaging and deployment, while profiles remain the authority for their own transitive references.

<!-- field: implementation -->
#### Implementation

Use a schema-versioned YAML document. Reject duplicate runtime paths, missing sources, unsafe traversal, unresolved compatibility, and request input that selects trusted roots or credentials. Keep deployment entries separate from reusable catalog references. Validate the manifest in the application audit and pass it unchanged to the closure resolver.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Composition is reviewable without reading build scripts or chart templates.
    - Package roots and provenance have one machine-readable source.
    - Local configuration ownership is explicit.
liabilities:
    - The manifest can drift unless packaging and deployment consume it directly.
    - It adds a maintained index alongside the referenced profile files.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [capability-ladder, one-canonical-home]
requires: [one-canonical-home, 'da:agent-as-data']
contains: [parameterization-wrapper]
enables: [package-time-closure, role-sharded-workload]
overlaps: []
```

<!-- field: syntax -->
#### Syntax

Reference each One Canonical Home from a Composition Manifest; use those roots for Package-Time Closure and deployment.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| OCI Image Index | oci-image-spec | external | A manifest identifies versioned component descriptors and content digests while blobs remain independently owned. |
| CycloneDX software bill of materials | cyclonedx-spec | external | A machine-readable root records components, dependencies, and provenance for later verification. |
| coding-agent application.yaml | reference-implementation | internal | The manifest records catalog release compatibility, direct role references, runtime paths, mount policy, serving profiles, and configuration surfaces. |
| Knowledge Manager composition reporting | reference-implementation | internal | The demo records one canonical reference and zero contributed agents even though it has no packaged closure. |

<!-- pattern: package-time-closure | number: 4 -->
### Package-Time Closure

<!-- field: also_known_as -->
#### Also known as

- Transitive Profile Closure
- Hermetic Bundle

<!-- field: confidence -->
#### Confidence

```yaml
2
```

<!-- field: intent -->
#### Intent

Resolve and verify every reachable profile asset before release so the deployed application has no source-catalog dependency.

<!-- field: context -->
#### Context

A Composition Manifest names direct profiles, but each profile may reference machines, declarations, tools, REST definitions, child profiles, and configuration files across several roots.

<!-- field: problem -->
#### Problem

How do you preserve canonical reuse during development while ensuring a released application cannot fail because a transitive profile asset is missing or changed after deployment?

<!-- field: applicability -->
#### Applicability

Use this whenever an application ships a package, chart, or disconnected deployment. A source-only local application can resolve references at startup, provided its checkout and revision remain explicit.

<!-- field: forces -->
#### Forces

- Canonical source must remain external during authoring.
- Runtime startup must not depend on a mutable catalog checkout.
- Every transitive reference and its provenance must be known before release.
- Unsafe, conflicting, or oversized assets must fail before deployment.

<!-- field: solution -->
#### Solution

Starting from the Composition Manifest, traverse profile references at package time, snapshot the compatible catalog revision, and copy the exact transitive closure into an application-owned runtime namespace. Preserve stable runtime paths, record content and source provenance, and reject any unresolved, escaping, conflicting, or unbounded member.

<!-- field: implementation -->
#### Implementation

Resolve relative references from the file that declares them and catalog references from the pinned catalog root. Exclude documented runtime-owned roots such as /opt/agent-core. Sort output deterministically, hash content, and emit an inventory that deployment consumes. Re-run profile validation against the assembled tree and verify that the release archive contains every inventory member and no undeclared profile source.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Missing dependencies fail packaging rather than a live workload.
    - Release artifacts are deterministic and independently deployable.
    - Every entry profile can be traced to an exact source revision.
liabilities:
    - Packaging duplicates bytes from the canonical source into release artifacts.
    - Closure traversal must track every reference-bearing schema field as formats evolve.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [capability-ladder]
requires: [one-canonical-home, composition-manifest]
contains: [profile-free-runtime]
enables: [profile-free-runtime, role-sharded-workload]
overlaps: []
```

<!-- field: syntax -->
#### Syntax

Traverse the Composition Manifest into a Package-Time Closure, validate the assembled tree, and mount it into a Profile-Free Runtime.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Nix closures | dolstra-nix-2006 | external | Deployment copies the complete transitive runtime dependency graph addressed by immutable paths. |
| Bazel runfiles | bazel-runfiles | external | A target's declared runtime data is assembled into a deterministic tree before execution. |
| OCI content-addressed images | oci-image-spec | external | Manifests close over immutable blobs and verify them by digest before distribution. |
| coding-agent profile closure | reference-implementation | internal | Packaging resolves one exact closure per serving role, records provenance and checksums, and rejects dangling or conflicting references. |
| chatbot-mesh corpus-ingest package | reference-implementation | internal | The application resolves the canonical corpus-ingest closure at build time and embeds it in the chart. |

<!-- pattern: profile-free-runtime | number: 5 -->
### Profile-Free Runtime

<!-- field: also_known_as -->
#### Also known as

- Behavior-Free Image
- Runtime-Profile Separation

<!-- field: confidence -->
#### Confidence

```yaml
1
```

<!-- field: intent -->
#### Intent

Keep reusable runtime images free of application profiles and inject a verified closure through an explicit mount and startup path.

<!-- field: context -->
#### Context

Package-Time Closure produces the application's behavior as data. Embedding that data in a shared runtime image would tie behavior releases to binary rebuilds and blur ownership between runtime and application.

<!-- field: problem -->
#### Problem

How do you give workloads complete agent behavior while preserving independent runtime and profile release cycles?

<!-- field: applicability -->
#### Applicability

Use this when several applications or roles share one agent runtime. A purpose-built appliance with inseparable code and behavior may choose a single image, but then it gives up independent profile governance.

<!-- field: forces -->
#### Forces

- Runtime code and application behavior need separate owners and release cadence.
- Startup must select an explicit, validated profile.
- Every workload must receive the package it was tested against.
- Operators need visible mounts and arguments rather than hidden image contents.

<!-- field: solution -->
#### Solution

Build runtime images with the interpreter and required execution toolchain, but no application or catalog profiles. Supply the verified Package-Time Closure as a mounted tree and pass one explicit profile path at process startup. Record and test the profile-free image invariant in package and deployment metadata.

<!-- field: implementation -->
#### Implementation

Scan image and archive inventories for forbidden profile roots. Mount the closure at a stable path such as /profiles, use an argv profile selector, and keep workspace mounts separate. Include only runtime toolchains needed by the selected agents. Test rendered workloads for image identity, mount source, profile argument, and closure checksum.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Runtime patches do not silently change application behavior.
    - One tested image can execute several role-specific closures.
    - Profile provenance remains visible outside the image.
liabilities:
    - Deployment must coordinate image and mounted closure compatibility.
    - Mount and startup misconfiguration becomes a new failure mode.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [capability-ladder, package-time-closure]
requires: [package-time-closure, 'da:agent-as-data']
contains: []
enables: [role-sharded-workload]
overlaps: [parameterization-wrapper]
```

<!-- field: syntax -->
#### Syntax

Mount each verified Package-Time Closure into a Profile-Free Runtime and select its entry profile explicitly.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Twelve-Factor build, release, run | twelve-factor-build-release-run | external | Immutable builds combine with configuration at release and run stages rather than hiding mutable behavior in code. |
| Kubernetes ConfigMap projection | kubernetes-configmaps | external | Configuration data is mounted into a reusable container image and selected by the workload. |
| coding-agent runtime image | reference-implementation | internal | Planner, executor, and critic share one runtime image containing the required toolchain but no profiles. |
| agent-core mounted profile contract | reference-implementation | internal | Runtime containers consume externally supplied profiles through an explicit --profile path. |

<!-- pattern: parameterization-wrapper | number: 6 -->
### Parameterization Wrapper

<!-- field: also_known_as -->
#### Also known as

- Application Overlay
- Configuration Wrapper

<!-- field: confidence -->
#### Confidence

```yaml
1
```

<!-- field: intent -->
#### Intent

Adapt a canonical realization with application-owned endpoints, configuration, or serving form while inheriting its role bindings and behavior.

<!-- field: context -->
#### Context

One Canonical Home forbids application forks, yet applications need local collections, endpoints, model choices, request servers, and packaging paths that do not belong in a reusable catalog profile.

<!-- field: problem -->
#### Problem

How do you vary deployment and configuration without copying canonical machines or inventing new semantic roles for each variant?

<!-- field: applicability -->
#### Applicability

Use this when the canonical realization already has the required behavior and only bindings or presentation differ. If the application changes the reusable responsibility or machine semantics, create a justified local realization or promote a new canonical member.

<!-- field: forces -->
#### Forces

- Canonical behavior must remain owned and tested upstream.
- Application-specific authority and endpoints must remain local.
- Variants need stable, descriptive identities.
- Role and sub-role bindings must not multiply with deployment forms.

<!-- field: solution -->
#### Solution

Create a thin application profile that selects the canonical machine or closure and supplies only application-owned configuration, endpoints, serving lifecycle, or packaging paths. Inherit the canonical role-realization binding. Name the wrapper by context, primary role, and optional deployment or configuration suffix under the semantic model's grammar.

<!-- field: implementation -->
#### Implementation

Keep wrappers under applications/<application>/agents/. Record the canonical inheritance target and reject cycles. Permit overrides of domain, execution scope, deployment form, and configuration; require an explicit responsibility justification before replacing role bindings. Package the wrapper and canonical assets in one closure without modifying the source checkout.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Applications vary configuration without forking reusable behavior.
    - Actor names communicate context while canonical role coverage remains stable.
    - Endpoint and deployment authority stays with the consuming application.
liabilities:
    - Deep wrapper chains can obscure the effective configuration.
    - An overly broad wrapper can become an ungoverned fork in disguise.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [one-canonical-home, composition-manifest]
requires: [one-canonical-home, composition-manifest, 'da:agent-as-data']
contains: []
enables: [package-time-closure, role-sharded-workload]
overlaps: [profile-free-runtime]
```

<!-- field: syntax -->
#### Syntax

Add a Parameterization Wrapper around the One Canonical Home, record it in the Composition Manifest, and resolve both into one closure.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Kustomize bases and overlays | kubernetes-kustomize | external | An application-specific overlay changes selected configuration while retaining a canonical base. |
| Decorator pattern | gamma-design-patterns-1994 | external | A wrapper adds responsibility around an existing component without copying or replacing its core implementation. |
| chatbot-mesh corpus-ingest wrapper | reference-implementation | internal | The mesh selects the canonical ingest machine and adds local REST, model, and collection configuration. |
| coding-agent serving profiles | reference-implementation | internal | Server and request profiles inherit canonical planner, executor, and critic realizations while adding application-owned serving behavior. |

<!-- pattern: role-sharded-workload | number: 7 -->
### Role-Sharded Workload

<!-- field: also_known_as -->
#### Also known as

- One Role per Workload
- Deployment Role Shard

<!-- field: confidence -->
#### Confidence

```yaml
2
```

<!-- field: intent -->
#### Intent

Run substantial agent roles in separate workloads so lifecycle, authority, resources, and evidence remain attributable.

<!-- field: context -->
#### Context

A composition contains several persistent or independently scalable realizations. Package-Time Closure and Profile-Free Runtime allow each workload to receive only its reachable program and select one entry profile.

<!-- field: problem -->
#### Problem

How do you deploy a multi-agent composition without turning all roles into one process with shared authority, coupled failure, and ambiguous health?

<!-- field: applicability -->
#### Applicability

Use this for independently addressable, scalable, privileged, or long-running roles. Keep tightly coupled finite child runs inside a parent boundary when separate process lifecycle would add no isolation or operational value.

<!-- field: forces -->
#### Forces

- Authority and workspace access should narrow to each role.
- Health, traces, failures, and resource use must be attributable.
- Roles may scale or roll out independently.
- Excessive service boundaries increase latency and operational cost.

<!-- field: solution -->
#### Solution

Assign each substantial serving role its own workload, entry profile, reachable closure, mounts, service identity, probes, resources, and authority. Connect roles through declared agent boundaries and propagate trace context. Co-locate only when the loss of isolation is explicitly cheaper than another managed workload.

<!-- field: implementation -->
#### Implementation

Generate role manifests from the Composition Manifest and closure inventory. Mount only reachable files and required workspaces. Give each workload truthful role-owned readiness and lifecycle endpoints, not an unrelated sidecar probe. Scope credentials and network access by role, include closure checksums in rollout identity, and test the real inter-role path.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Least authority, independent rollout, and role-attributable evidence.
    - Failure and resource boundaries mirror the application composition.
    - Workloads can scale according to role-specific demand.
liabilities:
    - More workloads increase deployment, networking, and observability overhead.
    - Remote boundaries add latency and partial-failure modes.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [capability-ladder]
requires: [composition-manifest, package-time-closure, profile-free-runtime, 'da:boundary-tool']
contains: [orchestrate-act-split]
enables: [orchestrate-act-split]
overlaps: []
```

<!-- field: syntax -->
#### Syntax

Give each deployed role its own closure-backed, Profile-Free Runtime and connect Role-Sharded Workloads through declared Boundary Tools.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Microservices | newman-microservices-2021 | external | Independently deployable services isolate ownership, lifecycle, scaling, and failure around a bounded responsibility. |
| Principle of least privilege | saltzer-schroeder-1975 | external | Separate workloads can hold only the mounts, credentials, and network authority required by their role. |
| Kubernetes workload controllers | kubernetes-workloads | external | Deployments give independently managed processes explicit rollout, replica, probe, and resource boundaries. |
| coding-agent planner, executor, and critic | reference-implementation | internal | Kubernetes runs the three serving roles separately with role closures, services, health, workspace authority, and connected traces. |
| chatbot mesh | reference-implementation | internal | Chatbot, RAG, ingest, control-plane, actuation, and telemetry realizations run as separately governed workloads. |

<!-- pattern: orchestrate-act-split | number: 8 -->
### Orchestrate-Act Split

<!-- field: also_known_as -->
#### Also known as

- Control-Actuation Separation
- Decide Then Apply

<!-- field: confidence -->
#### Confidence

```yaml
0
```

<!-- field: intent -->
#### Intent

Separate intent sequencing from privileged actuation so the orchestrator cannot directly change deployment state.

<!-- field: context -->
#### Context

A Role-Sharded Workload topology includes a control plane that translates user intent into several dependent changes, some requiring deployment authority and rollback.

<!-- field: problem -->
#### Problem

How do you let an agent plan and coordinate a change without granting that same actor the credentials and mechanics to apply it?

<!-- field: applicability -->
#### Applicability

Use this when orchestration and actuation have materially different authority, audit, validation, or rollback requirements. Do not split a simple local action only to imitate a control plane. The current application-specific form is tentative because only chatbot-mesh ships it.

<!-- field: forces -->
#### Forces

- Intent sequencing needs broad system context but not deployment credentials.
- Actuation needs narrow privileged authority and deterministic validation.
- Decisions and applied effects need separate audit evidence.
- Extra boundaries add failure states and coordination cost.

<!-- field: solution -->
#### Solution

Put intent decomposition and sequencing in an orchestrator workload. Send a bounded, decided change request to a separate actuator that alone holds deployment authority, validates the request, applies it atomically, verifies rollout, and rolls back on failure. Return status through the declared boundary rather than sharing credentials or mutable control state.

<!-- field: implementation -->
#### Implementation

Define separate profiles, services, credentials, and network policy. Keep deployment clients out of the orchestrator's tool inventory. Give the actuator a closed request schema, idempotency or transaction boundary, preflight validation, rollout observation, and compensation path. Link traces across both workloads and retain the decision and effect records.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Compromise of the planner does not directly confer deployment authority.
    - Decision and actuation failures are independently diagnosable.
    - Rollback and rollout validation stay close to the privileged tools.
liabilities:
    - The extra remote boundary introduces latency and partial failure.
    - Request schema evolution must be coordinated across both workloads.
    - Evidence in this repository is presently limited to one application topology.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [role-sharded-workload]
requires: [role-sharded-workload, 'da:phase-scoped-toolset', 'da:boundary-tool']
contains: []
enables: []
overlaps: ['da:approval-gate']
```

<!-- field: syntax -->
#### Syntax

Within Role-Sharded Workloads, let the orchestrator decide and let a separately authorized actuator validate, apply, verify, and compensate.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Kubernetes controller and API server | burns-borg-omega-kubernetes-2016 | external | Controllers decide desired reconciliation while the API server mediates authorized state changes, a related but not identical split. |
| Command-query responsibility segregation | fowler-cqrs-2011 | external | Separating models by responsibility supports the decision/effect boundary, though it does not establish this agent-specific topology. |
| chatbot-mesh provisioning-workflow-orchestrator, creator, and applier | reference-implementation | internal | The provisioning-workflow-orchestrator sequences intent, the creator manages lifecycle, and the applier alone holds Helm and kubectl actuation; this is the full form, with a decision plane driving the actuator. |
| coding-agent applier | reference-implementation | internal | A second application realizes the actuator half without an in-cluster decision plane; an authorized operator or CI caller supplies the decided values patch, and the applier alone validates, applies, verifies the role rollouts, and compensates with a Helm rollback, holding Helm and kubectl actuation apart from the serving roles. |

<!-- pattern: persistent-application-platform | number: 9 -->
### Persistent Application Platform

<!-- field: also_known_as -->
#### Also known as

- Shared Cluster
- Retained-Data Application Rig

<!-- field: confidence -->
#### Confidence

```yaml
1
```

<!-- field: intent -->
#### Intent

Share platform infrastructure while preserving one application namespace, collector, bucket, lifecycle authority, and retained-data boundary.

<!-- field: context -->
#### Context

Several Helm-managed applications need the same ingress, object-storage endpoint, metrics, and storage provisioning. Their compute may be removed independently, while telemetry history must survive application and platform recreation.

<!-- field: problem -->
#### Problem

How do applications share expensive platform services without merging their lifecycle, identity, telemetry, or deletion authority?

<!-- field: applicability -->
#### Applicability

Use this when two or more applications run on one Kubernetes platform and retained data must outlive a release. Keep an owned cluster for a cluster-mutating scenario whose configuration cannot safely share the platform.

<!-- field: forces -->
#### Forces

- Shared infrastructure should be provisioned and diagnosed once.
- Application compute needs independent install, upgrade, and teardown.
- Retained data must not disappear with a namespace or cluster.
- Shared object storage must not erase application identity.
- Local emulation cannot claim cloud authorization it does not enforce.

<!-- field: solution -->
#### Solution

Run one persistent platform with shared external ingress and object service. Give each application one manifest-derived stable namespace, Helm release, collector gateway, and application-specific bucket. Separate ordinary down from explicit application purge and local platform reset. Route internal REST and OTLP through Service DNS, and restore retained query access by redeploying the application collector against the same bucket.

<!-- field: implementation -->
#### Implementation

Derive application identity and bindings from agents/application.yaml. Use a shared application-rig package for lifecycle mechanics and thin Mage verbs. Record immutable object identity in a collector-local WAL before upload. Keep external HTTP at Traefik and send internal REST, OTLP, object, and vector traffic directly to their declared endpoints. Enforce cloud isolation with workload identity and bucket IAM; document local fake GCS as non-adversarial configuration isolation only.

<!-- field: consequences -->
#### Consequences

```yaml
benefits:
    - Applications share platform cost without sharing compute lifecycle.
    - Telemetry survives collector, namespace, and cluster recreation.
    - Destructive authority is explicit and application scoped.
    - Local and cloud deployments retain one application declaration.
liabilities:
    - Shared-service failure can affect every dependent application.
    - A stopped application has retained objects but no live query API.
    - Local fake GCS cannot resist a malicious pod without a future authenticated gateway.
```

<!-- field: grammar -->
#### Grammar

```yaml
within: [capability-ladder]
requires: [composition-manifest, role-sharded-workload]
contains: []
enables: []
overlaps: [profile-free-runtime]
```

<!-- field: syntax -->
#### Syntax

Deploy each Role-Sharded Workload from its Composition Manifest onto a Persistent Application Platform, retaining one collector and bucket per application while shared services stay platform-owned.

<!-- field: examples -->
#### Examples

| system | cite | kind | note |
| --- | --- | --- | --- |
| Kubernetes namespaces and workload identity | kubernetes-workloads | external | Namespaces scope application resources while workload identity and provider IAM can scope access to application-owned cloud objects. |
| Twelve-Factor backing services | twelve-factor-build-release-run | external | A deploy can be replaced independently while a bound backing service retains data across releases. |
| Declarative Agents da-platform | reference-implementation | internal | The target contract shares Traefik and fake GCS while each application keeps its namespace, collector, bucket, and lifecycle authority. |

<!-- field: grammar_check -->
## Grammar check

```yaml
note: >
    Capability Ladder is the root. Every local pattern is connected through containment, prerequisites, or refinements, and all `da:` references resolve to the declared sibling language.
```

<!-- field: bibliography -->
## Bibliography

```yaml
reference-implementation:
    text: "Nokia Bell Labs. (2026). Declarative Agents repository: applications/catalog, applications/chatbot-mesh, applications/coding-agent, applications/agent-architecture, and shared application documents. Internal evidence, not an independent source."
operator-framework-capabilities:
    text: "Operator Framework Authors. (2024). Operator Capability Levels. Operator SDK Documentation."
    url: https://sdk.operatorframework.io/docs/overview/operator-capabilities/
champeon-progressive-enhancement-2003:
    text: "Champeon, S., & Finck, N. (2003). Inclusive Web Design for the Future with Progressive Enhancement. South by Southwest Interactive."
bazel-external-dependencies:
    text: "Bazel Authors. (2025). External Dependencies Overview. Bazel Documentation."
    url: https://bazel.build/external/overview
git-submodules:
    text: "Chacon, S., & Straub, B. (2014). Pro Git (2nd ed.), Git Tools: Submodules. Apress."
    url: https://git-scm.com/book/en/v2/Git-Tools-Submodules
hunt-thomas-pragmatic-1999:
    text: "Hunt, A., & Thomas, D. (1999). The Pragmatic Programmer: From Journeyman to Master. Addison-Wesley. (Don't Repeat Yourself.)"
oci-image-spec:
    text: "Open Container Initiative. (2024). OCI Image Format Specification, Version 1.1."
    url: https://github.com/opencontainers/image-spec
cyclonedx-spec:
    text: "OWASP Foundation. (2024). CycloneDX Bill of Materials Standard, Version 1.6."
    url: https://cyclonedx.org/specification/overview/
dolstra-nix-2006:
    text: "Dolstra, E. (2006). The Purely Functional Software Deployment Model. PhD thesis, Utrecht University."
bazel-runfiles:
    text: "Bazel Authors. (2025). Runfiles and Runtime Dependencies. Bazel Documentation."
    url: https://bazel.build/extending/rules#runfiles
twelve-factor-build-release-run:
    text: "Wiggins, A. (2017). The Twelve-Factor App: Strictly Separate Build and Run Stages."
    url: https://12factor.net/build-release-run
kubernetes-configmaps:
    text: "Kubernetes Authors. (2025). ConfigMaps. Kubernetes Documentation."
    url: https://kubernetes.io/docs/concepts/configuration/configmap/
kubernetes-kustomize:
    text: "Kubernetes Authors. (2025). Declarative Management of Kubernetes Objects Using Kustomize. Kubernetes Documentation."
    url: https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/
gamma-design-patterns-1994:
    text: "Gamma, E., Helm, R., Johnson, R., & Vlissides, J. (1994). Design Patterns: Elements of Reusable Object-Oriented Software. Addison-Wesley. (Decorator.)"
newman-microservices-2021:
    text: "Newman, S. (2021). Building Microservices (2nd ed.). O'Reilly Media."
saltzer-schroeder-1975:
    text: "Saltzer, J. H., & Schroeder, M. D. (1975). The Protection of Information in Computer Systems. Proceedings of the IEEE, 63(9), 1278-1308."
kubernetes-workloads:
    text: "Kubernetes Authors. (2025). Workloads. Kubernetes Documentation."
    url: https://kubernetes.io/docs/concepts/workloads/
burns-borg-omega-kubernetes-2016:
    text: "Burns, B., Grant, B., Oppenheimer, D., Brewer, E., & Wilkes, J. (2016). Borg, Omega, and Kubernetes. Communications of the ACM, 59(5), 50-57."
fowler-cqrs-2011:
    text: "Fowler, M. (2011). CQRS. martinfowler.com."
    url: https://martinfowler.com/bliki/CQRS.html
```
