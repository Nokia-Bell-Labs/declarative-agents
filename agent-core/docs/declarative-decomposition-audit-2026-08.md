# Declarative Decomposition Audit -- 2026-08

Register for recurring audit GH-1395. The audit examines production Go against
the repository's own thesis: agent-visible workflow behavior belongs in
declarative machines interpreted by `agent-core/cmd/agent`, not in bespoke
imperative orchestration.

This file is the audit's durable output. Accepted findings are filed as separate
issues; rejected candidates are recorded here with the gate question they failed
so a later recurrence does not refile them.

## Scope and gate

The audit applies the eight-question gate from GH-1395: contract scope,
behavioral equivalence, provisioning, existing tests, declarative visibility,
compatibility spike, exception accuracy, and net value. A candidate that fails
any applicable question is recorded as a rejected candidate, not filed.

Excluded from production decomposition findings: `magefiles/`, `_test.go`,
generated code, fixtures, and test-support packages. Test-only orchestration is
out of scope unless the harness itself is the product under audit.

Previous runs: GH-417 (2026-07-25), GH-890 (2026-07-27), GH-1105 (2026-08-03).
GH-1410 recorded that the GH-1105 externalization heuristic was over-broad and
rewrote the gate; this run uses the rewritten gate.

## Coverage

Production Go under audit, from `mage stats`:

| Module | Production Go lines |
|---|---|
| `agent-core` | 45,575 |
| `applications/catalog` | 1,741 |
| `applications/prose-editor` | 1,197 |
| Total | 48,513 |

`applications/agent-architecture`, `applications/chatbot-mesh`, and
`applications/coding-agent` ship no production Go; they are composition-only
modules whose Go is confined to `magefiles/`.

## Executable inventory

Every production `package main` outside `magefiles/`, `testdata/`, and
`_test.go`. The set was confirmed by searching for `^package main` across all
Go files and subtracting the excluded directories; 116 files declare
`package main`, of which 106 are Mage targets and 10 are production.

| Executable | Files | Classification | Evidence |
|---|---|---|---|
| `agent-core/cmd/agent` | 7 | Product runtime -- the interpreter | `agent-core/cmd/agent/main.go` |
| `applications/catalog/cmd/catalog-test-evidence` | 2 | Build/test support | `applications/catalog/magefiles/test_evidence.go` is the only non-test invoker |
| `applications/prose-editor/cmd/prose-editor-tracer-boundary` | 1 | Declared boundary process | Bound as exec ToolDefs in `applications/prose-editor/agents/workflow-orchestrator/declarations.yaml` |

### `agent-core/cmd/agent`

The interpreter. Not a candidate: the audit's thesis is that behavior belongs in
machines this binary interprets, so the binary itself is the intended
destination, not a target for externalization. Whether it has absorbed workflow
policy is a separate question, audited under the runtime slice rather than here.

### `applications/catalog/cmd/catalog-test-evidence`

Build/test support. The binary forwards `go test -json` invocations so the
specification-critic audit profile reads Go test evidence from a stable
test-binary path
(`applications/catalog/cmd/catalog-test-evidence/runner.go:156-190`). Its only
non-test caller is `applications/catalog/magefiles/test_evidence.go`. It ships
in no runtime image and no agent selects it.

Rejected as a candidate under gate question 1: no agent-visible operation is
hidden, because no agent invokes it. Recorded in the rejected register below.

### `applications/prose-editor/cmd/prose-editor-tracer-boundary`

A deterministic boundary process for the Release 00.1 tracer. It is the
positive case for the Tool Contract pattern rather than a violation: the
machine owns sequencing and the process is reached only through declared exec
ToolDefs, each one atomic operation with declared parameters, emitted signals,
side effects, reversibility tier, and undo strategy. `capture_source`,
`write_original`, `append_manifest_revision`, `write_structure_attempt`,
`write_critique_attempt`, and `materialize_final_chain` are separate words with
separate rollback boundaries
(`applications/prose-editor/agents/workflow-orchestrator/declarations.yaml:1-251`).

Rejected as a candidate under gate question 5, inverted: the declarative
binding the gate asks for already exists.

## Declared word inventory

102 unique words are declared under `agent-core/tools/`. They are the vocabulary
a finding must consider before proposing a new tool.

Note the "In audited corpus" column. Only 68 of the 102 reach the specification
corpus that `ValidateToolContracts` checks; the other 34 are declared in
subdirectories that the corpus loader does not traverse. See "Corpus coverage
gap" below.

