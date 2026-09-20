<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Approval Gate

An Approval Gate halts execution at a declared machine state, checkpoints it,
and awaits external approval or rejection. The full pattern exceeds the
lifecycle conformance fixture in the reference implementation, and the two
remain separate here.

## Intent

Suspend execution at a declared state, checkpoint it, and resume or roll back
based on external decisions, preserving execution context across the
suspension.

## Reference implementation status

The shipped conformance fixture shows a Dolt-backed suspend and a CLI
invocation resuming stored Position and Execution with `Approved` or
`Rejected`. Approval results in `Succeeded`; rejection ends in `Rejected`.
This is a conformance fixture, not a deployed approval gate.

Authority notification, proposal and decision metadata, decider identity,
rejection-triggered rollback, multi-authority chains, and a deployed
confirmation API remain design intent. The standard checkpoint encapsulates
the resumable Position and ordered Execution log, omitting approval-decision
records.


## Motivation

Agents that modify production systems require confirmation gates: before
deployment, data deletion, or provisioning, a human must review the agent's
proposal and decide. **Callback gates** halt a thread or register a callback,
lost on process restart (crash, deployment, timeout, migration), requiring a
fresh start. **Polling gates** write to a queue and poll, but each gate
serializes and reconstructs its own state, lacking shared infrastructure and
guaranteeing no alignment with the original state.

Both bolt the gate onto control flow imperatively. The gate is a declared
transition, a signal triplet backed by the same checkpoint infrastructure that
supports suspend and resume. Structural, not procedural.


## Applicability

The Approval Gate is for agents executing irreversible actions—deployments,
deletions, transactions—needing prior human or automated approval. It's useful
when decisions span hours or days, require sequential sign-offs, or need an
auditable trail. Avoid gating autonomous decisions, as unnecessary gates add
human latency. Place gates per Chapter 4: before irreversible tools rather
than reversible ones.


## Structure

Suspending, gating, and resuming execution rely on five distinct participants,
whose relationships are shown in the class diagram (Fig. 26).

![](figures/fig-27-approval-gate-class.png)

| **Figure 26.** Class diagram. The participants: the Gate signal triplet, the Checkpoint, the Authority, the SuspendTool, and the ResumeEngine. |
|:---:|

### Participants

#### Gate

The signal triplet `AwaitApproval`/`Approved`/`Rejected` represents ordinary
machine signals handled by ordinary transitions, requiring nothing special for
the engine.

#### Checkpoint

The typed snapshot persisted at suspension through the Chapter 7 checkpoint
port, including the resumable Position (machine state, counters, folded
conversation) and the ordered Execution log, committed by the Dolt backend.

#### Authority

The external decision-maker, whether a human, policy engine, or chain of
approvers, operates independently of the prompt pattern. How it decides is
outside the pattern.

#### SuspendTool

The shipped tool emits `AwaitApproval` with the configured reason, records a
trace event, suspends the loop, and avoids contacting an authority.

#### ResumeEngine

The system loads a checkpoint, re-enters the loop, and proceeds beyond the
gate if the status is `Approved`. If `Rejected`, it routes to rollback or an
alternative path.


## Collaborations

Fig. 27 shows the state machine's transitions: `AwaitApproval` checkpoints and
suspends, `Approved` resumes, and `Rejected` may route to rollback. The
conformance fixture, however, terminates directly at `Rejected` without
rollback.

![](figures/fig-28-approval-gate-state.png)

| **Figure 27.** State machine diagram of the complete pattern. The fixture covers suspend and both decision signals, but not rejection rollback. {wide} |
|:---:|

**Suspension** follows a precise sequence (Fig. 28): the engine initiates the
suspend tool, which responds with `AwaitApproval` and its predefined reason.
The engine checkpoints the run via the Chapter 7 port, preserving Position and
Execution, with the Dolt backend committing changes, then exits the loop. The
process terminates, allowing arbitrary time to elapse with state stored in the
checkpoint. While notifying an authority with a proposal and checkpoint
reference completes the pattern, the reference runtime omits this
notification.

![](figures/fig-29-suspension-sequence.png)

