<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Inference Boundary

The Inference Boundary consolidates all inference processes behind a single
tool and provider adapter. Replacing a supported model requires only a
configuration change, while integrating a new provider requires writing a new
adapter, leaving the machine and other tools unchanged.

## Intent

Position model inference behind a single tool and adapter interface, confining
model changes to the configuration and isolating provider modifications within
the adapter. This prevents changes from affecting the machine or other tools,
maintaining separation of concerns.


## Motivation

Agent frameworks invoke the model across multiple sites, using distinct prompt
formats for planning, generation, and evaluation. Each site parses responses
and handles errors independently. **Boundary drift:** refactors silently shift
inference logic into tools. **Model coupling:** embedding prompt formats and
API calls in tools means changing providers (Ollama → OpenAI → Anthropic)
requires modifying every tool, and evaluating across models demands parallel
implementations. **Accounting fragmentation:** token and latency data
accumulate across sites, so per-task usage requires instrumenting every path.

A single tool and unified adapter interface for all model interactions address
the three concerns: exclusive model access via one tool, seamless translation
between the harness's prompt protocol and the provider's API, and confinement
of all other components to the harness side of the boundary.


## Applicability

The Inference Boundary applies to any agent needing support for model families
or provider adapters without changing its underlying machine or tools. It is
especially useful when evaluating diverse models with the same harness,
isolating model contributions from harness effects, or aggregating token usage
and latency at a single point. But if the model is actually fixed with no
planned changes or comparisons, this indirection layer provides no added
value.


## Structure

Five participants sit behind the single inference tool (Fig. 17), which shows
their component diagram.

![](figures/fig-18-model-adapter-class.png)

| **Figure 17.** Component diagram. InvokeLLM is the sole inference tool; it draws on the PromptAssembler and LLMConfig, requires the ProviderAdapter interface that a provider-specific adapter provides, and routes the reply through the ResponseParser. |
|:---:|

### Participants

#### InvokeLLM

Crossing the inference boundary, this tool provides the machine with a single
dispatch returning the `LLMResponded` signal. It orchestrates assembly,
adaptation, and parsing internally.

#### PromptAssembler

Construct a provider-agnostic prompt using the current state, which includes
system instructions, conversation history, and the state-filtered manifest
(Chapter 5). This process avoids creating provider-specific payloads.

#### ProviderAdapter

This activity translates a prompt into a provider's API call and its reply,
with one implementation per provider behind a single interface.

#### ResponseParser

Normalizes replies (tool calls, completion, free text) into a uniform result.

#### LLMConfig

The YAML knob (provider, model, temperature, limits) requires an edit to alter
the model, not a code modification.


## Collaborations

Every interaction follows the same sequence, regardless of provider (Fig. 18):
the engine **dispatches** `invoke_llm`. The assembler **builds** a
provider-agnostic prompt. The **adapter** serializes it to the provider's
format and sends the HTTP request; the **parser** normalizes the raw reply;
and the tool returns **`LLMResponded`** with the result. This process is
encapsulated in `Execute`. Externally, one dispatch and one signal are
visible, with no indication of the provider used.

![](figures/fig-19-model-adapter-sequence.png)

| **Figure 18.** Sequence diagram. A single `invoke_llm` dispatch flows through prompt assembly, the provider adapter's HTTP exchange, and response parsing before returning the `LLMResponded` signal, identical for every provider. {wide} |
|:---:|

Routed through a single tool, parsed results enable retrieval of input/output
token usage and duration at a centralized point. The shipped Ollama path lacks
monetary-cost accounting.


## Consequences

### Benefits

#### Model-agnostic evaluation

Running the same machine and tools across different models requires a
configuration change, enabling direct comparison of convergence rates, tokens,
and duration. This isolation ensures harness--model separability.

#### Provider portability

Changing models within a provider is configuration. Supporting a new provider
requires adapter code, leaving the machine and tool boundary unchanged. Ollama
and Cohere v2 are the shipped provider adapters.

#### Single instrumentation point

Spans and token accounting are tool-specific; `invoke_llm` maps to GenAI's
`chat` operation, creating a `chat <model>` span per invocation (Chapter 8).

