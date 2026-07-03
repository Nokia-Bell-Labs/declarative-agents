# Inference Boundary

This chapter presents the Inference Boundary pattern, which places all inference behind one tool and a pluggable provider adapter. Swapping the model is a configuration change in the tool's YAML; the machine, registry, and all other tools are unaffected. The chapter covers the adapter interface, prompt assembly, response parsing, and provider-specific configuration.

## Intent

Place all model inference behind one tool and a pluggable adapter so swapping providers is a configuration change that touches neither the machine nor any other tool.


## Motivation

Agent frameworks typically call the model from many sites, a planning step with one prompt format, a generation step with another, an evaluation step with a third, each parsing responses and handling errors differently. The line where deterministic harness ends and stochastic inference begins is implicit and scattered. **Boundary drift:** refactors silently move inference logic into tools. **Model coupling:** if prompt formats and API calls live in tools, switching provider (Ollama → OpenAI → Anthropic) means editing every tool, and evaluating across models means maintaining parallel implementations. **Accounting fragmentation:** token, latency, and cost data accrue across sites, so a single cost-per-task number requires instrumenting every path.

Funneling every model interaction through one tool and one adapter interface solves all three: one tool touches the model, one adapter translates between the harness's prompt protocol and the provider's API, and everything else stays on the harness side of the boundary.


## Applicability

The Inference Boundary fits any agent that needs to support multiple providers or model families without changing its machine or tools. It is especially useful when evaluation runs the same harness against different models to separate model contribution from harness contribution, or when token, latency, and cost accounting should aggregate at one point. When the model is fixed with no prospect of change or comparison, the indirection adds nothing.


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

Because every interaction passes through one tool, token usage aggregates at a single point: parsed results carry input/output tokens and cost, recorded per execution entry, so per-task accounting is a sum over entries.


## Consequences

### Benefits

#### Model-agnostic evaluation

Running the same machine and tools against different models is a config change, making convergence rates, cost, and latency comparable across models. Harness--model separability depends on this isolation.

#### Provider portability

Migration is a config change; the adapter is the only code that moves.

#### Single instrumentation point

Spans, token accounting, and cost tracking attach to one tool; `invoke_llm` maps to a `gen_ai.chat` span (Chapter 8), one span per invocation.

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

Switching models is one line; switching providers changes `provider` and its options, leaving machine, tools, and harness untouched. Each adapter implements three operations: `Send` (prompt + config → raw response), `ParseResponse` (raw → tool calls, content, usage), and `StreamSend` (the streaming variant). It hides all provider-specific logic (endpoints, auth, serialization, retries, rate limits). Adding a provider means implementing those three; nothing else changes.

The parser handles three output shapes, all yielding one `ParsedResult` type: **structured** tool calls (OpenAI, Anthropic) map directly; **embedded** tool calls (markdown or XML in open-weight output) are extracted by regex or schema, with malformed output raising `ParseFailed` rather than failing silently; and **completion-only** responses become the task output. Conversation history accrues between calls and is truncated to the context window by the assembler (sliding window, summarization, or priority pruning), so the adapter always receives a ready-to-send prompt. Inference telemetry attaches here too: `SpanOverride` labels the `invoke_llm` span `gen_ai.chat` and records GenAI attributes (model, token counts, temperature), one span per dispatch.


## Relationships in the Pattern Language

Inference Boundary sits within Agent-as-Data and requires Machine Interpreter, Agent-as-Data, and Tool Contract: model calls are just declared tools bound by profile data. It overlaps Boundary Tool because both describe controlled crossings, but Inference Boundary is the model-specific crossing while Boundary Tool is the general hierarchical composition primitive. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Generator profile variants.** Generators share one `machine.yaml` and tool set but bind different LLM configs, namely `qwen.yaml`, `deepseek.yaml`, and `claude.yaml`. Running the bench/evaluator stack over the three yields a directly comparable grid (success rate, convergence distribution, cost, latency) because the harness is held constant; without the adapter, each model would need its own prompt formatting and parsing, tripling maintenance and confounding the comparison.

**Evaluation harness isolation.** In the bench/evaluator/subject stack, changing the subject's model changes only its config while oracle checks and metrics stay identical, so Model A's Clean rate compares to Model B's without contamination. The same boundary supports production failover (a composite adapter tries a primary provider and retries a fallback) while machine and tools still see one `invoke_llm` tool.

**Adapter and Ports-and-Adapters.** The structure is the GoF **Adapter** [@gamma-gof-1994] — convert one interface into the one a client expects — instantiated once per provider behind a single interface. At the architectural scale it is **Hexagonal Architecture** [@cockburn-hexagonal-2005]: the engine core depends on a port and each model provider plugs in as an adapter, exactly how the harness reaches any provider without change. The **Model Context Protocol** [@anthropic-mcp-2024] generalizes the same idea to a uniform boundary between an agent and heterogeneous external capabilities, models included.