| Word | Type | Vis | Reversibility | In audited corpus | Source | Capability |
|---|---|---|---|---|---|---|
| `read` | builtin | external | reversible | yes | `tools/builtin.yaml` | Read a single file's contents. Path must point to a file, not a directory. Use find to disco... |
| `write` | builtin | external | reversible | yes | `tools/builtin.yaml` | Create or overwrite a file. Provide the complete file content — this replaces the entire fil... |
| `edit` | builtin | external | reversible | yes | `tools/builtin.yaml` | Replace the first occurrence of an exact string in a file. Use read first to see the current... |
| `find` | builtin | external | reversible | yes | `tools/builtin.yaml` | Search for text patterns in the workspace using ripgrep. The query is a regex, not a glob — ... |
| `parse_response` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Parse raw LLM output into a tool call or task-completed signal. |
| `report_parse_error` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Report a parse error back to the LLM for correction. |
| `reset_history` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Clear the conversation history and restart the LLM context. |
| `nudge_reread` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Append a re-read instruction after file edits to prompt the model to verify changes. |
| `done` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Signal that the generation task is complete. |
| `suspend` | builtin | internal | compensatable | yes | `tools/builtin.yaml` | Suspend the run at an approval gate. The loop saves through the configured Checkpoint port w... |
| `extract_task` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Extract the next unblocked task from the dependency graph. |
| `select_all_ready` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Select all ready requirements as one pass-through task. |
| `seed_passthrough_plan` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Seed profile-configured pass-through plan text. |
| `mark_nodes_planning` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Advance only selected task nodes to Planning. |
| `project_planner_context` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Project prompt-neutral task and SRD context. |
| `capture_planner_failure` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Publish preceding validation output for retry prompt composition. |
| `parse_plan` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Parse the LLM YAML response into an ImplementationPlan. |
| `mark_nodes_executing` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Advance selected task nodes to Executing. |
| `format_task_file` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Project the current plan into write parameters for doc/task.yaml. |
| `mark_task_done` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Mark the current planner task's graph nodes done. |
| `mark_task_failed` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Mark the current planner task's graph nodes failed after retry exhaustion. |
| `remaining_work` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Query whether the planner graph has ready, completed, or blocked work. |
| `parse_suite_config` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Read and validate evaluator suite YAML metadata without discovering samples or creating sess... |
| `discover_suite_samples` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Discover evaluator samples from the parsed suite samples directory. |
| `expand_eval_grid` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Expand evaluator suite grid parameters into concrete grid points. |
| `init_eval_session` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Create the evaluator session output directory and resolve runtime defaults. |
| `report_suite_summary` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Report suite point count after config parsing, sample discovery, grid expansion, and session... |
| `materialize_eval_points` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Materialize deterministic profile, grid, sample, and repetition combinations for declared it... |
| `run_point` | builtin | internal | compensatable | yes | `tools/builtin.yaml` | Run the per-point evaluation pipeline via a nested core.Loop with the point machine. |
| `report_session` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Print session summary with pass/fail/timeout counts and total duration. |
| `create_point_dir` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Create the per-point evaluation directory and record trace artifact paths. |
| `sample_docs` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Report optional sample docs and expose shared copy_dir parameters when present. |
| `run_agent` | builtin | internal | compensatable | yes | `tools/builtin.yaml` | Run the agent binary on the prepared workspace and collect exit code, timing, and output. |
| `record_oracle_result` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Record the configured oracle exec result in the current point context. |
| `collect_trace_tokens` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Read the point trace file and record total GenAI input/output token usage. |
| `check_agent_version` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Compare the configured harness agent version with the version reported in the point trace. |
| `summarize_point_results` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Summarize previously collected point oracle, trace, and version state. |
| `record_point_failure` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Project a failed point command result into failure stage and cause fields. |
| `collect_metrics` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Write evaluation metadata (exit code, duration, test results, tokens) to meta.json. |
| `record_agent_commit` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Record a configured rev_parse result in the current point context. |
| `dump_config` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Serialize the full experiment configuration (harness, model, tools, prompts) into experiment... |
| `load_corpus` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Load the specification corpus from the project directory. |
| `validate_specs` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Run consistency checks on the loaded specification corpus. |
| `format_report` | builtin | internal | reversible | yes | `tools/builtin.yaml` | Format the validation results as a human-readable report. |
| `checkpoint_history` | builtin | internal | reversible | yes | `tools/builtin/checkpoint-history.yaml` | Read a run's execution history from the Dolt checkpoint backend. |
| `checkpoint_rollback` | builtin | internal | compensatable | yes | `tools/builtin/checkpoint-rollback.yaml` | Roll back a run to a target step by reverting the Dolt branch and replaying receipts. |
| `list_resource` | builtin | external | reversible | **no** | `tools/builtin/filesystem/all.yaml` | Shape externally discovered paths from a configured filesystem resource. |
| `read_resource` | builtin | external | reversible | **no** | `tools/builtin/filesystem/all.yaml` | Read one document from a configured filesystem resource. |
| `format_issue` | builtin | internal | reversible | yes | `tools/builtin/format-issue.yaml` | Format planner state as tracker-agnostic issue parameters. |
| `exit_agent` | builtin | internal | compensatable | **no** | `tools/builtin/lifecycle/exit-agent.yaml` | Request a controlled agent exit through lifecycle vocabulary. |
| `list_files` | exec | external | reversible | yes | `tools/builtin/list-files.yaml` | List files and directories in a tree format. Use this first to understand the workspace layo... |
| `load_graph` | builtin | internal | reversible | yes | `tools/builtin/load-graph.yaml` | Load the specification corpus and build the requirement dependency graph into pipeline state. |
| `otlp_receiver_launch` | builtin | internal | compensatable | **no** | `tools/builtin/otlp/all.yaml` | Bind a declared OTLP/gRPC receiver for trace and metric exports and return without waiting f... |
| `await_spans` | builtin | internal | irreversible | **no** | `tools/builtin/otlp/all.yaml` | Wait for and consume one complete FIFO trace batch from a named OTLP receiver. |
| `load_otlp_batch` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Read and decode one trusted OTLP protobuf-JSON trace batch. |
| `spool_spans` | builtin | internal | irreversible | **no** | `tools/builtin/otlp/all.yaml` | Append a selected OTLP batch as complete stdouttrace-compatible NDJSON span lines. |
| `relay_spans` | builtin | internal | irreversible | **no** | `tools/builtin/otlp/all.yaml` | Export a selected complete trace batch unchanged to one trusted OTLP/gRPC endpoint. |
| `otlp_receiver_stop` | builtin | internal | irreversible | **no** | `tools/builtin/otlp/all.yaml` | Stop a named OTLP receiver, reject new exports, and unblock waiting commands. |
| `spool_list_traces` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Read the NDJSON trace spool and return paginated trace summaries, newest first. |
| `spool_get_trace` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Read all spans for one trace from the NDJSON spool, returning the fields a waterfall UI needs. |
| `spool_span_stats` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Filter spooled spans and return a duration-over-time heatmap and group-by counts. |
| `spool_span_breakdown` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Rank the attributes that most distinguish a selected span region from its complement. |
| `await_metrics` | builtin | internal | irreversible | **no** | `tools/builtin/otlp/all.yaml` | Wait for and consume one complete FIFO metric batch from a named OTLP receiver. |
| `spool_metrics` | builtin | internal | irreversible | **no** | `tools/builtin/otlp/all.yaml` | Append a selected OTLP metric batch as complete NDJSON metric lines carrying resource, scope... |
| `spool_list_metrics` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Read the NDJSON metric spool and return paginated metric summaries by name. |
| `spool_get_metric` | builtin | internal | reversible | **no** | `tools/builtin/otlp/all.yaml` | Read all spooled records for one metric name from the NDJSON metric spool. |
| `parse_structured` | builtin | internal | reversible | yes | `tools/builtin/parse-structured.yaml` | Parse selected model output as JSON and validate it against a declared JSON Schema. |
| `partition` | builtin | internal | reversible | yes | `tools/builtin/partition.yaml` | Split an ordered array into matched and unmatched values using one declared field comparison. |
| `record_tracker_issue` | builtin | internal | reversible | yes | `tools/builtin/record-tracker-issue.yaml` | Record an issue ID returned by the configured tracker exec word. |
| `render_each` | builtin | internal | reversible | yes | `tools/builtin/render-each.yaml` | Render each value in an ordered array with one item template and join the parts. |
| `rest_client_get` | builtin | external | reversible | **no** | `tools/builtin/rest/all.yaml` | Read a configured REST resource through trusted REST config. |
| `rest_client_set` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Update a configured REST resource through trusted REST config. |
| `rest_client_create` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Create a configured REST resource through trusted REST config. |
| `rest_client_delete` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Delete or deactivate a configured REST resource. |
| `rest_client_invoke` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Invoke a configured RPC-shaped REST operation. |
| `rest_client_send` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Start a configured asynchronous REST operation. |
| `rest_client_await` | builtin | external | reversible | **no** | `tools/builtin/rest/all.yaml` | Await completion of a configured asynchronous REST operation. |
| `rest_server_launch` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Launch configured REST server routes without blocking on requests. |
| `rest_server_await` | builtin | external | reversible | **no** | `tools/builtin/rest/all.yaml` | Await inbound events from a configured REST server queue. |
| `rest_await_event` | builtin | external | reversible | **no** | `tools/builtin/rest/all.yaml` | Await one inbound REST event from configured server sources. |
| `rest_server_stop` | builtin | external | compensatable | **no** | `tools/builtin/rest/all.yaml` | Stop a configured REST server and drain or fail queued events. |
| `select_subset` | builtin | internal | reversible | yes | `tools/builtin/select-subset.yaml` | Keep candidate names only when they occur in a declared vocabulary. |
| `reduce_consistency_checks` | builtin | internal | reversible | **no** | `tools/builtin/spec-validation/reduce-consistency-checks.yaml` | Reduce externally loaded YAML and path inventory into consistency findings. |
| `reduce_grep_checks` | builtin | internal | reversible | **no** | `tools/builtin/spec-validation/reduce-grep-checks.yaml` | Shape joined ripgrep events into deterministic jurist findings. |
| `reduce_ref_checks` | builtin | internal | reversible | **no** | `tools/builtin/spec-validation/reduce-ref-checks.yaml` | Reduce joined external ref_check scans into deterministic findings. |
| `load_test_claims` | builtin | internal | reversible | **no** | `tools/builtin/spec-validation/test-evidence.yaml` | Load formal test-suite claims without requiring a full specification corpus. |
| `resolve_test_evidence` | builtin | internal | reversible | **no** | `tools/builtin/spec-validation/test-evidence.yaml` | Resolve formal go_test claims against declared Go inventory outputs. |
| `reduce_test_evidence_run` | builtin | internal | reversible | **no** | `tools/builtin/spec-validation/test-evidence.yaml` | Reduce declared go test JSON events against resolved formal claims. |
| `value_predicate` | builtin | internal | reversible | yes | `tools/builtin/value-predicate.yaml` | Compare two operands and emit one of two declared signals, so a machine can branch on a valu... |
| `build` | exec | external |  | yes | `tools/exec.yaml` | Compile all Go packages with go build ./... Returns compiler errors on failure. |
| `vet` | exec | external |  | yes | `tools/exec.yaml` | Run go vet ./... on the workspace. Reports suspicious constructs. |
| `lint` | exec | external |  | yes | `tools/exec.yaml` | Run golangci-lint run ./... on the Go workspace. |
| `test` | exec | external |  | yes | `tools/exec.yaml` | Run go test -count=1 on the workspace. Returns test output including pass/fail results. |
| `copy_dir` | exec | external | reversible | yes | `tools/exec.yaml` | Copy a directory tree. Provide source and destination paths. |
| `make_dir` | exec | external | reversible | yes | `tools/exec.yaml` | Create a directory and any missing parent directories. |
| `git_init` | exec | external | reversible | yes | `tools/exec.yaml` | Initialize a new git repository in the current directory. |
| `stage_all` | exec | external | reversible | yes | `tools/exec.yaml` | Stage all changes including untracked files (git add -A). |
| `workspace_status` |  | external |  | yes | `tools/exec.yaml` | Report git workspace state: changed files with status codes (M/A/D/??). |
| `commit` | exec | external | compensatable | yes | `tools/exec.yaml` | Create a git commit from staged changes. Fails if nothing is staged. |
| `rev_parse` |  | external |  | yes | `tools/exec.yaml` | Return the short hash of HEAD. |
| `diff_stat` |  | external |  | yes | `tools/exec.yaml` | Show a summary of uncommitted changes (files changed, insertions, deletions). |
| `log_oneline` |  | external |  | yes | `tools/exec.yaml` | Show the last 10 commits as one-line summaries. |

