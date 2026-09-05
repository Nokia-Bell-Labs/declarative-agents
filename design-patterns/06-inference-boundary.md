<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Inference Boundary

This chapter presents the Inference Boundary pattern, which places all inference behind one tool and a provider adapter. Swapping a model supported by that adapter is a configuration change; adding a provider requires an adapter implementation while leaving the machine and other tools unchanged.

## Intent

Place all model inference behind one tool and adapter interface so model changes stay in configuration and provider changes stay inside a new adapter rather than spreading into the machine or other tools.


## Motivation

Agent frameworks typically call the model from many sites, a planning step with one prompt format, a generation step with another, an evaluation step with a third, each parsing responses and handling errors differently. The line where deterministic harness ends and stochastic inference begins is implicit and scattered. **Boundary drift:** refactors silently move inference logic into tools. **Model coupling:** if prompt formats and API calls live in tools, switching provider (Ollama → OpenAI → Anthropic) means editing every tool, and evaluating across models means maintaining parallel implementations. **Accounting fragmentation:** token and latency data accrue across sites, so per-task usage requires instrumenting every path.

Funneling every model interaction through one tool and one adapter interface solves all three: one tool touches the model, one adapter translates between the harness's prompt protocol and the provider's API, and everything else stays on the harness side of the boundary.


## Applicability

The Inference Boundary fits any agent that needs to support model families or provider adapters without changing its machine or tools. It is especially useful when evaluation runs the same harness against different models to separate model contribution from harness contribution, or when token usage and latency should aggregate at one point. When the model is fixed with no prospect of change or comparison, the indirection adds nothing.


## Structure

Five participants sit behind the single inference tool. Fig. 17 is their component diagram.

![](figures/fig-18-model-adapter-class.png)

| **Figure 17.** Component diagram. InvokeLLM is the sole inference tool; it draws on the PromptAssembler and LLMConfig, requires the ProviderAdapter interface that a provider-specific adapter provides, and routes the reply through the ResponseParser. |
|:---:|

### Participants

#### InvokeLLM

Is the only tool that crosses the inference boundary. To the machine it is one dispatch returning one signal (`LLMResponded`); internally it orchestrates assembly, adaptation, and parsing.

#### PromptAssembler

Builds a provider-agnostic prompt from the current state (system instructions), conversation history, and the state-filtered manifest (Chapter 5); it never constructs provider-specific payloads.

#### ProviderAdapter

Translates that prompt into a provider's API call and the reply back, one implementation per provider behind a single interface.

#### ResponseParser

Normalizes the reply (tool calls, completion, free text) into a uniform result.

#### LLMConfig

Is the YAML knob (provider, model, temperature, limits); changing the model is an edit, not a code change.


## Collaborations

Every interaction follows the same path, regardless of provider, traced by the sequence diagram in Fig. 18: the engine **dispatches** `invoke_llm`; the tool has the assembler **build** a provider-agnostic prompt; the **adapter** serializes it to the provider's format and issues the HTTP request; the **parser** turns the raw reply into a normalized result; and the tool returns **`LLMResponded`** with that result. All of this runs inside `Execute`. From outside, one dispatch occurred and one signal returned, and no other participant knows which provider answered.

![](figures/fig-19-model-adapter-sequence.png)

| **Figure 18.** Sequence diagram. A single `invoke_llm` dispatch flows through prompt assembly, the provider adapter's HTTP exchange, and response parsing before returning the `LLMResponded` signal, identical for every provider. {wide} |
|:---:|

Because every interaction passes through one tool, parsed results can return input/output token usage and duration at one point. The shipped Ollama path does not establish monetary-cost accounting.


## Consequences

### Benefits

#### Model-agnostic evaluation

Running the same machine and tools against different models is a config change, making convergence rates, tokens, and duration comparable across models. Harness--model separability depends on this isolation.

#### Provider portability

Changing models within one provider is configuration. Supporting a new provider requires adapter code, but the machine and tool boundary remain stable. Ollama is the shipped provider adapter; srd048 specifies Cohere v2 as the second adapter.

