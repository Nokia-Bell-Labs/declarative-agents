# Declarative Decomposition Audit -- 2026-08-13

Register for recurring audit GH-1580. The audit examines production Go against
the repository's own thesis: agent-visible workflow behavior belongs in
declarative machines interpreted by `agent-core/cmd/agent`, not in bespoke
imperative orchestration.

This file is this run's durable output. Accepted findings are filed as separate
issues. Rejected candidates from GH-1395 are carried forward below; a later
recurrence must not refile them without new evidence.

Prior register: `agent-core/docs/declarative-decomposition-audit-2026-08.md`
(GH-1395). Remediations for that run closed under epic GH-1582 (PR #1601).

## Scope and gate

The eight-question gate is the one in GH-1580: contract scope, behavioral
equivalence, provisioning, existing tests, declarative visibility, compatibility
spike, exception accuracy, and net value. A candidate that fails any applicable
question is recorded as a rejected candidate, not filed.

Excluded from production decomposition findings: `magefiles/`, `_test.go`,
generated code, fixtures, and test-support packages. Test-only orchestration is
out of scope unless the harness itself is the product under audit.

Previous runs: GH-417 (2026-07-25), GH-890 (2026-07-27), GH-1105 (2026-08-03),
GH-1395 (2026-08-10). GH-1410 recorded that the GH-1105 externalization
heuristic was over-broad; this run uses the rewritten gate.

## Coverage

Production Go under audit, from `mage stats` plus a per-package line count
(excluding `_test.go`, `magefiles/`, `testdata/`):

| Module | Production Go lines | Delta vs GH-1395 |
|---|---|---|
| `agent-core` | 46,400 | +825 |
| `applications/catalog` | 1,741 | 0 |
| `applications/prose-editor` | 0 | -1,197 (removed by GH-1624) |
| Total | 48,141 | -372 |

`applications/agent-architecture`, `applications/chatbot-mesh`, and
`applications/coding-agent` still ship no production Go; they are
composition-only (or agent-YAML-only) modules whose Go is confined to
`magefiles/`.

## Executable inventory

Every production `package main` outside `magefiles/`, `testdata/`, and
`_test.go`. Confirmed by searching for `^package main` across all Go files and
subtracting the excluded directories. Two production executables remain.

| Executable | Files | Classification | Evidence |
|---|---|---|---|
| `agent-core/cmd/agent` | 8 | Product runtime -- the interpreter | `agent-core/cmd/agent/main.go` |
| `applications/catalog/cmd/catalog-test-evidence` | 2 | Build/test support | `applications/catalog/magefiles/test_evidence.go` is the only non-test invoker |

The GH-1395 inventory also listed `applications/prose-editor/cmd/prose-editor-tracer-boundary`.
That tree is gone (GH-1624 / PR #1626). It is not a candidate on this run.

### `agent-core/cmd/agent`

The interpreter. Not a candidate: the audit's thesis is that behavior belongs in
machines this binary interprets. Eight production files, one more than GH-1395:
`child_agent_config.go` maps a parsed `ChildAgentConfig` onto `execute.Config`
(`child_agent_config.go:10-17`). That is composition-root wiring, not a second
executable. Whether the binary has absorbed workflow policy is audited under
the runtime slice (GH-1634), not here.

### `applications/catalog/cmd/catalog-test-evidence`

Build/test support. The binary forwards `go test` JSON so the
specification-critic audit profile reads Go test evidence from a stable
test-binary path (`runner.go:156-190`). Its only non-test caller is
`applications/catalog/magefiles/test_evidence.go`. It ships in no runtime image
and no agent selects it.

Rejected as a candidate under gate question 1: no agent-visible operation is
hidden, because no agent invokes it. Same disposition as GH-1395; no new
evidence.

## Declared word inventory

103 unique words are declared under `agent-core/tools/`. The set is unchanged
from GH-1395. `mage audit` now reports 103 tool declarations (was 68 on
2026-08-10) because GH-1525 taught the corpus loader to traverse `includes:`
and subdirectory files (`pkg/spec/tool_declaration_loading.go:49-59`). Every
row below therefore reaches the audited corpus.

`invoke_llm` remains a per-profile declaration (`tools/builtin.yaml:30-32`),
not a shared `agent-core/tools` word. Profile-local LLM and application words
are not duplicated here; later slices name them when a package implements
them.

| Word | Type | Vis | Reversibility | In corpus | Source | Capability |
|---|---|---|---|---|---|---|
| `await_metrics` | builtin | internal | irreversible | yes | `tools/builtin/otlp/all.yaml` | Wait for and consume one complete FIFO metric batch from a named OTLP receiver. |
| `await_spans` | builtin | internal | irreversible | yes | `tools/builtin/otlp/all.yaml` | Wait for and consume one complete FIFO trace batch from a named OTLP receiver. |
| `build` | exec |  | reversible | yes | `tools/exec/build.yaml` | Compile all Go packages with go build ./... Returns compiler errors on failure. |
| `capture_planner_failure` | builtin | internal | reversible | yes | `tools/builtin/assemble-prompt.yaml` | Publish the preceding validation failure under a stable command-state label. |
| `check_agent_version` | builtin | internal | reversible | yes | `tools/builtin/check-agent-version.yaml` | Compare the configured harness agent version with the version reported in the point t... |
| `checkpoint_history` | builtin | internal | reversible | yes | `tools/builtin/checkpoint-history.yaml` | Read a run's execution history from the Dolt checkpoint backend. |
| `checkpoint_rollback` | builtin | internal | compensatable | yes | `tools/builtin/checkpoint-rollback.yaml` | Roll back a run to the last persisted step of a target iteration by reverting the Dol... |
| `collect_metrics` | builtin | internal | reversible | yes | `tools/builtin/collect-metrics.yaml` | Write evaluation metadata (exit code, duration, test results, tokens) to meta.json. |
| `collect_trace_tokens` | builtin | internal | reversible | yes | `tools/builtin/collect-trace-tokens.yaml` | Read the point trace file and record total GenAI input/output token usage. |
| `commit` | exec |  | compensatable | yes | `tools/exec/commit.yaml` | Create a git commit from staged changes. Fails if nothing is staged. |
| `copy_dir` | exec |  | reversible | yes | `tools/exec/copy-dir.yaml` | Copy a directory tree. Provide source and destination paths. |
| `create_point_dir` | builtin | internal | reversible | yes | `tools/builtin/create-point-dir.yaml` | Create the per-point evaluation directory and record trace artifact paths. |
| `diff_stat` | exec |  | reversible | yes | `tools/exec/diff-stat.yaml` | Show a summary of uncommitted changes (files changed, insertions, deletions). |
| `discover_suite_samples` | builtin | internal | reversible | yes | `tools/builtin/discover-suite-samples.yaml` | Discover evaluator samples from the parsed suite samples directory. |
| `done` | builtin | internal | reversible | yes | `tools/builtin/done.yaml` | Signal that the generation task is complete. |
| `dump_config` | builtin | internal | reversible | yes | `tools/builtin/dump-config.yaml` | Serialize the full experiment configuration (harness, model, tools, prompts) into exp... |
| `edit` | builtin |  | reversible | yes | `tools/builtin/edit.yaml` | Replace the first occurrence of an exact string in a file. Use read first to see the ... |
| `exit_agent` | builtin | internal | compensatable | yes | `tools/builtin/lifecycle/exit-agent.yaml` | Request a controlled agent exit through lifecycle vocabulary. |
| `expand_eval_grid` | builtin | internal | reversible | yes | `tools/builtin/expand-eval-grid.yaml` | Expand evaluator suite grid parameters into concrete grid points. |
| `extract_task` | builtin | internal | reversible | yes | `tools/builtin/extract-task.yaml` | Extract the next unblocked task from the dependency graph. |
| `find` | builtin |  | reversible | yes | `tools/builtin/find.yaml` | Search for text patterns in the workspace using ripgrep. The query is a regex, not a ... |
| `format_issue` | builtin | internal | reversible | yes | `tools/builtin/format-issue.yaml` | Format planner state as tracker-agnostic issue parameters. |
| `format_report` | builtin | internal | reversible | yes | `tools/builtin/format-report.yaml` | Format the validation results as a human-readable report. |
| `format_task_file` | builtin | internal | reversible | yes | `tools/builtin/execute-task.yaml` | Project the current plan into one profile-configured write request. |
| `git_init` | exec |  | reversible | yes | `tools/exec/git-init.yaml` | Initialize a new git repository in the current directory. |
| `init_eval_session` | builtin | internal | reversible | yes | `tools/builtin/init-eval-session.yaml` | Create the evaluator session output directory and resolve runtime defaults. |
| `lint` | exec |  | reversible | yes | `tools/exec/lint.yaml` | Run golangci-lint run ./... on the Go workspace. |
| `list_files` | exec |  | reversible | yes | `tools/builtin/list-files.yaml` | List files and directories in a tree format. Use this first to understand the workspa... |
| `list_resource` | builtin |  | reversible | yes | `tools/builtin/filesystem/all.yaml` | Shape externally discovered paths from a configured filesystem resource. |
| `load_corpus` | builtin | internal | reversible | yes | `tools/builtin/load-corpus.yaml` | Load the specification corpus from the project directory. |
| `load_graph` | builtin | internal | reversible | yes | `tools/builtin/load-graph.yaml` | Load the specification corpus and build the requirement dependency graph into pipelin... |
| `load_otlp_batch` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Read and decode one trusted OTLP protobuf-JSON trace batch. |
| `load_test_claims` | builtin | internal | reversible | yes | `tools/builtin/spec-validation/test-evidence.yaml` | Load formal test-suite claims without requiring a full specification corpus. |
| `log_oneline` | exec |  | reversible | yes | `tools/exec/log-oneline.yaml` | Show the last 10 commits as one-line summaries. |
| `make_dir` | exec |  | reversible | yes | `tools/exec/make-dir.yaml` | Create a directory and any missing parent directories. |
| `mark_nodes_executing` | builtin | internal | reversible | yes | `tools/builtin/execute-task.yaml` | Advance the selected task nodes to Executing. |
| `mark_nodes_planning` | builtin | internal | reversible | yes | `tools/builtin/extract-all.yaml` | Advance only the selected task nodes to Planning. |
| `mark_task_done` | builtin | internal | reversible | yes | `tools/builtin/mark-task-done.yaml` | Mark only the current planner task's graph nodes done. |
| `mark_task_failed` | builtin | internal | reversible | yes | `tools/builtin/mark-task-failed.yaml` | Mark only the current planner task's graph nodes failed after retry exhaustion. |
| `materialize_eval_points` | builtin | internal | reversible | yes | `tools/builtin/materialize-eval-points.yaml` | Materialize deterministic profile, grid, sample, and repetition combinations for decl... |
| `nudge_reread` | builtin | internal | reversible | yes | `tools/builtin/nudge-reread.yaml` | Append a re-read instruction after file edits to prompt the model to verify changes. |
| `otlp_receiver_launch` | builtin | internal | compensatable | yes | `tools/builtin/otlp/all.yaml` | Bind a declared OTLP/gRPC receiver for trace and metric exports and return without wa... |
| `otlp_receiver_stop` | builtin | internal | irreversible | yes | `tools/builtin/otlp/all.yaml` | Stop a named OTLP receiver, reject new exports, and unblock waiting commands. |
| `parse_plan` | builtin | internal | reversible | yes | `tools/builtin/parse-plan.yaml` | Parse the LLM YAML response into an ImplementationPlan. |
| `parse_response` | builtin | internal | reversible | yes | `tools/builtin/parse-response.yaml` | Parse raw LLM output into a tool call or task-completed signal. |
| `parse_structured` | builtin | internal | reversible | yes | `tools/builtin/parse-structured.yaml` | Parse selected model output as JSON and validate it against a declared JSON Schema. |
| `parse_suite_config` | builtin | internal | reversible | yes | `tools/builtin/parse-suite-config.yaml` | Read and validate evaluator suite YAML metadata without discovering samples or creati... |
| `partition` | builtin | internal | reversible | yes | `tools/builtin/partition.yaml` | Split an ordered array into matched and unmatched values using one declared field com... |
| `project_planner_context` | builtin | internal | reversible | yes | `tools/builtin/assemble-prompt.yaml` | Project task and SRD data without owning planner prompt wording. |
| `read` | builtin |  | reversible | yes | `tools/builtin/read.yaml` | Read a single file's contents. Path must point to a file, not a directory. Use find t... |
| `read_resource` | builtin |  | reversible | yes | `tools/builtin/filesystem/all.yaml` | Read one document from a configured filesystem resource. |
| `record_agent_commit` | builtin | internal | reversible | yes | `tools/builtin/record-agent-commit.yaml` | Record a configured rev_parse result in the current point context. |
| `record_oracle_result` | builtin | internal | reversible | yes | `tools/builtin/record-oracle-result.yaml` | Record the configured oracle exec result in the current point context. |
| `record_point_failure` | builtin | internal | reversible | yes | `tools/builtin/record-point-failure.yaml` | Project a failed point command result into failure stage and cause fields. |
| `record_tracker_issue` | builtin | internal | reversible | yes | `tools/builtin/record-tracker-issue.yaml` | Record an issue ID returned by the configured tracker exec word. |
| `reduce_consistency_checks` | builtin | internal | reversible | yes | `tools/builtin/spec-validation/reduce-consistency-checks.yaml` | Reduce externally loaded YAML and path inventory into consistency findings. |
| `reduce_grep_checks` | builtin | internal | reversible | yes | `tools/builtin/spec-validation/reduce-grep-checks.yaml` | Shape joined ripgrep events into deterministic jurist findings. |
| `reduce_ref_checks` | builtin | internal | reversible | yes | `tools/builtin/spec-validation/reduce-ref-checks.yaml` | Reduce joined external ref_check scans into deterministic findings. |
| `reduce_test_evidence_run` | builtin | internal | reversible | yes | `tools/builtin/spec-validation/test-evidence.yaml` | Reduce declared go test JSON events against resolved formal claims. |
| `relay_spans` | builtin | internal | irreversible | yes | `tools/builtin/otlp/all.yaml` | Export a selected complete trace batch unchanged to one trusted OTLP/gRPC endpoint. |
| `remaining_work` | builtin | internal | reversible | yes | `tools/builtin/remaining-work.yaml` | Query ready, completed, or blocked planner graph state. |
| `render_each` | builtin | internal | reversible | yes | `tools/builtin/render-each.yaml` | Render each value in an ordered array with one item template and join the parts. |
| `report_parse_error` | builtin | internal | reversible | yes | `tools/builtin/report-parse-error.yaml` | Report a parse error back to the LLM for correction. |
| `report_session` | builtin | internal | reversible | yes | `tools/builtin/report-session.yaml` | Print session summary with pass/fail/timeout counts and total duration. |
| `report_suite_summary` | builtin | internal | reversible | yes | `tools/builtin/report-suite-summary.yaml` | Report suite point count after config parsing, sample discovery, grid expansion, and ... |
| `reset_history` | builtin | internal | reversible | yes | `tools/builtin/reset-history.yaml` | Clear the conversation history and restart the LLM context. |
| `resolve_test_evidence` | builtin | internal | reversible | yes | `tools/builtin/spec-validation/test-evidence.yaml` | Resolve formal go_test claims against declared Go inventory outputs. |
| `rest_await_event` | builtin |  | reversible | yes | `tools/builtin/rest/all.yaml` | Await one inbound REST event from configured server sources. |
| `rest_client_await` | builtin |  | reversible | yes | `tools/builtin/rest/all.yaml` | Await completion of a configured asynchronous REST operation. |
| `rest_client_create` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Create a configured REST resource through trusted REST config. |
| `rest_client_delete` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Delete or deactivate a configured REST resource. |
| `rest_client_get` | builtin |  | reversible | yes | `tools/builtin/rest/all.yaml` | Read a configured REST resource through trusted REST config. |
| `rest_client_invoke` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Invoke a configured RPC-shaped REST operation. |
| `rest_client_send` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Start a configured asynchronous REST operation. |
| `rest_client_set` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Update a configured REST resource through trusted REST config. |
| `rest_server_await` | builtin |  | reversible | yes | `tools/builtin/rest/all.yaml` | Await inbound events from a configured REST server queue. |
| `rest_server_launch` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Launch configured REST server routes without blocking on requests. |
| `rest_server_stop` | builtin |  | compensatable | yes | `tools/builtin/rest/all.yaml` | Stop a configured REST server and drain or fail queued events. |
| `rev_parse` | exec |  | reversible | yes | `tools/exec/rev-parse.yaml` | Return the short hash of HEAD. |
| `run_agent` | builtin | internal | compensatable | yes | `tools/builtin/run-agent.yaml` | Run the agent binary on the prepared workspace, record exit code and timing, and emit... |
| `run_point` | builtin | internal | compensatable | yes | `tools/builtin/run-point.yaml` | Run the per-point evaluation pipeline via a nested core.Loop with the point machine. |
| `sample_docs` | builtin | internal | reversible | yes | `tools/builtin/sample-docs.yaml` | Report whether optional sample docs exist and expose copy_dir parameters when present. |
| `seed_passthrough_plan` | builtin | internal | reversible | yes | `tools/builtin/extract-all.yaml` | Seed the selected pass-through task with profile-configured plan text. |
| `select_all_ready` | builtin | internal | reversible | yes | `tools/builtin/extract-all.yaml` | Select all ready requirements as one pass-through task without changing graph lifecycle. |
| `select_subset` | builtin | internal | reversible | yes | `tools/builtin/select-subset.yaml` | Keep candidate names only when they occur in a declared vocabulary. |
| `spool_get_metric` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Read one bounded page of spooled records for one metric name from the NDJSON metric s... |
| `spool_get_trace` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Read all spans for one trace from the NDJSON spool, returning the fields a waterfall ... |
| `spool_list_metrics` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Read the NDJSON metric spool and return paginated metric summaries by name. |
| `spool_list_traces` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Read the NDJSON trace spool and return paginated trace summaries, newest first. |
| `spool_metrics` | builtin | internal | irreversible | yes | `tools/builtin/otlp/all.yaml` | Append a selected OTLP metric batch as complete NDJSON metric lines carrying resource... |
| `spool_span_breakdown` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Rank the attributes that most distinguish a selected span region from its complement. |
| `spool_span_group_by` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Filter spooled spans and count them by one requested key. |
| `spool_span_heatmap` | builtin | internal | reversible | yes | `tools/builtin/otlp/all.yaml` | Filter spooled spans and return a duration-over-time heatmap. |
| `spool_spans` | builtin | internal | irreversible | yes | `tools/builtin/otlp/all.yaml` | Append a selected OTLP batch as complete stdouttrace-compatible NDJSON span lines. |
| `stage_all` | exec |  | reversible | yes | `tools/exec/stage-all.yaml` | Stage all changes including untracked files (git add -A). |
| `summarize_point_results` | builtin | internal | reversible | yes | `tools/builtin/summarize-point-results.yaml` | Summarize previously collected point oracle, trace, and version state. |
| `suspend` | builtin | internal | compensatable | yes | `tools/builtin/suspend.yaml` | Suspend the run at an approval gate. The loop persists a checkpoint through the Check... |
| `test` | exec |  | reversible | yes | `tools/exec/test.yaml` | Run go test -count=1 on the workspace. Returns test output including pass/fail results. |
| `validate_specs` | builtin | internal | reversible | yes | `tools/builtin/validate-specs.yaml` | Run consistency checks on the loaded specification corpus. |
| `value_predicate` | builtin | internal | reversible | yes | `tools/builtin/value-predicate.yaml` | Compare two operands and emit one of two declared signals, so a machine can branch on... |
| `vet` | exec |  | reversible | yes | `tools/exec/vet.yaml` | Run go vet ./... on the workspace. Reports suspicious constructs. |
| `workspace_status` | exec |  | reversible | yes | `tools/exec/workspace-status.yaml` | Report git workspace state: changed files with status codes (M/A/D/??). |
| `write` | builtin |  | reversible | yes | `tools/builtin/write.yaml` | Create or overwrite a file. Provide the complete file content — this replaces the ent... |

### Corpus coverage

GH-1525 HELD. `discoverAndParseToolDeclarations` loads `tools/builtin.yaml`,
`tools/exec.yaml`, then every YAML file under `tools/builtin/` and
`tools/exec/`, following `includes:` recursively
(`tool_declaration_loading.go:16-40, 49-59`). The 34-word gap recorded in
GH-1395 is closed. `mage audit` reports 103 declarations, matching this
inventory.

Names that still appear in both an aggregator (`tools/builtin.yaml` /
`tools/exec.yaml`) and a leaf file are a load-order question for the tool-package
slice (prior GH-1547 / GH-1562), not a baseline executable finding.

## MachineSpec expressiveness baseline

What the machine format can express today. A decomposition finding that needs a
construct absent from this list must be filed as an expressiveness gap against
the format rather than routed around in Go.

Source of truth: `agent-core/docs/specs/config-formats/machine-format.yaml`
and `agent-core/internal/runtime/core/machine.go`.

| Construct | Support | Evidence |
|---|---|---|
| States with meaning and terminal run status | Yes | `machine.go:16-31`, `machine_state_spec.go:8-12` |
| Declared signals with trigger metadata | Yes | `machine.go:28`, `machine.go:33-40` |
| Transition table keyed by state and signal | Yes | `machine.go:125-140` |
| Named tool action | Yes | `machine-format.yaml:317-322` |
| LLM-selected dynamic dispatch (`$tool`) | Yes | `machine-format.yaml:324-330` |
| Terminal transition with no action | Yes | `machine-format.yaml:332-336` |
| Command-state step labels for stable addressing | Yes | `machine.go:134`, `machine-format.yaml:373-407` |
| Parameter sources via `$from(label).path` selectors | Yes | `machine-format.yaml:281-291` |
| Data-driven iteration (`for_each`) over selected items | Yes | `iterator_spec.go:16-25` |
| Bounded parallel iteration with `max_concurrency` | Yes | `iterator_spec.go:19-20`, `machine-format.yaml:298-300` |
| Fork-join with all-success/partial/failed/empty outcomes | Yes | `iterator_spec.go:27-40` |
| Iterator failure policy (`fail_fast`, `collect_all`) | Yes | `iterator_spec.go:21`, `machine-format.yaml:301-304` |
| Checkpoint and resume across iteration | Yes | `machine-format.yaml:356-362` |
| Budgets: iterations, tokens, duration, parse errors | Yes | `machine.go:97-103` |
| Declared per-command timeout | Yes (new vs GH-1395) | `machine.go:101`, `machine-format.yaml:130`, `machine_policy.go:31` |
| Declared `summary_signal` | Yes (new vs GH-1395) | `machine.go:24`, `machine-format.yaml:90-96` |
| Declared `resume_signal` | Yes (new vs GH-1395) | `machine.go:25`, `machine-format.yaml:98-104` |
| Transition `report_output` selector | Yes (new vs GH-1395) | `machine.go:135`, `machine-format.yaml:252-259` |
| Transition `summary` flag | Yes (new vs GH-1395) | `machine.go:136`, `machine-format.yaml:260-268` |
| Parse-retry routing with exhaustion signal | Yes | `report_parse_error` emits `BudgetExhausted` |
| Phase-scoped tool availability derived from transitions | Yes | `machine-format.yaml:410-442` |
| Static diagnostics for dead grammar | Yes | `machine-format.yaml:510-528` |
| Diagnostics for implicit summary/resume/timeout | Yes (new vs GH-1395) | `machine_diagnostics.go:16-18` |
| Workflow metric labels | Parsed and validated; format spec still `status: planned` | `machine.go:21,138`, `metric_config.go:71-80`, `machine-format.yaml:60-68,444-445` |
| Expressions, mutation, or dynamic action names | No, by design | `machine-format.yaml:369-371` |
| Nested programs inside `for_each` | No, by design | `machine-format.yaml:369-371` |

GH-1558 asked the format to declare six interpreter decisions. Five of those
fields now exist (`summary_signal`, `resume_signal`, `command_timeout`,
`report_output`, `summary`). Whether Go still applies undeclared fallbacks when
the fields are omitted is a runtime-slice question (GH-1634), not a format gap.

## Summary

This run re-audits production Go after the GH-1582 remediations and the
GH-1624 prose-editor removal. Slice sections below are filled as each
sub-issue completes.

| Slice | Issue | Scope | Status |
|---|---|---|---|
| Baseline | GH-1630 | executables, declarations, format | complete |
| REST and service | GH-1631 | `internal/tools/rest`, `internal/tools/service` | pending |
| Remaining tools | GH-1632 | ten focused tool packages | pending |
| Runtime and root | GH-1634 | runtime, cmd, support | pending |
| Spec and planning | GH-1633 | `pkg/spec`, `internal/planning` | pending |
| Telemetry and model | GH-1635 | evaluation, OTLP, observability, model | pending |
| Applications | GH-1636 | catalog Go and consolidation | pending |

## Accepted findings

Filed by this audit. Empty until later slices land.

| Issue | Axis | Target | Hidden contract boundary |
|---|---|---|---|

## Rejected candidates

Carried forward from GH-1395. This run reconfirmed the baseline row for
`catalog-test-evidence`. Later slices may append new rows; they must not
delete a carried row without new evidence.

Candidates considered and not filed, with the gate question each failed. A
later recurrence should not refile these without new evidence.

| Candidate | Slice | Failed question | Reason |
|---|---|---|---|
| Move `catalog-test-evidence` behind a declared word | Baseline | Q1 contract scope | Build/test support with no agent caller; its only non-test invoker is a Mage target. Excluded by the scope boundary. |
| Lower non-CIDR REST client operations to a `curl` exec word | REST/service | Q2 behavioral equivalence | GH-1385, already judged defective. Typed transport, the credential-scope gate (`client_target.go:195-214`), secret redaction out of error text (`client_response.go:238-251`), traceparent injection, and the staged error taxonomy do not survive a CLI boundary. |
| Replace the Go mock HTTP server with a bound mock CLI | REST/service | Q2 behavioral equivalence | GH-1386, already judged defective. `mock.go` is the srd039 fixture surface, loaded at server launch so a malformed fixture fails the launch. |
| Split `rest_server_stop` because it shuts down and drains | REST/service | Q1 contract scope | One shutdown transaction with one rollback boundary (relaunch). Drain counts are reporting, not a second selectable operation. |
| Decompose `doWithRetry` into machine states | REST/service | Q1 contract scope | Same-request transport retry inside one protocol transaction, sanctioned by srd028 R5.8 and the GH-1379 resolution. The cancellation defect is filed as GH-1529; the decomposition reading is not supported. |
| Decompose `awaitMatching` and `waitAnySource` loops | REST/service | Q1 contract scope | A single wait for one matching event, parking non-matching events so another filter can see them. No delay and no repeated domain operation. |
| Treat `handleMachineRequest` running a nested machine as hidden orchestration | REST/service | Q1 contract scope | This is the declarative answer, not a violation: the handler runs the MachineSpec the endpoint declares, and `validateMachineResponses` rejects a response map the machine cannot produce. |
| Treat the monitor view packages as an Article D4 violation | REST/service | Q7 exception accuracy | D4 governs documentation and never names monitor. The surface is specified by srd033 G1-G6, every view is a read, and which endpoints exist is profile config. The view vocabulary being a closed Go enum is noted but is the same pattern as every other closed set in the package. |
| Decompose the SIGTERM/bounded-wait/SIGKILL walk in `child.stop` | REST/service | Q1 contract scope | One atomic termination walk. |
| Externalize scenario directory traversal in `discovery.go` | REST/service | Q2 behavioral equivalence | Traversal inside one atomic discovery word. Same shape as GH-1384, already reversed. |
| Drive the serving-profile conformance harness through rest and service words | REST/service | Q1 contract scope | GH-1388, already judged defective. Replacing an independent observer with the system under test's own words makes conformance circular. |
| Replace `runOneValidator`'s child-agent spawn with a CLI | REST/service | Q2 behavioral equivalence | One process run plus result mapping through the shared `execute.RunAgent` path. The typed `ValidatorOutcome`, timeout enforcement, and OTLP endpoint propagation have no equivalent. |
| Split the twelve-way `init` switch in either package's `ExecuteContext` | REST/service | Q1 contract scope | `init` is bound per-ToolDef at factory time and is not agent-selectable. Standard builtin-registry shape. |
| Split `collect_scenario_verdict`, reused by four ToolDefs | REST/service | Q1 contract scope | The config selects reason text, not a distinct domain operation. All four record exactly one verdict, and each is separately declared with an `overlaps` note. |
| Move `compose` composition into MachineSpec | Tool packages | Q1 contract scope | It renders one template into one output and emits one signal. The alternative is the `carry_forward` chain srd038 replaced. |
| Convert `render_each` into a machine `for_each` | Tool packages | Q1 contract scope | It renders one string from one resolved array and dispatches nothing. `for_each` exists to dispatch a word per item, a different operation. |
| Split `read`'s `raw` flag or `read_resource`'s four modes | Tool packages | Q1 contract scope | Both select output format for one document read, not a distinct domain operation. The undeclared `raw` parameter is filed as a declaration gap in GH-1543. |
| Externalize `list_files`'s bash program | Tool packages | Q2 behavioral equivalence | Recorded exception at `list-files.yaml:143-148` from GH-1376, and re-litigating traversal externalization is what GH-1410 reversed. |
| Externalize `read` to `sed`/`nl`/`file` | Tool packages | Q8 net value | GH-1392 measured it: 0.13 ms/op in-process against a 2.1 ms/op single-fork floor, roughly 15x, paid on the highest-frequency word in a coding loop. Recorded exception at `read.yaml:79-89`. |
| Split `edit`'s `count != 1` branch | Tool packages | Q1 contract scope | Internal precondition validation on one atomic replacement; both outcomes are `ToolFailed` with different text. |
| Decompose `delay` into machine states | Tool packages | Q1 contract scope | One bounded, cancellation-aware wait with two declared outcomes. The machine already owns the retry loop -- srd040 R2.2 assigns the probe/delay/retry/timeout branches to MachineSpec, and the declaration says so. This is the pattern working. |
| Treat `self_invoke` as hidden child-tool dispatch | Tool packages | Q1 contract scope | One child agent process through the shared `execute.RunAgent` path, mapped to one signal. It dispatches no tools; the child's own machine does. Its declaration is wrong (GH-1544); its contract is not. |
| Split `invoke_llm`'s prompt assembly, history, or seed resolution | Tool packages | Q1 contract scope | `design-patterns/06-inference-boundary.md:35,39,56,114` places all of it behind one dispatch by design. One POST `/api/chat` per dispatch, no probe, no retry. |
| Split `value_predicate`, `partition`, or `select_subset` | Tool packages | Q1 contract scope | One comparison or membership test per dispatch. Multiple output arrays and multiple outcome signals are one result and its routing, not separable operations. |
| Split `validate_specs` into graph build and charter execution | Tool packages | Q1 contract scope | Two pure functions over an already-loaded corpus, with no independent outcome, no separate signal, and no branch between them. |
| Treat `load_corpus` lowering three charter kinds as compound | Tool packages | Q1 contract scope | The design assigns it: `jurist-charter-format.yaml:35-37,43-44`. Its undeclared *output* is a real defect (GH-1543); its contract scope is the specified one. |
| Lower the remaining `reduce_*` YAML evaluation to `yq`/`jq` | Tool packages | Q6 compatibility spike | Documented exception at `jurist-charter-format.yaml:43-50`, and GH-1101 permitted a Go reducer where line provenance cannot survive the CLI contract. Provenance needs `yaml.Node` positions, which value extraction discards. |
| Treat per-role factory family names as an Article D4 violation | Tool packages | Q7 exception accuracy | D4 governs documentation and Go binaries, not internal wiring struct fields. The families are init-name groups in one binary and every word stays profile-selected. The real defect in that file is list drift (GH-1548). |
| Externalize catalog's YAML loader | Tool packages | Q2 behavioral equivalence | Not attempted; same class as GH-1384. |
| "The dispatch loop is imperative Go" | Runtime | Q1 contract scope | The interpreter is supposed to be imperative. Only decisions a declaration should have made are findings. |
| Decompose the `for_each` join count-to-signal rules | Runtime | Q1 contract scope | The signal names are declared and validated; the aggregation rule is documented `for_each` semantics, not per-workflow policy. |
| Decompose the parallel worker pool, channels, and WaitGroup | Runtime | Q1 contract scope | `max_concurrency` is declared and validated. This is the bounded-parallelism implementation GH-1095 asked for. |
| Externalize the `DiagnoseMachineSpec` reachability walk | Runtime | Q1 contract scope | Static validation is a named benefit of the Machine Interpreter pattern. |
| Externalize the output-redaction path walk | Runtime | Q1 contract scope | Paths come from the tool's `Result.Redaction`; core only applies them. |
| Move Dolt SQL out of the checkpoint adapter | Runtime | Q1 contract scope | Adapter implementation behind the `Checkpoint` port, containing no state or signal literals. The terminal predicate is injected from the spec. |
| Treat `sql.Register("dolt", ...)` as a leak | Runtime | Q1 contract scope | Nineteen lines of textbook composition-root wiring. |
| Treat the `/opt/agent-core` prefix as policy | Runtime | Q1 contract scope | A deployment path constant, not workflow policy. |
| Treat `exec/procgroup.go` as a surviving duplicate | Runtime | Q1 contract scope | It is now a 27-line delegating alias, which is what GH-1393 asked for. A thin named seam is not a duplicate. The real residual is GH-1556. |
| Treat `os.ReadFile` of the machine and request file as externalizable | Runtime | Q1 contract scope | Interpreter preflight, the same carve-out GH-884 made for `--validate-config`. |
| Decompose `LoadCorpus`'s seven discovery passes | Spec/planning | Q1 contract scope | No independently meaningful intermediate: a corpus missing its use cases is not a state any machine routes on. Three declared words already own the load boundary (`load_corpus`, `load_test_suites`, `load_graph`). |
| Decompose `BuildGraph`'s six node and eleven edge passes | Spec/planning | Q1 contract scope | Pure in-memory construction of one artifact behind `validate_specs`. There is no partial graph a machine wants. |
| Decompose `Validate`'s 36 sequential checkers | Spec/planning | Q1 contract scope | Not a sequence of effects -- 36 independent pure functions over one graph, already individually selectable *from YAML* via charter `checks:`. The declarative selection the gate asks for exists. |
| Decompose `ExecuteCharters`' dispatch by check kind | Spec/planning | Q1 contract scope | The opposite of hidden workflow: three of four kinds deliberately return nil so the machine executes them as visible `rg`/scan states, with the reason written down at `charter_execute.go:39-48`. |
| Decompose finding sort/filter/format | Spec/planning | Q1 contract scope | Pure presentation over an in-memory slice, behind `format_report`. |
| Restructure the `Build*Plans`/`Reduce*` pairs | Spec/planning | Q1 contract scope | Already the target shape: lower policy to a plan, let the machine run the external search, reduce the captured output. `BuildGrepSearchPlans` explicitly does not read target files; `ReduceGrepSearch` explicitly never opens them. |
| Externalize Go-test evidence resolution (886 lines) | Spec/planning | Q1 contract scope | The profile already owns `go list` and `go test` as declared exec words; this is schema-aware reduction of their output. |
| Externalize `parse.go`'s YAML node walking or the `yaml_path` selector engine | Spec/planning | Q1 contract scope | Parsers. The charter queries are already declarative; making the interpreter declarative is not the goal. |
| Externalize `pkg/spec` discovery I/O to `find` | Spec/planning | Q2 behavioral equivalence | Same class as GH-1384. |
| Split `load_graph` because it loads a corpus and builds a graph | Spec/planning | Q1 contract scope | No machine can route on a loaded-but-ungraphed corpus, there is no sibling `build_graph` word, and the word exists to stop the machine dereferencing a nil graph. |
| Refile the stale `extract-all.yaml` / `execute-task.yaml` / `assemble-prompt.yaml` filenames | Spec/planning | No defect | The filenames are stale after GH-1088/1089/1091 but the contents were rewritten to declare the current words. No orphaned declaration exists. |
| Externalize the spool query and analytics engine to duckdb | Telemetry | Q2, Q3, Q4, Q6 -- all four | GH-1382's proposal. Not attempted, and not attemptable: duckdb is in no runtime image the audit could name, and the deterministic exemplar ordering, the divergence tie-break, the half-open duration-bucket rule with an overflow bucket, the `skipped_lines` malformed-line accounting, and the rotated-file discovery order are behavioral contracts that 34 existing tests assert directly. No equivalence matrix exists for any of them. |
| Delete `load_otlp_batch` as "cat" | Telemetry | Q1 contract scope | Not a byte copy: the protobuf-JSON decode *is* the trust-boundary check that makes a batch safe to hand to `relay_spans` (srd042 R3.10), and its separate `BatchLoaded` signal is what lets a machine distinguish a read/decode failure from an export failure. |
| Externalize spool writing and rotation to `stdouttrace` or an otelcol file exporter | Telemetry | Q6 compatibility spike | GH-1382 and GH-1387 combined. The spool word converts an *inbound protobuf* request into the stdout shape; `stdouttrace` serializes an *outbound SDK* span. No supported path exists between them without reconstructing SDK span objects, and the sort-stable attribute ordering and homogeneous-array typing are test-asserted. No provisioning answer for otelcol. |
| Collapse the evaluator's trace reader onto `tracetest.SpanStub` | Telemetry | Q6 compatibility spike | GH-1387 verbatim, already reversed. `eval_trace.go` is a deliberately partial, tolerant reader of *child agent* trace files whose producer version is not pinned to the evaluator's. Decoding into a versioned upstream struct would make an SDK bump a silent zero-span parse -- the exact failure GH-1387 was reversed for. |
| Probe Ollama through its CLI, or restore a preflight probe | Telemetry | Q2 behavioral equivalence | GH-1389, reversed: the CLI does not consume the profile's resolved `provider_url`. GH-1375 moved the probe to a declared REST word that does. The correct residual is deleting the unreachable `checkModel` (GH-1574), not re-adding a probe. |
| Externalize spool file discovery to `find` or `ls` | Telemetry | Q2 behavioral equivalence | Portable `os.Stat` inside one atomic read word. Same shape as GH-1384. |
| Split `await_spans` because it waits, decodes, and computes metadata | Telemetry | Q1 contract scope | One queue read producing one result. The metadata is projection of the batch it just took, and each field is declared. Three outcome signals is routing, not compounding. |
| Split `otlp_receiver_launch` because it binds a listener and registers two services | Telemetry | Q1 contract scope | One listener serving two OTLP signals on one port, as the declaration says. One bind, one rollback boundary. |
| Split `spool_spans` into encode and append | Telemetry | Q1 contract scope | Encoding is the append's payload. No state between them and no signal a machine could route on. |
| Decompose the receiver's overflow policy or the graceful-then-force stop walk | Telemetry | Q1 contract scope | GH-1382 recorded the overflow half as an exception because back-pressure is coupled to the machine's consumption rate. The stop is one bounded termination walk, the same shape already rejected for `child.stop`. |
| Treat the `provider != "ollama"` rejection as a closed set leaked into Go | Telemetry | Q7 exception accuracy | The documented adapter contract: `06-inference-boundary.md:112` says a new provider requires a new adapter behind the existing interface. A rejected unknown provider is the boundary working. |
| Treat the four standard dispatch metrics as policy in Go | Telemetry | Q1 contract scope | Runtime-owned dispatch instrumentation, explicitly modeled as such, and tool-supplied bindings extend it through `RecorderConfig.Bindings`. The mechanism for profile-supplied metrics exists and is used. |
| Treat `internal/observability` as carrying an application-specific concern | Telemetry | Q7 exception accuracy | Same rigor as the REST monitor question, same answer. `telemetry` is OTel setup and W3C traceparent, `tracing` is a four-method port plus a noop, `genai` is a semconv constant table, `monitor` is a schema-validated store whose vocabulary comes from `RecorderConfig`. The two application-flavored `Snapshot` booleans are dead code (GH-1574), not a D4 violation. |
| Treat the OTLP and monitor timeout/limit defaults as policy leaks | Telemetry | Q1 contract scope | Each is a fallback for a value the declaration exposes and the shipped declarations set. A declared knob with a Go default is the pattern working -- which is why GH-1570's ten-minute point timeout *is* filed: it is not exposed in the config block at all. |
| Drive the serving-profile conformance harness through rest and service words | Applications | Q1 contract scope | GH-1388, already reversed. Replacing the independent process/HTTP observer with the system under test's own words makes conformance circular and discards the process-death watchdog at `conformance/serve.go:136-140`, `:180-193`. The file states this itself at `:29-32`. |
| Audit `conformance/harness.go`, `otel.go`, `dolt.go`, `ollama.go` | Applications | Q1 contract scope | Test-support harness; the harness is not the product under audit. Every entry point takes `*testing.T`. |
| Audit `catalogroot/root.go` and `agentbuild/build.go` | Applications | Q1 contract scope | Build/test support. `catalogroot` has only magefile callers; `agentbuild` is one `go build` shared by a magefile and the harness. |
| Externalize the prose-editor tracer boundary | Applications | Q5 declarative visibility, inverted | Already bound as six atomic exec ToolDefs. GH-1575 and GH-1576 refine *what* crosses the boundary; they do not propose replacing it. |
| Replace the `serve` fixture double inside the tracer binary | Applications | Q2 behavioral equivalence | The deterministic model and RAG stub for a hermetic gate, invoked only from `magefiles/integration_tracer.go:512`. No equivalence or provisioning story for replacing it. Its co-location with product code is noted in GH-1578, not filed as decomposition. |
| Externalize the prose-editor tracer boundary | Baseline | Q5 declarative visibility, inverted | Already bound declaratively as six atomic exec ToolDefs with declared side effects, reversibility, and undo. |
| Treat the tool-contract completeness gap as a decomposition finding | Baseline | Q1 contract scope | A validation-coverage defect, not hidden workflow. Filed as GH-1525 and carried as repository work in this epic. |

### Defective findings from GH-1105 -- never refile

GH-1410 reviewed the merged remediation of the 2026-08-03 run and identified
these as defective. They are recorded so a later recurrence recognizes a repeat
proposal.

| Issue | Proposal | Why it was wrong |
|---|---|---|
| GH-1382 | Externalize OTLP spool query to duckdb and rotation to otelcol | Dependencies absent from the runtime images; equivalence for the spool contract never demonstrated. |
| GH-1384 | Externalize directory traversal to `find` | Replaced portable `os.ReadDir` with undeclared `exec.Command("find")`, which is not declarative externalization and cost portability. |
| GH-1385 | Lower non-CIDR REST client operations to a `curl` exec word | Typed transport, security policy, receipts, and error taxonomy do not survive the substitution. |
| GH-1386 | Replace the Go mock HTTP server with a bound mock CLI | Fixture compatibility not established; the mock is test support. |
| GH-1387 | Decode conformance traces with the upstream `SpanStub` type | Added an OTel SDK dependency and 200+ lines while still mirroring the wire format and silently skipping drifted spans. |
| GH-1388 | Drive the serving-profile conformance harness through rest and service words | Replaces an independent observer with the system under test's own words, making conformance circular. |
| GH-1389 | Probe Ollama through the CLI | The CLI does not consume the profile's resolved `provider_url`, so the probe would not test the configured endpoint. |

## Slice sections

Each audit slice appends its section below.

### Baseline -- GH-1630

Complete. Rebuilt the executable inventory, the declared-word inventory, and
the expressiveness baseline above. Filed no decomposition findings: the two
production executables are the interpreter and a build/test-support shim, and
neither passes the gate as hidden orchestration. The third GH-1395 executable
was removed with the prose-editor application.

GH-1525 HELD: the corpus loader traverses subdirectory declarations and
`includes:`, and `mage audit` reports 103 words, matching this inventory.

The format now declares the GH-1558 summary, resume, timeout, and
report-output fields. Workflow metric labels parse and validate in Go while
the format spec still marks them planned; that status lag is recorded in the
expressiveness table for the runtime slice rather than filed here, because no
agent-visible operation is hidden.