### Corpus coverage gap

Two loaders read the same declaration files with different semantics.

The runtime loader resolves the `includes:` key, so a profile that loads
`agent-core/tools/builtin/all.yaml` receives every word the included
subdirectory files declare
(`agent-core/internal/tools/catalog/loading.go:127`,
`agent-core/internal/tools/catalog/tooldef.go:269`).

The specification corpus loader does neither. It globs `tools/builtin/*.yaml`
and `tools/exec/*.yaml` non-recursively
(`agent-core/pkg/spec/corpus.go:358-366`,
`agent-core/pkg/spec/profile_assets.go:75-82`), and its file model has no
`Includes` field (`agent-core/pkg/spec/tool_models.go:103-105`). So
`tools/builtin/all.yaml` contributes nothing to the corpus, and the 34 words
declared beneath it are never contract-completeness-checked.

`mage audit` reports 68 tool declarations for agent-core, which reconciles
exactly with the non-recursive glob and confirms the gap is live rather than
theoretical. The 34 skipped words are all 11 REST words, all 14 OTLP words, the
2 filesystem document-resource words, `exit_agent`, and 6 spec-validation
words.

Reconciling the count exposed a wider problem. `ValidateToolContracts` and
`ReviewToolAuthoring` have no non-test caller, and the public mirror
`checkSelectedToolContractCompleteness` iterates machine-derived tool
selections, which are empty for a corpus with no machines. So the completeness
check that `design-patterns/04-tool-contract.md:112` names as the enforcement
point runs on nothing this repository ships. Measured against
`missingToolContractFields`, 66 of the 102 declared words are incomplete.

