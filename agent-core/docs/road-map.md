---
id: agent-core-roadmap
title: Agent Core Release Road Map
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- release: 00.0 | status: done -->
## 00.0 - Core engine and supporting packages

Import the generic agentic loop engine from the generator project and add
the support packages that make it usable as a library.

<!-- field: srds -->
### Srds

- srd001-core-types
- srd002-state-machine
- srd003-command-dispatch
- srd004-tool-registry
- srd005-agentic-loop
- srd006-yaml-machine-config
- srd007-tracer-interface
- srd008-telemetry
- srd009-ollama-adapter
- srd010-conversation
- srd011-prompt-schema
- srd012-system-prompt-template
- srd013-standard-tool-library
- srd014-spec-graph
- srd015-genai-semconv
- srd016-traceparent
- srd017-llm-extensions
- srd018-cli-flag-contract

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel00.0-uc001-domain-agent-integration | Domain agent imports agent-core and runs an agentic loop | done |
| rel00.0-uc002-spec-corpus-validation | Maintainer validates the specification corpus graph | done |

<!-- field: follow_up -->
### Follow up

| id | summary | status |
| --- | --- | --- |
| gh-123 | Generalize the jurist validator to deterministic data-driven charter suites | done |

<!-- release: 01.0 | status: done -->
## 01.0 - Unified agent binary and configuration-driven architecture

Consolidate generator behavior into the cmd/agent binary. Planner and
evaluator behavior move there too, but YAML profiles still choose the
active machine and selected tools. Profile-driven machines and selected
tool declarations become the normal runtime path. Model configuration,
planner packages, evaluator support, bench UI support, native Mage
targets, and Go tests round out the release.

<!-- field: srds -->
### Srds

- srd019-eval-harness
- srd020-bench-web-ui
- srd021-agent-cli-exec-tool
- srd022-builtin-tools
- srd023-cli-exec-tools
- srd024-blocking-input-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel01.0-uc001-generator-coding | Generator agent solves Go coding tasks using YAML config | done |
| rel01.0-uc002-evaluator-benchmark | Evaluator runs generator across models and collects results | done |
| rel01.0-uc003-bench-visualization | Bench serves web UI for evaluation result exploration | done |

<!-- field: follow_up -->
### Follow up

| id | summary | status |
| --- | --- | --- |
| gh-1867 | Add the project collection word for ordered field extraction from structured rows. | implemented |

<!-- release: 02.0 | status: done -->
## 02.0 - Rollback lifecycle and tool language documentation

Runtime checkpoint behavior, suspend, resume, rollback foundations, tool
authoring contracts, sentence-tool migration proof, and the internal/tools
package split are implemented. Lifecycle profiles expose checkpoint
history and rollback through focused lifecycle tools. The srd035 typed
checkpoint port and srd036 Dolt adapter replace the retired key/blob
StateStore model, adding git-style Revert for DB-persisted state while
tools own external compensation through opaque receipts.

<!-- field: srds -->
### Srds

- srd025-rollback-lifecycle
- srd026-lifecycle-tools
- srd035-checkpoint-port
- srd036-dolt-state-persistence

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel02.0-uc001-approval-suspend-resume | Agent suspends at approval gate and resumes from checkpoint | done |
| rel02.0-uc002-history-rollback | User inspects Dolt commit history and rolls back with Revert plus a receipt-driven restore of external effects | done |
| rel02.0-uc003-tool-authoring | Developer authors a new tool as a vocabulary word | done |
| rel02.0-uc004-sentence-tool-migration | Developer migrates a sentence-shaped tool into machine-composed words | done |

<!-- field: follow_up -->
### Follow up

| id | summary | status |
| --- | --- | --- |
| agent-core-vk8j | Reorganize standard tool library packages and tool declarations | done |

<!-- release: 03.0 | status: done -->
## 03.0 - Declarative REST client and server tools

REST client and REST server tools are declarative boundary tool families.
Outbound words call configured upstreams; inbound words launch from agent
profiles and bind configured addresses. Generic await words can fan in
events across configured server queues, including lifecycle control
requests that route through MachineSpec. OpenAPI documents and REST
definitions live with configuration. The runtime package is
internal/tools/rest. net/http supplies client and server behavior.
OpenAPI 3 loading uses kin-openapi. Shared REST definitions and async
inbox support are implemented, while generated Go clients and servers are
excluded. The Agent Architecture uses machine_request server bindings
so one inbound HTTP request can run one short-lived MachineSpec sentence
and map terminal output back to HTTP. Declarative document resources
connect that request grammar to filesystem documentation corpora through
reusable resource words. Generic Go tests cover the reusable REST
machine_request path. The concrete Knowledge Manager browser proof is a
profile-owned tracer bullet in applications/catalog.

Closed REST conformance work now covers client safety, async request
metadata, OpenAPI endpoint preservation, REST compensation mementos and
rollback execution, machine_request response schemas, and explicit
document_resources reservation. Modeled lifecycle_control endpoints and
non-default rest_await_event read policies remain reserved or rejected
until future work implements them. The formal suite maps the implemented
contracts to named executable tests.

<!-- field: srds -->
### Srds

- srd027-rest-tool-runtime
- srd028-rest-client-tools
- srd029-rest-server-tools
- srd030-machine-request-rest-binding
- srd031-declarative-document-resources

<!-- field: semantic_models -->
### Semantic models

- sm-rest-tool-language
- sm-machine-request-http
- sm-document-resource-language

<!-- field: config_formats -->
### Config formats

