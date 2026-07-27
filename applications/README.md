# Applications

`applications/` contains runnable application modules and the repository's
reusable declarative catalog. These are different ownership classes.

## Directory classes

- A **runnable application** owns a composition that a developer or operator can
  execute. It owns application documentation, audit evidence, tests where code
  exists, and root orchestration participation.
- A **composition-only application** is runnable but contributes no canonical
  agent implementation. It references catalog members and reports reuse without
  adding those agents to repository totals.
- `applications/catalog/` is a **reusable catalog module**. It owns canonical
  declarative blocks and conformance evidence. It is not a runnable application.

The shared contract is in [`docs/`](docs/). Application-local architecture and
SRDs extend that contract with business behavior, topology, and evidence. They
cite the shared requirements instead of copying them.

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
- `knowledge-manager-demo/`: composition-only runnable application; the
  `runnable_module` capability is implemented and its local `audit`, Go tests,
  and composition-only `stats` participate in root orchestration. Its
  `managed_service` capability is partial because live lifecycle, health,
  telemetry, and shutdown observations are dependency-gated. `packaged`,
  `helm_managed`, and `kind_demo` are not applicable. The canonical
  documentation-curator remains in `catalog/`.
- `catalog/`: reusable catalog module, not a runnable application.

Root Mage orchestration is the source of truth for participating modules.
`magefiles/build.go` separates reusable submodules from runnable applications;
`mage audit`, `mage test`, and `mage stats` dispatch through that classification.
Application-local Mage targets provide the evidence behind each claimed
capability.
