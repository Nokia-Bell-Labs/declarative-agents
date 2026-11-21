<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Statement coverage of the format references

This map takes each format reference under
`agent-core/docs/specs/config-formats/` that describes the language, plus the
application-profile manifest check and the agent operator, and maps it onto the
1.0 statement database in `language.yaml`. Each row of a table takes one
verdict:

- `covered` names the statements that state the rule.
- `detail` marks behaviour the language deliberately does not state, and names
  its kind: presentation, observability, engine policy, deployment, repository
  layout, tooling, or authoring guidance.
- `gap` marks a rule the engine has but no statement yet states, and names the
  issue that tracks it.

Navigation sections of a reference (purpose, examples, related documents,
implementation pointers, status ledgers, glossaries) carry no rule and have no
row. The jurist charter format (`jurist-charter-format.md`) describes
repository audit tooling rather than the language, and carries no table. The
audit checks this map: every id a `covered` row names resolves in
`language.yaml`, every `detail` row names its kind, and every `gap` row names
an issue.

A change that alters what a conforming document, processor, runtime, or
population must do lands its statement, fixture, and coverage-row edits in the
same pull request as the engine change.

Table 1: cfg-machine-format.

| Area | Verdict | Where |
|---|---|---|
| Action semantics (named tool, $tool, empty/omitted) | covered | R-MACH-011, R-MACH-012 |
| Schema: name | covered | R-MACH-001 |
| Schema: purpose, invariants, lifecycle, configuration | covered | R-MACH-003 |
| Schema: metric_labels, pipeline_diagram, view_tags | detail | presentation: presentation keynames; membership in the closed set is R-MACH-003 |
| Schema: initial_state | covered | R-MACH-006 |
| Schema: summary_signal, resume_signal | covered | R-MACH-009 |
| Schema: budget (max_iterations, max_tokens, max_duration, command_timeout, max_consecutive_parse_errors) | covered | R-MACH-008, R-MACH-010 |
| Schema: states (short and long form, run_status, tags) | covered | R-MACH-005, R-MACH-007 |
| Schema: terminal_states | covered | R-MACH-006, R-MACH-007 |
| Schema: signals (short and long form) | covered | R-MACH-005 |
| Schema: transitions (state, signal, next, action, label, report_output, summary) | covered | R-MACH-004, R-MACH-006, R-MACH-011, R-MACH-013 |
| Schema: transitions.metric_labels | detail | observability: planned per-transition metric labels |
| Schema: transitions.for_each | covered | R-MACH-016, R-MACH-017 |
| Data driven iteration (trust boundary, item binding, join, checkpoint resume, concurrency, non goal) | covered | R-MACH-016, R-MACH-017, R-MACH-018 |
| Command state step labels (purpose, addressing, persistence, relationship to metric labels, example) | covered | R-MACH-013, R-MACH-014, R-MACH-015 |
| State scoped dynamic dispatch (source of truth, manifest rule, validation rule, rejection signal, ownership) | covered | R-MACH-021, R-MACH-022 |
| Workflow metric labels | detail | observability: planned workflow metric labels |
| Machine presentation views | detail | presentation: the engine preserves view_tags and state tags without interpreting them |
| Validation rules (phase 1, phase 2, profile closure timeout inspection) | covered | R-MACH-006, R-MACH-007, R-MACH-011, R-MACH-012, R-MACH-013, R-MACH-016, R-MACH-017, R-MACH-019, R-MACH-020, R-MACH-021 |
| Diagnostics | detail | presentation: non-fatal warnings; the one promoted to a load error is R-MACH-020 |
| Stage fragments | covered | R-MODEL-004, R-MODEL-005, R-MODEL-008, R-MODEL-009, R-MODEL-010, R-MODEL-011, R-MACH-003 |
| Machine templates | covered | R-MACH-002, R-MODEL-003, R-MODEL-006, R-MODEL-007, R-MODEL-011 |

Table 2: cfg-agent-profile-format.

| Area | Verdict | Where |
|---|---|---|
| Schema | covered | R-PROF-001, R-PROF-004, R-PROF-005 |
| Path resolution | covered | R-PROF-006 |
| Blueprints | covered | R-PROF-002, R-PROF-003 |
| `name` optional on a profile | covered | R-PROF-001 |

Table 3: cfg-tool-declaration-format.

| Area | Verdict | Where |
|---|---|---|
| Builtin fields | covered | R-TOOL-002 |
| Document resource config | detail | engine policy: per-builtin configuration decoded by its own factory |
| Schema | covered | R-TOOL-002, R-TOOL-003, R-TOOL-004, R-TOOL-005, R-TOOL-008 |
| Metric declaration fields | detail | observability: planned metric declaration vocabulary |
| Child agent config | detail | engine policy: self_invoke alias configuration decoded by its factory |
| Exec fields | covered | R-TOOL-003, R-TOOL-004 |
| Parameter extensions | covered | R-TOOL-004 |
| Declaration files | detail | repository layout: where shipped declarations live |
| Fragment files | covered | R-TOOL-007, R-TOOL-008 |
| Library imports | covered | R-TOOL-008, R-PROF-005, R-PROF-006 |
| Inline pattern | detail | authoring guidance: the declaration and selection split; the split itself is R-TOOL-009 and R-TOOL-010 |
| Contract validation | detail | tooling: a corpus audit of declaration contracts, not a loader rule |
| Structured output strategy | covered | R-TOOL-004 |
| Mode local overrides | covered | R-TOOL-010 |
| Target grouped layout | detail | repository layout: planned layout migration |
| `phases` checked against the machine-profile at startup | covered | R-RUN-001 |

