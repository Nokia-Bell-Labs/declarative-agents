# Phase-Scoped Toolset

The phase-scoped toolset derives model-visible tools by analyzing `$tool`
transitions, emitted signals, visibility, and optional tool-level phase
restrictions. This manifest shrinks the model's decision space and prevents
un-routable calls.

## Intent

Derive the model's phase-specific tools from the machine's routable
transitions, then let tool declarations restrict their availability.

## Motivation

An agent accumulates tools such as file manipulation, shell, web search, test
running, build, lint, and reporting. Because the full manifest arrives with
every invocation, two problems emerge. First, wasted decision bandwidth: the
model must evaluate every tool and reject most of them. Second,
phase-inappropriate use: nothing structurally stops the model from invoking a
destructive tool—for example, a deletion or a deployment— in a phase where it
is premature. Prompt instructions discourage this but can be ignored.

Filtering the manifest manually before each call ties policy to the engine.
The declarative approach uses the transition graph as the authority: a `$tool`
transition marks a model-facing phase, and its declared outcomes decide phase
routing.

## Applicability

The Phase-Scoped Toolset supplies agents with more tools than any single phase
needs. This approach proves useful when phases require different tool
subsets—composition tools for generation, validation tools for checking, and
none for deterministic dispatch. It also helps when global visibility causes
recovery loops, wasted tokens, or safety violations. If every external tool is
relevant across the model-facing grammar, derived scoping and optional phase
metadata provide no benefit.

## Structure

Five participants filter the manifest before each LLM invocation—Fig. 15—,
shaping the LLM's input through their distinct roles and contexts. This
filtering aligns the manifest with task requirements and enhances the LLM's
output effectiveness.

![](figures/fig-16-scoped-toolset-class.png)

| Figure 15. Class diagram. Machine transitions and ToolDef outcomes derive phase availability; the Registry emits the filtered ToolManifest for the current state. |
|---|

### Participants

#### Machine

In the workflow sequence, a `action: $tool` transition directly identifies the
target phase for executing a parsed, model-selected tool.

#### Registry

The system maintains all registered tools and their derived phases.
`Manifest(state)` and dynamic dispatch follow a shared availability rule.

#### State

Names the current grammar phase used to filter the registry.

#### PromptAssembler

The registry provides the current phase's manifest, which is serialized for
model integration.

#### ToolManifest

The output is sent to the model, incorporating only accessible external tools
within the current grammar phase.

## Collaborations

Before each LLM call, the catalog derives availability by examining the
machine and ToolDefs. For every `$tool` transition, it identifies the target
state and filters tools based on their `phases` that permit that state. It
retains tools where every emitted signal has a valid transition from that
state or leads to a terminal target. The registry then constructs the
current-state manifest, making absent tools invisible.

Each `parse_response` word governs the `manifest_state` needed to validate its
model response. Startup traces the `invoke_llm -> parse_response -> $tool` path,
rejecting participating words whose state differs from the `$tool` target.
Unrelated invoke words are excluded. Startup also rejects external words
lacking a phase, citing empty explicit-phase intersection, missing signals, or
unroutable signals.

![](figures/fig-17-scoped-toolset-sequence.png)

| Figure 16. Sequence diagram. Availability is derived from machine routes and tool outcomes; the registry then builds the current-state manifest. |
|---|

The model's tool calls follow the manifest's registry availability rule during
parsing and dispatch. Tool calls that reference unknown, internal, or
out-of-phase registered tools are rejected before execution. Fixed,
machine-dispatched actions like `parse_response` stay internal and are
excluded from the model manifest.

## Consequences

### Benefits

* Smaller prompts -- displaying only relevant tools, not the entire registry, excludes unrelated entries from prompts and boosts cumulative efficiency.  
* Fewer misuse errors -- the model cannot call a tool it cannot see; a tool absent from the manifest is structurally prevented rather than blocked by instruction following. Hallucinated tool names are caught by manifest validation before dispatch.  
* Declarative control -- visibility and optional narrowing are implemented via ToolDef YAML edits; workflow availability remains tied to machine transitions.  
* Separation of concerns -- machine authors define routable phases and outcomes, tool authors define visibility, emitted signals, and narrower phases if needed, and the registry computes their intersection.

### Liabilities

