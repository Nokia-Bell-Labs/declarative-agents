# Lifecycle, Checkpoint, Resume, and Rollback Operations

This guide explains how to turn on lifecycle features, how to operate approval
gates, and what rollback can and cannot make safe.

The design requirements live in
`docs/specs/software-requirements/srd025-rollback-lifecycle.yaml`.
Lifecycle tool adapter requirements live in
`docs/specs/software-requirements/srd026-lifecycle-tools.yaml`. The tool
authoring contract for side effects, reversibility, undo, and confirmation
lives in `docs/specs/config-formats/tool-authoring-standard.yaml`. The
historical design note in `roll-backs.md` is useful background, but the spec
files are the source of truth.

## Defaults

By default, lifecycle persistence is off. A normal agent run without `--dolt-dsn`
uses `NoopCheckpoint`, creates no checkpoint repository, records no durable
history, and cannot be resumed from another process.

Set `--dolt-dsn <dsn>` when a run must persist checkpoints. The DSN connects to
a running `dolt sql-server` through the MySQL wire protocol. The runtime opens
`DoltCheckpoint`, saves after each dispatch cycle, and stores the typed
`Position` plus ordered `Execution` log defined by `srd035`.

Reserve persistent Dolt storage for shared operator history, retained artifacts,
suspend/resume flows, or rollback investigations. Tests that need durable state
should use an isolated Dolt database on a local dolt sql-server.

## Checkpoint Model

Rollback is safe only when persistent runtime state and external effects stay
separate.

Persistent runtime state flows through the typed `Checkpoint` port. `Save` stores
the resumable `Position` and ordered `Execution` log. `Position` carries the
current state, last signal, loop counters, and folded conversation snapshot.
`Execution` carries one entry per dispatched command, including the command name,
from/to state, signal, result digest, and opaque receipt.

External effects are tool-owned. A tool that changes files, calls an external
API, launches a child agent, or mutates another boundary must encode the receipt
its `Undo` needs during `Execute`. The engine and checkpoint adapters persist the
receipt bytes but never interpret them.

Dolt rollback is DB-only. `DoltCheckpoint.Revert(run_id, step_index)` rewinds the
checkpoint tables to the target step. The `checkpoint_rollback` lifecycle tool
then walks restored execution entries in reverse and hands each receipt-bearing
result to the originating tool's receipt-consuming `Undo`.

## Runtime Data

On the main `agent` command, universal flags carry lifecycle runtime data.
`--directory <path>` sets the workspace root for file tools and child agents.
`--dolt-dsn <dsn>` enables the persistent checkpoint backend. `--resume-checkpoint
<id>` loads a persisted checkpoint before entering the loop again and requires a
persistent backend. `--resume-signal <signal>` supplies the first resumed
transition signal, with `Approved` as the default.

Lifecycle request files carry history and rollback targets. The profile selects
the MachineSpec and ToolDef. In the request, `checkpoint` selects a checkpoint
ID or `latest`, and `to_iteration` is required for rollback. The workspace root
remains the universal `--directory` runtime data channel for tools that need a
filesystem boundary.

Examples:

History request:

```yaml
checkpoint: latest
```

Rollback request:

```yaml
checkpoint: suspend-4-1780000000000000000
to_iteration: 2
```

Lifecycle profile invocations:

```bash
agent \
  --profile agents/lifecycle/history/profile.yaml \
  --directory "$PWD" \
  --dolt-dsn "$DOLT_DSN" \
  --request requests/history.yaml

agent \
  --profile agents/lifecycle/rollback/profile.yaml \
  --directory "$PWD" \
  --dolt-dsn "$DOLT_DSN" \
  --request requests/rollback.yaml

agent \
  --profile agents/generator/profile.yaml \
  --dolt-dsn "$DOLT_DSN" \
  --resume-checkpoint rollback-suspend-4-1780000000000000000-to-2-1780000000000000001 \
  --resume-signal Approved \
  --directory "$PWD"
```

Enable persistent history with Dolt:

```bash
agent \
  --profile agents/lifecycle/history/profile.yaml \
  --directory "$PWD" \
  --dolt-dsn "$DOLT_DSN" \
  --request requests/history.yaml
```

## Approval Gates

Approval gates are ordinary machine transitions. The machine routes a risky
state to the `suspend` builtin, the builtin emits `AwaitApproval`, and the loop
returns `StatusSuspended` after saving through the configured `Checkpoint` port.
If the selected suspend tool requires persistence, `--dolt-dsn` must be set so
the checkpoint is not a no-op.

Minimal machine shape:

