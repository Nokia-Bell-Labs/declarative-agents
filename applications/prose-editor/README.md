# Prose Editor

## Purpose

Prose Editor is a planned declarative-agent application for editing a source
document without losing its meaning or provenance. The planned full scenario
captures an immutable GitHub original, improves structure, voice, and style,
obtains an independent critique, and publishes an accepted result once.

## Status

The module status remains `audit_only`: Prose Editor is not in the root runnable
or release registries. Release `00.1` now has an executable program closure for
the workflow orchestrator, structure-stage specialist editor, independent
critic, and structure-RAG wrapper. Structural tests load the profiles, prove
machine/tool/authority closure, resolve a portable manifest closure, and run
agent-core `--validate-config` for every local profile.

Release `00.1`, use case `rel00.1-uc001-tracer-saga`, and test suite
`test-rel00.1-prose-editor` are partial and structural-only. Deterministic
end-to-end fixture evidence, real boundary helpers, services, and root runnable
promotion remain separate work.

## Composition

`agents/application.yaml` is the composition authority. It declares four
executable local roots: `workflow-orchestrator`, `specialist-editor`,
`voice-critic`, and `structure-rag`.

The only canonical catalog reference is
`applications/catalog/agents/knowledge-manager/corpus-reader/profile.yaml`,
pinned as compatible with `v0.20260804.0`. The local structure-RAG profile is a
thin configuration wrapper over that canonical program.

## Capabilities

- `runnable_module`: `audit_only`
- `managed_service`: `not_applicable`
- `packaged`: `not_applicable`
- `helm_managed`: `not_applicable`
- `kind_demo`: `not_applicable`
- `ui`: `not_applicable`

The runnable baseline is not claimed until deterministic tracer execution
evidence exists.

## Ownership Boundaries

Prose Editor owns its manifest, tracer programs, application-specific
documentation, audit policy, and composition accounting. The catalog owns the
canonical corpus-reader implementation. Agent-core owns profile, machine, tool,
REST, lifecycle, telemetry, checkpoint, and execution semantics.

The workflow orchestrator alone declares workproduct mutation. The editor
returns candidate data, the critic returns verdict data, and the RAG wrapper is
read-only. Release `00.1` declares no voice/style stage, Pangram, GitHub
publication, Helm, kind, or applier authority.

## Planned Entry Points

The loadable program entry points are the four local roots named in
`agents/application.yaml`. There is no run, package, Helm, integration, or demo
target, and the application remains outside the root runnable registry.

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

The audit parses every local YAML document, validates and resolves the shared
application manifest, checks role and authority closure, stages the portable
profile closure, and validates all four local profiles through agent-core.
Stats reports three application-owned role realizations and one canonical
wrapper dependency without adding an `agents` total to the runnable registry.

## Documentation

The design extends shared contracts by reference; it does not copy or replace
them. Start with `docs/VISION.yaml`, `docs/ARCHITECTURE.yaml`,
`docs/SPECIFICATIONS.yaml`, and `docs/road-map.yaml`. Release `00.1` is defined
by `docs/specs/use-cases/rel00.1-uc001-tracer-saga.yaml` and
`docs/specs/test-suites/test-rel00.1-prose-editor.yaml`.
