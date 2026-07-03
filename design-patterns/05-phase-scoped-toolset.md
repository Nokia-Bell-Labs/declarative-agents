# Phase-Scoped Toolset

This chapter presents the Phase-Scoped Toolset pattern, which declares which tools the model may call in each machine state. By restricting the tool manifest per phase, the pattern shrinks the model's decision space, prevents cross-phase misuse, and makes it statically verifiable that a tool unreachable in a state cannot be called there.

## Intent

Declare which tools the model may call in each state so the manifest narrows per phase and cross-phase misuse is statically verifiable.


## Motivation

An agent accumulates tools, including file manipulation, shell, web search, test running, build, lint, and reporting. Sent to the model in every invocation, the full manifest grows large, and two problems follow. **Wasted decision bandwidth:** every tool is a choice the model must evaluate and reject, and misuse rates rise with manifest size, since models call tools that are plausible in isolation but wrong for the current phase, forcing recovery cycles. **Phase-inappropriate use:** nothing structurally stops the model from invoking a destructive tool (a deletion, a deployment) in a phase where it is premature; prompt instructions discourage this but can be ignored.

Filtering the manifest by hand before each call couples the logic to the engine. The declarative alternative: the machine declares which tools are visible in each state, and the prompt assembler reads that declaration without knowing which tools exist.


## Applicability

The Phase-Scoped Toolset fits agents with more tools than are relevant in any single phase. It becomes worthwhile when different phases need different subsets — composition tools during generation, validation tools during checking, none during deterministic dispatch — and when global visibility causes recovery loops, wasted tokens, or safety violations. When the tool set is small and every tool is relevant in every phase, the per-state configuration surface is overhead without benefit.


## Structure

The manifest is filtered before each LLM invocation by five participants, whose class relationships appear in Fig. 15.

![](figures/fig-16-scoped-toolset-class.png)

| **Figure 15.** Class diagram. The PromptAssembler reads the Machine's per-state tool list for the current State, resolves it against the full Registry, and emits a filtered ToolManifest. |
|:---:|

### Participants

#### Machine

Declares, per non-terminal state, the list of tool names visible to the model, a list of strings, not logic.

#### Registry

Holds every registered tool (name, parameters, signals, description), the universe from which subsets are drawn.

#### State

Indexes into the machine's per-state list.

#### PromptAssembler

Resolves that list against the registry and builds the manifest, knowing nothing about which tools exist or why they are grouped.

#### ToolManifest

Is the output sent to the model, bounded by the per-state list, not the full registry.


## Collaborations

Before every LLM call the assembler runs a lookup-filter-build cycle, traced by the sequence diagram in Fig. 16: **query** the machine for the current state's visible tools, **resolve** each name against the registry (unresolved names are load-time errors), **build** a manifest of only those tools, and **send** it with the prompt. Tools absent from the manifest are invisible. The model cannot call what it does not know exists.

![](figures/fig-17-scoped-toolset-sequence.png)

| **Figure 16.** Sequence diagram. The lookup-filter-build cycle before each LLM call: the assembler queries the machine, resolves against the registry, builds the manifest, and sends it to the model. {wide} |
|:---:|

When the model returns a tool call, two guards apply before dispatch: **manifest validation** (was the tool in the manifest the model was shown? rejecting hallucinated names) and the Machine Interpreter's existing **static check** (is it a registered tool whose signals are handled in this state?). Machine-dispatched tools (fixed actions like `parse_response`) never appear in the manifest; a `visibility` field marks each tool `external` (manifest-eligible) or `internal`.


## Consequences

### Benefits

#### Smaller prompts

Showing only the tools relevant to a state, rather than the whole registry, keeps the rest out of every prompt, a saving that compounds over a run.

#### Fewer misuse errors

A tool the model cannot see, it cannot call. A tool absent from the manifest is prevented structurally, not by instruction-following; a hallucinated tool name is caught by manifest validation before dispatch.

#### Declarative control

Visibility is a YAML edit, never a code change; prompt-assembly logic never changes.

#### Separation of concerns

Machine authors decide which tools belong in which phase, tool authors decide what each does, and the assembler connects them blind to both.

### Liabilities

#### Configuration surface

Every state carries a tool list; a tool added to the registry but no state's list is invisible (caught by orphan-detection, but only as a warning).

#### Over-restriction

Too narrow a list blocks solutions needing an unexpected tool. Excluding `web_search` from Composing stops the model searching docs even when the task demands it.

#### Cache fragmentation

State-specific manifests share fewer prompt-cache prefixes than one stable manifest, which can matter for latency-sensitive deployments.


## Implementation

Tool visibility is declared alongside the transition table; each state carries an optional `tools` field, and each tool declaration a `visibility`:

```yaml
states:
  Composing:  { tools: [write, read, shell, web_search, patch] }
  Parsing:    { tools: [] }
  Validating: { tools: [build, test, lint] }
  Succeeded:  { terminal: true }
# tool declarations
- { name: write,          visibility: external, emits: [ToolDone, ToolFailed] }
- { name: parse_response, visibility: internal, emits: [ToolCall, Completion] }
```

An empty `tools: []` means "no tools in this state"; an absent field means "all external tools" (the backward-compatible default). The two filters stack: a tool must be `external` *and* listed in the current state to reach the model. Three checks run at load time: every listed name **resolves** to a registered tool; every listed tool is **external** (listing an internal tool is an error); and every external tool appears in **some** state's list (orphan detection, a warning).

Different profiles compose different per-state lists over the same registry. A generator shows `write/read/shell` in Composing, an evaluator shows `dump_config/run_agent`, so the same engine and registry serve different agents with only YAML changing.


## Relationships in the Pattern Language

Phase-Scoped Toolset sits within Agent-as-Data and requires Machine Interpreter, Agent-as-Data, and Tool Contract: a scoped manifest needs declared states, profile-level tool inventory, and tool visibility metadata. It enables Approval Gate because a gate can be made structurally unavoidable by hiding commitment tools until the approved state. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Generator agent.** Composing shows editing tools (`write`, `read`, `shell`, `patch`), Validating shows `build`, `test`, `lint`. Before scoping, the model occasionally called `build` mid-edit, triggering failures and recovery cycles; scoping made `build` invisible during Composing, so the model could no longer call it before the code was ready, and shrank the per-call tool manifest.

**Evaluator agent.** A configure→run→check→report pipeline shows different tools per stage (`dump_config` and `set_variant`, then `run_agent`, then `run_oracle_check` and `classify_convergence`). Scoping stopped the model re-running the subject during the checking stage by making `run_agent` invisible past Running. Lifecycle agents benefit most: a `deploy` tool scoped to a post-validation state makes premature deployment structurally impossible (Chapter 10).
