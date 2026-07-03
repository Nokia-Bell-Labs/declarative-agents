# Bidirectional Log

This chapter presents the Bidirectional Log pattern, which treats the recorded execution as a bidirectional log persisted one commit per step. Forward traversal is normal execution; backward traversal is rollback — a git-style revert of the persisted state followed by a reverse walk that replays each step's receipt through the owning tool's Undo. The chapter covers the typed checkpoint port, the Dolt-backed history, receipt-driven undo, the two-part rollback, and the handling of irreversible tools.

## Intent

Record execution as an ordered log persisted one commit per step, so recovery from a mistaken step is mechanical rather than probabilistic: rewind the persisted state with a git-style revert, then reverse the external effects by replaying each step's receipt through its tool's Undo.

## Motivation

An execution is not an append-only log. Logs record the past; an execution is a record the engine traverses both ways. Forward traversal is normal execution: dispatch a tool, record $(state, signal, tool, result)$ together with the tool's opaque receipt, commit the step, and advance. Backward traversal is undo: revert the persisted state to a target step, then hand each reverted entry's restored result back to its tool's `Undo` in reverse order. Both directions read the same record. Because every entry carries the tool's receipt — everything needed to reverse that one step — no separate undo log exists.

This matters because agents make mistakes. The model picks the wrong tool, writes broken code, misreads a requirement. Without rollback the only recovery is to restart or to ask the model to fix its own errors, which compounds mistakes rather than correcting them. With rollback the engine retracts the last N steps — reverting the database, replaying receipts to reverse files and resources — and continues down a different path.

## Applicability

Bidirectional Log fits agents whose actions have side effects that may need undoing — file writes, state mutations, provisioned resources. It is particularly valuable when recovery should be mechanical rather than a second LLM attempt layered on a contaminated history, when speculative execution is useful (try a reversible plan, roll back if unsatisfactory), and when irreversible effects need to be recorded for audit. It presumes persisted state is versioned step by step (the Dolt backend commits each step) and that external effects are reversible from a receipt. Agents with no side effects, or whose side effects are naturally idempotent, gain little from the pattern.

## Structure

A correct rollback separates two concerns bound to the same step index, shown as a package diagram in Fig. 19. **Persisted state** is the resumable Position — machine state, signal, iteration, budget counters, and the folded conversation — together with the ordered Execution log, each entry carrying its result digest and the tool's opaque receipt. It is saved through one typed checkpoint port and versioned commit-per-step by the Dolt backend. **External effects** are the world outside the database — files, provisioned resources — which no snapshot captures; they are reversed by replaying each step's receipt through the owning tool's `Undo`. Rewinding persisted state while leaving external effects in place, or the reverse, is incomplete: a rollback reverts the database to a step and then walks the receipts back to that same step.

![](figures/fig-20-rollback-layers.png)

| **Figure 19.** Package diagram. The two concerns a rollback coordinates at one step index: persisted state (Position and Execution) saved through the checkpoint port and versioned by Dolt, and external effects reversed through per-step receipts. |
|:---:|

### Participants

#### Receipt

The Receipt is an opaque string a tool encodes during `Execute`, carrying enough to reverse the effect without the original object — a file path and prior content, a resource identifier, a commit hash. The tool owns the receipt's schema and is its only decoder; the engine and the checkpoint adapters persist it verbatim and never interpret it.

#### Checkpoint port

The Checkpoint port is the typed, two-method persistence seam: `Save(Position, Execution)` records the resumable position and the ordered log as one unit, and `Load` restores them. Adapters own serialization and storage. The default persistent adapter is the Dolt backend, which commits each step; `NoopCheckpoint` disables persistence with no overhead.

#### Execution

The Execution is the ordered dispatch log — each entry an iteration, state pair, signal, command, result digest, and receipt — preserved in dispatch order for forward inspection and reverse traversal, and versioned as Dolt commit-per-step history.

#### Lifecycle tool

Rollback runs as a declared lifecycle tool (`checkpoint_rollback`), not engine code and not part of the domain machine; keeping it a tool separates *what the agent does* from *how it recovers*. It performs the two-part rollback — Dolt `Revert` for persisted state, then the reverse receipt walk — and reads the reversibility tier of Chapter 4:

| Reversibility tier | Undo behaviour |
|---|---|
| Noop | Read-only; empty receipt; skipped during rollback |
| Reversible | `Undo` decodes the receipt and restores the prior state |
| Compensatable | `Undo` derives a corrective action from the receipt, logs the difference |
| Irreversible | Skipped, logged as non-reversible |

## Collaborations

### The lifecycle tool

Rollback is a declared tool dispatched by a minimal lifecycle machine over the run's Execution and Dolt commit history. The state machine diagram in Fig. 20 shows it: from **Reverting** (a git-style `Revert(run_id, step_index)` rewinds persisted state to the target step), the tool walks the later entries in reverse, each **Undone** (reversible/compensatable) or **Skipped** (irreversible, logged); **Done** returns a resumable position to the primary machine.

![](figures/fig-21-rollback-lifecycle.png)

| **Figure 20.** State machine diagram. The lifecycle tool reverts persisted state to the target step, then walks the later entries backward, undoing reversible tools through their receipts and skipping irreversible ones. {wide 0.7} |
| :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------: |

A lifecycle tool pays off three ways: it is small enough to validate exhaustively; different rollback strategies are just different lifecycle tools; and it reads the execution and commit history and calls tool `Undo`, but never dispatches a domain tool, so it cannot accidentally advance the domain machine.

### Reversal by tier

