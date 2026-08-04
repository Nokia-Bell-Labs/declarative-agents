# Prose Editor

## Purpose

Prose Editor is a planned declarative-agent application for editing a source
document without losing its meaning or provenance. The planned full scenario
captures an immutable GitHub original, improves structure, voice, and style,
obtains an independent critique, and publishes an accepted result once.

## Status

The module status is `audit_only`. The executable evidence added for release
`00.1` is limited to parsing the application manifest and documentation,
checking status consistency, and reporting composition statistics. No profile,
machine, tool, service, package, deployment, test fixture, or runnable
composition is implemented or claimed.

Release `00.1`, use case `rel00.1-uc001-tracer-saga`, and test suite
`test-rel00.1-prose-editor` remain planned and runtime-unimplemented. Promotion
to a runnable module requires a later evidence-backed status transition.

## Composition

`agents/application.yaml` is the composition authority. It declares seven
planned local roots: `workflow-orchestrator`, `specialist-editor`,
`voice-critic`, the `corpus-ingest` wrapper, and the `structure-rag`,
`voice-rag`, and `tightening-rag` instances. None of those paths exists as a
runtime profile.

The only canonical catalog reference is
`applications/catalog/agents/knowledge-manager/corpus-ingest/profile.yaml`,
pinned as compatible with `v0.20260803.0`. The reference remains planned in
this audit-only module and contributes no duplicate agent implementation.

## Capabilities

- `runnable_module`: `audit_only`
- `managed_service`: `planned`
- `packaged`: `planned`
- `helm_managed`: `planned`
- `kind_demo`: `planned`
- `ui`: `not_applicable`

These statuses describe future intent, not executable runtime evidence.

## Ownership Boundaries

Prose Editor owns its manifest, planned topology, application-specific
documentation, audit policy, and composition accounting. The catalog owns the
canonical corpus-ingest implementation. Agent-core owns profile, machine, tool,
REST, lifecycle, telemetry, checkpoint, and execution semantics.

No application actor currently owns runtime behavior. The planned
workflow-orchestrator alone would own saga and GitHub mutation, while the
planned voice critic alone would own consent-gated Pangram access. Those are
documented authority boundaries only.

## Planned Entry Points

The planned runtime entry points are the roots named in
`agents/application.yaml`. They are all marked `planned: true`; there is no
`run`, build, package, Helm, integration, or demo target.

The only current Mage entry points are governance surfaces:

- `mage audit`
- `mage stats`

`demo.yaml` declares only the optional catalog ownership root used by those
governance checks.

## Verification

From `applications/prose-editor`:

```text
go test ./...
mage audit
mage stats
```

The audit parses every local YAML document, validates the shared application
manifest contract, checks the exact canonical dependency, and rejects runtime
claims. Stats reports zero contributed agents and has no `agents` section.

## Documentation

The design extends shared contracts by reference; it does not copy or replace
them. Start with `docs/VISION.yaml`, `docs/ARCHITECTURE.yaml`,
`docs/SPECIFICATIONS.yaml`, and `docs/road-map.yaml`. Release `00.1` is defined
by `docs/specs/use-cases/rel00.1-uc001-tracer-saga.yaml` and
`docs/specs/test-suites/test-rel00.1-prose-editor.yaml`.