- cfg-rest-tool-format

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel03.0-uc001-rest-client-sync | Grammar invokes configured REST client resource words | done |
| rel03.0-uc002-rest-client-async | Grammar sends and awaits an asynchronous REST client operation | done |
| rel03.0-uc003-rest-server-signals | Grammar receives inbound REST server signals | done |
| rel03.0-uc004-openapi-import | OpenAPI import defines REST operations and endpoint bindings | done |
| rel03.0-uc005-ollama-openapi-model-listing | Qwen agent lists Ollama models through an OpenAPI REST tool | done |
| rel03.0-uc006-documentation-curator | Knowledge Manager serves documentation curator UX and REST tools | done |
| rel03.0-uc007-machine-request-documentation-ux | Knowledge Manager UX requests use machine_request and declarative document resources | done |
| rel03.0-uc008-rest-await-any-fan-in | Staged REST await-any fan-in adds a later event source after the first selected event | done |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel03.0-rest-tools | Formal tests for REST runtime, client/server tools, fan-in, lifecycle exit routing, listener shutdown, OpenAPI import, Ollama E2E proof, profile-owned documentation-curator evidence, browser machine_request UX coverage, and closed REST conformance fixes | implemented |

<!-- release: 04.0 | status: done -->
## 04.0 - Embedded monitor metrics and REST API

Embedded monitor support is a reusable runtime capability inside the
agent process. It records tool-owned metric samples in a bounded
in-memory store, exports normalized samples through OpenTelemetry
metrics, and exposes REST/OpenAPI read paths over cached monitor state.

The monitor is not a Kubernetes sidecar, daemon, or required external
process. External collectors and supervisors may read or receive exported
data, but the runtime owns the cache and recorder ports. The release proof
is mage integration:monitor.

<!-- field: srds -->
### Srds

- srd032-tool-metrics-monitor-store
- srd033-monitor-rest-api

<!-- field: semantic_models -->
### Semantic models

- sm-tool-metrics-monitoring

<!-- field: config_formats -->
### Config formats

- cfg-rest-tool-format

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel04.0-uc001-configurable-tool-metrics | Agent records tool-owned metrics in the embedded monitor store and exports them through OpenTelemetry | done |
| rel04.0-uc002-monitor-rest-api | Supervisor reads cached monitor state through configured REST and OpenAPI endpoints | done |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel04.0-monitor | Formal tests for monitor store behavior, metric emission, OpenTelemetry export, and REST/OpenAPI cached-state reads | implemented |

<!-- release: 05.0 | status: done -->
## 05.0 - Monitored live model integrations

Later live integration proofs combine reusable embedded monitor behavior
with model-boundary flows. The first proof starts the monitor from the
Qwen/Ollama state machine without blocking model progress, polls cached
token metrics while the agent process remains active, and exits through
monitor control.

<!-- field: srds -->
### Srds

- srd032-tool-metrics-monitor-store
- srd033-monitor-rest-api

<!-- field: semantic_models -->
### Semantic models

- sm-tool-metrics-monitoring

<!-- field: config_formats -->
### Config formats

- cfg-machine-format
- cfg-rest-tool-format

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel05.0-uc001-qwen-monitor-token-progress | Qwen run exposes increasing token metrics through the embedded monitor | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel05.0-monitor-qwen | Formal test for monitored Qwen token metric progress through mage integration:ollamaMonitor | implemented |

<!-- release: 06.0 | status: done -->
## 06.0 - External agent profiles and mounted runtime assets

Profile migration separates runtime ownership from agent program
ownership. Agent Core keeps the cmd/agent binary, runtime packages,
Docker/Mage release machinery, and core-owned shared tool declarations.
applications/catalog owns profile.yaml, machine.yaml, tools.yaml, profile-local
declarations, REST definitions, UX config, demos, and agent integration
fixtures.

Runtime containers consume profiles from external paths. Callers mount or
otherwise provide profile directories, pass --profile to that mounted
profile, and pass --directory to the mounted workspace. The runtime image
does not copy migrated agent profile directories from agent-core.

<!-- field: srds -->
### Srds

- srd034-external-agent-profiles

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel06.0-uc001-mounted-profile-container-runtime | Runtime container runs an externally mounted agent profile | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel06.0-profile-migration | Formal tests for external profile root resolution, runtime image profile exclusion, and mounted-profile container execution | implemented |

<!-- field: proof -->
### Proof

- go test ./pkg/spec -run TestResolveProfilePathMapsInstalledCoreHome
- go test ./magefiles -run TestDockerfileRuntimeExcludesAgentProfiles
- go test ./magefiles -run TestContainerBuildSummaryForDocker
- mage docker
- applications/coding-agent mage integration:executorLive
- applications/coding-agent mage integration:criticGate
- applications/catalog mage containerSmoke

<!-- field: follow_up -->
### Follow up

| id | summary | status |
| --- | --- | --- |
| agent-core-xcbg | Normalize stale in-repo agent path references after profile migration | done |

<!-- release: 07.0 | status: implemented -->
## 07.0 - Command-state store and two-plane persistence

Release 07.0 lifts declared-param selection out of the REST package into a
core-level command-state store. Any word selects declared params from a
labeled prior step in the run through a $from(...) selector, not only from
the immediately previous Result. The store is a read-only forward view over
the run's execution log, rehydrated from the srd036 tool_outputs plane after
a restart, with a declared-param-only authority boundary that never exposes
transport or receipts.

Persistence splits the tool_results table into a forward plane
(tool_outputs, readable by the store) and a reverse plane (receipts,
consumed only by the rollback walk), so a forward selector can never reach a
receipt. REST client words become the first consumer through body_source
command_state. This release records the documentation and the tracer-bullet
use cases; the core store and REST consumption land in epics GH-269 and
GH-270.

<!-- field: srds -->
### Srds