This is a validation-coverage defect rather than a decomposition finding: no
agent-visible workflow is hidden, so it fails gate question 1 as an audit
finding. It is filed as GH-1525 and carried as repository work in this epic.

## MachineSpec expressiveness baseline

What the machine format can express today. A decomposition finding that needs a
construct absent from this list must be filed as an expressiveness gap against
the format rather than routed around in Go.

Source of truth: `agent-core/docs/specs/config-formats/machine-format.yaml`
and `agent-core/internal/runtime/core/machine.go`.

| Construct | Support | Evidence |
|---|---|---|
| States with meaning and terminal run status | Yes | `machine.go:24-25`, `machine_state_spec.go:8-12` |
| Declared signals with trigger metadata | Yes | `machine.go:26`, `machine.go:37-40` |
| Transition table keyed by state and signal | Yes | `machine.go:131-140` |
| Named tool action | Yes | `machine-format.yaml:277-283` |
| LLM-selected dynamic dispatch (`$tool`) | Yes | `machine-format.yaml:284-291` |
| Terminal transition with no action | Yes | `machine-format.yaml:292-297` |
| Command-state step labels for stable addressing | Yes | `machine.go:136`, `machine-format.yaml:333-368` |
| Parameter sources via `$from(label).path` selectors | Yes | `machine-format.yaml:299-302`, `tool-declaration-format.yaml:505` |
| Exec stdin sources via the same selector | Yes | `tooldef.go:53-54`, `tool-declaration-format.yaml:573-580` |
| Data-driven iteration (`for_each`) over selected items | Yes | `iterator_spec.go:16-25` |
| Bounded parallel iteration with `max_concurrency` | Yes | `iterator_spec.go:19-20`, `machine-format.yaml:323-328` |
| Fork-join with all-success/partial/failed/empty outcomes | Yes | `iterator_spec.go:27-40` |
| Iterator failure policy (`fail_fast`, `collect_all`) | Yes | `iterator_spec.go:21`, `machine-format.yaml:457-459` |
| Checkpoint and resume across iteration | Yes | `machine-format.yaml:316-322` |
| Budgets: iterations, tokens, duration, parse errors | Yes | `machine.go:100-105` |
| Parse-retry routing with exhaustion signal | Yes | `report_parse_error` emits `BudgetExhausted` |
| Phase-scoped tool availability derived from transitions | Yes | `machine-format.yaml:369-402` |
| Static diagnostics for dead grammar | Yes | `machine-format.yaml:470-491` |
| Expressions, mutation, or dynamic action names | No, by design | `machine-format.yaml:329-331` |
| Nested programs inside `for_each` | No, by design | `machine-format.yaml:329-331` |
| Workflow metric labels | Planned | `machine-format.yaml:404-405` |

