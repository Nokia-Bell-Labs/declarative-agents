<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Convergence Taxonomy

The Convergence Taxonomy maps a completed execution to one of four convergence
types—Clean, Recovery, Stuck, or Divergent—by examining its transition
patterns. Because each type reflects a distinct root cause, the classification
rules, the classifier's interface to the execution record, and the associated
remediation all derive from this four-way split.

## Intent

The taxonomy classifies a completed execution into one of four convergence
types based on its transition patterns, ensuring that every outcome points to
an actionable root cause without re-running the agent.

## Motivation

Pass/fail evaluation reports only *what* happened, not *why*. When success
rates sit in the middle range, three questions arise: is the model
insufficient, are the tasks overly challenging, or is the harness
misconfigured? Pass/fail metrics cannot separate these scenarios. Teams often
debug the wrong layer: they retune prompts for cycling models, upgrade models
when most tasks succeed on the first attempt, or increase budget for agents
that wander into unrelated calls.

Each mode demands a distinct remediation. **Stuck** mode requires a prompt or
model change. Budget exhaustion after productive correction calls for more
budget. **Divergent** mode, characterized by aimless exploration, needs
tighter constraints. **Clean** mode, where the first execution succeeds,
requires no intervention. By assigning a name, detection rule, and remediation
to each mode, the convergence classification becomes a mechanical, LLM-free,
deterministic process: identical executions always yield identical types.

## Applicability

The Convergence Taxonomy applies to evaluation runs that produce structured
traces containing discernible state transitions. Its value grows when
evaluations cover many task-model pairs, when per-run diagnosis must be
automated, when model comparison relies on the distribution of Clean versus
Recovery runs, or when regression detection depends on shifts in taxonomy
distribution (e.g., a move from Clean to Recovery signals degradation despite
a constant pass rate). The taxonomy is less useful when traces lack
state-transition data or when the four outcome types are too coarse for the
observed failure modes—auth errors, rate limits, and outages cut across all
four types and therefore require separate detection.

## Structure

Four distinct participants characterize an execution, as shown in Fig. 29.

![](figures/fig-30-classifier-class.png)

| **Figure 29.** Class diagram. The Classifier reads a completed Execution and assigns a ConvergenceType; the EvalHarness collects assignments across the grid into a Report. |
|:---:|

### Participants

#### Execution

The trace consists of $(state, signal, tool, result)$ tuples from a single run
(Chapter 2). The classifier uses only this trace; it does not require the
model, tools, or workspace.

#### Classifier

A pure, inference-free function scans the transition list, counts cycles,
detects repetition, and inspects the terminal state to produce a
ConvergenceType.

#### ConvergenceType

The four-valued taxonomy:

| Type     | Terminal | Pattern                                   | Root cause                                   |
|----------|----------|-------------------------------------------|----------------------------------------------|
| Clean    | Succeeded| no retry cycles                           | handled on first approach                    |
| Recovery | Succeeded| one or more Composing->Validating->Composing cycles | self-corrected after validation failures |
| Stuck    | Failed (budget) | repeated identical dispatches late | cycling; prompt/model change                 |
| Divergent| Failed (budget) | varied but unproductive                 | not converging; tighter constraints          |

#### EvalHarness

The harness gathers types across a grid of (model × task × profile),
aggregates per-type rates and deltas, and surfaces regressions at the taxonomy
level, not the binary level.

## Collaborations

Classification reads the execution's state-visit sequence and applies three
detectors: a **cycle** counter (each Validating->Composing return counts as one
retry), a **repetition** detector (identifies recurring $(state, tool)$ pairs
beyond a threshold in the later entries), and a **terminal-state** check. The
decision tree in Fig. 30 then maps the run to one of four outcomes: **Clean**
(Succeeded with no cycles), **Recovery** (Succeeded with cycles), **Stuck**
(Failed with repetition), or **Divergent** (Failed without repetition).

![](figures/fig-31-classifier-decision-tree.png)

| **Figure 30.** Activity diagram. The decision-tree classifier maps each run to exactly one convergence type from its terminal state, cycle count, and repetition flag. |
|:---:|

The classification is exhaustive: every completed or exhausted execution
receives exactly one type. Runs that fail outside the taxonomy (infrastructure
crash, timeout before dispatch) are recorded as infrastructure errors before
classification. Across a grid, the per-(model, profile)
Clean/Recovery/Stuck/Divergent rates sum to 1.0, with pass rate defined as
Clean + Recovery. Consequently, two models with a 70 % pass rate can differ
markedly (55 % Clean / 15 % Recovery vs. 40 % Clean / 30 % Recovery). A prompt
change that preserves the pass rate but shifts 10 % from Clean to Recovery
reveals degradation that the headline number would miss.

## Consequences

### Benefits

*Actionable diagnostics* -- each type points to a specific remedy (Clean: no action; Recovery: budget adjustment; Stuck: prompt or model alteration; Divergent: tighter constraints), allowing teams to stop speculative debugging.

