# Phase-Scoped Toolset

This chapter presents the Phase-Scoped Toolset pattern, which derives model-visible tools from the machine's `$tool` transitions, each tool's emitted signals, visibility, and optional tool-level phase restrictions. The derived manifest shrinks the model's decision space and prevents calls the current grammar cannot route.

## Intent

Derive which tools the model may call in each phase from routable machine transitions, then allow tool declarations to narrow that availability.


## Motivation

An agent accumulates tools, including file manipulation, shell, web search, test running, build, lint, and reporting. Sent to the model in every invocation, the full manifest grows large, and two problems follow. **Wasted decision bandwidth:** every tool is a choice the model must evaluate and reject, and misuse rates rise with manifest size, since models call tools that are plausible in isolation but wrong for the current phase, forcing recovery cycles. **Phase-inappropriate use:** nothing structurally stops the model from invoking a destructive tool (a deletion, a deployment) in a phase where it is premature; prompt instructions discourage this but can be ignored.

Filtering the manifest by hand before each call couples policy to the engine. The declarative alternative uses the transition graph as the authority: a `$tool` transition identifies a model-facing phase, and declared tool outcomes determine whether that phase can route each tool.


## Applicability

The Phase-Scoped Toolset fits agents with more tools than are relevant in any single phase. It becomes worthwhile when different phases need different subsets — composition tools during generation, validation tools during checking, none during deterministic dispatch — and when global visibility causes recovery loops, wasted tokens, or safety violations. When every external tool is relevant throughout the model-facing grammar, derived scoping and optional phase metadata add no benefit.


## Structure

The manifest is filtered before each LLM invocation by five participants, whose class relationships appear in Fig. 15.

![](figures/fig-16-scoped-toolset-class.png)

| **Figure 15.** Class diagram. Machine transitions and ToolDef outcomes derive phase availability; the Registry emits the filtered ToolManifest for the current state. |
|:---:|

### Participants

#### Machine

Owns workflow order. A transition with `action: $tool` identifies the target phase where a parsed model-selected tool may execute.

#### Registry

Holds every registered tool and its derived phases. `Manifest(state)` and dynamic dispatch use the same availability rule.

#### State

Names the current grammar phase used to filter the registry.

#### PromptAssembler

Requests the registry manifest for the current phase and serializes it for the model.

#### ToolManifest

Is the output sent to the model, containing only external tools available in the current grammar phase.


## Collaborations

Before every LLM call, the catalog derives availability from the machine and ToolDefs. For each `$tool` transition it takes the transition's target state, considers external tools whose optional `phases` allow that state, and keeps only tools whose every declared emitted signal has a transition from that state (or whose target is terminal). The registry then builds the current-state manifest. Tools absent from the manifest are invisible.

![](figures/fig-17-scoped-toolset-sequence.png)

| **Figure 16.** Sequence diagram. Availability is derived from machine routes and tool outcomes, then the registry builds the current-state manifest. {wide} |
|:---:|

When the model returns a tool call, parsing and dispatch call the same registry availability rule used for the manifest. Unknown tools, internal tools, and registered tools outside the current phase are rejected before execution. Machine-dispatched fixed actions such as `parse_response` remain internal and never enter the model manifest.


## Consequences

### Benefits

#### Smaller prompts

Showing only the tools relevant to a state, rather than the whole registry, keeps the rest out of every prompt, a saving that compounds over a run.

#### Fewer misuse errors

A tool the model cannot see, it cannot call. A tool absent from the manifest is prevented structurally, not by instruction-following; a hallucinated tool name is caught by manifest validation before dispatch.

#### Declarative control

Visibility and optional narrowing are ToolDef YAML edits; workflow availability remains derived from machine transitions.

#### Separation of concerns

Machine authors define routable phases and outcomes; tool authors define visibility, emitted signals, and optional narrower phases; the registry computes their intersection.

### Liabilities

#### Configuration surface

Availability depends on transition and emitted-signal completeness. A missing follow-up transition can remove a tool from the manifest, so diagnostics must name the unroutable signal.

#### Over-restriction

Too narrow a list blocks solutions needing an unexpected tool. Excluding `web_search` from Composing stops the model searching docs even when the task demands it.

#### Cache fragmentation

Changing the available tool set by state reduces shared prompt-cache prefixes, which can matter for latency-sensitive deployments.


## Implementation

Machine states carry no tool lists. This complete machine example gives
`Composing` a dynamic `$tool` route whose target can handle both outcomes emitted
by `write`:

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

Tool declarations supply vocabulary metadata and may narrow derived availability
with `phases`. In this loadable declaration, `write` derives `Composing`;
`web_search` is unavailable because its explicit `Reviewing` phase does not
admit `Composing`; `parse_response` is internal:

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
    phases: [Reviewing]
    emits: [ToolDone, ToolFailed]
  - name: parse_response
    type: builtin
    init: parse_response
    visibility: internal
    emits: [ToolDone, TaskCompleted, ParseFailed]
```

`ApplyDynamicToolPhases` derives phase metadata from the machine grammar and
intersects it with explicit ToolDef phases. `Registry.Manifest`, parse-time
validation, and dynamic dispatch all call the same
`ResolveExternalTool`/`AvailableIn` rule.


## Relationships in the Pattern Language

Phase-Scoped Toolset sits within Agent-as-Data and requires Machine Interpreter, Agent-as-Data, and Tool Contract: a scoped manifest needs declared states, profile-level tool inventory, and tool visibility metadata. It enables Approval Gate because a gate can be made structurally unavoidable by hiding commitment tools until the approved state. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Executor agent.** The shipped executor machine routes `$tool` results back into `Composing`. External tool outcomes that the `Composing` state handles remain model-visible there; internal parsing, validation, and lifecycle actions stay outside the manifest.

**Explicit narrowing.** Tool-level `phases` can reduce a tool's derived set for compatibility or policy, but cannot make it available where the transition graph cannot route its emitted signals. Deployment scoping remains design intent until a shipped profile and test exercise it (Chapter 10).

**Least privilege and capabilities.** The pattern is the **Principle of Least Privilege** [@saltzer-schroeder-1975] applied per machine state: a component holds only the authority its current task requires. It is realized in the manner of **capability-based security** [@dennis-vanhorn-1966], where authority is conferred by holding an unforgeable capability — a state's tool manifest is the set of capabilities held in that phase. **OAuth 2.0 scopes** [@hardt-oauth-2012] apply the same attenuation to access tokens, narrowing authority rather than granting it wholesale, exactly as state-derived scoping narrows which actions are exposed.