Two capabilities deferred by earlier audits have since shipped: data-driven
iteration and fork-join (GH-883) and bounded parallel `for_each` (GH-1095).

## Accepted findings

Filed by this audit. Completed by the consolidation slice.

| Issue | Axis | Target | Hidden contract boundary |
|---|---|---|---|
| GH-1528 | compound-tool, orchestration | `internal/tools/rest` | Declared side effect and rollback receipt execute in a detached goroutine after the word returns |
| GH-1529 | orchestration | `internal/tools/rest` | Cancellation: dispatch cannot bound the effect it started |
| GH-1530 | expressiveness, visibility | `internal/tools/rest` | Emitted-signal contract unenforced between operation config and ToolDef |
| GH-1531 | visibility | `internal/tools/rest` | Undo declared noop while implemented; declared output keys the word never emits |
| GH-1532 | contract enforcement | `internal/tools/rest` | Approval boundary: declared auth gate with no enforcement code |
| GH-1533 | orchestration | `internal/tools/service` | Reaping the started process set: a rollback boundary no word owns |
| GH-1534 | compound-tool | `internal/tools/service` | Partial rollback boundary inside one word |
| GH-1535 | visibility | `internal/tools/service` | Published outputs the retry loop depends on are undeclared |
| GH-1536 | declared tool reuse | `internal/tools/service` | Registered inits with no declaration; a duplicate domain operation |
| GH-1539 | compound-tool, visibility | `internal/tools/lifecycle`, `exec` | The rollback contract's declared tiers are unreachable; compensation-required is indistinguishable from failure |
| GH-1540 | visibility, declared tool reuse | `internal/tools/catalog`, `exec`, `filesystem` | `undo.strategy` describes rollback it does not select |
| GH-1541 | visibility | `internal/tools/catalog` | Load gate: exposure, rollback tier, and phase availability unenforced |
| GH-1542 | visibility | `internal/tools/catalog` | Config keys a word does not understand are silently discarded |
| GH-1543 | visibility | `llm`, `validation`, `filesystem`, `control` | Output schemas and emitted-signal sets that describe a different word |
| GH-1544 | compound-tool | `internal/tools/control` | A rollback boundary the declaration denies exists |
| GH-1545 | visibility | `internal/tools/llm` | A declared guarantee with no reachable implementation |
| GH-1546 | declared tool reuse, expressiveness | `llm`, `control`, `exec`, `undo`, `filesystem` | Ownership boundary: profile policy compiled into the runtime |
| GH-1547 | declared tool reuse | `agent-core/tools` | Four words with two declarations each, resolved by load order |
| GH-1548 | declared tool reuse | `registry`, `exec`, `catalog` | Hand-maintained duplicates that have drifted from the code they mirror |
| GH-1549 | visibility | `internal/tools/compose` | A declared signal the dispatch path cannot deliver |
| GH-1550 | orchestration | `internal/runtime/core` | Resume restores control state but not the data channel or the declared parse budget |
| GH-1551 | orchestration | `internal/runtime/core` | A terminal state's success is decided by its spelling |
| GH-1552 | orchestration | `runtime/core`, `cmd/agent` | Which output is the run's answer is decided twice in Go |
| GH-1553 | orchestration | `cmd/agent` | An output-reporting decision keyed on a tool name |
| GH-1554 | orchestration, visibility | `cmd/agent` | Declared agent identity is never read |
| GH-1555 | orchestration, expressiveness | `cmd/agent` | The dispatch boundary bounds effect but not time |
| GH-1556 | declared tool reuse | `internal/tools/service`, `support` | Process-group semantics opted out of silently |
| GH-1557 | declared tool reuse | `internal/support/cli`, `runtime/core` | A dead package emitting flags that do not exist |
| GH-1558 | expressiveness | `machine-format.yaml` | Six interpreter decisions the format cannot declare |
| GH-1559 | visibility | `cmd/agent` | `--request` means different things by word, decided in Go |
| GH-1560 | compound-tool, orchestration | `internal/planning/pipeline` | A graph lifecycle transition invisible in two of three machines |
| GH-1561 | visibility | `internal/planning` | A declared batching capability nothing can set |
| GH-1562 | declared tool reuse, visibility | `agent-core/tools` | Twelve words with two contracts, one contradicting the undo the Go performs |
| GH-1563 | visibility | `pkg/spec` | Two advertised checks that cannot fire |
| GH-1564 | declared tool reuse | `pkg/spec`, `internal/planning` | An unwired validator; undeclared filesystem writes reachable from no word |
| GH-1565 | declared tool reuse | `internal/planning` | Output paths and a domain classification owned by Go |
| GH-1566 | visibility | `pkg/spec` | A validator that reports "missing" when it means "unparsed" |
| GH-1567 | compound-tool | `internal/tools/otlp` | A promised compensation the walk cannot reach; the listener stays bound |
| GH-1568 | visibility | `internal/tools/otlp` | Silent truncation behind a declaration promising completeness |
| GH-1569 | compound-tool | `internal/tools/otlp` | A declared non-goal ("does not delete") the code violates |
| GH-1570 | orchestration, visibility | `internal/evaluation` | A declared machine state performed inside another word |
| GH-1571 | visibility | `internal/tools/otlp` | An undeclared output contract and a field that cannot vary |
| GH-1572 | compound-tool | `internal/tools/otlp` | Two independent analytical results behind one signal |
| GH-1573 | declared tool reuse | `internal/evaluation` | Sample layout, convergence policy, and undeclared effects owned by Go |
| GH-1574 | declared tool reuse | `evaluation`, `otlp`, `observability`, `model` | Dead surface, incl. the probe GH-1375 made unreachable |