- srd038-command-state-store
- srd036-dolt-state-persistence
- srd028-rest-client-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel07.0-uc001-command-state-threading | A three-word REST chain where the third word selects the first word's output by label and resumes mid-chain after restart | implemented |
| rel07.0-uc002-two-plane-revert | Reverting to an earlier step reaps later tool_outputs and receipts rows with the branch reset while the receipt walk still reverses the surviving step | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel07.0-command-state | Formal tests for label selection, duplicate-label resolution, restart rehydration, the R13.3 rejection table, redaction-before-persistence, five-table round-trip, and revert reap | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/rest/client -run TestRESTClient_CommandStateTracerBulletNonAdjacentWithRestart
- go test ./internal/runtime/core ./internal/runtime/checkpoint/dolt -run 'TestCommandStateView|TestDoltCheckpointRevertReapsBothPlanes|TestDoltCheckpointReceiptWalkReversesSurvivingStep|TestDoltCommandStateWritesOnlyNewStepBlob'
- go test ./internal/tools/rest/client -run 'TestRESTClient_CommandState'

<!-- release: 08.0 | status: implemented -->
## 08.0 - HTTP trace propagation

Release 08.0 extends srd016 so a W3C trace continues across REST calls
between running agents. Outbound REST client requests inject the active
dispatch span's traceparent (and tracestate when present); inbound REST
server endpoints extract it and parent the request and machine_request
spans on the remote span, so a caller's client span and a callee's server
span join into one connected trace. Absent or malformed headers fall back to
a new root without failing. Injection is a uniform transport concern with no
per-operation authoring surface, and the subprocess --otel-parent-span path
is unchanged and complementary. This release records the documentation and
the tracer-bullet use case; the inject/extract implementation lands in
Epic #295 (GH-310).

<!-- field: srds -->
### Srds

- srd016-traceparent

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel08.0-uc001-http-trace-propagation | Two agents linked by a REST call produce one connected trace, with malformed-header fallback to a new root | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel08.0-trace-propagation | Formal tests for outbound injection, inbound extraction parenting the machine_request span, absent/malformed fallback, and the unchanged subprocess path | implemented |

<!-- release: 09.0 | status: done -->
## 09.0 - Scenario rig runtime

Release 09.0 gives the runtime the words a scenario test rig needs. A
mock REST endpoint binding (srd039) serves canned, scripted responses
from fixture data and records a request log, so a mock profile
can stand in for a real upstream service and a validator can assert what
a subject agent actually sent. Service-child tools (srd040) start, health
check, and stop background serve-mode child agents, run validator
machines concurrently to completion, and discover scenario directories
under configured roots.

