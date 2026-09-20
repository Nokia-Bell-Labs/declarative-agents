<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Convergence Taxonomy

The Convergence Taxonomy assigns one of four convergence types — Clean,
Recovery, Stuck, or Divergent — to a completed execution based on its
transition patterns. Each type indicates a distinct root cause, so
classification rules, the classifier's interface with the execution record,
and remediation all stem from this four-way split.

## Intent

Classify a completed execution into one of four convergence types from its
transition patterns, so every outcome has an actionable root cause without
re-running the agent.


## Motivation

Pass/fail evaluation reveals only *what* happened, not *why*. A middling
success rate raises three questions: insufficient model, overly challenging
tasks, or improper harness setup? Pass/fail metrics cannot differentiate these
scenarios. Teams often debug the wrong layer: retuning prompts for cycling
models, upgrading models when most tasks succeed on first attempt, or
increasing budget for agents straying into unrelated calls.

Each mode requires a distinct remediation. **Stuck** mode needs a prompt or
model change. Budget exhaustion after productive correction requires more
budget; **Divergent** mode, marked by aimless exploration, needs tighter
constraints; **Clean** mode, with correct first execution, needs no
intervention. Convergence classification assigns each mode a name, detection
rule, and remediation. This process is mechanical, LLM-free, and
deterministic: identical executions yield identical types.


## Applicability

The Convergence Taxonomy applies to evaluation runs yielding structured traces
with discernible state transitions. Its value increases when evaluations span
multiple task-model pairs, per-run diagnosis requires automation, model
comparison is essential (e.g., distinguishing if a model wins by producing
more Clean runs or recovering from more failures), or regression detection is
needed (a shift from Clean to Recovery signals degradation despite constant
pass rate). It is less useful when traces lack state-transition information or
when the four outcome types are too coarse for the failure modes in
question — auth errors, rate limits, and outages cut across all four types
and need separate detection.


## Structure

Four distinct participants characterize an execution, as shown in Fig. 29.

![](figures/fig-30-classifier-class.png)

| **Figure 29.** Class diagram. The Classifier reads a completed Execution and assigns a ConvergenceType; the EvalHarness collects assignments across the grid into a Report. |
|:---:|

### Participants

#### Execution

The trace, comprising $(state, signal, tool, result)$ tuples from one run
(Chapter 2), is the classifier's sole input. It does not need the model,
tools, or workspace.

#### Classifier

A pure, inference-free function scans transitions, counts cycles, detects
repetition, and checks the terminal state from execution to type.

#### ConvergenceType

The four-valued taxonomy:

| Type | Terminal | Pattern | Root cause |
|------|----------|---------|------------|
| Clean | Succeeded | no retry cycles | handled on first approach |
| Recovery | Succeeded | one or more Composing→Validating→Composing cycles | self-corrected after validation failures |
| Stuck | Failed (budget) | repeated identical dispatches late | cycling; prompt/model change |
| Divergent | Failed (budget) | varied but unproductive | not converging; tighter constraints |

#### EvalHarness

Collects types across a grid of (model × task × profile), aggregating per-type
rates and deltas to surface regressions at the taxonomy level, not the binary
level.


## Collaborations

Classification reads the execution's state-visit sequence, applying three
detectors: a **cycle** counter (each Validating→Composing return counts as one
retry), a **repetition** detector (identifies recurring $(state, tool)$ pairs
past a threshold in late entries), and a **terminal-state** check. It then
applies the decision tree in Fig. 30, classifying outcomes as: **Clean**
(Succeeded with no cycles), **Recovery** (Succeeded with cycles), **Stuck**
(Failed with repetition), or **Divergent** (Failed without repetition).

![](figures/fig-31-classifier-decision-tree.png)

| **Figure 30.** Activity diagram. The decision-tree classifier maps each run to exactly one convergence type from its terminal state, cycle count, and repetition flag. |
|:---:|

Classification is exhaustive; every completed or exhausted execution maps to
exactly one type. Runs failing outside the taxonomy (infrastructure crash,
timeout before dispatch) are recorded as infrastructure errors before
classification. Across a grid, per-(model, profile)
Clean/Recovery/Stuck/Divergent rates sum to 1.0, with pass rate defined as
Clean + Recovery. Thus, two models at 70% pass can differ markedly (55/15 vs.
40/30 Clean/Recovery), and a prompt change maintaining pass rate but shifting
10% from Clean to Recovery reveals degradation unseen in the headline number.


## Consequences

### Benefits

#### Actionable diagnostics

Each type points to a distinct resolution (Clean: no action; Recovery: budget
adjustment; Stuck. Prompt or model alteration; Divergent. Tighter
constraints), enabling teams to cease speculative debugging.

#### Quantitative comparison

Equal pass rates, despite differing distributions, reveal quality differences
binary metrics miss. These types can be cost-weighted: Clean is cheapest,
Stuck/Divergent most costly.

