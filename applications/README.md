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
  agent implementation. It references catalog members and reports reuse without
  adding those agents to repository totals.
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

## Current classification

- `chatbot-mesh/`: runnable application; managed service, packaged,
  Helm-managed, and kind-demo capabilities.
- `coding-agent/`: composition-only runnable application; managed service,
  packaged, Helm-managed, and kind-demo capabilities. Canonical planner,
  executor, and critic implementations remain in `catalog/`.
- `agent-architecture/`: composition-only runnable application; the
  `runnable_module` capability is implemented and its local `audit`, Go tests,
  and composition-only `stats` participate in root orchestration. Its
  `managed_service` capability is partial because live lifecycle, health,
  telemetry, and shutdown observations are dependency-gated. `packaged`,
  `helm_managed`, and `kind_demo` are declared and planned: a chart runs the
  documentation-curator and the collector from mounted catalog-owned closures,
  and a standalone declarative applier actuates it, specified in
  `srd001-helm-deployment` and `srd002-applier` with their resources landing in
  later sub-issues. The canonical documentation-curator remains in `catalog/`.
- `catalog/`: reusable catalog module, not a runnable application.

Root Mage orchestration is the source of truth for participating modules.
`magefiles/build.go` separates reusable submodules from runnable applications;
`mage audit`, `mage test`, and `mage stats` dispatch through that classification.
Application-local Mage targets provide the evidence behind each claimed
capability.