```yaml
states:
  - Planning
  - AwaitingApproval
  - Applying
  - Rejected
  - Done

initial_state: Planning
terminal_states: [Rejected, Done]

signals:
  - Seed
  - ToolDone
  - AwaitApproval
  - Approved
  - Rejected
  - TaskCompleted
  - CommandError

transitions:
  - state: Planning
    signal: Seed
    next: AwaitingApproval
    action: suspend_for_approval

  - state: AwaitingApproval
    signal: AwaitApproval
    next: AwaitingApproval

  - state: AwaitingApproval
    signal: Approved
    next: Applying
    action: apply_change

  - state: AwaitingApproval
    signal: Rejected
    next: Rejected

  - state: Applying
    signal: TaskCompleted
    next: Done
```

Tool selection must include a `suspend` tool declaration. The shared builtin
declaration in `tools/builtin.yaml` exposes `init: suspend` and supports
configuration such as `reason` and `require_checkpoint`.

## Backtracking Workflow

Use history before rollback:

`requests/history.yaml`:

```yaml
checkpoint: latest
```

```bash
agent \
  --profile agents/lifecycle/history/profile.yaml \
  --directory "$PWD" \
  --request requests/history.yaml
```

Pick the last known-good iteration from the digest. Each row includes iteration,
command name, state transition, signal, and receipt presence.

Create a rollback checkpoint:

`requests/rollback.yaml`:

```yaml
checkpoint: latest
to_iteration: 7
```

```bash
agent \
  --profile agents/lifecycle/rollback/profile.yaml \
  --directory "$PWD" \
  --request requests/rollback.yaml
```

Resume from the rollback checkpoint printed by the lifecycle tool:

```bash
agent \
  --profile agents/<agent>/profile.yaml \
  --resume-checkpoint <rollback-checkpoint-id> \
  --resume-signal Approved \
  --directory "$PWD"
```

The resume path validates the current machine and tool declarations before
re-entering the loop. If the current config is incompatible with the checkpoint,
resume refuses to continue.

## Receipts

Checkpoint history stores `Execution` entries, not live `Command` objects. Each
entry may carry an opaque receipt copied from `Result.Receipt`. Commands that
need persisted rollback encode everything their later `Undo` needs into that
receipt during `Execute`.

Receipt shape belongs to the originating tool. File tools can encode prior file
content and mode. Conversation tools can encode a prior conversation snapshot.
Boundary tools can encode child run IDs, artifact paths, issue IDs,
server/user-action details, REST compensation metadata, or nested-machine
context. The checkpoint adapter stores receipts verbatim; only the tool decodes
them.

Rollback reports irreversible or missing receipts instead of inventing a restore
path. Read-only commands return no receipt and use no-op Undo.

## Operational Safety

Rollback is not a time machine for every side effect. It coordinates Dolt
Revert for DB-persisted checkpoint state with best-effort receipt-consuming Undo
for external effects.

Use `--directory` only for the workspace the selected tools are allowed to
mutate. The Dolt adapter does not reset that workspace. File, REST, child-agent,
and other boundary effects are reversed only when their tools encoded sufficient
receipts.

Treat boundary tools as risky. Tools that call subprocesses, nested machines,
external APIs, humans, or models must declare their side effects and rollback
story in the tool contract. If the external effect cannot literally be undone,
the tool should be classified as compensatable or irreversible.

Require confirmation for irreversible tools that affect user data, external
services, shared infrastructure, or published artifacts. Approval gates are the
normal way to enforce that confirmation in a machine.

No-op undo is acceptable only for truly read-only commands or for migration
steps that are explicitly tracked in the command undo audit. A no-op undo on a
state-mutating command leaves residual risk after rollback.

If rollback reports a partial failure, stop and inspect the details before
resuming. A partial rollback can mean Dolt Revert failed, a restored receipt was
missing or irreversible, or a receipt-consuming Undo failed.

## Migration Notes

Older notes may mention hidden `agent history`, `agent rollback`, `.agent-state`,
`--state-store-dir`, `StateStore`, `Workspace`, `GitWorkspace`, or
`CheckpointPolicy`. Treat those as historical background. Normal lifecycle
operation uses `agent --profile agents/lifecycle/history/profile.yaml` and
`agent --profile agents/lifecycle/rollback/profile.yaml`, with `--dolt-dsn` when
durable persistence is required. Runtime data stays on the universal `agent`
command. Checkpoint selection and target iteration stay in request files or typed
tool config.

## Related Documents

Canonical requirements live in
`docs/specs/software-requirements/srd025-rollback-lifecycle.yaml` and
`docs/specs/software-requirements/srd026-lifecycle-tools.yaml`. Runtime and tool
contracts live in `docs/specs/config-formats/runtime-contract.yaml` and
`docs/specs/config-formats/tool-authoring-standard.yaml`. Semantic detail lives
in `docs/specs/semantic-models/rollback-lifecycle.yaml` and
`docs/specs/semantic-models/command-undo-audit.yaml`. Operator scenarios live in
`docs/specs/use-cases/rel02.0-uc001-approval-suspend-resume.yaml` and
`docs/specs/use-cases/rel02.0-uc002-history-rollback.yaml`. Historical notes
remain in `roll-backs.md` and `tools-as-dsl.md`.