#### Regression sensitivity

Distributions catch shifts that cancel out in a pass rate.

#### Deterministic

No inference; same execution, same type, cacheable and diffable.

### Liabilities

#### Granularity

Four types can be too coarse. A rate-limited agent seems Divergent but needs
infrastructure scaling rather than machine changes; adding subtypes raises
classifier complexity.

#### Threshold sensitivity

The repetition detector's window and count need careful calibration to avoid
oversensitivity (false positives, over-calling Stuck) or leniency (missed
instances, under-calling it). Empirical tuning is essential.

#### Execution dependency

Agents logging only final outputs or lacking distinct machine phases cannot be classified.

#### Confidence level

This pattern has the least external validation in the language framework. The
four-type taxonomy and its detection rules, derived mainly from the reference
implementation, remain tentative until verified across more independent agent
systems. Its inclusion is justified by the classifier's high diagnostic value
and deterministic nature; however, its class names and thresholds are more
prone to evolution than earlier structural patterns.


## Implementation

Classification reads the state component of each entry, extracting `visits =
[(e.state, e.signal) for e in execution.entries]` or, for OTel traces, the
equivalent `agent.state` attributes on `execute_tool` spans (Chapter 8). Cycle
counting increments the count on each Validating-to-Composing transition,
treating consecutive retries as separate counts. Repetition detection uses a
sliding window to identify N identical late dispatches, a histogram to flag
$(state, tool)$ pairs exceeding a fraction of all dispatches, or both. The
classifier then decides based on four distinct branches.

```
classify(execution):
    terminal = execution.entries[-1].state
    cycles, repeated = count_retry_cycles(visits), detect_repetition(visits)
    if terminal == Succeeded: return Clean if cycles == 0 else Recovery
    if terminal == Failed:    return Stuck if repeated else Divergent
```

It runs in microseconds with no external dependencies. Grid evaluation
(Chapter 9) invokes it after each generator run, storing the type alongside
the pass/fail verdict and metrics. A report groups by type. Consider two
models reaching the same pass rate via different routes:

| Model | Clean | Recovery | Stuck | Divergent | Pass |
|-------|-------|----------|-------|-----------|------|
| Model A | 58% | 14% | 12% | 16% | 72% |
| Model B | 51% | 21% | 9% | 19% | 72% |

Both achieve a 72% pass rate (Clean + Recovery). Model A secures more first
attempts, while Model B recovers better. Model A's higher Stuck rate and Model
B's higher Divergent rate reveal distinct failure behaviors, despite equal
overall performance.


## Relationships in the Pattern Language

Convergence Taxonomy, a Machine Interpreter component, relies on the Machine
Interpreter and Transition Spans. It requires structured state-transition
evidence and stable trace attributes to classify runs without modifying the
machine. It processes completed executions, providing evaluation, reporting,
and remediation. The full grammar specification is in `pattern-language.yaml`.


## Known Uses

**Bench convergence reports.** The reference evaluator uses this pattern
variant. It reads per-tool metric snapshots, categorizes each tool's
progression into six classes (`CLEAN`, `CONVERGED`, `IMPROVING`, `FLAT`,
`REGRESSING`, `NO_DATA`), and derives a single overall class per run,
generating a text timeline (e.g., `2ok/3fail` → `PASS`). Grid reports
aggregate these classifications into `CleanRate`, `RecoveryRate` (converged
runs over runs with failures), and `StuckRate` (flat-or-regressing runs over
the same), revealing quality differences a simple pass rate would obscure.

**Planned diagnostics (design intent rather than yet shipped).** The
four-colored badges (green Clean, yellow Recovery, orange Stuck, red
Divergent) and a CI regression gate triggering on distribution shifts (Clean
down, Stuck up) represent the pattern's goal rather than current behavior. The
shipped classifier provides per-run classes and aggregate rates, but lacks
per-run Recovery/Stuck badges and thresholded gates. Implementing these
requires aligning the taxonomy across the eval-harness spec (srd019 R4.4), the
classifier, and this chapter to ensure consistent class naming.

**Classify-then-remediate precedents.** The **circuit breaker** [@nygard-2018]
runs a state classifier (closed/open/half-open) from observed outcomes and
drives a distinct action per state, following the "classify state, choose
remedy" discipline applied by the taxonomy to a completed run. The **Result /
Either monad** [@wadler-monads-1995] carries outcomes as a typed set of cases,
requiring exhaustive handling by the caller, mirroring the exhaustiveness
enforced by the four-type taxonomy.

**Trace-based diagnosis.** Process-mining techniques directly classify
executions from traces without re-running them: **process mining**
[@van-der-aalst-process-mining-2016] extracts and diagnoses process behavior
from event logs, while **conformance checking**
[@van-der-aalst-conformance-2012] compares observed traces against expected
models to identify deviations, strengthening the classifier's use of
transition patterns as diagnostic evidence.
