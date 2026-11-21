---
id: cfg-tool-declaration-format
title: Tool Declaration Format (tool_config_dirs and declaration YAML)
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines the YAML schema for declaring tools that agents can use.
Tool declarations describe purpose, invocation, parameters, and
runtime contract. Two tool types exist: builtin tools
(compiled into the agent binary) and exec tools (external executables).

<!-- field: builtin_fields -->
## Builtin fields

<!-- builtin_field: init -->
### init

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Factory name in the BuiltinRegistry. Maps to a Go constructor
that creates the tool instance at startup (e.g. "file_read",
"invoke_llm", "parse_response").

<!-- builtin_field: config -->
### config

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Tool-specific configuration passed to the factory function.
Decoded into typed Go structs via DecodeToolConfig at registration
time. Type mismatches and unknown top-level keys produce errors mentioning
the tool name; older runtimes reject config authored for a newer word
rather than silently ignoring behavior they do not implement.
Structure varies per tool. Boundary tools that spawn child agents use
profile-only child_agent_config for self_invoke and profile aliases such as invoke_executor.
LLMToolConfig configures invoke_llm. LoadSuiteConfig configures
parse_suite_config and evaluator session setup tools. RunPointConfig
configures run_point. EvaluationArtifactsConfig configures read-only
evaluation artifact query words.
Filesystem document resource words use document_resource_config.
See cfg-llm-config-format for invoke_llm details.

<!-- field: document_resource_config -->
## Document resource config

<!-- field: purpose -->
### Purpose

Defines trusted read-only filesystem resources for list_resource and
read_resource builtin words. list_resource shapes newline-delimited paths
produced by a preceding declared filesystem CLI word; read_resource accepts
a relative path inside a configured resource. Runtime input cannot replace
roots, include patterns, extension allowlists, parser modes, or size limits.

<!-- field: fields | form: "typed table" -->
### Fields

| field | type | required | description |
| --- | --- | --- | --- |
| resources | object | true | Map of resource names to filesystem resource definitions. |

<!-- field: resource_fields -->
### Resource fields

<!-- resource_field: root -->
#### root

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Workspace-relative or absolute trusted corpus root.

<!-- resource_field: include -->
#### include

<!-- field: type -->
##### Type

[]string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Slash-style include patterns for list and read operations.

<!-- resource_field: extensions -->
#### extensions

<!-- field: type -->
##### Type

[]string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Extension allowlist without or with leading dots.

<!-- resource_field: modes -->
#### modes

<!-- field: type -->
##### Type

[]string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Enabled response modes such as raw_yaml, parsed_yaml, raw_markdown, and parsed_markdown.

<!-- resource_field: max_bytes -->
#### max_bytes

<!-- field: type -->
##### Type

int

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Maximum readable file size before size_limit_exceeded.

<!-- resource_field: category_rules -->
#### category_rules

<!-- field: type -->
##### Type

[]object

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Ordered profile-owned path rules with pattern and category fields.
The first matching slash-style include pattern supplies presentation
metadata. Unmatched root files use root; nested files use their first
directory segment.

<!-- field: examples -->
### Examples

<!-- field: docs_resource -->
#### Docs resource

```yaml
config:
  resources:
    docs:
      root: docs
      include: ["**/*.yaml", "*.yaml", "**/*.md", "*.md"]
      extensions: [yaml, yml, md]
      modes: [raw_yaml, parsed_yaml, raw_markdown]
      max_bytes: 65536
      category_rules:
        - {pattern: road-map.md, category: release}
        - {pattern: "specs/software-requirements/**", category: srd}
```

<!-- field: schema -->
## Schema

<!-- schema_entry: name -->
### name

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Unique tool identifier. Referenced by tool selection files
(tools.yaml), machine transition actions, and LLM tool-call
output. Must be unique across all loaded declaration files.

<!-- schema_entry: type -->
### type

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Tool type discriminator. Either "builtin" (compiled-in Go
function) or "exec" (external executable). Defaults to "exec"
when omitted.

<!-- schema_entry: category -->
### category

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Tool vocabulary category used by contract validation and authoring
guidance. Known categories are word, sentence, boundary, and
stateful_internal. Boundary tools intentionally cross a program
boundary, such as invoking an LLM, spawning an agent subprocess, or
running a nested state machine. If omitted, validation categorizes
internal tools as internal, external builtins as external_builtin,
and exec tools as exec.

<!-- schema_entry: contract -->
### contract

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Short contract summary for the tool. A concise statement of
the tool's purpose as a grammar word. Used by authoring and
validation tooling; not sent to the LLM.

