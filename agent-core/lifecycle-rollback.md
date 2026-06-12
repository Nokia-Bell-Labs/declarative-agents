# Lifecycle, Checkpoint, Resume, and Rollback Operations

This guide explains how to turn on lifecycle features, how to operate approval
gates, and what rollback can and cannot make safe.

The design requirements live in
`docs/specs/software-requirements/srd025-rollback-lifecycle.yaml`. The tool
authoring contract for side effects, reversibility, undo, and confirmation lives
in `docs/specs/config-formats/tool-authoring-standard.yaml`. The historical
design note in `roll-backs.md` is useful background, but the spec files are the
source of truth.

## Defaults

Lifecycle behavior is opt-in. A normal agent run without lifecycle flags keeps
the existing runtime behavior:

- no checkpoint directory is created;
- no checkpoint JSON is persisted;
- no resume state is restored;
- no workspace restore is attempted;
- history stays minimal unless the runtime is configured with a
  `CheckpointPolicy`.

The main CLI enables local checkpoint persistence with `--state-store-dir`.
Without that flag, suspend can still emit lifecycle signals, but checkpoint
persistence is unavailable. Tools that set `require_checkpoint: true` fail
explicitly when no `StateStore` is configured.

## State Model

Rollback is safe only when the runtime keeps three state layers separate.

Agent state is loop-owned JSON: state-machine position, current signal,
iteration count, budget counters, token counters, cost counters, and run
summary data. It is stored in `Checkpoint.agent_state`.

Command and domain state is command-owned JSON or in-memory undo state:
conversation history, planner state, evaluator session state, graph state, and
other mutable state that is not a filesystem tree. Commands that mutate this
layer must implement `Command.Undo()` for in-memory rollback and, when the state
must survive checkpoint persistence, provide an `UndoMemento()` payload that can
be written into history.

Workspace state is environment state: files and directories changed by file
tools, exec tools, child agents, or external processes. It is tracked by a
`Workspace` reference, usually a `GitWorkspace` commit. Workspace refs are not
serialized agent state; they point at a filesystem snapshot.

Do not store workspace trees in `StateStore`, and do not use git commits as the
serialization format for agent or conversation state.

## Runtime Flags

Use these flags on the main `agent` command:

- `--state-store-dir <dir>` enables the local `FileStore` for lifecycle
  checkpoints. Checkpoints are stored as JSON under keys like
  `checkpoint/<id>`.
- `--resume-checkpoint <id>` loads a persisted checkpoint before entering the
  loop again. It requires `--state-store-dir`.
- `--resume-signal <signal>` supplies the signal that drives the first
  transition after resume. The default is `Approved`.
- `--directory <path>` sets the workspace root for file tools and workspace
  restore. When rollback or resume needs to restore a workspace ref, this path
  must be a managed git repository root accepted by `GitWorkspace`.

Use these lifecycle subcommands:

- `agent history --state-store-dir <dir> --checkpoint latest` prints the
  checkpoint history digest. Use an explicit checkpoint ID instead of `latest`
  when you need reproducibility.
- `agent rollback --state-store-dir <dir> --checkpoint <id> --to-iteration N`
  creates a new rollback checkpoint at the target iteration.
- Add `--directory <path>` to `agent rollback` when the target history entry has
  a workspace ref. The command refuses unmanaged workspace restore without it.

Examples:

```bash
agent history \
  --state-store-dir .agent-state \
  --checkpoint latest

agent rollback \
  --state-store-dir .agent-state \
  --checkpoint suspend-4-1780000000000000000 \
  --to-iteration 2 \
  --directory "$PWD"

agent \
  --machine agents/generator/machine.yaml \
  --tools agents/generator/tools.yaml \
  --tools-declaration tools/builtin.yaml \
  --state-store-dir .agent-state \
  --resume-checkpoint rollback-suspend-4-1780000000000000000-to-2-1780000000000000001 \
  --resume-signal Approved \
  --directory "$PWD"
```

## Approval Gates

Approval gates are ordinary machine transitions. The machine routes a risky
state to the `suspend` builtin, the builtin emits `AwaitApproval`, and the loop
returns `StatusSuspended` after saving a checkpoint when a `StateStore` is
configured.

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

```bash
agent history --state-store-dir .agent-state --checkpoint latest
```

Pick the last known-good iteration from the digest. Each row includes iteration,
command name, state transition, signal, undo memento status, and optional
workspace ref.

Create a rollback checkpoint:

```bash
agent rollback \
  --state-store-dir .agent-state \
  --checkpoint latest \
  --to-iteration 7 \
  --directory "$PWD"
```

Resume from the rollback checkpoint printed by the command:

```bash
agent \
  --machine <machine.yaml> \
  --tools <tools.yaml> \
  --tools-declaration tools/builtin.yaml \
  --state-store-dir .agent-state \
  --resume-checkpoint <rollback-checkpoint-id> \
  --resume-signal Approved \
  --directory "$PWD"
```

The resume path validates the current machine and tool declarations before
re-entering the loop. If the current config is incompatible with the checkpoint,
resume refuses to continue.

## Undo Mementos

Checkpoint history stores `HistoryDigest` rows, not live `Command` objects.
Commands that need persisted rollback implement `UndoMementoProvider`; the loop
captures that versioned JSON memento after command execution and stores it in
the history digest.

Memento kinds tell rollback what is possible:

- `noop` means the command has no rollback-managed state.
- `reversible` carries enough JSON to restore command/domain state or identify
  workspace paths for restore.
- `compensatable` carries the metadata needed for child command undo, workspace
  restore, or explicit operator/API compensation.
- `irreversible` records why rollback cannot safely undo the effect.

Current reversible mementos cover conversation/retry state, planner pipeline
state, file/workspace paths, evaluator session state, point context, and
validation state. Boundary tools such as `execute_task`, `self_invoke`,
`run_point`, `run_agent`, `launch_eval`, `serve_ui`, `suspend`, and issue tools
record `boundary_compensation` payloads with child run IDs, artifact paths,
issue IDs, server/user-action details, or nested-machine context.

## Operational Safety

Rollback is not a time machine for every side effect. It is a coordinated
best-effort restore across the three state layers.

Use `GitWorkspace` only on a workspace that can tolerate reset-style restore.
Rollback and resume restore workspace refs through git. Do not point
`--directory` at an unmanaged directory, a shared checkout with unrelated
changes, or a repository where destructive restore would surprise another
process.

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
resuming. A partial rollback can mean command/domain state, workspace state, or
checkpoint persistence did not fully restore.

## Related Documents

- `docs/specs/software-requirements/srd025-rollback-lifecycle.yaml`
- `docs/specs/semantic-models/rollback-lifecycle.yaml`
- `docs/specs/semantic-models/command-undo-audit.yaml`
- `docs/specs/config-formats/runtime-contract.yaml`
- `docs/specs/config-formats/tool-authoring-standard.yaml`
- `docs/specs/use-cases/rel02.0-uc001-approval-suspend-resume.yaml`
- `docs/specs/use-cases/rel02.0-uc002-history-rollback.yaml`
- `roll-backs.md`
- `tools-as-dsl.md`
