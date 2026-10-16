---
id: rel01.0-uc001-generator-coding
title: Generator agent solves Go coding tasks using YAML config
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

The external generator profile configures the generator agent. It points to
the machine, tool selection, shared tool declarations, and per-model LLM tool
declaration. During a run, the agent invokes an LLM, parses tool calls,
executes file/build/test tools, and stops after validation passes. The same
profile-first path keeps the generator workflow in YAML instead of
mode-specific Go branches, so the integration target checks runtime wiring as
well as task completion.

applications/coding-agent mage integration:executorLive exercises the path with
the canonical executor. That target
validates the Qwen parsing pipeline, including thinking-block stripping,
[tool_call] envelope extraction, and the Composing/Parsing loop through
build/lint/test validation.

<!-- field: actor -->
## Actor

Developer invoking the agent binary

<!-- field: trigger -->
## Trigger

```yaml
|
    A developer, or the coding-agent executorLive integration, runs:
      agent --profile "$AGENT_CATALOG_ROOT/generator/profile.yaml" \
            --directory <workspace>
```

<!-- field: preconditions -->
## Preconditions

- Ollama is running at localhost:11434
- The qwen3.6:35b-mlx model (or equivalent qwen3.6 variant) is pulled
- The workspace contains go.mod, an SRD, a source stub, and oracle tests

<!-- field: flow -->
## Flow

- F1: cmd/agent parses CLI flags and loads the external generator profile, resolving machine, tool selection, tool declaration files, and tool config directories relative to the profile file (cfg-runtime-contract, srd013).
- F2: Tool declarations are loaded from explicit profile files and configured directories; tool selection filters the declarations to the subset listed in generator/tools.yaml relative to the profile root (cfg-tool-selection-format).
- F3: The invoke_llm tool definition provides model, provider URL, and system prompt. cmd/agent extracts this to configure the Ollama adapter (srd009, srd017).
- F4: The invoke_llm config selects response_profile to configure per-model parsing pipelines (srd017).
- F5: The LLM profile registry matches the "qwen" prefix and selects the qwen profile with [tool_call] envelope and strip_thinking_blocks pipeline (srd017, internal/model/llm/profiles/qwen.yaml).
- F6: Builtin and exec tool factories are registered (file tools, LLM tools, build/lint/test, done, etc.) and matched to their YAML declarations (srd013).
- F7: The state machine is loaded from generator/machine.yaml relative to the profile root and validated against the registered tools (srd006).
- F8: core.Loop drives the state machine. Idle → Composing via invoke_llm sends the task prompt to the LLM (srd005, srd009).
- F9: Qwen 3.6 responds with a <think>...</think> block followed by a [tool_call]...[/tool_call] envelope containing a JSON tool call.
- F10: parse_response runs the qwen extraction pipeline — strip_thinking_blocks removes <think> content, extract_envelope isolates the tool call JSON (srd017).
- F11: The extracted tool call dispatches via $tool to file tools (read, write, edit) for code changes (srd013, generator/machine.yaml).
- F12: The Composing ↔ Parsing cycle repeats until the LLM calls done (generator/machine.yaml).
- F13: After done, machine-declared validation runs build → lint → test. All oracle tests pass, transitioning to Succeeded (srd006, srd013).

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | srd006-yaml-machine-config R1.1, R1.2, R1.3, R1.4, R1.5, R1.6, R1.7, R1.8, R1.9, R2.1, R2.2, R2.3, R2.4, R2.5, R2.6, R2.7, R2.8, R2.9, R2.10, R2.11, R3.1, R3.2, R3.3, R3.4, R3.5, R4.1, R4.2, R4.3, R4.4 | MachineSpec loading, $tool dispatch, transition validation |
| T2 | srd009-ollama-adapter R1.1, R1.2, R1.3, R1.4, R2.1, R2.2, R2.3, R2.4, R4.1, R4.2, R4.3, R4.4 | configured adapter, Ollama Chat API request, model availability |
| T3 | srd013-standard-tool-library R1.1, R1.2, R1.3, R1.4, R1.5, R1.6, R2.1, R2.2, R2.3, R2.4, R3.1, R3.2, R3.3, R3.4, R3.5, R3.6, R3.7, R5.1, R5.2, R5.3, R5.4, R5.5, R5.6, R5.7, R8.1, R8.2, R8.3, R9.1, R9.2 | file tools, build tools, LLM commands, declarations, validation, done |
| T4 | srd017-llm-extensions R2.1, R2.2, R2.3, R2.4, R2.5, R2.6, R2.7, R2.8, R2.9, R4.1, R4.2, R4.3, R4.4, R4.5 | qwen profile resolution, thinking-block stripping, envelope extraction |
| T5 | cfg-runtime-contract | profile-first invocation and compatibility flags |
| T6 | cfg-tool-declaration-format | builtin, exec, and llm tool declarations |
| T7 | cfg-tool-selection-format | tools.yaml name-list filtering |
| T8 | generator/machine.yaml | state machine states, signals, lifecycle |
| T9 | internal/model/llm/profiles/qwen.yaml | model-specific parsing pipeline |
| T10 | generator/profile.yaml | profile-relative runtime entry point under AGENT_CATALOG_ROOT |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

Agent starts from YAML configs without any Go code changes.

<!-- field: traces -->
#### Traces

- srd006-yaml-machine-config AC3

<!-- sc: S2 -->
### S2

LLM is invoked with the configured model and system prompt.

<!-- field: traces -->
#### Traces

- srd009-ollama-adapter AC1

<!-- sc: S3 -->
### S3

The qwen profile is selected automatically from the model name prefix.

<!-- field: traces -->
#### Traces

- srd017-llm-extensions AC1

<!-- sc: S4 -->
### S4

Thinking blocks (<think>...</think>) are stripped before tool call extraction.

<!-- field: traces -->
#### Traces

- srd017-llm-extensions AC2

<!-- sc: S5 -->
### S5

Tool calls are extracted from the [tool_call]...[/tool_call] envelope.

<!-- field: traces -->
#### Traces

- srd017-llm-extensions AC3

<!-- sc: S6 -->
### S6

Tool calls extracted from LLM responses are dispatched and results fed back.

<!-- field: traces -->
#### Traces

- srd013-standard-tool-library AC1

<!-- sc: S7 -->
### S7

The done tool triggers build/vet/lint/test validation before declaring success.

<!-- field: traces -->
#### Traces

- srd013-standard-tool-library AC2

<!-- sc: S8 -->
### S8

The agent reaches the Succeeded terminal state after passing build/lint/test validation.

<!-- field: traces -->
#### Traces

- srd006-yaml-machine-config AC3

<!-- sc: S9 -->
### S9

Oracle tests in the sample workspace pass without modification.

<!-- field: traces -->
#### Traces

- srd013-standard-tool-library AC2

<!-- field: integration_test -->
## Integration test

applications/coding-agent mage integration:executorLive

<!-- field: test_data -->
## Test data

applications/coding-agent/testdata/integration/coding-loop/workspace

<!-- field: test_suite_trace -->
## Test suite trace

test-rel01.0

<!-- field: out_of_scope -->
## Out of scope

- Multi-file project scaffolding beyond the workspace directory.
- Custom tool implementations not in builtin.yaml or exec.yaml.
- Comparison with other model families (handled by the evaluator use case).

<!-- field: machine_config -->
## Machine config

generator/machine.yaml