<!-- schema_entry: description -->
### description

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Human-readable description of the tool's purpose and behavior.
Included in the tool schema sent to the LLM so it can decide
when and how to use the tool. Recommended but not enforced by
validateToolDefs.

<!-- schema_entry: problem -->
### problem

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field describing the vocabulary gap this tool fills.
It should explain why this tool exists as a single word in the
agent language. This is not currently sent to the LLM manifest.

<!-- schema_entry: goals -->
### goals

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field listing measurable success goals for the tool.
Goals describe what a correct implementation makes possible.

<!-- schema_entry: requirements -->
### requirements

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field grouping observable "must" statements by
input, output, side_effects, undo, and errors. These sections
define the developer-facing requirements for the tool as a
deterministic vocabulary word.

<!-- schema_entry: non_goals -->
### non_goals

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field listing behavior this tool intentionally does
not provide. Non-goals help the agent choose between overlapping
tools and avoid using one word as a hidden workflow.

<!-- schema_entry: parameters -->
### parameters

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

JSON Schema describing the tool's input parameters. This schema
is sent to the LLM for function-calling and is used for input
validation at runtime. Omitted for builtin tools that take no
parameters (e.g. parse_response, done). Exec tools may include
parameter extensions (see parameter_extensions) that are stripped
before sending to the LLM. Builtins may not declare exec-only source,
flag, positional, or position extensions. Defaults are strings and
position values are integers; startup rejects other types.

<!-- schema_entry: visibility -->
### visibility

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Controls whether the tool appears in the LLM's tool list.
"external" (default) — visible to the LLM and available for
tool calls. "internal" — hidden from the LLM; can only be
invoked by machine transitions (e.g. invoke_llm, parse_response).
Startup rejects every other value so a typo cannot expose an internal
word through the permissive default.

<!-- schema_entry: phases -->
### phases

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Optional additional restriction on dynamic-tool availability. Every
authored phase must name a state in the loaded MachineSpec; startup
rejects an unknown state rather than registering an invisible word.

<!-- schema_entry: emits -->
### emits

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Declarative signal contract for the tool. Lists the Result.Signal
values the tool may return, such as ToolDone, ToolFailed, CommandError,
or domain-specific signals like EvalPointsMaterialized and PointsCompleted. Startup
validation checks action tools with emits metadata against the loaded
machine: emitted signals must be declared by the machine, and each
emitted signal must have a transition from the action's target state
unless that target state is terminal.

<!-- schema_entry: output -->
### output

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field describing the structured output produced by the
tool. Contains optional description, schema, and mode fields. The schema
is JSON Schema-compatible and describes the machine-readable output
contract. For exec tools, mode may be raw (the default) or structured.
Structured mode sets core.Result.Output to a JSON object containing the
captured output string and exit_code so a later command-state selector can
address $from(<label>).output. Other tools and raw exec tools retain the
human-readable/runtime string channel.

<!-- schema_entry: metrics -->
### metrics

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: status -->
#### Status

planned

<!-- field: description -->
#### Description

Planned contract field for tool metric declarations. It describes metric
instruments emitted by this tool, the source of each value, and the
allowed low-cardinality attributes. It is documentation only until the
monitor recorder and validation code land.

Tools own specialized metric meaning. Runtime code may attach standard
dispatch context such as tool name, state, signal, duration, and status.
Metric labels must not include raw prompts, full outputs, secrets,
unbounded paths, arbitrary URLs, or user-supplied free text.

<!-- schema_entry: side_effects -->
### side_effects

<!-- field: type -->
#### Type

string or list

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field declaring world mutations beyond the returned output.
The legacy scalar string form is still accepted for compatibility
and may be appended to the LLM-visible description. New declarations
should use the structured list form with entries containing kind,
target, paths, state, and description.
Serialization preserves a legacy scalar as a scalar and structured
effects as a list; the internal representation is never emitted.

Known side-effect kind vocabulary (from current declarations):
  none — no side effects (pure read or computation).
  filesystem_read — reads files but does not modify them.
  filesystem_write — creates, modifies, or deletes files.
  state_read — reads in-memory agent state without mutation.
  state_mutation — mutates in-memory agent/domain state.
  command_state — modifies command-local state carried across calls.
  stderr_write — writes diagnostic output to stderr.
  child_process — spawns an external process (exec tools).
  child_agent_execution — spawns a child agent subprocess.
  child_tool_execution — runs child commands within the same loop.
  nested_machine_execution — runs a nested core.Loop state machine.
  external_api — calls an external API or issue tracker.
  human_boundary — blocks on human input (UI or approval).