* Configuration surface -- availability depends on complete transitions and emitted signals. A missing follow-up transition, an explicit phase that excludes all `$tool` targets, or a mismatched selector state indicates a startup error rather than an empty manifest.  
* Over-restriction -- a narrow toolset limits solutions that need unexpected tools. Excluding `web_search` from composing blocks, for example, prevents document searches even when required.  
* Cache fragmentation -- changing the available tool set by state reduces shared prompt-cache prefixes, which matters for latency-sensitive deployments.

## Implementation

Machine states lack tool lists. This complete machine example provides
`composing` with a dynamic `$tool` route, enabling its target to handle both
outcomes from `write`.

```yaml
# phase-scoped-machine-example
name: phase-scoped-example
initial_state: Idle
states: [Idle, Composing, Parsing, Done, Failed]
terminal_states: [Done, Failed]
signals: [Seed, LLMResponded, ToolDone, ToolFailed, TaskCompleted, ParseFailed, CommandError]
transitions:
  - {state: Idle, signal: Seed, next: Composing, action: invoke_llm}
  - {state: Composing, signal: LLMResponded, next: Parsing, action: parse_response}
  - {state: Parsing, signal: ToolDone, next: Composing, action: $tool}
  - {state: Parsing, signal: TaskCompleted, next: Done}
  - {state: Parsing, signal: ParseFailed, next: Composing, action: invoke_llm}
  - {state: Composing, signal: ToolDone, next: Composing, action: invoke_llm}
  - {state: Composing, signal: ToolFailed, next: Failed}
  - {state: Composing, signal: CommandError, next: Failed}
  - {state: Parsing, signal: CommandError, next: Failed}
```

Tool declarations provide vocabulary metadata and narrow derived availability
with `phases`. In this loadable declaration, `write` derives `Composing`;
`web_search` admits that phase; `parse_response` is internal.

```yaml
# phase-scoped-tools-example
tools:
  - name: write
    type: builtin
    init: file_write
    visibility: external
    emits: [ToolDone, ToolFailed]
  - name: web_search
    type: builtin
    init: web_search
    visibility: external
    phases: [Composing]
    emits: [ToolDone, ToolFailed]
  - name: parse_response
    type: builtin
    init: parse_response
    visibility: internal
    emits: [ToolDone, TaskCompleted, ParseFailed]
    config:
      manifest_state: Composing
```

`ApplyDynamicToolPhases` derives phase metadata from the machine grammar and
intersects it with explicit ToolDef phases. `Registry.Manifest`, parse-time
validation, and dynamic dispatch share the `ResolveExternalTool/AvailableIn`
rule. `ValidateToolPhases` runs pre-registration, rejecting empty
intersections or selector/parser path mismatches. The parser reads its ToolDef
state, unaffected by registration order.

## Relationships in the Pattern Language

Phase-Scoped Toolset operates within the Agent-as-Data framework, using
Machine Interpreter, Agent-as-Data, and Tool Contract. It requires a scoped
manifest with declared states, a profile-level tool inventory, and tool
visibility metadata. This setup ensures gates are structurally unavoidable by
hiding commitment tools until the approved state, thereby facilitating the
Approval Gate. The complete grammar resides in `pattern-language.yaml`.

## Known Uses

Executor agent. The shipped executor machine routes `$tool` results back into
the `Composing` state. Outcomes from external tools that the `Composing` state
handles remain model-visible there; internal parsing, validation, and
lifecycle actions stay outside the manifest.

Explicit narrowing. Tool-level `phases` reduce a tool's derived set for
compatibility or policy when a machine has multiple `$tool` targets. But a
tool cannot become available if the transition graph cannot route its emitted
signals. Excluding all targets fails at startup. Deployment scoping remains
design intent until a shipped profile and test exercise it (Chapter 10).

Least privilege and capabilities. The pattern follows the Principle of Least
Privilege [@saltzer-schroeder-1975], ensuring components hold only the
authority necessary for their current task. This is realized through
capability-based security [@dennis-vanhorn-1966], where authority is conferred
by an unforgeable capability—the state's tool manifest. OAuth 2.0 scopes
[@hardt-oauth-2012] similarly narrow authority in access tokens, akin to
state-derived scoping that limits exposed actions.