## Rejected candidates

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

## Enforcement of the ToolDef contract

Produced by the tool-package slice and recorded here because later slices and
later recurrences need it. Of ToolDef's 29 exported fields:

| Class | Count | Meaning |
|---|---|---|
| Enforced at startup | 12 | Several for presence only, not value |
| Validated only in a checker with no production caller | 9 | See GH-1525 |
| Read at runtime but never validated | 12 | A typo silently changes behavior |
| Never read by any code path | 8 leaf fields | Declared and inert |

What is actually guaranteed before a word can be dispatched: it has a name; its
type is one of three; a builtin has an `init` resolving to a registered factory;
an exec word has a binary; every `$from` selector parses; the precondition is a
known gate; the metric config is well-formed; every named machine action is
selected; a parse-retry budget is completely routed; every declared emitted
signal has a routable successor; and a word declared reversible-with-mutation
has some non-`noop` undo string.

Everything else in the six-section contract is documentation. That is the
through-line of GH-1541 through GH-1543.

## The loop decision table

Produced by the runtime slice. Every decision the dispatch loop and the
composition root make that is not read from the loaded MachineSpec, classified
as interpreter mechanism (legitimate) or workflow policy (a finding).

34 decisions total: 26 mechanism, 8 policy. The loop itself is in good shape.
The policy concentrates in two places -- the terminal-status and
summary-signal defaults, and `cmd/agent`'s `OnResult` hook.

The eight policy decisions, with the finding each became:

| Decision | Evidence | Finding |
|---|---|---|
| Terminal run status inferred from state spelling | `loop.go:160-169` | GH-1551 |
| `TaskCompleted` decides which output is the run summary | `loop_runner.go:95-100`, `:392-394` | GH-1552 |
| `OnResult` overwrites the summary from any non-empty output | `main.go:620-622` | GH-1552 |
| Suspend keyed on the signal `AwaitApproval` | `loop_runner.go:426` | GH-1558 |
| Resume defaults the re-entry signal to `Approved` | `resume.go:58-61`, `main.go:114` | GH-1558 |
| `AgentName` hardcoded; `MachineSpec.Name` never read | `main.go:585`, `runtime_config.go:195` | GH-1554 |
| `OnResult` special-cases one tool name and signal | `main.go:50-51`, `:655-676` | GH-1553 |
| Default budget `MaxIterations: 100` in Go | `main.go:614-616` | GH-1558 |