<!-- schema_entry: reversibility -->
### reversibility

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field classifying whether effects can be undone.
classification is one of reversible, compensatable, or irreversible.
undo describes the undo/compensation mechanism. requires_confirmation
marks tools that require explicit approval before use. Startup rejects an
unknown classification so it cannot disable receipt validation.

<!-- schema_entry: undo -->
### undo

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field describing how to reverse or compensate for the
tool's effects. This complements reversibility by naming the
specific rollback mechanism and metadata the persisted
UndoMemento captures.

<!-- field: sub_fields -->
#### Sub fields

<!-- sub_field_entry: strategy -->
##### strategy

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

The undo strategy name. Known values: noop,
conversation_truncate, file_snapshot_restore, workspace_restore,
compensating_action, child_command_undo, and irreversible. Builtin
strategy names describe typed Undo implementations. Exec words are
restricted to noop, workspace_restore, compensating_action, or
irreversible because one shared interpreter executes their Undo.

<!-- sub_field_entry: description -->
##### description

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

Human-readable description of the rollback mechanism.

<!-- sub_field_entry: payload -->
##### payload

<!-- field: type -->
###### Type

string

<!-- field: description -->
###### Description

Memento payload type. When set to "boundary_compensation",
the UndoMemento carries a BoundaryCompensationPayload with
structured metadata about the external boundary crossed
(child process, issue system, nested machine, etc.).
Omitted for simple workspace_restore payloads.

<!-- sub_field_entry: captures -->
##### captures

<!-- field: type -->
###### Type

[]string

<!-- field: description -->
###### Description

Names of runtime parameter or structured-output values captured in
the UndoMemento payload. Generic exec words project these names into
receipt.captures; they are executable contract data, not comments.
Examples include
issue_id, child_run_id, workspace_paths, server_addr, and
suite_path.

<!-- sub_field_entry: requires -->
##### requires

<!-- field: type -->
###### Type

[]string

<!-- field: description -->
###### Description

Names of external context needed to perform the undo.
For example, ["approval_decision", "checkpoint_id"] for
a suspend tool that requires re-obtaining approval state.

<!-- schema_entry: errors -->
### errors

<!-- field: type -->
#### Type

[]object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field describing expected failure modes. Each entry can
declare signal, condition, message_shape, and state_after_failure.
This lets developers and agents understand what state the world is
in after a failed command.

<!-- schema_entry: relationships -->
### relationships

<!-- field: type -->
#### Type

object

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Contract field documenting common composition neighbors and
overlapping tools. before lists tools that typically precede this
tool, after lists tools that typically consume this tool's output,
and overlaps explains similar tools and how they differ. These are
guidance, not execution constraints.

<!-- field: metric_declaration_fields -->
## Metric declaration fields

<!-- field: status -->
### Status

planned

<!-- field: purpose -->
### Purpose

Defines the planned shape of ToolDef metrics config. Metric declarations
reserve vocabulary for follow-up monitor SRDs and code. They should be
written only for values the tool can emit safely through a shared recorder.

<!-- field: fields -->
### Fields

<!-- field_entry: instruments -->
#### instruments

<!-- field: type -->
##### Type

[]object

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Metric instruments emitted by the tool. Each item declares name, kind,
unit, description, value_source, and allowed attributes.

<!-- field_entry: attributes -->
#### attributes

<!-- field: type -->
##### Type

[]object

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Shared attribute declarations for this tool. Attribute values must come
from trusted config, bounded runtime enums, or redacted low-cardinality
values allowed by the metric schema.

<!-- field_entry: disabled -->
#### disabled

<!-- field: type -->
##### Type

bool

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Explicit opt-out for a tool that would otherwise receive standard
dispatch metrics from profile or runtime policy.

<!-- field: instrument_fields -->
### Instrument fields

<!-- instrument_field: name -->
#### name

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Stable metric name scoped by tool family or package.

<!-- instrument_field: kind -->
#### kind

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

One of counter, up_down_counter, histogram, or gauge.

<!-- instrument_field: unit -->
#### unit

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Unit string such as ms, By, "{dispatch}", or "{retry}".

<!-- instrument_field: description -->
#### description

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Human-readable metric meaning.

<!-- instrument_field: value_source -->
#### value_source

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Named source for the metric value. Examples include dispatch_duration,
result_output_bytes, exit_code, http_status_code, prompt_tokens, or a
tool-specific safe field.