| **Figure 28.** Sequence diagram of the complete pattern. The shipped path persists and exits; the authority-notification step remains design intent. {wide} |
|:---:|

**Resumption** injects a signal into a loaded checkpoint. The runtime restores
Position and conversation state, then injects the value from
`--resume-signal`. In the fixture, `Approved` signals route from `done` to
`Succeeded`, while `Rejected` signals go directly to `Rejected`, bypassing
rollback. Production machines may route rejection to revision or checkpoint
rollback, but these paths await deployed profiles and focused tests.


## Consequences

### Benefits

#### Safe irreversible operations

Deployment, deletion, and provisioning tools can reside within the registry
without posing a threat, as the machine executes them only when approved.

#### Cross-session continuity

The state, stored in the checkpoint store rather than within a process, allows
the agent to survive restarts, migrations, and long delays.

#### Auditable decisions

A complete gate captures the proposal, decision, time, and decider as a
compliance artifact. The shipped checkpoint lacks these fields.

#### Compositional gates

Gates function as transitions, enabling multi-stage approval to compose
naturally. Staging precedes production, ensuring structured progression
through each gate.

### Liabilities

#### Human latency

Each gate requires human intervention, creating a dependency. Timeouts, `
(Suspended, Timeout) -> Failed`, reduce but don't remove this serialization.

#### Checkpoint storage

Each gate persists potentially large state, needing cleanup policies.

#### Context staleness

The machine re-validates preconditions before executing the gated action, as
the world may change between suspend and resume.


## Implementation

Suspend functions prevent the model from bypassing the gate by omitting its
invocation, acting as an `internal` lifecycle boundary tool. The shipped
declaration is compensatable, enabling the process to resume with an explicit
decision or roll back to an earlier checkpoint.

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

The fixture uses the standard checkpoint: Position (machine state, counters,
and folded conversation) plus ordered Execution. Gate metadata such as
proposal, status, decider, decision time, and rationale is not part of the
shipped checkpoint. Adding those fields is design intent.

Resume uses universal runtime flags with the standard `agent` command. It
lacks a lifecycle-specific resume subcommand and a `--reason` flag.

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

The conformance test gets `RUN_ID` from the persisted Dolt run branch. A
future authority service could use the same flags or an equivalent
control-plane signal path, but pending-gate discovery and programmatic
authority notification are not shipped.


## Relationships in the Pattern Language

Approval Gate, part of the Machine Interpreter, requires the Machine
Interpreter, Tool Contract, Bidirectional Log, and Phase-Scoped Toolset. It
acts as a declared transition: the gated operation follows a contract,
rollback mechanisms handle rejection, and scoped manifests keep commitment
tools locked until approval. This gate enables the Operator Port to accept the
resume decision as a validated control-plane signal. Full grammar is in
`pattern-language.yaml`.


## Known Uses

**Lifecycle approval conformance fixture.** Located at
`applications/catalog/testdata/conformance/lifecycle/approval`, this fixture
handles `suspend` operations, persists data via Dolt, and resumes
functionality using universal CLI flags. It validates approved and rejected
terminal routing, excluding notification, decision metadata, rejection
rollback, and deployment.

**Deployment confirmation (design intent).** A coding agent acts as a gate
between validation and deployment: approval triggers deployment, rejection
rolls back to the pre-deployment checkpoint. Currently, no shipped
coding-agent profile implements this flow.

**Multi-step authorization chains (design intent).** The generic pattern uses
sequential thresholds and independent authority records, but the reference
implementation lacks a deployed chain and a defined authority-record schema.

**Two-phase commit** [@gray-1978]. The coordinator prepares participants,
waits for a decision, then proceeds to the irreversible commit phase,
mirroring the suspend-decide-commit structure enforced by the gate before
executing an irreversible tool.

**Durable waits in workflow engines.** **Temporal** [@temporal-2024] ensures
durable execution by pausing workflows via signals until an external decision
is received, maintaining continuity across process restarts. **AWS Step
Functions callback tasks** [@aws-step-functions-callback-2024] suspend
execution using an external callback token, resuming upon signal receipt. Both
mechanisms match the wait-and-resume structure of approval gates, confirming
that surviving arbitrary suspension is a well-established pattern.
