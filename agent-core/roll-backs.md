# Externalized State, Checkpointing, and Agent Lifecycle

> Status: historical design note.
>
> This note was the design input for the official rollback/lifecycle
> specification set. Do not treat it as the source of truth for requirements.
> Future changes should update the canonical documents below and use this file
> only for background rationale.

## Canonical Documents

- Requirements: `docs/specs/software-requirements/srd025-rollback-lifecycle.yaml`
- Semantic model: `docs/specs/semantic-models/rollback-lifecycle.yaml`
- Command undo audit: `docs/specs/semantic-models/command-undo-audit.yaml`
- Approval/suspend/resume use case: `docs/specs/use-cases/rel02.0-uc001-approval-suspend-resume.yaml`
- History/rollback use case: `docs/specs/use-cases/rel02.0-uc002-history-rollback.yaml`
- Lifecycle formal tests: `docs/specs/test-suites/test-rel02.0-lifecycle.yaml`
- Tool contract formal tests that cover rollback metadata: `docs/specs/test-suites/test-rel02.0-tool-contracts.yaml`

## Coverage Map

- Three independent state layers are canonicalized in `srd025` R1 and
  `sm-rollback-lifecycle.state_layers`.
- `StateStore`, `FileStore`, `Workspace`, and `GitWorkspace` are canonicalized
  in `srd025` R2/R3 and the lifecycle formal tests.
- `Command.Undo`, undo categories, and command-owned state restoration are
  canonicalized in `srd025` R4, `sm-command-undo-audit`, and
  `test-rel02.0-lifecycle`.
- `HistoryEntry`, rollback traversal, partial rollback failure handling, and
  target iteration semantics are canonicalized in `srd025` R5/R7 and
  `rel02.0-uc002-history-rollback`.
- `Checkpoint`, `CheckpointPolicy`, suspend, resume, approval gates, feature
  flags, and zero-overhead disabled behavior are canonicalized in `srd025`
  R6/R8/R9/R10 and `rel02.0-uc001-approval-suspend-resume`.

---

## Three concepts

1. **Externalized state** -- the agent's internal state and the environment's
   state are independently trackable and serializable.
2. **Checkpointing with forward/backward traversal** -- the engine records
   the state machine's evolution. You can walk forward (replay) or backward
   (rollback) through checkpoints.
3. **Suspend and resume** -- the agent serializes, exits, and restarts from
   a checkpoint. Enables approval gates, user backtracking, and long-running
   workflows.

All opt-in via feature flag. Zero overhead when disabled.

---

## State model: three independent layers

There are three distinct layers of state. They are independent and tracked by
different mechanisms:

### Layer 1: Agent state

The state machine position, budget counters, iteration count, run result
accumulator. This is the engine's own bookkeeping.

Tracked by: the loop itself. Serializable to JSON.

### Layer 2: Command state (internal)

Each command has internal state. `invoke_llm` mutates the conversation history.
`reset_history` clears it. `extract_task` mutates the pipeline graph.
`parse_response` is stateless. Each command knows what it changed.

Tracked by: the commands themselves. Each command records what it needs to
undo (conversation length, saved messages, graph snapshot, etc.).

### Layer 3: Environment state (external)

The workspace filesystem. Changed by `write`, `edit`, `copy_dir`, `make_dir`,
and any `exec` tool that produces side effects. This is external to the agent
process.

Tracked by: git. The workspace is a git repository. File mutations can be
checkpointed with git commits.

**Key insight**: git tracks the environment, not the agent. Agent internal
state (conversation, state machine position) should NOT be stored in git.
Git is the wrong tool for serializing structured in-memory data. Agent state
is JSON. Environment state is a filesystem tree, which is exactly what git
is designed for.

---

## The StateStore interface

The `StateStore` is used by **commands**, not by the agent loop directly.
Commands that need to persist state (checkpoints, conversation snapshots,
domain state) write through this interface. The implementation determines
where data lives.

```go
// StateStore persists and retrieves agent state. Commands use this to
// save checkpoints and restore state. The implementation determines
// the backing store (filesystem, DynamoDB, etc.).
type StateStore interface {
    Save(ctx context.Context, key string, data []byte) error
    Load(ctx context.Context, key string) ([]byte, error)
    List(ctx context.Context, prefix string) ([]string, error)
    Delete(ctx context.Context, key string) error
}
```

This is deliberately simple -- it's a key-value store. Checkpoints, agent
state, domain state are all JSON blobs stored under structured keys. The
interface can be implemented by:

- **FileStore**: JSON files in a directory (default, simplest)
- **DynamoStore**: DynamoDB table
- **S3Store**: S3 bucket
- **GitBlobStore**: git blob objects (for co-locating with workspace state)

