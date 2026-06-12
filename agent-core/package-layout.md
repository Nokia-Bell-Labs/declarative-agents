# Package Layout

This repository is moving toward a domain-oriented Go package layout. The
current `pkg/` tree grew as a flat set of implementation packages; new work
should use `internal/` for application-private code unless a package is
intentionally supported as a public Go API.

## Ownership Rules

- `cmd/` remains the composition root for binaries.
- `internal/` contains private implementation packages for this repository.
- `pkg/` is reserved for stable, documented library APIs. If a package remains
  under `pkg/`, the reason should be explicit in the migration issue.
- `pkg/spec` is intentionally retained as a public package for the current
  restructuring. It provides typed specification artifacts, parsing, corpus
  loading, graph construction, validation, and formatted findings used by both
  planning and audit flows.
- `agents/`, `tools/`, `docs/`, and `testdata/` remain configuration,
  specification, and fixture directories rather than Go package domains.
- Each migration should preserve behavior first. Rename symbols or redesign APIs
  only in separate follow-up work.

## Target Domains

- `internal/runtime`: agent loop runtime, state machines, dispatch,
  checkpoints, rollback, and workspace refs.
- `internal/tools`: tool contracts, tool config/profile loading, and generic
  file, exec, lifecycle, validation, and LLM tool implementations. This domain
  currently contains the former standard tool library under
  `internal/tools/stl`.
- `internal/evaluation`: evaluator session/point runtime, result artifacts,
  metrics, convergence, trace analysis, and bench orchestration/UI support.
  This domain contains the former `pkg/bench` package under
  `internal/evaluation/bench`.
- `internal/model`: LLM clients, provider adapters, prompt rendering, model
  profiles, and tool manifest assembly.
- `internal/planning`: task extraction, spec graphs used for planning,
  implementation plans, issue materialization, and pipeline orchestration.
- `internal/audit`: constitution-auditor orchestration and audit-specific tool
  glue. Shared specification parsing and validation remain in `pkg/spec`.
- `internal/observability`: tracing ports, OpenTelemetry adapters, GenAI span
  helpers, and trace replay support. This domain currently contains the former
  `pkg/tracing` and `pkg/telemetry` packages.
- `internal/support`: private process, workspace, and CLI helper code. This
  domain currently contains the former `pkg/execute`, `pkg/subprocess`,
  `pkg/worktree`, and `pkg/cli` packages.

## Migration Order

1. Introduce the `internal/` skeleton and this ownership document.
2. Move observability first or alongside runtime, because runtime depends on
   tracing and telemetry types. Done: observability and support utilities now
   live under `internal/observability` and `internal/support`.
3. Move runtime/core packages under `internal/runtime`.
4. Move LLM and prompt packages under `internal/model`.
5. Move planning pipeline packages under `internal/planning`.
6. Split the standard tool library by domain before moving evaluator, audit, or
   model-specific tool implementations. Done: generic STL code now lives under
   `internal/tools/stl`, and evaluator session/point/result code lives under
   `internal/evaluation`.
7. Move the remaining bench runtime under `internal/evaluation`. Done: bench
   server, UI support, and bench-specific tools now live under
   `internal/evaluation/bench`.
8. Keep shared specification parsing and validation in `pkg/spec`; move only
   constitution-auditor-specific orchestration under `internal/audit`.
9. Update docs, build scripts, audit rules, and remove empty old package paths.

## Guardrails

- Keep each migration small enough to review as one domain move.
- Run `go test ./...` and `go vet ./...` after each migration.
- Avoid adding compatibility shims for unshipped package paths on the current
  branch; update imports directly unless a package is intentionally public.
- Do not move configuration YAML files as part of Go package moves unless the
  owning issue explicitly includes config layout.