Table 4: cfg-tool-selection-format.

| Area | Verdict | Where |
|---|---|---|
| Schema | covered | R-TOOL-009, R-TOOL-010 |
| CLI mapping | detail | tooling: the `--profile` command-line surface |
| Selection files | detail | repository layout: inventory of shipped selection files |
| Grouped declaration loading | covered | R-TOOL-009, R-TOOL-010 |

Table 5: cfg-rest-tool-format.

| Area | Verdict | Where |
|---|---|---|
| Timeout envelope | detail | engine policy: timeouts, environment expansion, and retry aggregation |
| Schema | covered | R-REST-001, R-REST-003, R-REST-004, R-REST-005, R-REST-006, R-REST-007, R-REST-008 |
| Fragment files | covered | R-REST-002 |
| Library imports | covered | R-REST-002 |
| Service libraries | detail | repository layout: inventory of shipped REST units |
| Validation rules | covered | R-REST-001, R-REST-002, R-REST-003, R-REST-005, R-REST-006, R-REST-007, R-REST-008 |
| Invalid examples | covered | R-REST-003, R-REST-006, R-REST-007 |

Table 6: cfg-llm-config-format.

| Area | Verdict | Where |
|---|---|---|
| Schema | covered | R-LLM-001, R-LLM-002, R-LLM-003, R-LLM-004, R-LLM-005 |
| Provider behavior | detail | engine policy: provider adapters behind one grammar |
| Parse response config | detail | engine policy: parse_response configuration and parser-profile resolution |
| Per model pattern | detail | authoring guidance: per-model file layout |
| Config files | detail | repository layout: inventory of shipped model files |

Table 7: the application-profile manifest check (`magefiles/appmanifest`).

| Area | Verdict | Where |
|---|---|---|
| Manifest header and schema | covered | R-APP-001, R-APP-002 |
| Capabilities | covered | R-APP-003 |
| Roots | covered | R-APP-001, R-APP-004 |
| Runtime, deployment, UI, and package assets | detail | deployment: mount paths, the profile-free image, and asset staging |
| Package-time profile closure | detail | deployment: packaging of the profile closure; population closure is R-POP-001 |
| Templated fragment variants | detail | deployment: packaging of environment-templated fragments |

Table 8: cfg-runtime-contract.

| Area | Verdict | Where |
|---|---|---|
| Configuration authority | detail | authoring guidance: which repository corpus governs the contracts |
| Operator paths | detail | deployment: paths arrive as arguments, never from the environment |
| Invocation | covered | R-RUN-003, R-LCM-003, R-LCM-006 |
| Integration telemetry | detail | observability: OTLP export, resource identity, and service naming |
| Process result contract | covered | R-RUN-008, R-RUN-009 |
| Termination contract | covered | R-RUN-010 |
| Startup sequence | covered | R-RUN-001, R-RUN-002, R-RUN-003, R-RUN-007 |
| Tool parametrization | covered | R-RUN-011 |
| Suite input contract | detail | engine policy: how the evaluator tool interprets a suite request |
| LLM parser ownership | detail | engine policy: parser configuration of boundary tools |
| Nested machines | covered | R-RUN-002, R-RUN-011 |
| Child agent boundaries | covered | R-RUN-011 |
| Lifecycle | covered | R-LCM-003, R-LCM-004, R-LCM-005, R-RUN-007 |
| History and rollback profiles | covered | R-LCM-005, R-LCM-006 |
| Config file layout | detail | deployment: catalog directory layout |
| Container runtime layout | detail | deployment: image contents |
| Invariants | covered | R-RUN-001, R-RUN-002, R-RUN-012, R-LCM-006 |
| Runtime safety boundary | covered | R-RUN-005, R-RUN-006, R-LCM-004, R-RUN-012 |
| Engine policy audit | detail | engine policy: inventory of engine policy symbols; its normative content is the safety boundary row |
| Default SIGINT and SIGTERM before the run starts | covered | R-RUN-010 |
| Receipts handed unchanged to the producing tool's undo | covered | R-LCM-005 |
| Reverse order of the rollback walk | covered | R-LCM-005 |
| Exit code 1 for a run error | covered | R-RUN-008 |

Table 9: the agent operator (`operator/api`, `operator/internal/controller`).

| Area | Verdict | Where |
|---|---|---|
| Agent resource realization (srd009 R1-R2.3) | covered | R-POP-005, R-POP-007 |
| Profile validation before scheduling (srd009 R2.4) | covered | R-POP-006 |
| Status reporting (srd009 R3.1) and CRD installation (internal/crd) | detail | observability: Ready condition shape and CRD installation mechanics |
| One Agent resource per agent service across an application-profile | gap | no test asserts exactly one Agent resource per agent service; R-POP-005 states the per-resource half. Tracked in #2689 |

## Where a reference and the engine differ

The engine is normative for 1.0, so a statement records what the engine does.
These differences were found while building the map. The reference text is the
one to correct.

- `machine-format.md` says a machine-profile requires `name`. The loader does
  not check it; only the document rule R-MACH-001 binds it.
- `machine-format.md` implies a `max_duration` that does not parse is rejected.
  The engine ignores it.
- `runtime-contract.md` says the latest non-empty output is written as the
  terminal summary. The engine writes a summary only when the machine-profile
  declares `summary_signal` (R-RUN-009).
- `tool-declaration-format.md` documents an exec `includes` field. The code has
  none, and the loader rejects it.
- `llm-config-format.md` says `provider_url` is required with a dialect. The
  loader does not enforce it.
- `agent-profile-format.md` describes blueprints. The profile loader rejects
  the blueprint keynames (#2938).