One further decision, `state.go:62` returning a magic `State("Failed")` for an
unhandled pair, is policy-shaped but inert: the caller discards the value
(`loop_runner.go:196-206` uses `r.state`). Recorded rather than filed.

## Slice sections

Each audit slice appends its section below.

### Baseline -- GH-1518

Complete. Produced the executable inventory, the declared word inventory, and
the expressiveness baseline above. Filed no decomposition findings: the three
production executables are the interpreter, a build/test-support shim, and an
already-declared boundary process, and none passes the gate as hidden
orchestration.

Reconciling the declared-word count against `mage audit` surfaced one
repository defect, filed as GH-1525: the tool-contract completeness check has
no live caller, so 66 of 102 declared words are incomplete against the
pattern's own standard without any check reporting it.

### REST and service -- GH-1519

Complete. Audited `internal/tools/rest` (8,301 lines, 26 files) and
`internal/tools/service` (1,631 lines, 6 files) in full, plus their 45 and 4
test files.

Nine of the eleven declared REST words and ten of the twelve service inits are
atomic. The package is in better decompositional shape than its size suggests:
the health-retry loop, the async send/await grammar, and the scenario pipeline
are all genuinely sequenced by MachineSpec rather than by Go.

Nine findings filed (GH-1528 through GH-1536). They cluster in three groups
rather than one: effects that escape their declaring dispatch (GH-1528,
GH-1529, GH-1533, GH-1534), declarations that do not describe their
implementation (GH-1530, GH-1531, GH-1535, GH-1536), and one declared
enforcement point with no enforcement code (GH-1532). Only the first group is
decomposition in the classic sense; the second is the more common failure here,
and it matters because the pattern's whole claim is that the declaration is the
program.

Thirteen candidates rejected, recorded above. Six of them restate proposals the
previous run filed and GH-1410 reversed.

Tests: `go test ./internal/tools/rest/... ./internal/tools/service/...` passes
(3.4s and 4.0s).

Prior findings verified against current code: GH-882 HELD (runtime target
selection with all safety gates intact), GH-1102 HELD (the polling loop is gone
and `await_operation` is rejected at load with a migration message), GH-886 HELD
(no `net/http` import remains in the service package; probe and retry are
machine states), GH-1379 PARTIAL (backoff landed, cancellation is inert --
filed as GH-1529), GH-1385 and GH-1386 correctly not implemented, GH-1388 not
applicable to these packages.

### Remaining tool packages -- GH-1520

Complete. Audited ten packages in full, about 8,500 production lines:
`catalog` (2,114), `filesystem` (1,276), `llm` (1,125), `validation` (960),
`control` (951), `lifecycle` (802), `exec` (608), `compose` (308),
`registry` (302), `undo` (78).

Almost every word in these packages is atomic, and several of the constructs
that look like violations are the pattern working correctly -- `delay` is a
bounded wait whose retry loop lives in the machine, `self_invoke` is a Boundary
Tool whose child runs its own machine, and `invoke_llm` is one inference
transaction with no probe and no retry. Sixteen candidates were rejected on
that basis.

The eleven findings are almost all about enforcement rather than
decomposition, and GH-1539 is the most serious result of the whole audit: the
rollback walk never reads a declared reversibility tier, so the three-tier
model the Tool Contract pattern is built on is unreachable, and 18 shipped exec
words -- including the core `commit` word -- are guaranteed to turn any
rollback that crosses them into a reported failure. The corpus linter mandates
the exact `undo.strategy` token the exec runtime rejects.

Tests: all ten packages pass (`undo` has no test files, itself noted).

Prior findings verified against current code: GH-1381 HELD (the `.git` stat is
gone, preconditions are load-validated), GH-1100 HELD (the documentation
taxonomy is profile-supplied and a second profile proves it), GH-1097 HELD and
held the way GH-1410 requires -- traversal moved to a *declared* find word, not
an undeclared `exec.Command` -- GH-1392 HELD with the measurement recorded in
the declaration, GH-1375 HELD, GH-891 HELD, GH-1098 HELD, GH-1103 HELD,
GH-1101 HELD under a documented exception, GH-895 HELD, GH-892 HELD,
GH-1377 PARTIAL (the observability half landed; the decomposition was
consciously declined and recorded as an exception, and the tracing improvement
did not fix the misclassification GH-1539 describes), GH-1376 REGRESSED in
scope (it collapsed the `list_files` duplicate and left `read`, `write`,
`edit`, and `find` duplicated -- filed as GH-1547).

### Runtime core and composition root -- GH-1521

Complete. Audited `internal/runtime/core` (5,102 lines, 25 files),
`cmd/agent` (1,875 lines, 7 files), and `internal/support` with its five
subpackages (about 510 lines).