The `StateStore` does NOT track the environment. That's a separate concern.

---

## The Workspace interface

The environment is tracked separately from agent state. The `Workspace`
interface captures the filesystem and provides checkpoint/restore operations:

```go
// Workspace tracks the state of the agent's working environment
// (filesystem). It provides checkpoint and restore operations for
// rollback support.
type Workspace interface {
    Checkpoint(ctx context.Context, label string) (ref string, err error)
    Restore(ctx context.Context, ref string) error
    CurrentRef(ctx context.Context) (string, error)
}
```

The default implementation is `GitWorkspace`:

```go
// GitWorkspace uses git to checkpoint and restore the filesystem.
type GitWorkspace struct {
    dir string
}

func (g *GitWorkspace) Checkpoint(ctx context.Context, label string) (string, error) {
    // git add -A && git commit --allow-empty -m "checkpoint: <label>"
    // return the commit SHA
}

func (g *GitWorkspace) Restore(ctx context.Context, ref string) error {
    // git reset --hard <ref>
}

func (g *GitWorkspace) CurrentRef(ctx context.Context) (string, error) {
    // git rev-parse HEAD
}
```

If you want checkpointing, you set up your workspace as a git repo. The
evaluator already does this -- `prepare_workspace` copies a sample directory,
runs `git init`, and creates a baseline commit. The same pattern applies.

In the future, `Workspace` could be implemented by a tarball snapshotter,
an overlayfs, a container image layer, etc.

---

## How commands interact with these interfaces

Commands receive `StateStore` and `Workspace` through their builders, just
like they already receive the conversation, tracer, and registry. The command
decides what to do:

### File-mutating commands know about the workspace

`write`, `edit`, `copy_dir` receive the `Workspace`. After successfully
mutating a file, they can tell the workspace to checkpoint:

```go
type writeCmd struct {
    root      string
    path      string
    content   string
    workspace Workspace  // nil when checkpointing is disabled
}

func (w *writeCmd) Execute() core.Result {
    // ... existing write logic ...

    // If workspace tracking is enabled, capture the ref after writing.
    if w.workspace != nil {
        ref, _ := w.workspace.Checkpoint(ctx, "write:"+w.path)
        w.workspaceRef = ref
    }

    return result
}
```

### LLM commands know about their own state

`invoke_llm` captures conversation length before execution. It doesn't need
the workspace or state store -- it only needs the conversation reference it
already has:

```go
func (c *invokeLLMCmd) Execute() core.Result {
    c.prevConvLen = c.history.Len()
    // ... existing logic ...
}

func (c *invokeLLMCmd) Undo() core.Result {
    c.history.TruncateTo(c.prevConvLen)
    return core.Result{Signal: core.ToolDone}
}
```

### Stateless commands do nothing

`read`, `find`, `build`, `test`, `parse_response` etc. don't receive
workspace or state store references. Their undo is a no-op.

---

## Environment checkpointing: commands own their workspace

File-mutating commands receive a `Workspace` reference through their builder
(the same pattern used for tracer, conversation, and registry today). After
successfully mutating a file, the command calls `Checkpoint`:

```go
func (w *writeCmd) Execute() core.Result {
    // ... write the file ...
    if w.workspace != nil {
        w.workspaceRef, _ = w.workspace.Checkpoint(ctx, "write:"+w.path)
    }
    return result
}
```

The workspace reference is stored in the command's own state, so the
HistoryEntry captures it. On rollback, the engine restores to the target
entry's workspace ref.

Why this approach:

1. **It's explicit** -- you can see which commands checkpoint by looking at
   their code. No magic in the loop.
2. **The command knows the ref** -- the workspace reference is part of the
   command's internal state, which the HistoryEntry captures. On rollback,
   the command knows exactly which workspace ref to restore to.
3. **It composes with undo** -- the command's `Undo` can call
   `Workspace.Restore(w.workspaceRef)` if the command itself needs to undo
   its environment changes, or defer to a bulk workspace rollback.
4. **It's opt-in per command** -- commands that don't receive a `Workspace`
   have zero overhead. You can add checkpointing to existing tools one at a
   time without changing the state machine or other tools.
5. **No state machine changes** -- checkpointing is a storage concern, not a
   workflow concern. The state machine stays focused on control flow.

A convenience helper keeps the boilerplate minimal:

```go
// CheckpointAfter wraps a command's Execute result with a workspace
// checkpoint. Used by file-mutating commands.
func CheckpointAfter(ws Workspace, label string, res core.Result) core.Result {
    if ws == nil || res.Signal == core.ToolFailed || res.Signal == core.CommandError {
        return res
    }
    ws.Checkpoint(context.Background(), label)
    return res
}
```