#### Cache stability

Deterministic prompt structure gives providers predictable prefixes to cache.

### Liabilities

#### Abstraction overhead

Translating from a provider-agnostic to a provider-specific interface adds
costs and risks semantic drift, requiring ongoing API tracking for each
supported provider.

#### Lowest-common-denominator risk

Provider-unique features like structured output, tool-use modes, and caching
hints require adapter-specific extensions to function or remain unused.
Without these adaptations, they cannot be leveraged, as standard interfaces do
not inherently support them. Lacking such extensions, their potential benefits
are lost, rendering them impractical in the system.

#### Parsing fragility

Open-weight models integrate tool calls within free text, and as API formats
evolve, the parser must handle malformed output effectively.


## Implementation

The YAML configuration loads at startup and passes to the adapter on every call.

```yaml
provider: ollama
model: qwen2.5-coder:32b
temperature: 0.0
max_tokens: 16384
```

Switching among models within Ollama requires one configuration edit.
Switching to an unimplemented provider requires more: a new adapter behind the
existing interface, with provider-specific options, leaving machine, tools,
and harness unchanged. The adapter handles endpoint, authentication,
serialization, retry, and rate-limit behavior.

The Cohere configuration keeps the same boundary:

```yaml
provider: cohere
model: command-r7b-12-2024
provider_url: "${COHERE_API_URL:-https://api.cohere.com}"
response_profile: cohere
```

The adapter resolves `COHERE_API_KEY` only when Chat executes, transmitting it
as bearer authentication. It converts ordered content blocks into raw text for
existing capture and parse mechanisms. The response profile parses this text,
bypassing direct handling of the Cohere HTTP response object.

The parser handles three output formats, each yielding a `ParsedResult`:
**structured** tool calls from OpenAI or Anthropic map directly. **embedded**
tool calls in markdown or XML within open-weight output are extracted via
regex or schema, with errors returning `ParseFailed`; **completion-only**
responses are treated as task output. The assembler maintains conversation
history, trimming it to fit the context window using sliding window,
summarization, or priority pruning, ensuring the adapter receives a
ready-to-send prompt. Inference telemetry is attached: `SpanOverride` tags the
`invoke_llm` span as `gen_ai.chat`, logging GenAI details (model, token
counts, temperature), with one span per dispatch.


## Relationships in the Pattern Language

Inference Boundary, a component in the Agent-as-Data framework, relies on
Machine Interpreter, Agent-as-Data, and Tool Contract. Model calls are
declared as tools, constrained by profile data. Like Boundary Tool, it
involves controlled crossings, but Inference Boundary focuses on model
interactions, while Boundary Tool is a broader hierarchical composition
primitive. The system's full grammar is in `pattern-language.yaml`.


## Known Uses

**Executor profile variants.** The `profile.yaml`, `profile-qwen35b.yaml`, and
`profile-qwen27b.yaml` files are loadable entry points for different executor
profiles. These profiles share a common `machine.yaml` and `tools.yaml`, with
the same tool roots. The default and qwen35b wrappers use `llm/default.yaml`
(`qwen3.6:35b-mlx`), while the qwen27b wrapper uses `llm/qwen27b.yaml`
(`qwen3.6:27b-mlx`). This creates a grid of three shipped wrappers and two
model configurations, integrated within a single harness through Ollama.

**Evaluation harness isolation.** Altering the executor's Ollama model
declaration in the bench/critic/executor stack does not impact the machine,
tools, oracle checks, or metrics. Cohere support follows the same adapter
boundary under srd048. Multi-provider failover aligns with the design intent.

**Adapter and Ports-and-Adapters.** This structure aligns with the **Adapter**
pattern [@gamma-gof-1994], transforming one interface into another that a
client expects, implemented once per provider behind a unified interface. At
the architectural level, it corresponds to **Hexagonal Architecture**
[@cockburn-hexagonal-2005]: the core engine relies on a port, and each model
provider integrates as an adapter, enabling the harness to interact with any
provider without modification. The **Model Context Protocol**
[@anthropic-mcp-2024] extends this concept, establishing a consistent boundary
between an agent and diverse external capabilities, including models.
