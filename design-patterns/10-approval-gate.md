<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Approval Gate

This chapter presents the Approval Gate pattern, which suspends execution at a declared machine state, checkpoints the execution, and waits for an external authority to approve or reject. It separates the complete pattern from the narrower lifecycle conformance fixture in the reference implementation.

## Intent

Suspend execution at a declared state, checkpoint it, and resume or roll back on an external decision without losing execution context across the suspension.

## Reference implementation status

The shipped conformance fixture proves a Dolt-backed suspend and a second CLI invocation that resumes the stored Position and Execution with either `Approved` or `Rejected`. Approval reaches `Succeeded`; rejection reaches the fixture's `Rejected` terminal state. This is a conformance fixture, not a deployed coding-agent approval gate.

Authority notification, proposal and decision metadata, decider identity, rejection-triggered rollback, multi-authority chains, and a deployed confirmation API remain design intent. The standard checkpoint contains the resumable Position and ordered Execution log; it has no approval-decision record.


## Motivation

Agents that modify production systems need confirmation gates: before deploying, deleting data, or provisioning, a human should review what the agent proposes and decide. Two ad-hoc approaches both fail. **Callback gates** block a thread or register a callback, which is lost if the process restarts (crash, deployment, timeout, migration), forcing a restart from scratch. **Polling gates** write to a queue and poll, but every gate then serializes its own context and reconstructs its own state, with no shared infrastructure and no guarantee the restored state matches the original.

Both bolt the gate onto control flow imperatively. In the Machine Interpreter the gate is a declared transition, a signal triplet backed by the same checkpoint infrastructure that supports suspend and resume. Structural, not procedural.


## Applicability

The Approval Gate fits agents that can take irreversible actions — deployments, deletions, transactions — before which a human or automated authority must confirm. It is especially suited when a decision may take hours or days, requiring execution to survive process boundaries; when multiple authorities must sign off in sequence; or when an audited decision trail matters. Do not gate decisions the agent can make autonomously; unnecessary gates serialize a fast pipeline through human latency. Gate placement follows the reversibility tiers of Chapter 4: gate before irreversible tools, not before reversible ones.


## Structure

Suspending, gating, and resuming execution involves five participants, related as the class diagram in Fig. 26 shows.

![](figures/fig-27-approval-gate-class.png)

| **Figure 26.** Class diagram. The participants: the Gate signal triplet, the Checkpoint, the Authority, the SuspendTool, and the ResumeEngine. |
|:---:|

### Participants

#### Gate

The signal triplet `AwaitApproval`/`Approved`/`Rejected`, ordinary machine signals handled by ordinary transitions, nothing special to the engine.

#### Checkpoint

The typed snapshot persisted at suspension through the Chapter 7 checkpoint port: the resumable Position (machine state, counters, folded conversation) and the ordered Execution log, committed by the Dolt backend.

#### Authority

The external decision-maker (human, policy engine, or chain of approvers); how it decides is outside the pattern.

#### SuspendTool

Emits `AwaitApproval` with the configured reason. The shipped tool records a trace event and asks the loop to suspend; it does not contact an authority.

#### ResumeEngine

Loads a checkpoint and re-enters the loop, continuing past the gate on `Approved`, routing to rollback or an alternative path on `Rejected`.


## Collaborations

The state machine diagram in Fig. 27 shows the complete pattern: `AwaitApproval` checkpoints and suspends, `Approved` resumes, and `Rejected` may route to rollback. The conformance fixture instead terminates at `Rejected`.

![](figures/fig-28-approval-gate-state.png)

| **Figure 27.** State machine diagram of the complete pattern. The fixture covers suspend and both decision signals, but not rejection rollback. {wide} |
|:---:|

**Suspension** proceeds in strict order, traced by the sequence diagram in Fig. 28: the engine dispatches the suspend tool like any other; the tool returns `AwaitApproval` with its configured reason; the engine checkpoints the run through the Chapter 7 port — saving the Position and Execution, committed by the Dolt backend — and exits the loop. The process may then terminate; an arbitrary time may pass with state living in the checkpoint. Notifying an authority with a proposal and checkpoint reference is part of the complete pattern, but the reference runtime does not implement that notification.

![](figures/fig-29-suspension-sequence.png)

| **Figure 28.** Sequence diagram of the complete pattern. The shipped path persists and exits; the authority-notification step remains design intent. {wide} |
|:---:|

