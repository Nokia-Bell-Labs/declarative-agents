# Agent-Core Brainstorm

## Vision

Agent-core is a framework where you can **prove what your agent will do before
it does it**. Declare behavior in YAML, verify it statically, audit it with
OpenTelemetry, enforce budgets, require approvals, and replay deterministically.

If you already have LangChain, agent-core is still valuable because it solves a
different problem. LangChain is optimized for rapid prototyping and breadth of
integrations in Python. Agent-core is optimized for **production deployment of
agentic workflows in environments that demand determinism, auditability,
resource governance, and zero external dependencies**. It is a compiled binary
with explicit control flow -- not a scripting framework that hopes the LLM
behaves correctly. For teams that need to ship an AI agent into regulated,
air-gapped, or cost-sensitive infrastructure (telecom, defense, healthcare,
finance), agent-core provides guarantees that LangChain architecturally cannot.

---

## LangChain Comparison

### What they share

Both agent-core and LangChain/LangGraph address the same fundamental problem:
orchestrating an LLM in a loop with tool calls. Both have concepts of tools,
state, signals/edges, and budget limits.

### Key differences

| Dimension | agent-core | LangChain / LangGraph |
|---|---|---|
| **Language** | Go (single static binary) | Python / JS |
| **Dependencies** | 12 direct deps in `go.mod` | Hundreds of transitive deps |
| **Runtime** | Zero runtime -- deploy one binary + YAML | Requires interpreter, pip, virtualenv |
| **Control flow** | Explicit finite state machine with YAML transition tables | Implicit in Python code; LangGraph uses graph-of-functions |
| **Configuration** | Fully declarative YAML (machines, tools, profiles, prompts) | Code-driven; YAML/JSON for some config |
| **Tool system** | `Command` interface (2 methods: `Name`, `Execute`); tools are CLI binaries or Go functions | `BaseTool` hierarchy, decorators, Pydantic schemas, async variants, callbacks |
| **Observability** | Native OpenTelemetry with GenAI semantic conventions | LangSmith (proprietary SaaS); community OTel adapters |
| **Budget enforcement** | First-class: max iterations, tokens, duration, consecutive parse errors, all in YAML | `max_iterations` on AgentExecutor only |
| **LLM providers** | Ollama (local models) | OpenAI, Anthropic, Google, Cohere, dozens more |
| **RAG / retrieval** | None | First-class support |
| **Ecosystem** | Purpose-built, narrow | Massive community, tutorials, integrations |
| **Agent composition** | Subprocess isolation (OS-level boundaries) | In-process (shared state, cascading failures) |
| **Model profiles** | YAML-driven per-model parsing pipelines with prefix matching | Model-specific adapters in code |
| **Evaluation** | Built-in evaluator state machine with convergence analysis | LangSmith (paid SaaS) |

### Where LangChain wins

- **Ecosystem breadth**: hundreds of integrations (vector stores, retrievers, document loaders, API connectors).
- **RAG support**: first-class retrieval-augmented generation. Agent-core has none.
- **Community**: massive community, tutorials, examples, Stack Overflow answers.
- **Rapid prototyping**: Python + LangChain gets a working agent in 20 lines. Agent-core requires Go compilation and YAML config.
- **Multi-provider LLM support**: OpenAI, Anthropic, Google, Cohere, etc. out of the box. Agent-core only has an Ollama adapter.

### Where agent-core wins

- **Deployment simplicity**: one binary, no runtime, no dependency hell.
- **Auditability**: every transition is visible in YAML; OTel traces capture every state change.
- **Resource governance**: multi-dimensional budget enforcement built into the engine.
- **Performance**: Go is 10-50x faster than Python for CPU-bound work; lower memory; instant startup.
- **Process isolation**: agent composition via subprocesses prevents cascading failures.
- **Offline/air-gapped**: runs entirely locally with Ollama, zero network calls required.
- **Determinism**: explicit state machines produce predictable, testable behavior.

---

## Feature Ideas

### A. Double down on existing strengths

#### 1. Formal verification of state machines

Static analysis that runs at build time or via `agent verify`:
- Detect unreachable states
- Detect deadlocks (non-terminal states missing outbound transitions for possible signals)
- Detect missing error handling (states that don't handle `CommandError`)
- Generate Graphviz/Mermaid diagrams from YAML

Nothing like this exists in LangChain. You can prove properties about your
agent before it runs.

#### 2. Deterministic replay from traces

`agent replay --trace <file>` re-executes the exact sequence of transitions
with recorded LLM responses, verifying the state machine produces the same
path. Enables:
- Regression testing for state machine changes
- Debugging without burning LLM tokens
- Reproducibility for audits

#### 3. Budget policies as first-class YAML

Extend budgets beyond iteration/token/duration:
- Cost-based: `max_dollars: 0.50`
- Per-tool: `max_tool_calls.build: 5`
- Rate limits: `max_llm_calls_per_minute: 10`

Enterprise cost governance and rate limiting are table stakes.

### B. Enterprise / telecom differentiators

#### 4. Externalized state, checkpointing, and agent lifecycle

Three composable capabilities behind a feature flag:

- **Externalized state**: the agent's full internal state (conversation,
  state machine position, budget, domain state) is serializable to a
  pluggable `StateStore` interface (git, DynamoDB, filesystem).