#### Single instrumentation point

Spans and token accounting attach to one tool; `invoke_llm` maps to the GenAI `chat` operation and a `chat <model>` span (Chapter 8), one span per invocation.

#### Cache stability

Deterministic prompt structure gives providers predictable prefixes to cache.

### Liabilities

#### Abstraction overhead

Each provider-agnostic-to-specific translation adds cost and a risk of semantic drift, and every supported provider's API must be tracked.

#### Lowest-common-denominator risk

Provider-unique features (structured output, tool-use modes, caching hints) either need adapter-specific extensions or go unused.

#### Parsing fragility

Open-weight models embed tool calls in free text and API formats change, so the parser must handle malformed output gracefully.


## Implementation

The config is a YAML document loaded at startup and passed to the adapter on every call:

```yaml
provider: ollama
model: qwen2.5-coder:32b
temperature: 0.0
max_tokens: 16384
```

Switching among models served by Ollama is one configuration edit. Switching to an unimplemented provider is not: it requires a new adapter behind the existing interface, plus provider-specific options, while leaving machine, tools, and harness untouched. The adapter hides endpoint, authentication, serialization, retry, and rate-limit behavior.

The Cohere configuration keeps the same boundary:

```yaml
provider: cohere
model: command-r7b-12-2024
provider_url: "${COHERE_API_URL:-https://api.cohere.com}"
response_profile: cohere
```

The adapter resolves `COHERE_API_KEY` only when Chat executes and sends it only as bearer authentication. It converts the provider's ordered content blocks into the raw text that the existing capture and parse words consume. The response profile parses that text; it does not parse the Cohere HTTP response object.

The parser handles three output shapes, all yielding one `ParsedResult` type: **structured** tool calls (OpenAI, Anthropic) map directly; **embedded** tool calls (markdown or XML in open-weight output) are extracted by regex or schema, with malformed output raising `ParseFailed` rather than failing silently; and **completion-only** responses become the task output. Conversation history accrues between calls and is truncated to the context window by the assembler (sliding window, summarization, or priority pruning), so the adapter always receives a ready-to-send prompt. Inference telemetry attaches here too: `SpanOverride` labels the `invoke_llm` span `gen_ai.chat` and records GenAI attributes (model, token counts, temperature), one span per dispatch.


## Relationships in the Pattern Language

Inference Boundary sits within Agent-as-Data and requires Machine Interpreter, Agent-as-Data, and Tool Contract: model calls are just declared tools bound by profile data. It overlaps Boundary Tool because both describe controlled crossings, but Inference Boundary is the model-specific crossing while Boundary Tool is the general hierarchical composition primitive. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Executor profile variants.** `profile.yaml`, `profile-qwen35b.yaml`, and `profile-qwen27b.yaml` are loadable profile entry points. They share `machine.yaml`, `tools.yaml`, and the same tool roots. The default and qwen35b wrappers bind `llm/default.yaml` (`qwen3.6:35b-mlx`); qwen27b binds `llm/qwen27b.yaml` (`qwen3.6:27b-mlx`). The grid therefore has three shipped wrappers and two model configurations over one harness, all through Ollama.

**Evaluation harness isolation.** In the bench/critic/executor stack, changing the executor's Ollama model declaration leaves machine, tools, oracle checks, and metrics unchanged. Cohere support follows the same adapter boundary under srd048. Multi-provider failover remains design intent.

**Adapter and Ports-and-Adapters.** The structure is the GoF **Adapter** [@gamma-gof-1994] — convert one interface into the one a client expects — instantiated once per provider behind a single interface. At the architectural scale it is **Hexagonal Architecture** [@cockburn-hexagonal-2005]: the engine core depends on a port and each model provider plugs in as an adapter, exactly how the harness reaches any provider without change. The **Model Context Protocol** [@anthropic-mcp-2024] generalizes the same idea to a uniform boundary between an agent and heterogeneous external capabilities, models included.