Each reverted entry is processed by its tier. **Reversible: local undo.** The tool's `Undo` decodes the receipt and restores exactly what it changed (restore file content, write back a prior value). **Compensatable: corrective action.** Exact reversal is impossible, but `Undo` derives a corrective action from the receipt that restores equivalent state (delete a created resource); the receipt carries the resource identifier, and semantic differences are documented (a re-created resource gets a new ID). **Irreversible: skip and log.** An email sent or deployment published cannot be undone; the entry is logged explicitly in the rollback report with tool, iteration, and reason. Per-entry tier classification, not a global setting, lets a single rollback handle mixed tiers.

## Consequences

### Benefits

#### Mechanical recovery

Reverting the database to a step and replaying receipts to reverse external effects is deterministic, not a probabilistic second attempt; fresh continuation gives the model the pre-error context without the contamination of failed tries.

#### Cheap exploration

When a plan is all reversible, the agent executes speculatively and rolls back automatically, leaving no residue.

#### Auditable irreversibility

Skipped irreversible entries appear in the rollback report, so operators see exactly which effects persist.

#### One persistence seam

Position and Execution ride a single typed port into Dolt, so the ordered, versioned history that rollback and inspection traverse comes from the same commit-per-step store rather than a bespoke snapshot format.

### Liabilities

#### Commit overhead

Committing every step has cost, so persistence is opt-in; `NoopCheckpoint` keeps disabled runs free of it.

#### An irreversibility floor

Once an irreversible tool commits its external effect, the receipt walk cannot reverse it; `Revert` rewinds only the database, so the irreversible effect is permanent.

#### Two-part coordination

Rewinding persisted state and replaying receipts must target the same step index. This is simpler than coordinating three independent state layers, but the revert and the receipt walk still have to agree on where the rollback stops.

## Implementation

### Receipts and undo paths

**Live undo** applies within the same process: the tool object is still in memory and its `Undo` reverses the effect directly from the in-memory result, fast and precise. **Post-restart undo** applies after a process boundary (suspend then resume elsewhere): no original object remains, so `Load` restores the receipt-bearing result and a fresh tool instance's `Undo` consumes the receipt. Every state-mutating tool encodes a receipt during `Execute`; read-only tools return an empty receipt and a no-op `Undo`. Enforcement is split: the lifecycle validator checks receipt *presence* for reversible state-mutating tools, while receipt *sufficiency* — that the encoded receipt actually reverses the effect — is verified by each tool's own round-trip test, not by the engine. Static tier declaration enables planning; the presence check and the tool's round-trip test together keep the declaration honest.

### Commit-per-step history

The Dolt backend commits each dispatched step, so the execution log is a versioned history rather than a set of periodic snapshots. `Save` records the Position — machine state, signal, budget counters, and the folded conversation — and the appended Execution entry as one unit, in a single commit; the port persists on every step, so no policy decides when to snapshot. Suspend persists through the same port before the run exits, and resume `Load`s the Position and Execution and re-enters the machine at the restored position.

### Rollback: revert then replay

Rolling back to step $k$ is two moves over the same index. First, the Dolt adapter's `Revert(run_id, k)` rewinds persisted state — Position and Execution — to that step. Second, the lifecycle tool walks entries $k{+}1 \ldots n$ in reverse, handing each reverted entry's restored result to its tool's receipt-consuming `Undo`, skipping and logging irreversible entries in the rollback report. Rollback returns the machine position at step $k$, from which execution resumes as a *fresh continuation*, not a replay: replaying would reproduce the same mistake. Discarded entries survive only in the rollback report.

### Planning with reversibility

Tier classification turns into active planning strategies: **speculative execution** for all-reversible plans; **commitment phases** that explore with reversible tools and cross into irreversible commitment only when evidence suffices, guarded by a confirmation state; and **saga-style compensation** [@garcia-molina-sagas-1987] for mixed plans, where the lifecycle tool derives the compensation order from the execution (reverse of dispatch) rather than hardcoding it.

## Relationships in the Pattern Language

Bidirectional Log sits within Machine Interpreter and requires Machine Interpreter and Tool Contract: rollback needs a closed execution record plus tool-level receipt-driven undo and reversibility declarations. It enables Approval Gate, which checkpoints before an external decision, and Operator Port, which can expose rollback and lifecycle operations safely through the running machine. The complete grammar is maintained in `pattern-language.yaml`.

## Known Uses

**Coding agents.** Generators write files speculatively; when validation fails, `checkpoint_rollback` reverts the Dolt history to the last good step and replays the intervening receipts to restore the files, and the model retries with clean context instead of debugging its own contaminated history.

**Gated deployment.** An approval gate before an irreversible `deploy` tool sets the rollback floor; if post-deploy checks fail for reasons outside the deploy itself, execution returns to the pre-deploy step — the database reverted and reversible effects undone through their receipts — while the deploy is logged as non-reversible (Chapter 10).

**Multi-step API plans.** Sequences mixing compensatable resource creation and irreversible notifications let the lifecycle tool derive the compensation order from the execution: created resources are deleted through their receipts in reverse order, and irreversible steps are logged, without a hand-written compensation sequence.

**Database transactions and rollback** [@gray-1978]. The canonical model of a durable, reversible sequence of operations with a commit boundary, where rollback restores the prior consistent state, is more than an analogue here: the Dolt backend literally commits each step and reverts to a prior consistent state.

**Memento pattern** [@gamma-gof-1994]. Capturing an object's state so it can be restored later without violating encapsulation is exactly the Receipt each tool encodes during `Execute`: the tool owns the schema, the engine and the Dolt adapter store it opaquely, and only the originating tool decodes it on `Undo`. The encapsulation boundary the pattern prescribes is enforced rather than assumed.

**Event Sourcing** [@fowler-event-sourcing-2005]. Modelling state as a replayable and reversible log of events matches the Dolt commit-per-step execution directly: it is the same kind of traversable log, read forward to run and reverted backward to undo.