<!-- instrument_field: attributes -->
#### attributes

<!-- field: type -->
##### Type

[]string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Names of attributes allowed on this instrument.

<!-- instrument_field: buckets -->
#### buckets

<!-- field: type -->
##### Type

[]number

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Optional histogram buckets for duration, byte, or count data.

<!-- field: attribute_fields -->
### Attribute fields

<!-- attribute_field: name -->
#### name

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Stable attribute name.

<!-- attribute_field: source -->
#### source

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Trusted source for the value. Allowed source classes are config_literal,
tool_name, tool_family, state, signal, status, error_class,
workflow_label, and configured_operation.

<!-- attribute_field: cardinality -->
#### cardinality

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Must be low or bounded.

<!-- attribute_field: allowed_values -->
#### allowed_values

<!-- field: type -->
##### Type

[]string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Explicit allowed set for bounded string labels.

<!-- attribute_field: redaction -->
#### redaction

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Omit, hash, classify, or none. Defaults to omit for unsafe sources.

<!-- field: safety_rules -->
### Safety rules

- Do not use raw prompts as metric labels.
- Do not use full model responses or full command output as metric labels.
- Do not use secret values, authorization headers, cookies, passwords, or tokens.
- Do not use unbounded absolute paths, arbitrary URLs, request IDs, timestamps, or user free text.
- Prefer configured operation names, path classes, status enums, and workflow labels.

<!-- field: example -->
### Example

```yaml
metrics:
  instruments:
    - name: rest_client_request_duration
      kind: histogram
      unit: ms
      description: Duration of one configured REST client request.
      value_source: dispatch_duration
      attributes: [tool_name, state, signal, status, operation]
  attributes:
    - name: operation
      source: configured_operation
      cardinality: bounded
      allowed_values: [documentation_curator.documents]
      redaction: none
```

<!-- field: child_agent_config -->
## Child agent config

<!-- field: purpose -->
### Purpose

Defines config for builtin boundary tools that spawn another agent process.
This applies to self_invoke and profile aliases such as invoke_executor.

<!-- field: fields -->
### Fields

<!-- field_entry: profile -->
#### profile

<!-- field: type -->
##### Type

string

<!-- field: required | form: "scalar" -->
##### Required

true

<!-- field: description -->
##### Description

Path to the child agent profile YAML. The child process receives this
value as --profile. The profile owns the child's machine, tools.yaml,
tool_config_dirs, tool_declarations, and agent-local config.

<!-- field: legacy_fields -->
### Legacy fields

<!-- field: status -->
#### Status

planned_removal

<!-- field: fields -->
#### Fields

- machine
- tools
- tools_declarations

<!-- field: rule -->
#### Rule

New declarations and examples must not use these fields. Existing Go
structs still decode them until the profile-only child invocation code
cleanup lands. Documentation should mention them only as implementation
drift.

<!-- field: examples -->
### Examples

<!-- field: invoke_executor -->
#### Invoke executor

```yaml
init: self_invoke
config:
  profile: ${AGENT_CATALOG_ROOT}/generator/profile.yaml
```

<!-- field: self_invoke -->
#### Self invoke

```yaml
config:
  profile: ${AGENT_CATALOG_ROOT}/generator/profile.yaml
  request_from: $from(request).path
  output_from: $from(request).output_dir
```

<!-- field: exec_fields -->
## Exec fields

<!-- exec_field: binary -->
### binary

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

true

<!-- field: description -->
#### Description

Executable name or path. Resolved via PATH at runtime.

<!-- exec_field: args -->
### args

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Base arguments prepended to every invocation. Parameter values
from the LLM's tool call are appended after these.

<!-- exec_field: dir -->
### dir

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Working directory override for exec tool invocation. When set,
the external process runs in this directory instead of the
default workspace directory.

<!-- exec_field: precondition -->
### precondition

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

A named precondition that must be satisfied before the tool can
run (e.g. "git_repo" requires the workspace to be a git
repository).

<!-- exec_field: output_cap -->
### output_cap

<!-- field: type -->
#### Type

int

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Maximum number of output lines captured from the tool's stdout.
Excess lines are truncated to keep LLM context manageable.

<!-- exec_field: stdin_source -->
### stdin_source

<!-- field: type -->
#### Type

string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Canonical $from(<label>).<dotted-path> selector for a prior redacted
command-state output field. The resolved value must be a non-empty string
within stdin_max_bytes and is written to the declared binary's stdin.
Resolution and size validation finish before launch. No shell is invoked.