**Resumption** is signal injection into a loaded checkpoint. The runtime restores Position and conversation, then injects the value supplied by `--resume-signal`. In the fixture, `Approved` routes through `done` to `Succeeded`, while `Rejected` routes directly to the `Rejected` terminal state; no rollback runs. A production machine may instead route rejection to revision or checkpoint rollback, but those paths are design intent until a deployed profile and focused test ship.


## Consequences

### Benefits

#### Safe irreversible operations

Deployment, deletion, and provisioning tools can sit in the registry without being dangerous; the machine runs them only after approval.

#### Cross-session continuity

State lives in the checkpoint store, not a process, so the agent survives restarts, migrations, and long delays.

#### Auditable decisions

A complete gate records the proposal, decision, time, and decider as a compliance artifact. The shipped checkpoint does not yet carry these fields.

#### Compositional gates

Multiple gates are just ordinary transitions, so multi-stage approval (staging then production) composes naturally.

### Liabilities

#### Human latency

Each gate serializes execution through a human; timeouts (`(Suspended, Timeout) -> Failed`) mitigate but do not remove it.

#### Checkpoint storage

Each gate persists potentially large state, needing cleanup policies.

#### Context staleness

The world may change between suspend and resume, so the machine author must re-validate preconditions before executing the gated action.


## Implementation

Suspend is an `internal` lifecycle boundary tool, so the model cannot skip the gate by declining to call it. The shipped declaration is compensatable: resume with an explicit decision or roll back to an earlier checkpoint.

```yaml
- name: suspend
  type: builtin
  init: suspend
  visibility: internal
  emits: [AwaitApproval, CommandError]
  config:
    label: approval
    reason: awaiting approval
    require_checkpoint: false
  reversibility:
    classification: compensatable
    undo: resume_with_rejected_or_rollback_checkpoint
```

The fixture uses the standard checkpoint: Position (machine state, counters, and folded conversation) plus ordered Execution. Gate metadata such as proposal, status, decider, decision time, and rationale is not part of the shipped checkpoint. Adding those fields is design intent.

Resume uses universal runtime flags on the ordinary `agent` command. There is no lifecycle-specific resume subcommand and no `--reason` flag:

```bash
bin/agent --profile "$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/approval/profile.yaml" \
  --dolt-dsn "$DOLT_DSN"

bin/agent --profile "$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/approval/profile.yaml" \
  --dolt-dsn "$DOLT_DSN" \
  --resume-checkpoint "$RUN_ID" \
  --resume-signal Approved

bin/agent --profile "$AGENT_CATALOG_ROOT/testdata/conformance/lifecycle/approval/profile.yaml" \
  --dolt-dsn "$DOLT_DSN" \
  --resume-checkpoint "$RUN_ID" \
  --resume-signal Rejected
```

The conformance test obtains `RUN_ID` from the persisted Dolt run branch. A future authority service could invoke the same flags or an equivalent control-plane signal path, but pending-gate discovery and programmatic authority notification are not shipped.


## Relationships in the Pattern Language

Approval Gate sits within Machine Interpreter and requires Machine Interpreter, Tool Contract, Bidirectional Log, and Phase-Scoped Toolset: the gate is a declared transition, the gated operation has a contract, rollback handles rejection, and scoped manifests keep commitment tools unreachable before approval. It enables Operator Port because the resume decision can arrive as a validated control-plane signal. The complete grammar is maintained in `pattern-language.yaml`.


## Known Uses

**Lifecycle approval conformance fixture.** The fixture under `applications/catalog/testdata/conformance/lifecycle/approval` dispatches `suspend`, persists through Dolt, and resumes through the universal CLI flags. It proves approved and rejected terminal routing, but not notification, decision metadata, rejection rollback, or deployment.

**Deployment confirmation (design intent).** A coding agent could gate between validation and deployment: approval deploys, while rejection rolls back to the pre-deployment checkpoint. No shipped coding-agent profile implements this flow.

**Multi-step authorization chains (design intent).** Sequential thresholds and independent authority records are part of the generic pattern, but the reference implementation has no deployed chain or authority-record schema.

**Two-phase commit** [@gray-1978]. A coordinator prepares participants and waits for a decision before the irreversible commit, the same suspend-then-decide-then-commit structure the gate imposes before an irreversible tool.

**Durable waits in workflow engines.** **Temporal** [@temporal-2024] offers durable execution with signals that pause a workflow until an external decision arrives, surviving process restarts, and **AWS Step Functions callback tasks** [@aws-step-functions-callback-2024] suspend a task on an external callback token before resuming. Both match the declared wait-and-resume structure of an approval gate, corroborating that surviving arbitrary suspension is a solved, recurring shape.