- **Checkpointing with rollback**: every `Command` gets an `Undo` method
  (even if it's a no-op). The loop records a history of every step. You
  can walk backward through the state machine, undoing commands and
  restoring workspace state via git.
- **Suspend and resume**: the agent can serialize its state, exit, and be
  restarted later from the exact checkpoint. This enables approval gates
  (agent pauses before a privileged action, resumes on human approval),
  long-running workflows, and user-driven backtracking.

If you want rollbacks, your runtime environment must provide git (or another
`StateStore` implementation). The evaluator already sets up git-initialized
workspaces -- the same pattern applies.

**See [roll-backs.md](roll-backs.md) for the full design.**

#### 5. Air-gapped / offline-first as a headline feature

Already runs against Ollama with zero network calls. Formalize this:
- Model weight verification (checksum in profile YAML)
- Signed tool declarations (prove which tools were available)
- Manifest lockfile for reproducible builds
- Documentation and marketing as a first-class offline agent framework

#### 6. Multi-model orchestration in a single machine

The profile registry already resolves different parsers per model. Extend so a
single state machine uses different models for different states:

```yaml
transitions:
  - state: Composing
    signal: LLMResponded
    next: Parsing
    action: invoke_llm
    model: devstral       # fast model for parsing
  - state: Idle
    signal: Seed
    next: Composing
    action: invoke_llm
    model: nemotron       # strong model for generation
```

Declarative model routing is auditable; LangChain's equivalent is ad-hoc code.

### C. Evaluation and observability

#### 7. Built-in A/B evaluation framework

The evaluator machine already does `prepare_workspace -> run_agent ->
check_results -> collect_metrics`. Extend into a proper experiment runner:
- N model configs x M test cases = full matrix
- Structured metrics: convergence analysis, token usage, pass rates
- Comparison reports

The convergence classifier (`Classify` in `eval_convergence.go`) already works
per-run -- surface it as a first-class cross-experiment feature.

#### 8. OTel-native cost attribution

Already tracking `Cost{Duration, TokensIn, TokensOut, Dollars}` per command.
Emit as OTel metrics (not just span attributes) for Prometheus/Grafana:
- Per-tool cost breakdowns
- Per-model cost comparisons
- Cumulative cost alerting

Enterprise teams need "how much did the agent spend on this task?" -- LangChain
doesn't track cost at the tool level.

### D. Bigger bets

#### 9. Spec-driven agent generation

Already have `pkg/spec` (SRD corpus) and `pkg/graph` (requirement DAG). Close
the loop: given a spec corpus, automatically generate the machine YAML, tool
selection, and system prompt. `agent generate-machine --spec <corpus-dir>`.

Genuinely novel -- no framework does "requirements in, agent out."

#### 10. Formal tool contracts

Extend `ToolSpec` with pre/post-conditions and invariants:

```yaml
- name: edit
  precondition: "file exists at ${path}"
  postcondition: "file contains ${new_string}"
  invariant: "file size delta < 10KB"
```

The state machine verifies postconditions after each tool execution and can
automatically retry or fail based on contract violations. Design-by-contract
for agent tools.

---

## Priority matrix

| Feature | Effort | Impact | Why it matters |
|---|---|---|---|
| State machine verification | Low | High | Prove correctness before deployment |
| Externalized state / StateStore | Medium | High | Foundation for checkpointing, suspend/resume, rollback |
| Command Undo + history recording | Medium | High | Walk backward through state machine, safe recovery |
| Suspend / resume / approval gates | Medium | High | Human-in-the-loop, regulated environments |
| Deterministic replay | Medium | High | Audit trail, regression testing |
| Air-gapped headline | Low | High | Telecom infra can't call cloud APIs |
| Multi-model per state | Medium | Medium | Cost optimization, model routing |
| A/B evaluation framework | Medium | High | Systematic model selection |
| Formal tool contracts | Medium | Medium | Reliability, auto-retry |
| Spec-driven generation | High | High | Novel capability, automation of automation |
| Budget policies | Low | Medium | Enterprise cost governance |
| OTel cost attribution | Low | Medium | Dashboards, alerting |