<!-- exec_field: stdin_max_bytes -->
### stdin_max_bytes

<!-- field: type -->
#### Type

int

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

Positive byte limit for stdin_source. It is invalid without stdin_source.
When omitted, the exec runtime applies its finite default limit. Oversized
input is rejected before launch rather than truncated.

<!-- exec_field: env -->
### env

<!-- field: type -->
#### Type

[]string

<!-- field: required | form: "scalar" -->
#### Required

false

<!-- field: description -->
#### Description

KEY=VALUE entries overlaid on the inherited parent environment before
launch. KEY must match [A-Za-z_][A-Za-z0-9_]*. VALUE may contain
"{{ params.NAME }}" tokens expanded from resolved exec parameters after
command-state source mapping. Env is not a $from selector surface.
Omitted or empty env inherits the parent environment only. Builtin
declarations cannot set env. Resolved values are not written to traces,
metrics, or receipts.

<!-- field: parameter_extensions -->
## Parameter extensions

<!-- field: description -->
### Description

Exec tools support custom JSON Schema extensions on individual
parameter properties. These extensions control how parameter values
are mapped to CLI arguments when invoking the external executable.
A source extension can instead select a value from an earlier command-state
entry. All extensions are stripped from the schema before it is sent to the
LLM, so the model never sees CLI-level or command-state wiring details.

<!-- field: fields -->
### Fields

<!-- field_entry: flag -->
#### flag

<!-- field: type -->
##### Type

string

<!-- field: description -->
##### Description

CLI flag mapping. The parameter value is passed as this flag
(e.g. "--path" causes the value to be passed as --path <value>).

<!-- field_entry: positional -->
#### positional

<!-- field: type -->
##### Type

bool

<!-- field: description -->
##### Description

When true, the parameter value is passed as a positional
argument rather than a named flag.

<!-- field_entry: bool_flag -->
#### bool_flag

<!-- field: type -->
##### Type

bool

<!-- field: description -->
##### Description

Boolean flag marker. When true and the parameter value is
true, the CLI flag from the "flag" field is included in the
command (e.g. flag: "--all", bool_flag: true).

<!-- field_entry: default -->
#### default

<!-- field: type -->
##### Type

string

<!-- field: description -->
##### Description

Default value used when the LLM omits the parameter. Applied
before CLI argument construction.

<!-- field_entry: position -->
#### position

<!-- field: type -->
##### Type

int

<!-- field: description -->
##### Description

Positive ordering key for mapped arguments. Positioned mappings are
appended in ascending order after the tool's base args. Unpositioned
mappings follow in stable parameter-name order.

<!-- field_entry: source -->
#### source

<!-- field: type -->
##### Type

string

<!-- field: description -->
##### Description

Optional canonical $from(<label>).<dotted-path> selector. The exec
command resolves it from the injected command-state view before
subprocess launch. When omitted, the parameter keeps the existing
parameters.<name> lookup against the immediately previous Result.
A malformed selector fails declaration validation. An unresolved label
or path emits CommandError and prevents subprocess launch; it never
substitutes an empty value.

<!-- field: example -->
##### Example

$from(seed).parameters.directory

<!-- field: declaration_files -->
## Declaration files

<!-- declaration_file: tools/builtin/ -->
### tools/builtin/

<!-- field: description -->
#### Description

