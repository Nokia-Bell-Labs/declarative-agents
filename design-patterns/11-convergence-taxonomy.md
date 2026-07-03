# Convergence Taxonomy

This chapter presents the Convergence Taxonomy pattern, which reads a completed execution and assigns one of four convergence types — Clean, Recovery, Stuck, or Divergent — based on transition patterns. Each type points to a distinct root cause. The chapter covers the classification rules, the classifier's interface with the execution record, and how the taxonomy drives remediation.

## Intent

Classify a completed execution into one of four convergence types by reading its transition patterns so every outcome has an actionable root cause without re-running the agent.


## Motivation

Evaluation that reports only pass or fail says *what* happened, never *why*. When a dashboard shows a middling success rate, three questions follow: is the model too weak, are the tasks too hard, or is the harness misconfigured? Pass/fail cannot distinguish them. Teams then debug the wrong layer: retuning prompts when the model was cycling on identical calls, upgrading the model when most tasks succeeded first-try and only the rest needed correction cycles, or raising the budget when the agent was diverging into unrelated calls.

Each mode has a different fix. Cycling (**Stuck**) wants a prompt or model change; budget exhaustion after productive correction wants more budget; aimless exploration (**Divergent**) wants tighter machine or tool constraints; correct first-attempt execution (**Clean**) wants nothing. Convergence classification gives each mode a name, a detection rule, and a remediation. It is mechanical (no LLM in the classifier) and deterministic: the same execution always yields the same type.


## Applicability

The Convergence Taxonomy fits evaluation runs that produce structured traces with identifiable state transitions. The pattern becomes more valuable when evaluation spans many task-model pairs and per-run diagnosis needs automation; when model comparison matters (distinguishing whether a model wins by producing more Clean runs or by recovering from more failures); and when regression detection is needed (a shift from Clean to Recovery signals prompt degradation even at constant pass rate). It is less useful when traces lack state-transition information, or when the four outcome types are too coarse for the failure modes in question — auth errors, rate limits, and outages cut across all four types and need separate detection.


## Structure

An execution is classified by four participants, drawn as a class diagram in Fig. 29.

![](figures/fig-30-classifier-class.png)

| **Figure 29.** Class diagram. The Classifier reads a completed Execution and assigns a ConvergenceType; the EvalHarness collects assignments across the grid into a Report. |
|:---:|

### Participants

#### Execution

The trace, the $(state, signal, tool, result)$ tuples from one run (Chapter 2), and the classifier's sole input; no access to model, tools, or workspace is needed.

#### Classifier

A pure, inference-free function from execution to type: it scans transitions, counts cycles, detects repetition, and checks the terminal state.

#### ConvergenceType

The four-valued taxonomy:

| Type | Terminal | Pattern | Root cause |
|------|----------|---------|------------|
| Clean | Succeeded | no retry cycles | handled on first approach |
| Recovery | Succeeded | one or more Composing→Validating→Composing cycles | self-corrected after validation failures |
| Stuck | Failed (budget) | repeated identical dispatches late | cycling; prompt/model change |
| Divergent | Failed (budget) | varied but unproductive | not converging; tighter constraints |

#### EvalHarness

Collects types across a grid of (model × task × profile), aggregating per-type rates and deltas so regressions surface at the taxonomy level, not the binary level.


## Collaborations

Classification reads the execution's state-visit sequence and applies three detectors: a **cycle** counter (each Validating→Composing return is one retry), a **repetition** detector (the same $(state, tool)$ pair recurring past a threshold in the late entries), and a **terminal-state** check. It then applies the decision tree in Fig. 30: Succeeded with no cycles is **Clean**, Succeeded with cycles is **Recovery**, Failed with repetition is **Stuck**, Failed without is **Divergent**.

![](figures/fig-31-classifier-decision-tree.png)

| **Figure 30.** Activity diagram. The decision-tree classifier maps each run to exactly one convergence type from its terminal state, cycle count, and repetition flag. |
|:---:|

Classification is exhaustive; every completed-or-exhausted execution maps to exactly one type. Runs that fail outside the taxonomy (infrastructure crash, timeout before any dispatch) are recorded as infrastructure errors before the classifier runs. Across a grid, per-(model, profile) Clean/Recovery/Stuck/Divergent rates sum to 1.0 and pass rate is Clean + Recovery, so two models at 70% pass can differ markedly (55/15 vs. 40/30 Clean/Recovery), and a prompt change that holds pass rate but shifts 10% from Clean to Recovery exposes a degradation invisible to the headline number.


## Consequences

### Benefits

#### Actionable diagnostics

Each type points to a different fix (Clean none, Recovery a budget tweak, Stuck a prompt/model change, Divergent tighter constraints), so teams stop guessing which layer to debug.

#### Quantitative comparison

