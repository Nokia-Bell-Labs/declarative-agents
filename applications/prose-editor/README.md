# Prose Editor

## Purpose

Prose Editor is a planned declarative-agent application for editing a source
document without losing its meaning or provenance. The planned full scenario
captures an immutable GitHub original, improves structure, voice, and style,
obtains an independent critique, and publishes an accepted result once.

## Status

The module status is `implemented`: Prose Editor is in the root runnable and
release-test registries. Release `00.1` has an executable program closure for
the workflow orchestrator, structure-stage specialist editor, independent
critic, and structure-RAG wrapper. Structural tests load the profiles, prove
machine/tool/authority closure, resolve a portable manifest closure, and run
agent-core `--validate-config` for every local profile.

Release `00.1`, use case `rel00.1-uc001-tracer-saga`, and test suite
`test-rel00.1-prose-editor` are implemented through one deterministic,
interpreter-driven local tracer gate. The gate builds agent-core, starts the
shipped workflow-orchestrator profile, and observes its real `self_invoke`
executions of the shipped specialist-editor and voice-critic profiles.
Immutable fixtures supply only recorded source, RAG, and model boundaries while
the machines decide capture, candidate creation, criticism, bounded retry,
lineage, recovery, replay, and local finalization.
Managed services, production external boundaries, and publication are not
claimed.

## Composition

`agents/application.yaml` is the composition authority. It declares four
executable local roots: `workflow-orchestrator`, `specialist-editor`,
`voice-critic`, and `structure-rag`.

The only canonical catalog reference is
`applications/catalog/agents/knowledge-manager/corpus-reader/profile.yaml`,
pinned as compatible with `v0.20260804.0`. The local structure-RAG profile is a
thin configuration wrapper over that canonical program.

## Capabilities

- `runnable_module`: `implemented`
- `managed_service`: `not_applicable`
- `packaged`: `not_applicable`
- `helm_managed`: `not_applicable`
- `kind_demo`: `not_applicable`
- `ui`: `not_applicable`

The runnable baseline is limited to the deterministic Release `00.1` tracer.

## Ownership Boundaries

Prose Editor owns its manifest, tracer programs, application-specific
documentation, audit policy, and composition accounting. The catalog owns the
canonical corpus-reader implementation. Agent-core owns profile, machine, tool,
REST, lifecycle, telemetry, checkpoint, and execution semantics.

The workflow orchestrator alone declares workproduct mutation. The editor
returns candidate data, the critic returns verdict data, and the RAG wrapper is
read-only. Release `00.1` declares no voice/style stage, Pangram, GitHub
publication, Helm, kind, or applier authority.

## Run or Planned Entry Points

The loadable program entry points are the four local roots named in
`agents/application.yaml`. The only integration target is the hermetic tracer;
there is no run, managed-service, package, Helm, kind, or demo target.

Current Mage entry points are:

- `mage audit`
- `mage stats`
- `mage integration:tracer`
- `mage integration:all` (delegates only to `integration:tracer`)

`demo.yaml` declares only the optional catalog ownership root used by those
governance checks.

## Verification

From `applications/prose-editor`:

```text
go test ./...
mage audit
mage stats
mage integration:tracer
```

The audit parses every local YAML document, validates and resolves the shared
application manifest, checks role and authority closure, stages the portable
profile closure, and validates all four local profiles through agent-core.
Stats reports three application-owned role realizations in the runnable agents
total and one canonical wrapper dependency. The tracer records workflow and
child-machine terminals, `self_invoke` profiles, and every deterministic
boundary receipt; it records no publication, Pangram, voice/style, package,
Helm, kind, or Kubernetes action.

## Documentation

The design extends shared contracts by reference; it does not copy or replace
them. Start with `docs/VISION.yaml`, `docs/ARCHITECTURE.yaml`,
`docs/SPECIFICATIONS.yaml`, and `docs/road-map.yaml`. Release `00.1` is defined
by `docs/specs/use-cases/rel00.1-uc001-tracer-saga.yaml` and
`docs/specs/test-suites/test-rel00.1-prose-editor.yaml`.