Directory of individual shared filesystem and builtin tool declarations
loaded by current profiles through tool_config_dirs. The declarations
cover file operations (read, write, edit, find, and the exec-bound
list_files declaration), the LLM
pipeline (parse_response, report_parse_error, reset_history),
completion and control (done, suspend, nudge_reread, self_invoke),
planner tools, evaluator session and point tools, bench tools, and spec
validation tools. Note: invoke_llm is declared per-agent in <profile-root>/*/llm/*.yaml,
not in this directory. The tool count evolves with the codebase;
consult the directory directly for the current set.

<!-- declaration_file: tools/exec/ -->
### tools/exec/

<!-- field: description -->
#### Description

Directory of individual shared exec tool declarations loaded by
current profiles through tool_config_dirs. The declarations cover
Go build tools (build, vet, lint, test), filesystem utilities
(copy_dir, make_dir), git operations (git_init, stage_all,
workspace_status, commit, rev_parse, diff_stat, log_oneline), and
issue management (issue_create, issue_close, issue_list). The
tool count evolves with the codebase; consult the directory
directly for the current set.

<!-- declaration_file: <profile-root>/*/llm/*.yaml -->
### <profile-root>/*/llm/*.yaml

<!-- field: description -->
#### Description

Per-model invoke_llm declarations with config blocks specifying
model name, provider, endpoint URL, and prompts. Each agent
directory contains its own llm/ subdirectory with one file per
model variant. See cfg-llm-config-format for details.

<!-- field: fragment_files -->
## Fragment files

<!-- field: description -->
### Description

A tool declaration file can be a fragment: a unit with declared, typed
parameters that another file expands with arguments
(srd052-declaration-fragments). The two file-level fields below sit
beside `unit` and `imports` (srd050). A fragment is only ever
expanded; listing one under `imports` is a load error. Substitution
replaces `$param(name)` inside scalar values only, never inside mapping
names or list structure, and the expanded document is decoded and
validated exactly as a hand-written file would be. Nothing generated is
committed. Arriving with GH-2081.

<!-- field: fields -->
### Fields

<!-- field_entry: params -->
#### params

<!-- field: type -->
##### Type

[]param

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Declares the fragment's parameters. Present together with exactly one
body (`tools` here). Each entry carries `name` and `type`, and
optionally `enum`, `default`, and `description`. `type` is one of
string, integer, number, boolean. A file with `params` and no body,
or a body with a `$param(` reference and no `params`, fails to load.

<!-- field_entry: expand -->
#### expand

<!-- field: type -->
##### Type

[]expansion

<!-- field: required | form: "scalar" -->
##### Required

false

<!-- field: description -->
##### Description

Expands fragments into this file's closure. Each entry carries
`fragment`, a path relative to this file under the import rules;
`args`, a mapping from parameter name to value; and optionally `as`,
a prefix applied as `<as>_` to every name the expansion produces
so one fragment can be expanded more than once. Every parameter
without a default must be supplied; an unknown argument, a wrong type,
or a value outside an enum fails naming this file, the fragment, and
the parameter.

<!-- field: example -->
### Example

```yaml
# units/embed-provider.yaml -- the fragment
unit: embed-provider
imports:
- ./types-core.yaml
params:
- name: provider
  type: string
  enum: [cohere, openai, ollama]
- name: model
  type: string
tools:
- name: embed_query
  type: builtin
  init: rest_client_invoke
  category: boundary
  description: Embed the query with $param(provider) model $param(model).
  config:
    rest_ref: $param(provider)_api
    operation: embed

# agents/rag/declarations.yaml -- the importer
unit: rag
imports:
- ../units/types-core.yaml
expand:
- fragment: ../units/embed-provider.yaml
  as: cohere
  args: {provider: cohere, model: embed-english-v3.0}
- fragment: ../units/embed-provider.yaml
  as: openai
  args: {provider: openai, model: text-embedding-3-small}
tools: []
# produces cohere_embed_query and openai_embed_query
```

<!-- field: library_imports -->
## Library imports

An import path, and a fragment path under `expand`, is relative to the
declaring file or absolute under a library root (srd056-unit-libraries R1).
`/opt/agent-core/...` names agent-core's library: the Standard Tool Library,
its type units, and its machine templates. It maps through `--core-root` in
a checkout and is used as written in the runtime image, so one path works in
both:

  unit: catalog-collector-declarations
  imports:
  - /opt/agent-core/tools/units/types-core.yaml
  - ../units/types-collector.yaml

`/opt/<name>/...` names a root the profile declares under `libraries`
(cfg-agent-profile-format); an undeclared name is a load error naming the
prefix and the profile. Any other absolute path fails as srd050 R1.2 states.
A library is read-only: a tool in a file under a root cannot carry
`override: true`, and a relative import inside a root cannot leave it,
though it may import another root by prefix. Rooted files are closure files
and program identity assets, and the dump marks each with its root. Landed with
GH-2118 (agent-core root) and GH-2119 (declared roots).

<!-- field: boundary_compensation_example -->
## Boundary compensation example

Boundary tools that cross process, issue, or human boundaries declare
payload: boundary_compensation in their undo section. At runtime, the
tool's UndoMemento() method serializes a BoundaryCompensationPayload
containing the values named in captures. Example from self_invoke:

  undo:
    strategy: compensating_action
    description: Requires child evaluator artifact cleanup.
    payload: boundary_compensation
    captures: [suite_path, output_dir, child_profile]

The persisted UndoMemento carries:
  {
    "boundary_compensation": {
      "strategy": "child_eval_artifact_compensation",
      "child_profile": "${AGENT_CATALOG_ROOT}/evaluator/profile.yaml",
      "workspace_paths": ["/tmp/eval-output-123"],
      "artifact_paths": ["/path/to/suite.yaml"]
    }
  }

Tools with boundary_compensation: suspend, self_invoke aliases, run_point,
run_agent, self_invoke, commit,
issue_create, issue_close, and profile-owned tracker exec words.

<!-- field: inline_pattern -->
## Inline pattern

All agents now use the declaration+selection split. Current profiles
load shared declarations from directories such as tools/builtin/ and
tools/exec/ through tool_config_dirs, then load agent-local explicit
declarations such as LLM configs or mode-specific builtin overrides.
Each agent's tools.yaml contains only tool names (selection). This
ensures a single source of truth for tool schemas and config while
keeping active profiles on individual declaration files.

<!-- field: contract_validation -->
## Contract validation

Contract completeness is checked once, by the corpus audit in pkg/spec,
over every tool a profile selects. A declaration with contract: legacy
draws a warning; a migrated or strict one draws an error for each missing
field, named with the tool, its source file, and the selections that use
it. A signature discharges the blocks the load-time defaults fill, and the
audit reads the same discharge table the loader applies (srd051 R6), so the
two cannot disagree about what a signature covers. Runtime startup checks
wiring only: emitted signals the machine can route, selector labels that
resolve, and receipt-consuming undo where a tool mutates state.

<!-- field: structured_output_strategy -->
## Structured output strategy

Tool output has two compatible runtime forms:

1. Raw mode (default): core.Result.Output remains the human-readable command
   output used by existing machine transitions, parse-error repair, logs, and
   tests.
2. Declarative output contract: tool declarations may provide output.schema
   to describe machine-readable data. Exec declarations may additionally set
   output.mode: structured to wrap captured output and exit code in a JSON
   object for command-state selection.

output.schema and output.mode are not injected into the LLM-facing input
schema. Omitted mode does not change command execution. Structured exec mode
is opt-in so existing consumers remain compatible while producer output can
be selected as $from(<label>).output.

<!-- field: mode_local_overrides -->
## Mode local overrides

Some agents have mode-local builtin override files that add config
blocks to tools declared in tools/builtin/. These are loaded after
the shared declaration directory through profile tool_declarations:

  <profile-root>/bench/builtin.yaml — profile aliases for generic REST lifecycle, predicate, and self_invoke words
  <profile-root>/evaluator/builtin.yaml — evaluator-specific config (suite/point tools)
  <profile-root>/planner/builtin.yaml — planner-specific config (child agent, issue tools)

The generator and jurist agents do not have mode-local overrides;
they use the shared builtin declaration directory directly.

<!-- field: legacy_artifacts -->
## Legacy artifacts

tools/builtin.yaml and tools/exec.yaml are legacy aggregate declaration
files retained for compatibility examples and historical migration context;
active profiles prefer tools/builtin/ and tools/exec/ directories. The
internal/tools/stl/tools.yaml file is an older exec declaration file that
predates tools/exec.yaml. It is NOT loaded at runtime and is not referenced
by any code or configuration. It contains additional tools (branch_create,
branch_delete, branch_list, branch_current, worktree_add, worktree_remove,
worktree_list, issue_claim) that are not yet migrated to the canonical exec
declarations. Retained as a reference for tools that may be promoted to the
canonical set.

<!-- field: target_grouped_layout -->
## Target grouped layout

<!-- field: status -->
### Status

planned

<!-- field: rule -->
### Rule

Current profiles load flat declaration directories. The next YAML move
groups shared declarations by tool family while keeping compatibility
bundle files loadable through profile-owned declaration paths and tests.

<!-- field: builtin_groups | form: "typed table" -->
### Builtin groups

| path | owns | kind | note |
| --- | --- | --- | --- |
| tools/builtin/filesystem/ | [read, write, edit, find, list_files] | shared |  |
| tools/builtin/llm/ | [parse_response, report_parse_error, reset_history, nudge_reread, done] | shared | invoke_llm remains agent-local under <profile-root>/*/llm/*.yaml because its config binds model, provider, prompt, and profile behavior. |
| tools/builtin/lifecycle/ | [suspend, checkpoint_history, checkpoint_rollback] | shared |  |
| tools/builtin/control/ | [self_invoke] | shared |  |
| tools/builtin/planner/ | [extract_task, select_all_ready, seed_passthrough_plan, mark_nodes_planning, project_planner_context, capture_planner_failure, parse_plan, format_issue, write, record_tracker_issue, mark_nodes_executing, format_task_file, mark_task_done, mark_task_failed, remaining_work] | shared planner-family declarations |  |
| tools/builtin/critic-session/ | [parse_suite_config, discover_suite_samples, expand_eval_grid, init_eval_session, report_suite_summary, materialize_eval_points, run_point, report_session] | shared critic-session declarations |  |
| tools/builtin/critic-point/ | [create_point_dir, sample_docs, record_agent_commit, dump_config, run_agent, record_oracle_result, collect_trace_tokens, check_agent_version, summarize_point_results, record_point_failure, collect_metrics] | shared critic-point declarations | Workspace copying, git setup, and rev_parse reuse tools/exec declarations; the external critic profile owns its deterministic commit_workspace_baseline and run_oracle_check exec aliases. |
| tools/builtin/spec-validation/ | [load_corpus, validate_specs, format_report] | shared jurist declarations |  |

<!-- field: exec_groups | form: "typed table" -->
### Exec groups

| path | owns | kind |
| --- | --- | --- |
| tools/exec/go/ | [build, vet, lint, test] | shared |
| tools/exec/workspace/ | [copy_dir, make_dir] | shared |
| tools/exec/git/ | [git_init, stage_all, workspace_status, commit, rev_parse, diff_stat, log_oneline] | shared |
| tools/exec/issues/ | [issue_create, issue_close, issue_list] | shared |

<!-- field: compatibility_bundles -->
### Compatibility bundles

| path | role |
| --- | --- |
| tools/builtin.yaml | Compatibility bundle that includes every shared builtin group needed by profiles or tests that load the aggregate declaration file. |
| tools/exec.yaml | Compatibility bundle that includes every shared exec group needed by profiles or tests that load the aggregate declaration file. |
| tools/builtin/all.yaml | Planned explicit bundle for profiles or tests that need the full builtin declaration set. |
| tools/exec/all.yaml | Planned explicit bundle for profiles or tests that need the full exec declaration set. |
| tools/builtin/generator.yaml | Planned optional bundle for generator shared builtins; profiles may instead load the needed groups directly. |
| tools/builtin/planner.yaml | Planned optional bundle for planner shared builtins; agent-local overrides still live under <profile-root>/planner/. |
| tools/builtin/evaluator.yaml | Planned optional bundle for evaluator session and point builtins; agent-local config still lives under <profile-root>/evaluator/. |
| tools/builtin/bench.yaml | Planned optional bundle for bench builtins; agent-local config still lives under <profile-root>/bench/. |
| tools/builtin/jurist.yaml | Planned optional bundle for spec validation builtins. |
| tools/builtin/lifecycle.yaml | Planned optional bundle for lifecycle builtins. |

<!-- field: include_rules -->
### Include rules

- Includes are relative to the file that declares them.
- Included files load before local tools.
- Later declarations with the same tool name override earlier declarations.
- Shared group files define reusable identity, parameters, visibility, emits, and contracts.
- Agent-local files define behavior-shaping config for one profile or mode.
- Agent-local overrides preserve the same name and init unless they intentionally replace a capability.
- tools.yaml selection files remain name-only and do not include declaration paths.

<!-- field: migration_rules -->
### Migration rules

- Move declarations into grouped files only after this format contract is in place.
- Keep tools/builtin.yaml and tools/exec.yaml loadable as compatibility bundles during the move.
- Keep active profiles loadable through current tool_config_dirs and tool_declarations until code supports narrower group paths.
- Do not move invoke_llm into a shared group; keep model-boundary declarations agent-local.

<!-- field: related_documents -->
## Related documents

- [srd056-unit-libraries](../software-requirements/srd056-unit-libraries.md)
- [srd052-declaration-fragments](../software-requirements/srd052-declaration-fragments.md)
- [cfg-tool-authoring-standard](tool-authoring-standard.md)
- srd013
- srd021
- srd022
- srd023
- srd024
- srd004
- [cfg-tool-selection-format](tool-selection-format.md)
- [cfg-llm-config-format](llm-config-format.md)

<!-- field: implementation -->
## Implementation

- internal/tools/catalog/tooldef.go
- internal/tools/catalog/env.go
- internal/tools/catalog/loading.go
- internal/tools/catalog/contracts.go
- internal/tools/catalog/emits.go
- internal/tools/catalog/config.go
- internal/tools/registry/builtin.go
- internal/tools/exec/builder.go
- internal/tools/exec/env.go