*Quantitative comparison* -- equal pass rates can mask quality differences; weighting the types (Clean cheapest, Stuck/Divergent most costly) yields a more nuanced cost model.

*Regression sensitivity* -- distribution shifts that cancel out in the pass rate become visible.

*Deterministic* -- no inference; identical execution always yields identical type, enabling caching and diffing.

### Liabilities

*Granularity* -- four types may be too coarse; a rate-limited agent appears Divergent but actually needs infrastructure scaling rather than model changes. Introducing subtypes would increase classifier complexity.

*Threshold sensitivity* -- the repetition detector's window and count must be calibrated carefully. Overly sensitive settings generate false Stuck flags, while lenient settings miss true cases, so the thresholds are tuned empirically.

*Execution dependency* -- agents that log only final outputs or omit distinct machine phases cannot be classified.

*Confidence level* -- the taxonomy and its detection rules stem mainly from the reference implementation; broader validation across independent agent systems remains limited. Consequently, class names and thresholds may evolve more rapidly than earlier structural patterns.

## Implementation

Classification extracts the state component from each entry, building `visits
= [(e.state, e.signal) for e in execution.entries]` or, for OTel traces, the
equivalent `agent.state` attributes on `execute_tool` spans (Chapter 8). Cycle
counting increments on each Validating-to-Composing transition, treating
consecutive retries as separate counts. Repetition detection uses a sliding
window to flag N identical late dispatches, a histogram to identify $(state,
tool)$ pairs exceeding a fraction of all dispatches, or both. The classifier
then follows four branches:

```
classify(execution):
    terminal = execution.entries[-1].state
    cycles, repeated = count_retry_cycles(visits), detect_repetition(visits)
    if terminal == Succeeded: return Clean if cycles == 0 else Recovery
    if terminal == Failed:    return Stuck if repeated else Divergent
```

The function runs in microseconds and requires no external dependencies. Grid
evaluation (Chapter 9) invokes it after each generator run, storing the type
alongside the pass/fail verdict and metrics. A report groups results by type.
Consider two models that achieve the same pass rate via different routes:

| Model   | Clean | Recovery | Stuck | Divergent | Pass |
|---------|-------|----------|-------|-----------|------|
| Model A | 58 %  | 14 %     | 12 %  | 16 %      | 72 % |
| Model B | 51 %  | 21 %     | 9 %   | 19 %      | 72 % |

Both models report a 72 % pass rate (Clean + Recovery). Model A secures more
first-attempt successes, while Model B recovers more often. Model A's higher
Stuck rate and Model B's higher Divergent rate reveal distinct failure
behaviours that the simple pass metric would conceal.

## Relationships in the Pattern Language

Convergence Taxonomy, a Machine Interpreter component, relies on the Machine
Interpreter and Transition Spans. It requires structured state-transition
evidence and stable trace attributes to classify runs without modifying the
machine. It processes completed executions, providing evaluation, reporting,
and remediation. The full grammar specification resides in
`pattern-language.yaml`.

## Known Uses

**Bench convergence reports.** The reference evaluator employs this pattern variant. It reads per-tool metric snapshots, categorizes each tool's progression into six classes (`CLEAN`, `CONVERGED`, `IMPROVING`, `FLAT`, `REGRESSING`, `NO_DATA`), and derives a single overall class per run, producing a text timeline (e.g., `2ok/3fail` -> `PASS`). Grid reports aggregate these classifications into `CleanRate`, `RecoveryRate` (converged runs over runs with failures), and `StuckRate` (flat-or-regressing runs over the same), exposing quality differences that a simple pass rate would hide.

**Planned diagnostics (design intent rather than yet shipped).** The four-colored badges (green Clean, yellow Recovery, orange Stuck, red Divergent) and a CI regression gate that triggers on distribution shifts (Clean down, Stuck up) illustrate the pattern's goal. The shipped classifier currently provides per-run classes and aggregate rates but lacks per-run Recovery/Stuck badges and thresholded gates. Implementing these features requires aligning the taxonomy across the eval-harness spec (srd019 R4.4), the classifier, and this chapter to ensure consistent class naming.

**Classify-then-remediate precedents.** The **circuit breaker** [@nygard-2018] runs a state classifier (closed/open/half-open) from observed outcomes and drives a distinct action per state, mirroring the "classify state, choose remedy" discipline applied by the taxonomy to a completed run. The **Result / Either monad** [@wadler-monads-1995] carries outcomes as a typed set of cases, requiring exhaustive handling by the caller, just as the four-type taxonomy enforces exhaustiveness.

**Trace-based diagnosis.** Process-mining techniques directly classify executions from logs without re-running them: **process mining** [@van-der-aalst-process-mining-2016] extracts and diagnoses process behavior from event logs, while **conformance checking** [@van-der-aalst-conformance-2012] compares observed traces against expected models to identify deviations, reinforcing the classifier's reliance on transition patterns as diagnostic evidence.