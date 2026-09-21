# Inference Boundary

The Inference Boundary gathers every inference operation behind a single tool
and a provider adapter. Replacing a supported model requires only a
configuration edit, while adding a new provider demands a new adapter, leaving
the machine and all other tools untouched.

## Intent

Place model inference behind one tool and one adapter interface, so that model
swaps affect only the configuration and provider changes stay confined to the
adapter. This separation protects the machine and other tools from unintended
side effects.

## Motivation

Agent frameworks call the model from many locations, each using its own prompt
format for planning, generation, or evaluation. Every location parses
responses and handles errors independently. *Boundary drift* occurs when
refactors silently move inference logic into tools. *Model coupling* arises
because prompt formats and API calls are embedded in tools; changing providers
(e.g., Ollama -> OpenAI -> Anthropic) then forces edits in every tool, and
evaluating multiple models forces parallel implementations. *Accounting
fragmentation* spreads token and latency data across sites, requiring
instrumentation everywhere to collect per-task usage.

A single tool together with a unified adapter interface solves these three
problems: exclusive model access through one tool, seamless translation
between the harness's prompt protocol and the provider's API, and confinement
of all other components to the harness side of the boundary.

## Applicability

The Inference Boundary fits any agent that must support multiple model
families or provider adapters without altering its machine or tools. It is
especially valuable when the same harness evaluates diverse models, when model
contributions need to be isolated from harness effects, or when token and
latency metrics must be aggregated at a single point. If a deployment uses a
fixed model with no planned changes or comparisons, the indirection layer adds
little benefit.

## Structure

Five participants sit behind the single inference tool (Fig. 17), which
displays their component diagram.

![](figures/fig-18-model-adapter-class.png)

| Figure 17. Component diagram. *InvokeLLM* is the sole inference tool; it draws on *PromptAssembler* and *LLMConfig*, requires the *ProviderAdapter* interface that a provider-specific adapter implements, and routes the reply through *ResponseParser*. |
|:---:|

### Participants

InvokeLLM -- crossing the inference boundary, this tool returns a single
`LLMResponded` signal to the machine. Internally it orchestrates prompt
assembly, adaptation, and response parsing.

PromptAssembler -- builds a provider-agnostic prompt from the current state,
which includes system instructions, conversation history, and the
state-filtered manifest (Chapter 5). The assembler never creates
provider-specific payloads.

ProviderAdapter -- translates the assembled prompt into a concrete provider
API call and returns the raw reply. One implementation exists for each
provider, all adhering to the same interface.

ResponseParser -- normalizes replies (tool calls, completions, free text) into
a uniform result structure.

LLMConfig -- a YAML knob containing provider, model, temperature, and limits.
Editing this file changes the model; no code changes are required.

## Collaborations

Every interaction follows the same sequence, regardless of provider (Fig. 18).
The engine dispatches `invoke_llm`. The assembler builds a provider-agnostic
prompt. The adapter serializes the prompt, sends the HTTP request, and
receives the response; the parser normalizes the raw reply; finally, the tool
returns `LLMResponded` with the result. This entire flow is encapsulated in
`Execute`. Externally, only one dispatch and one signal are visible, with no
indication of which provider was used.

![](figures/fig-19-model-adapter-sequence.png)

| Figure 18. Sequence diagram. A single `invoke_llm` dispatch travels through prompt assembly, the provider adapter's HTTP exchange, and response parsing before returning the `LLMResponded` signal, identical for every provider. {wide} |
|:---:|

Routing all calls through one tool enables centralized retrieval of
input/output token usage and request duration. The shipped Ollama path
currently lacks monetary-cost accounting.

## Consequences

### Benefits

Model-agnostic evaluation -- Switching models only requires a configuration
change, allowing direct comparison of convergence speed, token consumption,
and latency while keeping the harness unchanged.

Provider portability -- Changing the model within a provider is a
configuration edit; supporting a new provider needs only a new adapter,
leaving the machine and tools untouched. Ollama and Cohere v2 are the shipped
adapters.

Single instrumentation point -- Spans and token accounting are tool-specific;
`invoke_llm` maps to GenAI's `chat` operation, producing a `chat <model>` span
per invocation (Chapter 8).