The rig itself — the assembler and mock profiles, scenario authoring
conventions, and the reference subject — is applications/catalog release 11.0
work over these words (Epic #662). This release implements the
documentation and runtime contracts through the mock binding from
GH-666 and the service-child tools from GH-667. The full end-to-end
scenario tracer remains planned.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-664 | Specify the mock REST binding and service-child tools (this release's documentation). | done |
| gh-666 | Implement the mock REST endpoint binding. | done |
| gh-667 | Add service-child, validator, and scenario-discovery builtin tools. | done |

<!-- field: srds -->
### Srds

- srd039-mock-endpoint-binding
- srd040-service-child-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel09.0-uc001-scenario-rig-run | A rig machine runs one scenario hermetically to a verdict over a mock, a subject, and machine-visible validator items. | specified |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel09.0-scenario-rig | Formal tests for scripted mock responses, the request log, fixture validation, serve-mode child lifecycle, validator item execution, deterministic discovery, and the end-to-end scenario tracer. | specified |

<!-- field: proof -->
### Proof

- go test ./internal/tools/rest -run 'TestRESTServer_MockBinding'
- go test ./internal/tools/service
- mage audit

<!-- release: 10.0 | status: implemented -->
## 10.0 - Value predicate word

Release 10.0 lets a machine branch on a value. A machine branches on
signals and signals come from words, so branching on a value needs a word
that emits one from a comparison -- and none existed. Response schema
validation implements only required and type and raises CommandError
rather than a domain signal; client status mapping branches on the HTTP
status, which is unchanged whether a counted collection holds zero
documents or five hundred; exec words emit two signals from an exit code.
The fallback was a bespoke Go tool per comparison, or pushing a caller's
decision into an unrelated agent so its exit code carried the answer.

srd041 specifies the word: operands, a closed operator set, declared
signals for both outcomes, and explicit operand typing, because values
read from REST responses arrive as strings and a count of "10" must not
order below "6". Its sharpest requirement separates an infrastructure
fault from a legitimate negative -- an operand that will not resolve emits
CommandError, never the unsatisfied signal -- so a machine cannot take a
plausible-looking branch on a comparison that never ran.

This is the third gap of the same shape, after terminal-state response
mapping (GH-615) and the raw file read (GH-521): a declarative agent could
express the flow but not one small decision inside it. The word is
specified in GH-769 and implemented in GH-770.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-769 | Specify the value predicate word (this release's documentation). | done |
| gh-770 | Implement the value predicate word in internal/tools/control. | done |

<!-- field: srds -->
### Srds

- srd041-value-predicate-tool

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel10.0-uc001-value-predicate-branch | A machine reads a value from a service and takes a different transition depending on it, through a declared word. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel10.0-value-predicate | Formal tests for the operator boundaries, emptiness, numeric coercion of string-typed values, the unresolvable-operand fault, registration validation, and the end-to-end two-transition branch. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/control
- mage audit

<!-- release: 11.0 | status: implemented -->
## 11.0 - OTLP receiver tools

Release 11.0 gives a declarative agent a trace-ingress vocabulary without
moving receive policy into a hidden collector process. Focused
`internal/tools/otlp` words launch and stop an OTLP/gRPC listener, await
complete bounded trace batches, append accepted spans in the existing
stdouttrace NDJSON shape, and relay a selected batch unchanged to an
upstream OTLP endpoint.

MachineSpec owns launch, await, filtering, spool, relay, retry, and stop
order. The ingress profile exports its own telemetry to file only. This
release receives traces only; deployments that require the persistent
metrics evidence from srd008 R9 retain a separate metrics path.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-858 | Specify the OTLP receiver tool family, use case, and formal test plan. | implemented |
| gh-859 | Implement OTLP/gRPC receive, bounded state, launch, and stop. | implemented |
| gh-860 | Implement await_spans and stdouttrace-compatible spool output. | implemented |
| gh-861 | Implement declared OTLP batch relay and self-loop rejection. | implemented |

<!-- field: srds -->
### Srds

- srd042-otlp-receiver-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel11.0-uc001-otlp-receiver-await | A persistent machine receives, records, and relays OTLP trace and metric batches through declared words. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel11.0-otlp-receiver | Formal tests for receive, backpressure, await, spool compatibility, relay identity, lifecycle cleanup, self-loop prevention, and metric intake, spool, and query. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/otlp
- mage audit

<!-- release: 12.0 | status: implemented -->
## 12.0 - Spool query tools

Release 12.0 adds a read path over the srd042 NDJSON trace spool.
Two focused builtin words, spool_list_traces and spool_get_trace, scan
the active spool and rotated files, aggregate spans into paginated trace
summaries (newest first), and return per-trace span detail with the
fields a waterfall UI needs. Both are read-only, tolerate malformed
lines, and treat an absent spool as empty.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1144 | Add spool query builtin tools (agent-core release 12.0). | done |

<!-- field: srds -->
### Srds

- srd044-spool-query-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel12.0-uc001-spool-query | A collector reads trace summaries and span detail from its NDJSON spool | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel12.0-spool-query | Formal tests for trace aggregation, rotation, pagination, malformed-line tolerance, empty spool, unknown trace, and error paths. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/otlp -run 'TestListTraces|TestGetTrace'
- mage audit

<!-- release: 13.0 | status: implemented -->
## 13.0 - Span analytics tools

Release 13.0 adds aggregate reads over the srd042 NDJSON trace spool for a
Honeycomb-style Explore panel. Three builtin words, spool_span_heatmap,
spool_span_group_by, and spool_span_breakdown, filter spans (time,
service, name, attribute equality), independently bucket them into a
duration-over-time heatmap or count them by an attribute with a reported
dropped remainder, and rank the attributes that most distinguish a
selected region from its complement. All are read-only, tolerate
malformed lines, treat an absent spool as empty, take all configuration
without environment variables, and produce deterministic output.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1240 | Add span-analytics builtin tools (agent-core release 13.0). | done |

<!-- field: srds -->
### Srds

- srd045-span-analytics-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel13.0-uc001-span-analytics | Aggregate reads turn a spool into a latency heatmap and a divergence ranking | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel13.0-span-analytics | Filtering, heatmap bucketing, group-by top-N, divergence ranking, determinism, and error paths. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/otlp -run 'TestSpanStats|TestSpanAnalytics|TestSpanBreakdown'
- mage audit

<!-- release: 14.0 | status: implemented -->
## 14.0 - Request-driven signals and configured Dolt words

Release 14.0 adds two independent agent-core seams. A model-free profile
can accept a validated request through a transport-neutral signal source,
admit its configured signal against the current machine state, and run
the ordinary transition path. Admission distinguishes accepted,
refused_undeclared, and refused_conflict without changing the existing
model-driven path. Checkpoint, suspend, and resume retain their normal
lifecycle contracts.

A separate boundary-word family provisions, queries, and writes a
configured Dolt database. Trusted ToolDef configuration owns connection,
database, statements, parameter schemas, and commit messages; callers
supply declared bound params only. Successful writes create one Dolt
commit. The word database must remain separate from the srd036 checkpoint
database. A real-Dolt integration target, not a mock-only test, proves
provisioning, schema, parameterized reads and writes, commit history,
authority refusal, and database isolation.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1731 | Specify the request-driven signal source and Dolt boundary-word seams. | done |
| gh-1732 | Implement the configured Dolt boundary words and real-Dolt integration gate. | implemented |
| gh-1733 | Implement the HTTP request signal source, model-free profiles, lifecycle coverage, and model regression. | implemented |

<!-- field: srds -->
### Srds

- srd046-request-driven-signal-source
- srd047-dolt-boundary-word

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel14.0-uc001-request-driven-signal | A model-free profile advances from a validated HTTP request and resumes from its checkpoint. | implemented |
| rel14.0-uc002-dolt-boundary-word | Declared Dolt words provision, update, commit, and query a separate versioned store. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel14.0-agent-input-and-dolt | Formal unit, compatibility, lifecycle, authority, and real-Dolt integration obligations for both seams. | implemented |

<!-- field: proof -->
### Proof

- go test ./cmd/agent -run TestRequestSignalSource_ for model-free serving, ordinary dispatch, suspend/resume, per-run checkpoints, and mock-model compatibility
- go test ./internal/runtime/core ./internal/tools/rest for admission, ownership, mapping, HTTP response, redaction, and trace behavior
- mage integration:dolt against a real Dolt server for checkpoint and request-signal continuation
- go test ./internal/tools/dolt ./internal/tools/catalog for GH-1732
- mage integration:doltWord against a real Dolt server for GH-1732
- mage audit

<!-- release: 15.0 | status: done -->
## 15.0 - Cohere invoke_llm provider adapter

Release 15.0 adds Cohere v2 Chat behind the existing llm.Client and
invoke_llm boundary. ToolDef configuration selects provider, model, URL,
and response profile. The provider adapter resolves COHERE_API_KEY only
when Chat executes, normalizes ordered content blocks and usage tokens,
and emits the same GenAI identity as the Ollama path. MachineSpec, prompt
assembly, conversation, capture, parse words, and REST-declared Cohere
words remain unchanged.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1951 | Specify the Cohere v2 adapter, provider configuration, parser profile, and formal proof. | done |
| gh-1952 | Implement the Cohere adapter, invoke_llm provider selection, parser profile, and hermetic tests. | implemented |

<!-- field: srds -->
### Srds

- srd048-cohere-adapter

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel15.0-uc001-cohere-invoke | A profile selects Cohere for an invoke_llm classifier without changing its machine or parse words. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel15.0-cohere-invoke | Hermetic adapter, credential, provider-selection, parser, error, and GenAI tracing tests. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/llm ./internal/model/llm ./internal/observability/telemetry/genai
- mage audit

<!-- release: 16.0 | status: done -->
## 16.0 - Pipeline word for marshalling blocks

Release 16.0 adds the pipeline builtin (srd049): one word that runs an
ordered sequence of pure data-transform stages -- the existing compose,
render_each, project, normalize_vector, flat_map, reorder_by_index,
parse_structured, and select_subset inits with their standalone configs
-- and emits one declared signal. Stages chain through the
previous-Result contract, the current-value selector inside a stage
means the previous stage's output, and $from selectors resolve against
the injected command-state view. The machine keeps every state that
decides a path; the word absorbs the states that only reshape. A
pipeline declaration co-selects its stage factory families, and
render_each accepts the current-value items selector under the srd041
both-forms precedent.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1957 | Specify and ship the pipeline word, stage co-selection, and the render_each current-value items selector. | implemented |

<!-- field: srds -->
### Srds

- srd049-pipeline-word

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel16.0-uc001-pipeline-word | A profile folds a render/compose marshalling block into one pipeline word. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel16.0-pipeline-word | Hermetic chaining, selector, registration-failure, failure-separation, and co-selection tests. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/pipeline ./internal/tools/compose ./internal/tools/registry
- mage audit

<!-- release: 17.0 | status: implemented -->
## 17.0 - Named declaration units and explicit imports

Release 17.0 treats profile loading as declaration compilation. Tool and
REST files expose named units and import explicit relative paths. The
closure loader resolves one deterministic dependency graph, rejects
duplicate names, cycles, invalid overrides, and unused imports, and
preserves source provenance in the canonical config dump.

Legacy tool includes are gone; GH-2012 converted the closure to units and
imports and GH-2010 deleted the mechanism. No flattened or generated
declaration output is committed.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1975 | Specify named declaration units, imports, merge semantics, and usedness. | implemented |
| gh-1977 | Implement tool declaration units and explicit imports. | implemented |
| gh-1976 | Implement REST declaration units and explicit imports. | implemented |
| gh-1978 | Enforce unused imports and add the control-imports closure proof. | implemented |

<!-- field: srds -->
### Srds

- srd050-declaration-imports

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel17.0-uc001-declaration-imports | A profile composes named tool and REST units through one explicit, validated closure. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel17.0-declaration-imports | Formal tests for graph resolution, merge semantics, usedness, compatibility, diagnostics, and the control-imports proof. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/catalog ./internal/tools/rest/definition ./internal/load
- mage audit

<!-- release: 18.0 | status: implemented -->
## 18.0 - Parameterized declaration fragments

Release 18.0 gives declarations function templates. A fragment is a
unit with declared, typed parameters; an importer expands it with
arguments, the loader substitutes them into scalar values before strict
decoding, and the result is validated exactly as a hand-written unit.
Three fragment kinds land in order of cost: tool declarations, REST
definitions, and machine stages. Nothing generated is committed; the
canonical dump lists every expansion.

The metric that judges the release landed first: stats:reuse counts
imported, shared, and single-importer units and expansions, and a
unit earns its place with two importers or one expansion.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2084 | Count imported, shared, single-importer units and expansions in stats:reuse. | implemented |
| gh-2080 | Specify fragments -- srd052, use case, test suite, and the config-format fields. | implemented |
| gh-2081 | Tool-declaration fragments and the hygienic substitution engine. | implemented |
| gh-2082 | REST-definition fragments through the REST import resolver. | implemented |
| gh-2083 | Machine stage fragments spliced under a prefix and validated as one machine. | implemented |
| gh-2085 | Factor the top duplicated blocks into fragments and shared units, reporting stats:reuse before and after. | implemented |

<!-- field: srds -->
### Srds

- srd052-declaration-fragments

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel18.0-uc001-declaration-fragments | A profile expands parameterized fragments in place of hand-copied blocks. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel18.0-declaration-fragments | Formal tests for the fragment format, expansion and substitution, usedness and provenance, and stage splicing. | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/tools/catalog ./internal/tools/rest/definition ./internal/load ./internal/runtime/core
- mage stats:reuse
- mage audit

<!-- release: 19.0 | status: implemented -->
## 19.0 - Signed boundary contracts

Release 19.0 retires the descriptive prose on boundary and
stateful_internal tools. Of 312 such tools, 36 carry a signature, and
their problem, goals, requirements, non_goals, and relationships blocks
hold about 5,900 lines that restate each description and are checked
only for presence. srd051 R6.6 already lets a signature discharge that
prose for every category while R6.8 and R6.9 keep the effect blocks
explicit, so the release adds the obligation, R6.14, that these tools
declare a signature naming their signals, and R6.15, that every tool
states a category. The declaration legacy gate lists the unsigned and
uncategorized tools in a shrink-only baseline, and each application's
migration signs its tools, deletes the prose, and deletes its lines.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2110 | Specify R6.14 and R6.15, gate unsigned boundary, stateful_internal, and uncategorized tools, and migrate coding-agent, agent-architecture, catalog, chatbot-mesh, and agent-core's library. | implemented |
| gh-2150 | Categorize or fold the catalog critic session and LLM overlay tools, then promote R6.14 and R6.15 into the corpus audit. | implemented |

<!-- field: srds -->
### Srds

- srd051-declaration-types

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel19.0-uc001-signed-boundary-contracts | A boundary tool states its contract through a signature and its effect blocks. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel19.0-signed-boundary-contracts | Formal tests for emits-only boundary signatures in the corpus audit and the untyped and uncategorized classes of the legacy gate. | implemented |

<!-- field: proof -->
### Proof

- go test ./pkg/spec ./internal/declstyle
- mage stats:reuse
- mage audit

<!-- release: 20.0 | status: in_progress -->
## 20.0 - Machine templates, agent blueprints, and unit libraries

Release 20.0 raises the grain of reuse from blocks to machines and
agents. Of 62 machine files, 26 share an identical state set with
another; the ten-state serve machine alone is written five times. A
machine template declares a whole machine once as a fragment, and a
machine file becomes one expansion of it. An agent blueprint
declares a whole profile once, naming its machine template and units
relative to itself, and each agent's profile becomes an instance
carrying a name, arguments, and only the fields that differ.

Unit libraries let those shared declarations live in one place: an
import may name a unit under the agent-core install root or under a
root a profile declares, and staging carries rooted files into the
container.

Machine templates and unit libraries shipped. Agent blueprints did not:
gh-2115 deferred the loader work to gh-2123 for want of a profile group
that is one agent with arguments, and release 21.0 carries it. The
blueprint use case is specified and its eleven test cases wait on that
implementation, so the release stays open rather than claiming a feature
it only specified.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2113 | Specify machine templates (srd054) and agent blueprints (srd055), their use cases, the suite, and the profile format. | implemented |
| gh-2117 | Specify unit libraries (srd056) -- prefix imports, declared library roots, staging, and the service-library convention. | implemented |
| gh-2118 | Prefix imports for the agent-core root; delete the application copies of agent-core types. | implemented |
| gh-2114 | Machine templates in the machine closure loader, with the control-template fixture. | implemented |
| gh-2115 | The chatbot-mesh serve agents and catalog's applier become instances of the agent-core serve-machine template. | implemented |
| gh-2123 | Agent blueprints in the profile loader, deferred from gh-2115. Release 21.0 measured the serving wrappers as the candidate group and did not find one agent with arguments; srd055 stays specified. | deferred |
| gh-2119 | Declared library roots, the --library flag, root read-only rules, and rooted staging. | implemented |
| gh-2116 | Migrate the remaining identical-state-set machine groups to templates. | implemented |

<!-- field: srds -->
### Srds

- srd054-machine-templates
- srd055-agent-blueprints
- srd056-unit-libraries

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel20.0-uc001-machine-templates | A machine file expands a whole-machine template. | implemented |
| rel20.0-uc002-agent-blueprints | A profile expands an agent blueprint. | specified |
| rel20.0-uc003-unit-libraries | A unit imports from the agent-core root or a declared library root. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel20.0-machine-templates | Formal tests for template and blueprint formats, expansion and path resolution, validation, and closure, identity, dump, staging, and audit provenance. The twelve template cases are implemented; the eleven blueprint cases wait on gh-2123. | in_progress |
| test-rel20.0-unit-libraries | Formal tests for import path forms, declared roots, read-only libraries, rooted closure and dump provenance, staging, and the service-library convention. All twelve cases are implemented; the two-importer case closed when cohere-demo and agentic-wiki-mesh adopted the retrieval client (gh-2527). | implemented |

<!-- field: proof -->
### Proof

- go test ./internal/runtime/core ./internal/tools/catalog ./internal/load ./pkg/profilestage ./pkg/profileaudit
- mage stats:reuse
- mage audit

<!-- release: 21.0 | status: in_progress -->
## 21.0 - Capability profiles

Release 20.0 raised the grain of reuse to the files an agent is made of.
Release 21.0 raises it to the agent: a profile that does one job,
terminates, and hosts nothing, consumed by whichever form a caller
needs.

The binding grammar shows why. Of 45 machine_request bindings outside
test data, 33 override the machine of their own serving profile, so the
behaviour exists only as a state set inside one agent and a second
caller must copy it. Naming the capability costs no mechanism: the
child-process, service-endpoint, and workload forms all ship today. What
is missing is the name, the statement of what a capability does not
carry, and a check that reports the machines more than one profile still
reaches.

The in-process nested-machine boundary stays out. Its only word,
run_point, carries evaluation-session semantics, and a general form
would be a mechanism this release does not add.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2165 | Specify capability profiles (srd057), the consumption forms, the binding discipline, and their use case and suite. | implemented |
| gh-2166 | The serve-lifecycle declaration fragment, so a wrapper's lifecycle words are one expansion. | implemented |
| gh-2167 | Adopt the monitor server fragment in the agents whose monitor server is the shipped one. | implemented |
| gh-2168 | Model labels and budget in the serve template, name every instance's workflow, and convert documentation-curator; observer, collector, and bench become instances when their extra states are cleaved. | implemented |
| gh-2123 | Closed as not planned after three measurements across this repository, cohere-demo, and agentic-wiki-mesh. Fragments and the serve template already share what serving and scenario profiles repeated, leaving 8-15 line profiles of self-relative paths a blueprint cannot absorb (srd055 R2.2). srd055 stays specified for a future group that differs only in scalars. | deferred |
| gh-2170 | Give the catalog the fleet observer and let a mesh wrap it, as the applications wrap the catalog's applier. | implemented |
| gh-2197 | Generic detached service start and list words, so a request capability can launch a child and return. | implemented |
| gh-2188 | Bench launches critic runs as detached children behind a request capability and ends as a serve-machine-template instance. | implemented |
| gh-2171 | Measured six shared request capabilities trapped behind machine overrides and closed as decided. Bench cleaved in gh-2188 and ended as a template instance; collector intake is not extractable under srd057 R1.1; vector query and the provisioning and creator request sides stay out of the catalog because their SRDs are tombstones; document read and the scenario rigs belong to consumer repositories. | done |

<!-- field: srds -->
### Srds

- srd057-capability-profiles

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel21.0-uc001-capability-profiles | One capability profile is consumed by more than one form. | specified |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel21.0-capability-profiles | Formal tests for the capability profile, the three supported consumption forms, the evaluation-scoped in-process boundary, and the shared-machine findings. | planned |

<!-- field: proof -->
### Proof

- go test ./internal/load ./internal/tools/rest ./internal/tools/control ./pkg/profileaudit
- mage stats:reuse
- mage audit

<!-- release: 22.0 | status: done -->
## 22.0 - Provider libraries

Release 21.0 standardized how a capability is served. Release 22.0 does
the same for the model provider behind it. Adding a provider today is a
compiled adapter, an embedded parser profile, and a new case in the
invoke_llm dispatch, and the Ollama embed block is written out twice
across two agents. A provider becomes one directory of YAML reached
through a declared providers root: a chat dialect that invoke_llm reads
as a template method, REST fragments for embed and rerank under one
signal vocabulary, and a parser profile. A deployment changes provider
by rebinding --library providers, and a new provider needs no Go.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2224 | Specify provider libraries (srd058), its use case, and its suite. | implemented |
| gh-2237 | Closed as not planned in the srd058 review. The REST body renderer already places a whole-scalar reference as structured data, so no fragment needs a structured parameter at load time. | deferred |
| gh-2238 | The providers root, the shared result-type unit, and the strict chat-dialect loader. | implemented |
| gh-2239 | invoke_llm as a template method over the chat dialect, with dialect replacing provider. | implemented |
| gh-2240 | Ollama and Cohere libraries and the capture-fixture parity harness. | implemented |
| gh-2241 | Retire the compiled adapters and migrate provider users to dialect. | implemented |
| gh-2242 | Embed and rerank stage fragments in tools/machines. | implemented |
| gh-2243 | Adopt the stage and REST fragments in the catalog corpus agents and drop the duplicated embed blocks. | implemented |
| gh-2245 | Adopt the embed-query stage in the chatbot-mesh chatbot request machine and chart. | implemented |
| gh-2246 | Selectors index arrays (srd038 R2.20), so the synthetic OpenAI-shaped library ships embed fragments with no Go. | implemented |
| gh-2244 | The synthetic OpenAI-shaped third-provider conformance suite. | implemented |

<!-- field: srds -->
### Srds

- srd058-provider-libraries

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel22.0-uc001-provider-libraries | A deployment changes and adds a model provider without editing an agent. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel22.0-provider-libraries | Formal tests for the dialect loader, load failures, library rebinding, adapter parity, credentials, parser precedence, stage fragments, adoption, and the synthetic provider. | implemented |

<!-- field: follow_up -->
### Follow up

```yaml
null
```

<!-- field: proof -->
### Proof

- go test ./internal/tools/llm/... ./internal/load ./internal/tools/rest ./internal/declstyle
- mage stats:reuse
- mage audit

<!-- release: 23.0 | status: done -->
## 23.0 - Provider failure signals

Release 22.0 gave every provider operation one failure vocabulary, and
the shipped dialects map their statuses to it. Chat is the one operation
that throws the result away: invoke_llm reports a provider that is
loading a model, one that is rate limiting, and one whose credential was
rejected as the same CommandError, so a machine cannot retry the first
without also retrying the third. Release 23.0 lets a ToolDef opt into
the taxonomy its dialect already declares, and classifies a transport
failure that carries no status as ProviderUnavailable, which is the case
that costs a run today. Without the opt-in nothing changes, so a pinned
consumer migrates when it chooses.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2305 | Specify the opt-in, the transport rule, and what stays CommandError. | implemented |
| gh-2306 | invoke_llm emits the dialect's failure signals when a ToolDef opts in. | implemented |
| gh-2307 | The rig-doctor retries on ProviderUnavailable instead of CommandError. | implemented |

<!-- field: srds -->
### Srds

- srd058-provider-libraries
- srd009-ollama-adapter

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel23.0-uc001-provider-failure-signals | A machine retries a busy model provider and gives up on a misconfigured one. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel23.0-provider-failure-signals | Formal tests for mapped statuses, the transport rule, the faults that stay CommandError, the default-off path, and the declared-emits agreement. | implemented |

<!-- field: follow_up -->
### Follow up

```yaml
null
```

<!-- field: proof -->
### Proof

- go test ./internal/tools/llm/... ./internal/tools/catalog
- cd ../applications/catalog && mage bootSmoke
- mage audit

<!-- release: 24.0 | status: done -->
## 24.0 - Objectstore tools

Agents persist artifacts only through the filesystem family, which
binds every object to the pod that wrote it, and the platform doctrine
forbids a per-provider answer: a gcs_write beside an s3_write would put
the provider into agent declarations. Release 24.0 adds one
objectstore family whose read/write/list words and internal destructive
delete-prefix boundary open a bucket by the declared URL scheme with
compiled-in drivers, bind the local rig through an
explicit endpoint field rather than any environment read, and undo a
write through a prior-object snapshot receipt. Prefix deletion is fixed
by declarations, bounded, verified, and externally audited. Moving a workload
between kind, GCS, and S3 changes a values overlay and nothing else.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2422 | Specify the family, srd059, the use case, and this suite. | implemented |
| gh-2423 | The Go package, drivers by URL, receipts, and factory wiring. | implemented |
| gh-2424 | Declarations and the hand-maintained gate sweep. | implemented |
| gh-2425 | The fake-gcs live proof and the release close-out. | implemented |

<!-- field: srds -->
### Srds

- srd059-objectstore-tools

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel24.0-uc001-objectstore-tools | One profile writes objects on kind and on GCS through a values overlay. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel24.0-objectstore-tools | Round trips, both undo directions, driver selection, the explicit endpoint with a decoy environment, the gate sweep, and the fake-gcs end-to-end proof. | implemented |

<!-- field: follow_up -->
### Follow up

- GH-2043 reuses the endpoint shape for the kindrig fake-gcs add-on.
- The filesystem-consumer migration is filed only after this family exists.

<!-- field: proof -->
### Proof

- go test ./internal/tools/objectstore/...
- mage test
- mage audit

<!-- release: 25.0 | status: done -->
## 25.0 - Monitor drain outcome

The serving lifecycle treats a slow drain as a failed run. Stopping a
monitor closes its listener at once and deregisters the server, and the
shutdown budget bounds only the wait for connections already in flight,
so a busy host turns a successful exit into a failed command and the
only remedy on offer is a longer timer. Release 25.0 separates the two
properties. Releasing the listener is what the machine asked for and
routes to the success terminal; whether the drain finished inside its
budget becomes a structured field of the stop output that a machine can
branch on. Faults that are not an expired budget keep failing.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2448 | Amend srd033 R6.5, add R6.10, and specify the use case and suite. | implemented |
| gh-2449 | Classify the deadline in the stop path and report the drain field. | implemented |

<!-- field: srds -->
### Srds

- srd033-monitor-rest-api

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel25.0-uc001-drain-outcome | A monitor exit on a loaded host succeeds and names its unfinished drain. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel25.0-drain-outcome | The incomplete drain that still succeeds, the reported field in both directions, the faults that stay failures, and the release proof on its shipped budget. | implemented |

<!-- field: follow_up -->
### Follow up

- The four shipped shutdown budgets stay as authored; this release does not retune them.

<!-- field: proof -->
### Proof

- go test ./internal/tools/rest/...
- mage integration:monitor
- mage audit

<!-- release: 26.0 | status: done -->
## 26.0 - Retrieval client library

Three applications query a retrieval server over one contract under
three names, and each declares its own client; the clients have
drifted, and chatbot-mesh's server drops the metadatas the others
return. Release 26.0 publishes the contract once in agent-core as the
first service library of srd056 R4: a QueryResult type unit and a client
fragment that an application expands with its route prefix and
selectors. The servers and their application policy stay where they
are. chatbot-mesh migrates here; cohere-demo and agentic-wiki-mesh adopt
at their next pin bump.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-2528 | Specify srd060, amend srd056 R4 for agent-core placement, and add the use case and suite. | implemented |
| gh-2529 | Add types-retrieval.yaml and the retrieval query client fragment with expansion tests. | implemented |
| gh-2530 | Migrate chatbot-mesh onto the fragment, return metadatas from rag-server, and file consumer adoption issues. | implemented |

<!-- field: srds -->
### Srds

- srd060-retrieval-client-contract
- srd056-unit-libraries

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel26.0-uc001-retrieval-client-library | A chatbot queries its retrieval servers through the shared agent-core client. | implemented |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel26.0-retrieval-client-library | Fragment expansion per route prefix, status mapping and side effect, loading through the agent-core root, and chatbot-mesh's migration. | implemented |

<!-- field: follow_up -->
### Follow up

- The contract's second and third importers are cohere-demo (petar-djukic/cohere-demo#789, type unit) and agentic-wiki-mesh (petar-djukic/agentic-wiki-mesh#188, fragment), each at its next pin bump.

<!-- field: proof -->
### Proof

- go test ./internal/tools/rest/... ./internal/load/...
- mage audit
- chatbot-mesh mage integration:ragServer, integration:chatbot, integration:embeddingExclusion

<!-- release: 99.0 | status: done -->
## 99.0 - Architectural migrations and cleanup

Release 99.0 records architectural cleanup work that does not belong to a
shipped feature release. Tool dispatch cleanup keeps the current core
dispatch model while planning smaller startup and availability-resolution changes:
registry errors instead of configuration-originated panics, shared builtin
registration behavior, and one external-tool availability rule for
manifest, parse_response, and dynamic $tool dispatch paths.

The application and catalog migration adopts applications/catalog,
applications/coding-agent, and applications/chatbot-mesh as active source
roots. Agent Core integration profiles become core-owned. Catalog and
application consumers resolve an explicit AGENT_CATALOG_ROOT without
changing process CWD. Source and Go module paths move while /profiles and
/opt/agent-core/tools runtime mounts remain stable. Canonical
applications/catalog/v0.* tags coexisted with agent-profiles/v0.*
compatibility tags until GH-1373 trimmed each release to the single
root v0.* tag.

<!-- field: planned_work -->
### Planned work

| id | summary | status |
| --- | --- | --- |
| gh-1040 | Specify the applications and catalog migration contract. | done |
| gh-1037 | Rename examples source tree to applications. | done |
| gh-1038 | Make core service integrations use core-owned profiles. | done |
| gh-1041 | Relocate agent profiles into the applications catalog. | done |
| gh-1047 | Retire obsolete core catalog discovery plumbing. | done |
| gh-1039 | Remove catalog dependencies from core tests and audit. | done |
| gh-1043 | Introduce explicit catalog-root discovery and compatibility aliases. | done |
| gh-1048 | Align the root module registry with the applications layout. | done |
| gh-1046 | Cut root Mage orchestration and release gates to applications. | done |
| gh-1044 | Migrate coding-agent to the applications catalog. | done |
| gh-1042 | Migrate chatbot-mesh to the applications catalog. | done |
| gh-1049 | Propagate chatbot catalog roots through integrations. | done |
| gh-1050 | Initialize the chatbot Helm checkpoint database. | done |
| gh-1045 | Update catalog taxonomy and sweep migrated path references. | done |

<!-- field: srds -->
### Srds

- srd037-tool-dispatch-simplification
- srd043-application-catalog-layout

<!-- field: use_cases -->
### Use cases

| id | summary | status |
| --- | --- | --- |
| rel99.0-uc001-tool-dispatch-simplification | Maintainer simplifies tool dispatch wiring without changing core dispatch | done |
| rel99.0-uc002-application-catalog-migration | Maintainers migrate applications and reusable catalog blocks to explicit owner roots | done |

<!-- field: test_suites -->
### Test suites

| id | summary | status |
| --- | --- | --- |
| test-rel99.0-tool-dispatch-simplification | Formal planning tests for tool dispatch simplification scope and follow-up code proof | implemented |
| test-rel99.0-application-catalog-migration | Formal issue-by-issue tests for source moves, core isolation, catalog roots, path classes, tags, applications, and final taxonomy | implemented |

<!-- field: follow_up -->
### Follow up

| id | summary | status |
| --- | --- | --- |
| GH-171#173 | Centralize dynamic tool availability checks in code | done |