The interpreter is sound. The loop decision table above is the evidence: 26 of
34 non-spec-driven decisions are legitimate interpreter mechanism, and the
things that look most like violations -- the parallel worker pool, the
`for_each` join arithmetic, the reachability walk, the Dolt SQL -- are all
mechanism whose parameters come from the declaration. Ten candidates were
rejected on that basis.

There is no role-keyed branching anywhere in `cmd/agent` or the loop, which is
the strongest single result of this slice: GH-884's removal of the
test-evidence modes held completely, and nothing has grown back.

Ten findings filed (GH-1550 through GH-1559). The pattern across them is that
the interpreter is faithful to the *machine* and careless with the *run*: the
machine's states, signals, transitions, and iteration are all read from the
spec, while the run's identity, its summary, its terminal classification, its
per-command bound, and its resumed data state are decided by Go literals. Six
of those decisions have no field in the machine format at all, filed together
as GH-1558.

Tests: `internal/runtime/core`, `cmd/agent`, and all `internal/support`
subpackages pass. `internal/support/envexpand` has no test file, noted in
GH-1557.

Prior findings verified: GH-884 HELD, GH-894 HELD (no `AfterDispatch` hook
remains; the retry counter is bound to explicit words and validated at
startup), GH-883 HELD in full against all ten of its required capabilities,
GH-1099 PARTIAL (the declared path landed and is preferred, but nothing warns
when a terminal state omits `run_status` and adoption is 3 machines of about
30 -- filed as GH-1551), GH-1393 PARTIAL (both cited sites consolidated; a
third process-group spawn exists in `internal/tools/service` -- filed as
GH-1556).

### Specification and planning -- GH-1522

Complete. Audited `pkg/spec` (5,835 lines, 27 files) and `internal/planning`
with its four subpackages (2,316 lines).

The central judgment for this slice was library API versus agent-visible
workflow, and the answer is unambiguous: every major `pkg/spec` stage is
already the body of a machine-dispatched word in `internal/tools/validation`.
The jurist machine sequences the stages and `pkg/spec` is the library those
words call. Eleven multi-step stages were rejected on that basis with their
consumers named, including the 36-checker `Validate`, which is already
individually selectable from charter YAML.

The planner is in similarly good shape: thirteen of its fifteen words are
atomic, and six of the seven prior findings held cleanly.

Seven findings filed (GH-1560 through GH-1566). Two are decomposition proper
-- `extract_task`'s hidden graph mutation and the unreachable weight budget --
and the rest are declaration integrity: conflicting duplicate declarations
whose undo strategy contradicts the Go, two advertised checks that cannot fire,
an unwired validator, Go-constant output paths, and a set of decode paths that
report "field missing" when they mean "could not parse".

Tests: `pkg/spec` and all four planning packages pass.

Prior findings verified: GH-885 HELD, GH-1086 HELD, GH-1088 HELD for
`extract_all` (though the same shape survives one word over, filed as
GH-1560), GH-1089 HELD, GH-1090 HELD, GH-1091 HELD, GH-1087 PARTIAL (the
removal held completely -- `BatchLimitReached`, `Paused`, and `batch_limit`
appear nowhere -- but its third criterion, that retained batch policy be
configured in YAML, is unmet: the policy stayed in Go and was left
unconfigurable, filed as GH-1561).

### Evaluation, OTLP, observability, and model -- GH-1523

Complete. Audited `internal/evaluation` (3,948 lines), `internal/tools/otlp`
(3,697), `internal/observability` with four subpackages (2,111), and
`internal/model` with three (1,682) -- 61 non-test files in full.

This slice needed the most care, because three of the four findings GH-1410
judged defective came from these packages. Fifteen candidates were rejected,
and the rejected register above records each proposal of that shape with the
gate question it failed. The duckdb externalization, the `stdouttrace` spool
replacement, the `SpanStub` trace decode, and the Ollama CLI probe were all
considered and none could complete the equivalence, provisioning,
compatibility-spike, and test-path sections the gate requires.

Two positive results worth recording. The eleven evaluator point words are the
cleanest word family in the audit: `create_point_dir` and `sample_docs` emit
copy *parameters* rather than copying, `record_point_failure` explicitly leaves
`meta.json` to `collect_metrics`, and every state mutation is declared with a
matching receipt strategy. And `internal/observability` is genuinely generic --
the same Article D4 question asked of the REST monitor views got the same
answer here.

Eight findings filed (GH-1567 through GH-1574). The two with real consequence
today are GH-1568, where `spool_get_metric` silently truncates at twenty
records while declaring it returns all of them, and GH-1569, where an omitted
`max_files` turns the first spool overflow into a full delete of a word whose
declared non-goal is "does not delete accepted backend spans".

Tests: all packages pass.

Prior findings verified: GH-1092 HELD, GH-1093 HELD, GH-1094 HELD, GH-1095
HELD, GH-1375 HELD, GH-1096 HELD with one qualification -- the machine
sequencing `load_otlp_batch` and `relay_spans` ships as an integration-test
profile rather than an application profile, which satisfies srd008 R7.1/R7.2
as written but is worth recording.