Cache stability -- Deterministic prompt structure gives providers predictable
prefixes, improving cache hit rates.

### Liabilities

Abstraction overhead -- Translating from a provider-agnostic request to a
provider-specific API adds latency and introduces semantic drift, requiring
ongoing maintenance of each provider's API contract.

Lowest-common-denominator risk -- Unique provider features such as structured
output, tool-use modes, or caching hints may be unavailable without
adapter-specific extensions. When extensions are missing, those capabilities
cannot be leveraged.

Parsing fragility -- Open-weight models embed tool calls within free text; as
API formats evolve, the parser must robustly handle malformed output or
missing markers.

## Implementation

The YAML configuration loads at startup and is passed to the adapter on every call.

```yaml
provider: ollama
model: qwen2.5-coder:32b
temperature: 0.0
max_tokens: 16384
```

Switching among Ollama models requires a single edit. Adding an unimplemented
provider demands a new adapter behind the existing interface; the machine,
tools, and harness remain unchanged. The adapter handles endpoint selection,
authentication, serialization, retry logic, and rate-limit behavior.

The Cohere configuration follows the same boundary:

```yaml
provider: cohere
model: command-r7b-12-2024
provider_url: "${COHERE_API_URL:-https://api.cohere.com}"
response_profile: cohere
```

The adapter resolves `COHERE_API_KEY` only when a chat executes, transmitting
it as a bearer token. It converts ordered content blocks into raw text for the
existing capture and parse mechanisms. The response profile then parses this
text, bypassing direct handling of the Cohere HTTP response object.

The parser supports three output formats, each yielding a `ParsedResult`:

* structured -- tool calls from OpenAI or Anthropic map directly;
* embedded -- tool calls hidden in markdown or XML are extracted via regex or schema, with failures returning `ParseFailed`;
* completion-only -- plain completions become task output.

The assembler maintains conversation history, trimming it to fit the context
window using a sliding window, summarization, or priority pruning. This
ensures the adapter receives a ready-to-send prompt. Inference telemetry is
attached: `SpanOverride` tags the `invoke_llm` span as `gen_ai.chat`, logging
model name, token counts, temperature, and creating one span per dispatch.

## Relationships in the Pattern Language

The Inference Boundary is a component of the Agent-as-Data framework. It
relies on the Machine Interpreter, Agent-as-Data, and Tool Contract. Model
calls are declared as tools and constrained by profile data. While the
Boundary Tool pattern also governs controlled crossings, the Inference
Boundary focuses specifically on model interactions; the broader Boundary Tool
handles hierarchical composition primitives. The complete grammar resides in
`pattern-language.yaml`.

## Known Uses

Executor profile variants. Files such as `profile.yaml`,
`profile-qwen35b.yaml`, and `profile-qwen27b.yaml` serve as entry points for
different executor profiles. All share a common `machine.yaml` and
`tools.yaml`, preserving identical tool roots. The default and qwen35b
wrappers use `llm/default.yaml` (`qwen3.6:35b-mlx`), whereas the qwen27b
wrapper uses `llm/qwen27b.yaml` (`qwen3.6:27b-mlx`). This arrangement yields a
grid of three shipped wrappers and two model configurations, all integrated
through a single harness via Ollama.

Evaluation harness isolation. Changing the executor's Ollama model declaration
in the `bench/critic/executor` stack does not affect the machine, tools,
oracle checks, or metrics. Cohere support follows the same adapter boundary
under srd048. Multi-provider failover aligns with the original design intent.

Adapter and Ports-and-Adapters. The structure mirrors the Adapter pattern
[@gamma-gof-1994], converting one interface into another expected by the
client, implemented once per provider behind a unified interface.
Architecturally, it matches Hexagonal Architecture [@cockburn-hexagonal-2005]:
the core engine depends on a port, and each model provider integrates as an
adapter, allowing the harness to interact with any provider without
modification. The Model Context Protocol [@anthropic-mcp-2024] extends this
idea, establishing a consistent boundary between an agent and diverse external
capabilities, including language models.