Equal pass rates with different distributions reveal quality differences binary metrics hide, and types can be cost-weighted (Clean cheapest, Stuck/Divergent most expensive).

#### Regression sensitivity

Distributions catch shifts that cancel out in a pass rate.

#### Deterministic

No inference; same execution, same type, cacheable and diffable.

### Liabilities

#### Granularity

Four types can be too coarse. A rate-limited agent looks Divergent but needs infrastructure scaling, not machine changes; subtypes add classifier complexity.

#### Threshold sensitivity

The repetition detector's window and count are sensitive (too aggressive over-calls Stuck, too lenient under-calls it) and need empirical tuning.

#### Execution dependency

Agents that log only final outputs, or whose machines lack distinct phases, cannot be classified.

#### Confidence level

This pattern has the lightest external corroboration in the language. The four-type taxonomy and its detection rules come primarily from the reference implementation, so they should be treated as tentative until validated across more independent agent systems. The mechanism is included because the diagnostic value is high and the classifier is deterministic, but the class names and thresholds are more likely to evolve than the structural patterns earlier in the language.


## Implementation

Classification reads only the state component of each entry (`visits = [(e.state, e.signal) for e in execution.entries]`; for OTel traces, the equivalent `agent.state` attributes on `execute_tool` spans, Chapter 8). Cycle counting scans linearly, incrementing on each Validating→Composing return (consecutive retries count separately). Repetition is detected by a sliding window (N identical late dispatches) or a histogram (one $(state, tool)$ pair exceeding a fraction of all dispatches), or both. The classifier is then a four-branch decision:

```
classify(execution):
    terminal = execution.entries[-1].state
    cycles, repeated = count_retry_cycles(visits), detect_repetition(visits)
    if terminal == Succeeded: return Clean if cycles == 0 else Recovery
    if terminal == Failed:    return Stuck if repeated else Divergent
```

It runs in microseconds with no external dependencies. Grid evaluation (Chapter 9) invokes it after each generator run, storing the type beside the pass/fail verdict and metrics. A report groups by type. To illustrate, consider two models that reach the same pass rate by different routes:

| Model | Clean | Recovery | Stuck | Divergent | Pass |
|-------|-------|----------|-------|-----------|------|
| Model A | 58% | 14% | 12% | 16% | 72% |
| Model B | 51% | 21% | 9% | 19% | 72% |

Both reach the same 72% pass rate (Clean + Recovery), but Model A wins more first attempts while Model B recovers more; Model A's higher Stuck and Model B's higher Divergent point to different failure behaviours under one headline number.


## Relationships in the Pattern Language

Convergence Taxonomy sits within Machine Interpreter and requires Machine Interpreter and Transition Spans: it needs structured state-transition evidence and stable trace attributes to classify a run. It does not change the machine; it reads completed executions and feeds evaluation, reporting, and remediation. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Bench convergence reports.** The reference evaluator ships a variant of this pattern. Rather than the four-type state-visit classifier described above, it reads the per-tool metric snapshots recorded during a run and classifies each tool's progression into one of six classes (`CLEAN`, `CONVERGED`, `IMPROVING`, `FLAT`, `REGRESSING`, `NO_DATA`), then derives a single overall class per run plus a text timeline (for example `2ok/3fail` → `PASS`). Grid reports aggregate these into a `CleanRate` and two derived rates that reuse this chapter's vocabulary: `RecoveryRate` (converged runs over runs that hit a failure) and `StuckRate` (flat-or-regressing runs over the same), surfacing quality differences a bare pass rate hides.

**Planned diagnostics (design intent, not yet shipped).** The four-type colored badges (green Clean, yellow Recovery, orange Stuck, red Divergent) and a baseline-comparison CI regression gate that fires on distribution shifts (Clean down, Stuck up) are the pattern's intended end state, not current behavior: the shipped classifier exposes per-run classes and aggregate derived rates, but no per-run Recovery/Stuck badge and no thresholded gate. Realizing them means first aligning the taxonomy across the eval-harness spec (srd019 R4.4), the classifier, and this chapter so all three name the same classes.

**Classify-then-remediate precedents.** The **circuit breaker** [@nygard-2018] runs a small state classifier (closed/open/half-open) derived from observed outcomes and drives a distinct action per state, the same "classify state, choose remedy" discipline the taxonomy applies to a completed run. The **Result / Either monad** [@wadler-monads-1995] carries outcomes as a small closed set of typed cases the caller must handle exhaustively, mirroring the exhaustiveness the four-type taxonomy enforces.

**Trace-based diagnosis.** Process-mining research directly supports classifying executions from their traces without re-running them: **process mining** [@van-der-aalst-process-mining-2016] extracts and diagnoses process behaviour from event logs, and **conformance checking** [@van-der-aalst-conformance-2012] compares observed traces against expected process models to identify deviations, strengthening the classifier's use of transition patterns as diagnostic evidence.