---

## Every Command has Undo

The `Command` interface gains an `Undo` method. Every command implements it,
even if it's a no-op:

```go
type Command interface {
    Name() string
    Execute() Result
    Undo() Result
}
```

### What Undo does per category

| Command | What Execute mutates | What Undo does |
|---|---|---|
| `invoke_llm` | Conversation: appends user + assistant messages | `conversation.TruncateTo(prevLen)` |
| `reset_history` | Conversation: clears all messages | Restores saved message list |
| `nudge_reread` | Conversation: appends a nudge message | `conversation.TruncateTo(prevLen)` |
| `parse_response` | Nothing | No-op |
| `report_parse_error` | Nothing | No-op |
| `write` | Filesystem: creates/overwrites file | `workspace.Restore(prevRef)` or no-op if deferred |
| `edit` | Filesystem: modifies file | `workspace.Restore(prevRef)` or no-op if deferred |
| `copy_dir` | Filesystem: copies directory | `workspace.Restore(prevRef)` or no-op if deferred |
| `make_dir` | Filesystem: creates directories | `workspace.Restore(prevRef)` or no-op if deferred |
| `read` | Nothing | No-op |
| `find` | Nothing | No-op |
| `list_files` | Nothing | No-op |
| `build` / `test` / `vet` / `lint` | Nothing persistent | No-op |
| `validate` | Nothing persistent | No-op |
| `done` | Nothing | No-op |
| `extract_task` | Pipeline graph state | Restores graph snapshot |
| `execute_task` | Spawns sub-agent | Roll back sub-agent workspace |
| `check_result` | Pipeline retry counter | Restores counter |

---

## The execution recording

The loop records every step as a `HistoryEntry`. This IS the recording of how
the state machine evolved:

```go
type HistoryEntry struct {
    Iteration       int
    Timestamp       time.Time
    FromState       State
    ToState         State
    Signal          Signal
    Command         Command
    Result          Result
    ConversationLen int     // conversation depth before this step
    WorkspaceRef    string  // workspace ref after this step (empty if no mutation)
}

type History struct {
    entries []HistoryEntry
}
```

The history is recorded in `coreLoop`. After every `Dispatch`, a new entry
is pushed. When checkpointing is enabled, the entry also captures the
workspace ref.

### Rollback

Rollback walks the recording backward:

1. Pop entries from the history in reverse order.
2. Call `Undo()` on each command (reverses internal state: conversation,
   pipeline graph, etc.).
3. Restore the workspace to the ref recorded in the target entry (reverses
   external state: filesystem).

```go
func Rollback(h *History, targetIteration int, ws Workspace,
    tr tracing.Tracer) (State, error) {

    // Phase 1: undo commands in reverse (internal state).
    for h.Len() > targetIteration {
        entry, _ := h.Pop()
        entry.Command.Undo()
    }

    // Phase 2: restore workspace (external state).
    if ws != nil && targetIteration > 0 {
        target := h.Entry(targetIteration - 1)
        if target.WorkspaceRef != "" {
            ws.Restore(ctx, target.WorkspaceRef)
        }
    } else if ws != nil && targetIteration == 0 {
        // Full rollback: restore to initial workspace state.
        ws.Restore(ctx, initialRef)
    }

    if targetIteration == 0 {
        return State("Idle"), nil
    }
    return h.Entry(targetIteration - 1).ToState, nil
}
```

---

## Suspend and resume

### Checkpoint structure

A checkpoint captures all three layers of state:

```go
type Checkpoint struct {
    ID              string          `json:"id"`
    Iteration       int             `json:"iteration"`
    Timestamp       time.Time       `json:"timestamp"`
    AgentState      AgentSnapshot   `json:"agent_state"`
    ConversationLog []llm.Message   `json:"conversation"`
    DomainState     json.RawMessage `json:"domain_state,omitempty"`
    WorkspaceRef    string          `json:"workspace_ref"`
    HistoryDigest   []HistoryDigest `json:"history"`
}

type AgentSnapshot struct {
    State       State     `json:"state"`
    Signal      Signal    `json:"signal"`
    Iteration   int       `json:"iteration"`
    TokensIn    int       `json:"tokens_in"`
    TokensOut   int       `json:"tokens_out"`
    TotalCost   float64   `json:"total_cost"`
}

type HistoryDigest struct {
    Iteration    int    `json:"iteration"`
    CommandName  string `json:"command_name"`
    FromState    string `json:"from_state"`
    ToState      string `json:"to_state"`
    Signal       string `json:"signal"`
    WorkspaceRef string `json:"workspace_ref,omitempty"`
}
```

