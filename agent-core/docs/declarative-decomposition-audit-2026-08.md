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
| _(none yet)_ | | | |

## Rejected candidates

Candidates considered and not filed, with the gate question each failed. A
later recurrence should not refile these without new evidence.

| Candidate | Slice | Failed question | Reason |
|---|---|---|---|
| Move `catalog-test-evidence` behind a declared word | Baseline | Q1 contract scope | Build/test support with no agent caller; its only non-test invoker is a Mage target. Excluded by the scope boundary. |
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