The `StateStore` persists the checkpoint. The `Workspace` holds the
filesystem state at `WorkspaceRef`. These are independent stores.

### Suspend

When the loop encounters `SigAwaitApproval`, it saves a checkpoint and exits:

```go
if p.StateStore != nil && sig == SigAwaitApproval {
    cp := buildCheckpoint(iteration, state, conv, rr, domainState, ws)
    p.StateStore.Save(ctx, "checkpoint/"+cp.ID, marshal(cp))
    rr.Status = StatusSuspended
    break
}
```

### Resume

`agent resume --checkpoint <id>` loads the checkpoint, restores all three
layers, and re-enters the loop:

```go
func Resume(store StateStore, ws Workspace, id string, params LoopParams) (RunResult, error) {
    data, _ := store.Load(ctx, "checkpoint/"+id)
    var cp Checkpoint
    json.Unmarshal(data, &cp)

    // Restore agent state.
    params.InitialState = cp.AgentState.State

    // Restore conversation (layer 2).
    for _, msg := range cp.ConversationLog {
        params.Conversation.Append(msg)
    }

    // Restore domain state (layer 2, domain-specific).
    if params.RestoreDomainState != nil {
        params.RestoreDomainState(cp.DomainState)
    }

    // Restore workspace (layer 3).
    if ws != nil && cp.WorkspaceRef != "" {
        ws.Restore(ctx, cp.WorkspaceRef)
    }

    return Loop(params, ctx)
}
```

### Approval gates

```yaml
signals:
  - AwaitApproval
  - Approved
  - Rejected

transitions:
  - state: Parsing
    signal: TaskCompleted
    next: AwaitingApproval
    action: suspend

  - state: AwaitingApproval
    signal: Approved
    next: Executing
    action: execute_task

  - state: AwaitingApproval
    signal: Rejected
    next: Rejected
```

### User backtracking

```bash
# Show what the agent did:
agent history --checkpoint latest

# Roll back to before the agent's 5th action:
agent rollback --to-iteration 4

# Resume from there:
agent resume --latest
```

---

## Feature flag and runtime requirements

```go
type LoopParams struct {
    // ... existing fields ...

    // StateStore enables agent state persistence. Commands use this
    // to save checkpoints. When nil, no persistence (current behavior).
    StateStore StateStore

    // Workspace enables environment checkpointing. File-mutating
    // commands use this to snapshot the filesystem. When nil, no
    // environment tracking.
    Workspace Workspace

    // CheckpointPolicy controls when checkpoints are taken.
    CheckpointPolicy CheckpointPolicy
}
```

**If you want environment checkpointing**: set up your workspace as a git
repo. The evaluator already does this.

**If you want agent state persistence**: provide a `StateStore` (FileStore
is the default, just needs a directory).

**If you want neither**: leave both nil. The agent runs exactly as today.

---

## Implementation plan

### Phase 1: Interfaces

1. Define `StateStore` interface in `pkg/core/statestore.go`.
2. Define `Workspace` interface in `pkg/core/workspace.go`.
3. Define `CheckpointPolicy` and `Checkpoint` types.
4. Implement `FileStore` (JSON files in a directory).
5. Implement `GitWorkspace` (wrapping existing git tool helpers).
6. Add `StateStore`, `Workspace`, `CheckpointPolicy` to `LoopParams`.
7. No behavior change -- all nil by default.

### Phase 2: Command Undo

1. Add `Undo() Result` to the `Command` interface.
2. Add no-op `Undo` to every existing command.
3. Add `TruncateTo(n int)` to `llm.Conversation`.
4. Implement real undo for `invokeLLMCmd`, `resetHistoryCmd`, `nudgeRereadCmd`.

### Phase 3: History recording

1. Define `HistoryEntry` and `History` in `pkg/core/history.go`.
2. Wire recording into `coreLoop` (after every Dispatch, push entry).
3. File-mutating commands receive `Workspace`, call `Checkpoint` in Execute.
4. Implement `Rollback()` function.

### Phase 4: Suspend and resume

1. Add `StatusSuspended`, `SigAwaitApproval`, `SigApproved`, `SigRejected`.
2. Implement `suspend` builtin tool.
3. Wire suspend detection into `coreLoop`.
4. Implement `agent resume` subcommand.
5. Implement `agent rollback` and `agent history` subcommands.

### Phase 5: Future stores

1. `DynamoStore` implementing `StateStore`.
2. Non-git `Workspace` implementations (tarball, overlayfs, etc.).
