# Asynchronous Boundary Tools

*Decoupling dispatch from receive for boundary words, with configurable UI
and LLM tools as the primary applications.*

---

## Problem

The current boundary tools (`invoke_llm`, `serve_ui`) are synchronous: they
send a request and block until the response arrives. This works for simple
agents with one boundary at a time, but prevents:

- **Multiple UIs active simultaneously** — a dashboard and a monitor panel
  can't both be running if the first one blocks the grammar.
- **Parallel LLM + UI** — can't serve a UI while an LLM request is in
  flight.
- **Work between send and receive** — can't prepare context or do cleanup
  while the LLM thinks.

Additionally, `serve_ui` is tightly coupled to the bench experiment viewer:
assets are embedded at compile time, action-to-signal mapping is hardcoded
in Go, and API routes are bench-specific. To serve a different UI you'd
need new Go code.

This document designs:
1. **The async boundary pattern** — splitting launch from await for any
   boundary word (model, human, agent).
2. **A configurable UI tool** — one Go implementation serving any web
   interface via YAML configuration.
3. **A configurable async LLM tool** — the same split applied to model
   boundaries.

## Design Goals

### Async boundary pattern

1. **Split dispatch from receive.** Every boundary word (model, human,
   agent) can be expressed as a non-blocking launch word paired with a
   blocking await word. The grammar controls when to wait and what to
   wait for.

2. **Multiplexed receive.** A single await word can block on input from
   any active boundary — multiple UIs, an LLM response, or a mix. The
   inbox is a fan-in channel all active boundaries write to.

3. **Backward compatible.** The fused blocking form (`invoke_llm`,
   `serve_ui`) remains valid. It is simply a launch + await fused into
   one `Execute()`. Simple agents that need one boundary at a time
   don't change.

### Configurable UI tool

4. **One Go implementation, many UI tools.** The same `launch_ui` /
   `serve_ui` init serves a bench experiment viewer, an approval gate, a
   monitoring dashboard, or any future interface — selected by YAML.

5. **Multiple UI tools per agent.** One agent can have several configured
   UI boundary words (e.g., `launch_dashboard`, `launch_approval`). The
   grammar dispatches them at different points. Each serves different
   content and maps different user actions to different signals.

6. **Assets are external.** HTML/JS/CSS are loaded from a filesystem path
   at runtime, not embedded in the binary. Different tool configurations
   point to different asset directories.

7. **Action-to-signal mapping is declared.** The YAML says which user
   actions this UI can emit and what grammar signal each maps to. The Go
   code performs the mapping without knowing the domain.

8. **Server lifecycle is shared.** One HTTP server runs for the agent's
   lifetime. Tools register views when launched and remain active until
   explicitly torn down. No server restart between dispatches.

9. **Data providers are pluggable.** Domain-specific API routes (experiment
   results, configs, documents) are configured as "data mounts" — directory
   paths exposed at URL prefixes.

### Configurable async LLM tool

10. **Non-blocking LLM send.** A `send_llm` word fires the request and
    returns immediately. The grammar can do other work while the model
    thinks.

11. **Same inbox pattern.** The LLM response handler writes to the shared
    inbox. `await_llm` (or a generic `await_input`) reads from it.

### HTTP call tool for agent-to-agent control

12. **Go-native REST client.** A generic `http_call` word implementation
    uses `net/http` directly — no shelling out to curl. Configured via YAML
    (method, URL, headers, body, timeout). Enables idiomatic agent-to-agent
    communication over REST.

13. **REST is the control plane.** Approvals, rollback, pause, signal
    injection — all exposed as REST endpoints. Works headlessly (curl, CI,
    another agent) without any UI. The UI is an optional frontend consumer
    of the same endpoints.

## UI Tool Architecture

```
┌───────────────────────────────────────────────────────────────┐
│ Agent Process                                                  │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ UIServer (shared resource, one per agent)                │  │
│  │                                                         │  │
│  │  port: 8080                                             │  │
│  │  ┌───────────────────────────────────────────────────┐  │  │
│  │  │ Active View (set by currently dispatched tool)    │  │  │
│  │  │  - assets path     → static file handler          │  │  │
│  │  │  - data mounts     → read-only JSON endpoints     │  │  │
│  │  │  - allowed actions → validated on POST            │  │  │
│  │  │  - action channel  → blocks tool's Execute        │  │  │
│  │  └───────────────────────────────────────────────────┘  │  │
│  │                                                         │  │
│  │  Standard endpoints:                                    │  │
│  │    GET  /                → SPA (active view's assets)   │  │
│  │    GET  /api/state      → current view name + context   │  │
│  │    GET  /api/actions    → declared available actions     │  │
│  │    POST /api/actions    → submit action (→ signal)      │  │
│  │    GET  /data/{mount}/* → data provider content         │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ Grammar dispatches:                                     │  │
│  │                                                         │  │
│  │  serve_dashboard.Execute()                              │  │
│  │    → sets active view to "dashboard"                    │  │
│  │    → mounts dashboard assets + data                     │  │
│  │    → blocks on action channel                           │  │
│  │    → user clicks "launch" → LaunchRequested signal      │  │
│  │    → returns to grammar                                 │  │
│  │                                                         │  │
│  │  ... grammar processes action ...                       │  │
│  │                                                         │  │
│  │  serve_approval.Execute()                               │  │
│  │    → sets active view to "approval"                     │  │
│  │    → mounts approval assets + data                      │  │
│  │    → blocks on action channel                           │  │
│  │    → user clicks "approve" → Approved signal            │  │
│  │    → returns to grammar                                 │  │
│  └─────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────┘
```

## Tool Declaration Schema

Each configured UI tool is a separate entry in the tool declarations YAML:

```yaml
tools:
  - name: serve_dashboard
    type: builtin
    category: boundary
    boundary_actor: human
    init: serve_ui
    emits: [LaunchRequested, CompareRequested, Shutdown]
    description: "Serve the experiment dashboard and wait for user action."

    parameters:
      type: object
      properties: {}

    output:
      description: "The user action payload as JSON."
      schema:
        type: object
        properties:
          action: { type: string }
          payload: { type: object }

    config:
      addr: ":8080"

      assets: ./ui/dashboard/dist

      actions:
        - name: launch_experiment
          signal: LaunchRequested
          description: "User requests a new evaluation run."
          payload_schema:
            type: object
            properties:
              suite: { type: string }
              models: { type: array, items: { type: string } }
            required: [suite]

        - name: compare_models
          signal: CompareRequested
          description: "User requests a model comparison view."
          payload_schema:
            type: object
            properties:
              session_ids: { type: array, items: { type: string } }

        - name: shutdown
          signal: Shutdown
          description: "User requests graceful shutdown."

      data_mounts:
        - prefix: sessions
          source: ./eval-results
          type: directory_listing
        - prefix: configs
          source: ./agents
          type: directory_listing
        - prefix: profiles
          source: ./pkg/llm/profiles
          type: directory_listing

    side_effects:
      - kind: network_listen
        target: configured_port
        description: "Starts HTTP server on configured address (once per agent lifetime)."
      - kind: blocking_input
        target: human_actor
        description: "Blocks until user submits an action via the web UI."

    reversibility:
      classification: reversible
      undo: noop

    undo:
      strategy: noop
      description: "UI interaction has no persistent side effects to reverse."

  - name: serve_approval
    type: builtin
    category: boundary
    boundary_actor: human
    init: serve_ui
    emits: [Approved, Rejected, Deferred]
    description: "Present an approval gate UI and wait for human decision."

    config:
      addr: ":8080"

      assets: ./ui/approval/dist

      actions:
        - name: approve
          signal: Approved
          description: "User approves the pending action."
        - name: reject
          signal: Rejected
          description: "User rejects the pending action."
        - name: defer
          signal: Deferred
          description: "User defers the decision."

      context_providers:
        - name: pending_action
          source: previous_result
          description: "Shows what action is pending approval."
```

## UI Tool Design Decisions

### 1. Shared server, multiple active views

One HTTP server per address runs for the agent's lifetime. UI tools register
views when launched and remain active until explicitly torn down. Multiple
views can be active simultaneously on the same server (each at its own route
prefix) or on different ports.

In the **simple blocking mode**, one tool claims the server as the active
view, blocks until an action arrives, and releases it when it returns.

In the **split launch/await mode**, multiple launch words register
concurrent views, and a single await word multiplexes across all of them.
The server exposes `GET /api/state` so frontends know which views are active
and what actions are available.

### 2. Assets from filesystem, not embed

Assets are loaded from a path in the tool's `config.assets`. This means:

- Different tool configs point to different built frontends
- You can rebuild a UI without recompiling the Go binary
- Development mode serves from the local filesystem
- Production can use a pre-built `dist/` directory alongside the binary
- The same binary serves completely different UIs by changing YAML

The Go code still supports `fs.FS` injection for cases where embedding is
desired (testing, single-binary distribution). The tool checks:
1. If an `fs.FS` was injected at registration time, use it
2. Otherwise, `os.DirFS(config.assets)`

### 3. Action-to-signal mapping from YAML

The Go code does not contain a switch statement mapping action types to
signals. Instead:

```go
func (c *serveUICmd) mapAction(action UserAction) core.Signal {
    for _, declared := range c.config.Actions {
        if declared.Name == action.Type {
            return core.Signal(declared.Signal)
        }
    }
    return core.CommandError
}
```

This means new actions and signals require only YAML changes.

### 4. Data mounts for domain content

The current bench has hardcoded routes like `/api/v1/sessions`. The generic
version replaces these with configurable data mounts:

```yaml
data_mounts:
  - prefix: sessions
    source: ./eval-results
    type: directory_listing
  - prefix: configs
    source: ./agents
    type: directory_listing
```

Each mount exposes a directory as a read-only JSON API:
- `GET /data/{prefix}` → list entries
- `GET /data/{prefix}/{path...}` → read file content

The frontend knows its data mounts from `GET /api/state` and fetches
accordingly. Domain-specific data shaping (if needed) is a future extension
via transform plugins.

### 5. Context injection from previous result

A UI tool often needs to show context from the grammar's execution — e.g.,
an approval gate needs to display what action is pending. The
`context_providers` config tells the tool where to source this:

- `source: previous_result` — inject the previous word's `Result.Output`
  into the UI's initial state
- `source: file:<path>` — inject file contents
- `source: env:<var>` — inject an environment variable

The context is served via `GET /api/state` alongside the view name and
available actions.

### 6. Multiple ports are supported but not required

By default, all UI tools sharing the same `config.addr` share one server.
If a tool declares a different `addr`, it gets its own server instance. This
handles edge cases where you need a public-facing UI on one port and an
admin UI on another.

## Concurrency Model: Split Dispatch and Receive

### The problem with blocking boundary words

The initial design has each UI tool blocking in `Execute()` until a user
action arrives. This works when the grammar dispatches one boundary word at
a time. But it breaks when:

- An agent needs **multiple UIs active simultaneously** (a dashboard and a
  monitor panel).
- An agent needs a **UI and an LLM request in flight** at the same time.
- An agent wants to **do work between sending a request and receiving the
  response** (e.g., prepare context while the LLM thinks).

If `serve_dashboard` blocks, `serve_monitor` never launches. The state
machine is single-threaded — a blocking word holds the entire grammar.

### Solution: separate launch from await

Split every boundary word into two phases:

| Phase | Word type | Behavior |
|---|---|---|
| **Launch** | Non-blocking | Starts the boundary (server, LLM request), returns immediately with a `Launched`/`Sent` signal |
| **Await** | Blocking | Waits for input from any active boundary, returns the first signal that arrives |

This is the async/await pattern expressed as grammar words.

### How it works

```
State Machine Thread                Background Goroutines
────────────────────                ─────────────────────
launch_dashboard.Execute()
  → starts dashboard server
  → registers on shared inbox
  → returns Signal: Launched
                                    dashboard server running...

launch_monitor.Execute()
  → starts monitor server
  → registers on shared inbox
  → returns Signal: Launched
                                    monitor server running...

await_input.Execute()
  → blocks on inbox channel         user clicks dashboard action
       (waiting)                      → handler writes to inbox
       (unblocks) ←────────────────
  → returns Signal from dashboard
                                    both servers still running...

grammar processes action...

await_input.Execute()
  → blocks on inbox channel         user acknowledges alert on monitor
       (waiting)                      → handler writes to inbox
       (unblocks) ←────────────────
  → returns Signal from monitor
```

### The Inbox: multiplexed signal reception

All active boundary words write to a shared inbox. The `await_input` word
reads from it:

```go
type Inbox struct {
    ch chan InboundMessage
    mu sync.Mutex
}

type InboundMessage struct {
    Source  string          // which boundary produced this ("dashboard", "llm", "monitor")
    Signal  core.Signal     // the grammar signal
    Payload json.RawMessage // action-specific data
}
```

Launch words register themselves with the inbox and set up their background
goroutine to write to `inbox.ch` when input arrives. Await words block on
`inbox.ch` and return whatever arrives first.

### Grammar patterns this enables

**Multiple concurrent UIs:**

```yaml
transitions:
  - state: Idle
    signal: Seed
    next: LaunchingDashboard
    action: launch_dashboard

  - state: LaunchingDashboard
    signal: Launched
    next: LaunchingMonitor
    action: launch_monitor

  - state: LaunchingMonitor
    signal: Launched
    next: Waiting
    action: await_input

  - state: Waiting
    signal: LaunchRequested
    next: Launching
    action: launch_eval

  - state: Waiting
    signal: AlertAcknowledged
    next: Waiting
    action: await_input

  - state: Launching
    signal: EvalCompleted
    next: Waiting
    action: await_input
```

**LLM with parallel work:**

```yaml
transitions:
  - state: Ready
    signal: ToolDone
    next: LLMInFlight
    action: send_llm

  - state: LLMInFlight
    signal: Sent
    next: PreparingContext
    action: prepare_next_context

  - state: PreparingContext
    signal: ToolDone
    next: AwaitingLLM
    action: await_llm

  - state: AwaitingLLM
    signal: LLMResponded
    next: Parsing
    action: parse_response
```

**Mixed boundaries (UI + LLM concurrently):**

```yaml
transitions:
  - state: Serving
    signal: UserAsked
    next: LLMInFlight
    action: send_llm

  - state: LLMInFlight
    signal: Sent
    next: WaitingForEither
    action: await_input

  # LLM responds first — process it, keep UI active
  - state: WaitingForEither
    signal: LLMResponded
    next: Presenting
    action: push_to_ui

  # User acts before LLM finishes — handle it
  - state: WaitingForEither
    signal: UserAction
    next: Processing
    action: process_action
```

### Backward compatibility with blocking mode

The existing `invoke_llm` (send + block + return) remains valid. A blocking
boundary word is simply a launch + await fused into one `Execute()`. The
grammar doesn't know the difference — it dispatches the word and gets a
signal back.

The split is opt-in at the grammar level: if your agent only needs one
boundary at a time, use the simple blocking form. If you need concurrency,
use launch + await.

```yaml
# Simple (blocking) — current behavior:
- state: Composing
  signal: ToolDone
  next: Composing
  action: invoke_llm          # blocks until response

# Split (non-blocking) — new capability:
- state: Composing
  signal: ToolDone
  next: LLMInFlight
  action: send_llm            # returns immediately
- state: LLMInFlight
  signal: Sent
  next: AwaitingLLM
  action: await_llm           # blocks until response
```

Both produce the same result. The split version just lets the grammar insert
work between send and receive.

### Implementation: launch word

```go
type launchUICmd struct {
    config ServeUIConfig
    state  *UIServerState
    inbox  *Inbox
}

func (c *launchUICmd) Execute() core.Result {
    srv := c.state.EnsureServer(c.config.Addr)

    assets := resolveAssets(c.config.Assets)
    srv.AddView(c.config.Name, &viewConfig{
        assets:  assets,
        actions: c.config.Actions,
        mounts:  c.config.DataMounts,
    })

    // Wire: HTTP action handler → inbox
    srv.OnAction(c.config.Name, func(action ActionMessage) {
        signal := mapAction(action, c.config.Actions)
        c.inbox.ch <- InboundMessage{
            Source:  c.config.Name,
            Signal:  signal,
            Payload: action.Payload,
        }
    })

    return core.Result{
        Signal:      core.Signal("Launched"),
        Output:      fmt.Sprintf("UI %s active on %s", c.config.Name, c.config.Addr),
        CommandName: c.config.Name,
    }
}
```

### Implementation: await word

```go
type awaitInputCmd struct {
    inbox *Inbox
}

func (c *awaitInputCmd) Execute() core.Result {
    msg := <-c.inbox.ch

    return core.Result{
        Signal:      msg.Signal,
        Output:      string(msg.Payload),
        CommandName: "await_input",
        ToolMetrics: core.ToolMetrics{
            ExtraFields: map[string]interface{}{
                "source": msg.Source,
            },
        },
    }
}
```

### Tool declarations for launch/await

```yaml
tools:
  - name: launch_dashboard
    type: builtin
    category: boundary
    boundary_actor: human
    init: launch_ui
    emits: [Launched]
    description: "Start the dashboard UI server (non-blocking)."
    config:
      addr: ":8080"
      assets: ./ui/dashboard/dist
      actions:
        - name: launch_experiment
          signal: LaunchRequested
        - name: shutdown
          signal: Shutdown

  - name: launch_monitor
    type: builtin
    category: boundary
    boundary_actor: human
    init: launch_ui
    emits: [Launched]
    description: "Start the monitoring panel (non-blocking)."
    config:
      addr: ":8081"
      assets: ./ui/monitor/dist
      actions:
        - name: acknowledge_alert
          signal: AlertAcknowledged

  - name: await_input
    type: builtin
    category: boundary
    boundary_actor: human
    init: await_input
    emits: [LaunchRequested, Shutdown, AlertAcknowledged]
    description: "Block until any active UI boundary sends an action."
```

Note that `await_input` declares ALL signals that any active boundary might
emit. The grammar must handle all of them in the state where `await_input`
is dispatched.

### The same principle applies to LLM tools

```yaml
tools:
  - name: send_llm
    type: builtin
    category: boundary
    boundary_actor: model
    init: send_llm
    emits: [Sent]
    description: "Send prompt to LLM (non-blocking). Use await_llm to get response."
    config:
      model: granite
      profile: default

  - name: await_llm
    type: builtin
    category: boundary
    boundary_actor: model
    init: await_response
    emits: [LLMResponded, CommandError]
    description: "Block until pending LLM response arrives."
```

The `send_llm` word fires the request and returns `Sent`. The grammar can
do other work. When ready, `await_llm` blocks until the response arrives.
This is the same inbox pattern — the LLM response handler writes to the
inbox, and `await_llm` (or `await_input` if multiplexed) reads from it.

### The http_call tool: Go-native REST client

A generic word for making HTTP requests, configured entirely via YAML.
Uses `net/http` internally — no subprocess, no curl dependency.

```yaml
tools:
  - name: http_call
    type: builtin
    category: word
    init: http_call
    emits: [ToolDone, ToolFailed]
    description: "Make an HTTP request. Method, URL, headers, body, and timeout from config."
    config:
      method: GET
      url: "http://example.com/api/endpoint"
      headers:
        Content-Type: application/json
        Authorization: "Bearer ${AUTH_TOKEN}"
      body_from: previous_result   # or inline body: '{"key":"value"}'
      timeout: 30s
      success_codes: [200, 201, 202]

    output:
      description: "HTTP response body and status."
      schema:
        type: object
        properties:
          status_code: { type: integer }
          body: { type: string }
          headers: { type: object }

    side_effects:
      - kind: network_call
        target: configured_url
        description: "Makes an outbound HTTP request to the configured endpoint."

    reversibility:
      classification: irreversible
      undo: noop
```

Implementation:

```go
type httpCallCmd struct {
    config HttpCallConfig
    input  string   // from previous result or inline body
}

func (c *httpCallCmd) Name() string      { return c.config.Name }
func (c *httpCallCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *httpCallCmd) Execute() core.Result {
    ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
    defer cancel()

    var body io.Reader
    if c.input != "" {
        body = strings.NewReader(c.input)
    }

    req, err := http.NewRequestWithContext(ctx, c.config.Method, c.config.URL, body)
    if err != nil {
        return core.Result{Signal: core.ToolFailed, Err: err, CommandName: c.Name()}
    }
    for k, v := range c.config.Headers {
        req.Header.Set(k, v)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return core.Result{Signal: core.ToolFailed, Err: err, CommandName: c.Name()}
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(resp.Body)

    signal := core.ToolDone
    if !c.config.IsSuccess(resp.StatusCode) {
        signal = core.ToolFailed
    }

    output, _ := json.Marshal(map[string]interface{}{
        "status_code": resp.StatusCode,
        "body":        string(respBody),
    })

    return core.Result{
        Signal:      signal,
        Output:      string(output),
        CommandName: c.Name(),
    }
}
```

This one implementation covers all HTTP-based tools:
- Agent control (`approve_child`, `rollback_child`, `check_child_state`)
- Webhook notifications
- External API calls
- Health checks
- Any REST integration

Each is just a different YAML configuration of the same `init: http_call`.

## Agent REST API Surface

The agent exposes REST endpoints organized by domain. Each domain can be
enabled independently. Together they make the agent fully introspectable
and controllable over HTTP — for humans, UIs, supervisors, or CI systems.

### Machine (grammar) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/machine` | Full MachineSpec as JSON (states, signals, transitions) |
| GET | `/api/machine/state` | Current state + last signal + iteration count |
| GET | `/api/machine/diagram` | Mermaid state diagram with current position highlighted |
| GET | `/api/machine/sentence` | Full execution history (the sentence so far) |
| PUT | `/api/machine` | **Hot-reload**: load new machine YAML, rebuild transition table |
| POST | `/api/machine/inject` | Inject a signal (force a transition) |

**Hot-reload the grammar at runtime.** `PUT /api/machine` pauses the agent,
loads the new YAML, validates the current state still exists in the new
machine, rebuilds the transition table, and resumes. This lets you change
agent behavior without restarting — useful in development and for adaptive
production agents.

### Tools (lexicon) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/tools` | List all registered tools with category, emits, side effects |
| GET | `/api/tools/{name}` | Full tool declaration (parameters, output, undo, etc.) |
| GET | `/api/tools/{name}/manifest` | LLM-visible manifest entry for this tool |
| POST | `/api/tools` | **Hot-register**: add a new tool to the registry |
| DELETE | `/api/tools/{name}` | Deregister a tool (only for `$tool` dynamic dispatch) |

**Hot-register tools at runtime.** `POST /api/tools` accepts a tool
declaration YAML body. The tool becomes available for `$tool` dynamic
dispatch immediately. Static transitions that name the tool require a
machine reload. Use case: a supervisor gives a child agent a new capability
mid-run (e.g., "you now have access to deploy").

The registry freeze invariant is preserved by distinguishing:
- **Static tools** — resolved at startup, part of the transition table. Cannot be removed.
- **Dynamic tools** — registered after freeze, available only via `$tool` dispatch. Can be added/removed at runtime.

### Telemetry (OTel) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/telemetry/spans` | Recent spans (last N, filterable by tool name) |
| GET | `/api/telemetry/spans/{id}` | Single span with attributes and events |
| GET | `/api/telemetry/metrics` | Aggregate metrics: token usage, costs, durations |
| GET | `/api/telemetry/stream` | SSE stream of spans and events in real time |

These expose the same data the OTel exporter writes to file or sends to
a collector — but accessible live over HTTP. Craft uses `/api/telemetry/stream`
for its real-time trace view.

### Budget endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/budget` | Current budget state (iterations used/max, tokens used/max, duration) |
| PUT | `/api/budget` | **Modify budget at runtime** (extend iterations, add tokens, extend duration) |

**Extend budget mid-run.** If an agent is about to hit its iteration limit
but making progress, a supervisor (or human via craft) can
`PUT /api/budget {"max_iterations": 200}` to extend it without restarting.

### Conversation (LLM state) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/conversation` | Full message history |
| GET | `/api/conversation/length` | Message count |
| DELETE | `/api/conversation?after={n}` | Truncate to N messages (manual LLM rollback) |
| POST | `/api/conversation` | Inject a message (system prompt override, correction) |

**Manual conversation surgery.** Sometimes the LLM goes down a bad path.
An operator can truncate the conversation back to a good point and inject
a corrective message — without rolling back workspace changes. This is
finer-grained than full rollback.

### Workspace (environment) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/workspace/status` | Git status (modified/added/deleted files) |
| GET | `/api/workspace/diff` | Full diff from baseline |
| GET | `/api/workspace/diff/{path}` | Diff for a single file |
| GET | `/api/workspace/files/{path}` | Read file contents |
| GET | `/api/workspace/refs` | List checkpoint refs (Git tags/branches) |
| POST | `/api/workspace/rollback` | Restore workspace to a specific ref |

### Lifecycle (control) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/lifecycle/status` | Agent status: running, paused, suspended, completed, failed |
| POST | `/api/lifecycle/pause` | Pause the grammar (finish current tool, then block) |
| POST | `/api/lifecycle/resume` | Resume from pause |
| POST | `/api/lifecycle/step` | Execute one transition, then pause |
| POST | `/api/lifecycle/kill` | Terminate immediately |
| POST | `/api/lifecycle/checkpoint` | Force a checkpoint now |
| GET | `/api/lifecycle/checkpoints` | List available checkpoints |
| POST | `/api/lifecycle/suspend` | Serialize state and exit (resume later) |

### Approval (gate) endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/approval/pending` | Current pending approval (tool, state, context) or null |
| POST | `/api/approval/approve` | Approve pending dispatch |
| POST | `/api/approval/reject` | Reject pending dispatch (with reason) |
| PUT | `/api/approval/policy` | Update approval policy at runtime |

### What can be changed at runtime?

| Resource | Read | Modify | Hot-reload |
|---|---|---|---|
| Machine (grammar) | Yes | Inject signal | Full YAML reload |
| Tools (lexicon) | Yes | Add/remove dynamic tools | — |
| Budget | Yes | Extend limits | — |
| Conversation | Yes | Truncate, inject messages | — |
| Workspace | Yes | Rollback to ref | — |
| Approval policy | Yes | Update rules | — |
| Telemetry | Yes (stream) | — | — |
| Lifecycle | Yes | Pause/resume/kill/suspend | — |

### Implications for the engine

Supporting hot-reload and runtime modification requires small engine changes:

1. **Registry split**: static tools (frozen at startup) vs. dynamic tools
   (mutable, available only via `$tool`). The registry already has a freeze
   point — dynamic tools live in a separate mutable map.

2. **Machine reload**: a `ReloadMachine(spec MachineSpec)` method on the
   engine that atomically swaps the transition table. Must validate that
   current state exists in the new spec. Only safe between iterations
   (during pause or between dispatch cycles).

3. **Budget mutation**: the `Budget` struct gains a mutex and setter methods.
   The loop reads budget atomically each iteration.

4. **Conversation access**: `pkg/llm.Conversation` already has a mutex. Add
   `TruncateTo(n)` (already designed in roll-backs.md) and `Inject(msg)`.

5. **OnBeforeDispatch hook**: already discussed for approval gates. The
   engine calls this hook before each dispatch; it can block or reject.

None of these break the existing model. An agent with no REST server enabled
runs exactly as today. The REST surface is opt-in — launched by a
`launch_craft` or `launch_api` tool at the grammar's discretion.

## Go Implementation Sketch

### UIServerState (replaces BenchState)

```go
type UIServerState struct {
    servers map[string]*uiServer  // keyed by addr
    mu      sync.Mutex
}

type uiServer struct {
    addr     string
    mux      *http.ServeMux
    actionCh chan ActionMessage
    view     *activeView
    started  bool
}

type activeView struct {
    name      string
    assets    fs.FS
    actions   []ActionDecl
    mounts    []DataMount
    context   json.RawMessage
}

type ActionMessage struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload,omitempty"`
}

type ActionDecl struct {
    Name   string `yaml:"name"`
    Signal string `yaml:"signal"`
}

type DataMount struct {
    Prefix string `yaml:"prefix"`
    Source string `yaml:"source"`
    Type   string `yaml:"type"`
}
```

### ServeUIBuilder (generic)

```go
type ServeUIBuilder struct {
    State  *UIServerState
    Config ServeUIConfig
}

func (b *ServeUIBuilder) Build(prev core.Result) core.Command {
    return &serveUICmd{
        state:   b.State,
        config:  b.Config,
        context: prev.Output,  // inject previous result as context
    }
}
```

### serveUICmd.Execute

```go
func (c *serveUICmd) Execute() core.Result {
    srv := c.state.EnsureServer(c.config.Addr)

    assets := c.resolveAssets()
    srv.SetActiveView(&activeView{
        name:    c.config.Name,
        assets:  assets,
        actions: c.config.Actions,
        mounts:  c.config.DataMounts,
        context: json.RawMessage(c.context),
    })

    action := <-srv.actionCh

    signal := c.mapAction(action)

    return core.Result{
        Signal:      signal,
        Output:      string(action.Payload),
        CommandName: c.config.Name,
    }
}
```

### Factory registration

```go
func ServeUIFactory(state *UIServerState) stl.BuiltinFactory {
    return func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
        var cfg ServeUIConfig
        if err := stl.DecodeToolConfig(def, &cfg); err != nil {
            return nil, err
        }
        cfg.Name = def.Name
        return &ServeUIBuilder{State: state, Config: cfg}, nil
    }
}
```

All tools using `init: serve_ui` share the same `UIServerState` instance
and the same factory function. They differ only in their decoded config.

## Standard HTTP Endpoints

The REST API is the primary interface. The UI (static assets) is an optional
consumer of these endpoints. All control operations — approvals, rollback,
pause, signal injection — work via REST without any UI. You can `curl` them,
call them from another agent, wire them into CI/CD, or drive them from a
monitoring service.

**The UI is optional.** If `config.assets` is omitted, the server still
exposes all REST endpoints. The agent is controllable via HTTP with no
frontend at all. The UI is convenience, not mechanism.

### Core API (always available)

| Method | Path | Description |
|---|---|---|
| GET | `/api/health` | Server health check |
| GET | `/api/state` | Current state, available actions, budget, context |
| GET | `/api/actions` | List of declared actions with descriptions |
| POST | `/api/actions` | Submit an action (validated against declared set) |
| GET | `/api/sentence` | Full execution history (sentence so far) |
| POST | `/api/rollback` | Roll back to a target step |
| POST | `/api/approve` | Approve a pending dispatch |
| POST | `/api/reject` | Reject a pending dispatch |
| POST | `/api/pause` | Pause the grammar |
| POST | `/api/resume` | Resume from paused state |
| POST | `/api/inject` | Inject an arbitrary signal |
| GET | `/api/stream` | SSE stream of state transitions, spans, events |

### Data API (when data mounts configured)

| Method | Path | Description |
|---|---|---|
| GET | `/data/{mount}` | List directory entries for a data mount |
| GET | `/data/{mount}/{path...}` | Read file from a data mount |

### UI API (only when assets configured)

| Method | Path | Description |
|---|---|---|
| GET | `/` | Serve static assets (SPA with fallback to index.html) |

### Example: headless approval via curl

No UI needed — just REST:

```bash
# Check what's pending approval
curl http://localhost:9000/api/state
# → {"state":"Composing","pending_approval":{"tool":"write","file":"main.go"}}

# Approve it
curl -X POST http://localhost:9000/api/approve \
  -H 'Content-Type: application/json' \
  -d '{"reason":"looks good"}'
# → {"status":"approved"}

# Or reject
curl -X POST http://localhost:9000/api/reject \
  -H 'Content-Type: application/json' \
  -d '{"reason":"wrong file"}'
# → {"status":"rejected","action":"rollback to last safe state"}
```

### Example: headless rollback via curl

```bash
# See execution history
curl http://localhost:9000/api/sentence
# → [{"step":1,"state":"Composing","tool":"invoke_llm"}, ...]

# Roll back to step 5
curl -X POST http://localhost:9000/api/rollback \
  -H 'Content-Type: application/json' \
  -d '{"target_step":5}'
# → {"status":"rolled_back","undone_steps":[7,6],"current_state":"Composing"}
```

### Example: another agent controlling this one

A supervisor agent can control a child agent via these REST endpoints —
giving approvals, forcing rollbacks, or injecting signals programmatically.

You could wrap this as an `exec` tool calling curl, but the idiomatic
approach is a Go-native `http_call` boundary word that uses `net/http`
directly:

```yaml
tools:
  - name: approve_child
    type: builtin
    category: boundary
    boundary_actor: agent
    init: http_call
    emits: [ToolDone, ToolFailed]
    description: "Approve a pending dispatch on the child agent."
    config:
      method: POST
      url: "http://localhost:9000/api/approve"
      body_from: previous_result   # pass context from parent grammar
      timeout: 30s

  - name: rollback_child
    type: builtin
    category: boundary
    boundary_actor: agent
    init: http_call
    emits: [ToolDone, ToolFailed]
    config:
      method: POST
      url: "http://localhost:9000/api/rollback"
      body: '{"target_step": 5}'
      timeout: 10s

  - name: check_child_state
    type: builtin
    category: word
    init: http_call
    emits: [ToolDone, ToolFailed]
    config:
      method: GET
      url: "http://localhost:9000/api/state"
      timeout: 5s
```

The `http_call` init is a generic Go implementation — `net/http` client
configured via YAML (method, URL, headers, body source, timeout). No
shelling out to curl. Same principle: one Go implementation, many
configurations.

This gives you agent-to-agent control via REST without subprocess overhead,
without curl dependencies, and with proper Go error handling, timeouts, and
connection pooling.

### POST /api/actions request

```json
{
  "type": "launch_experiment",
  "payload": {
    "suite": "basic-go",
    "models": ["granite", "deepseek"]
  }
}
```

### POST /api/actions response

```json
{ "status": "accepted" }
```

Or if the action type is not in the declared set:

```json
{ "error": "unknown action: foo", "available": ["launch_experiment", "compare_models", "shutdown"] }
```

### GET /api/state response

```json
{
  "view": "serve_dashboard",
  "actions": [
    { "name": "launch_experiment", "description": "User requests a new evaluation run." },
    { "name": "compare_models", "description": "User requests a model comparison view." },
    { "name": "shutdown", "description": "User requests graceful shutdown." }
  ],
  "data_mounts": ["sessions", "configs", "profiles"],
  "context": { ... }
}
```

## UI Contract

The served frontend (HTML/JS/CSS) must:

1. **Fetch state on load**: `GET /api/state` to know which view it should
   render and what actions are available.

2. **Submit actions via POST**: When the user acts, send
   `POST /api/actions` with `{ type, payload }`.

3. **Handle view changes**: If the SPA stays loaded across tool dispatches,
   poll or use SSE to detect view changes and re-render.

The frontend does NOT need to know about the grammar, signals, or state
machine. It only knows: "here are the actions I can take" and "here is the
data I can display."

## Migration Path from Current Bench

1. **Extract generic UIServerState** from `BenchState` — the server, action
   channel, and view management become `internal/tools/stl` or a new `pkg/ui` package.

2. **Move action-to-signal mapping to YAML** — delete the `Signal()` switch
   in `state.go`, replace with config-driven lookup.

3. **Make assets path configurable** — the bench tool declaration YAML gets
   `config.assets: ./pkg/bench/ui/dist` (keeping existing behavior). The
   embed fallback remains for single-binary builds.

4. **Extract bench-specific API routes into data mounts** — sessions,
   configs, profiles become data mount entries.

5. **Rename and generalize** — `RegisterFactories` becomes the generic
   `ServeUIFactory` with bench-specific config now living entirely in YAML.

After migration, the bench is just one YAML configuration of the generic
`serve_ui` implementation. The same implementation can serve an approval
gate, a monitoring dashboard, or any other human-facing interface.

## Example: Approval Gate Tool

An agent that needs human approval before destructive actions:

```yaml
# In the agent's machine.yaml:
transitions:
  - state: PendingApproval
    signal: Approved
    next: Executing
    action: execute_plan
  - state: PendingApproval
    signal: Rejected
    next: Revising
    action: revise_plan
  - state: PendingApproval
    signal: Deferred
    next: Suspended

# In the agent's tool declarations:
tools:
  - name: request_approval
    type: builtin
    init: serve_ui
    category: boundary
    boundary_actor: human
    emits: [Approved, Rejected, Deferred]
    description: "Present proposed changes and wait for human approval."
    config:
      addr: ":9090"
      assets: ./ui/approval/dist
      actions:
        - name: approve
          signal: Approved
        - name: reject
          signal: Rejected
        - name: defer
          signal: Deferred
      context_providers:
        - name: proposed_changes
          source: previous_result
```

The approval UI receives the proposed changes via `GET /api/state` context,
presents them to the user, and returns one of three signals.

## Example: Multi-View Agent

An orchestrator that shows different UIs at different workflow stages:

```yaml
tools:
  - name: serve_task_picker
    type: builtin
    init: serve_ui
    emits: [TaskSelected, Shutdown]
    config:
      addr: ":8080"
      assets: ./ui/task-picker/dist
      actions:
        - name: select_task
          signal: TaskSelected
        - name: shutdown
          signal: Shutdown
      data_mounts:
        - prefix: tasks
          source: ./pending-tasks
          type: directory_listing

  - name: serve_progress
    type: builtin
    init: serve_ui
    emits: [Acknowledged]
    config:
      addr: ":8080"
      assets: ./ui/progress/dist
      actions:
        - name: acknowledge
          signal: Acknowledged
      context_providers:
        - name: execution_result
          source: previous_result
```

Same server (`:8080`), different views at different points in the grammar.
The SPA detects the view change and renders the appropriate interface.

## Example: Craft — Real-Time Agent Observatory

The async pattern enables a fundamentally different kind of UI tool: one
that runs alongside the agent for its entire lifetime, streaming execution
state to a display. We call this **craft** — the agent's cockpit.

### What craft shows

- **Live state machine** — current state highlighted, transitions animated
  as they fire, terminal states marked.
- **Execution sentence** — the growing list of (state, signal, word, result)
  tuples as the agent produces them.
- **OTel traces** — spans and events streamed from the agent's telemetry
  exporter, showing timing, token usage, and errors.
- **Tool metrics** — per-tool success/failure counts, durations, costs.
- **Budget** — remaining iterations, tokens, and wall-clock time.
- **Conversation** — the LLM conversation history as it grows (for
  LLM-based agents).
- **Workspace diff** — file changes since baseline (from the Git workspace).

### What craft can do (optional intervention)

- **Pause** — suspend the grammar at the current state.
- **Step** — advance one transition at a time.
- **Rollback** — undo the last N steps.
- **Inspect** — view full tool output, workspace state, or conversation at
  any point in the sentence.
- **Override** — inject a signal manually (force a transition).
- **Kill** — terminate the agent immediately.

### Craft as the control plane

Observation is passive. The deeper capability is **control**: craft can
drive the state machine. The human operator becomes a co-pilot who can
steer the grammar at runtime.

**Approval gates without grammar changes.** Instead of encoding approval
states into every agent's machine.yaml, craft intercepts transitions
before they fire. The engine consults craft ("state X is about to enter,
tool Y is about to execute — approve?") and blocks until the operator
approves or rejects. This works via a LoopHook that checks with craft
before dispatching:

```
Engine → OnBeforeDispatch hook → craft: "about to run 'write' in state Composing"
                                   ↓
                               user sees: "Agent wants to write main.go — approve?"
                                   ↓
                               user clicks [Approve] or [Reject]
                                   ↓
hook returns ← ─────────────── craft responds: approved (or injects override signal)
```

This means any existing grammar gets human-in-the-loop approval by enabling
craft — no modifications to machine.yaml needed. The approval policy is
configured in craft:

```yaml
config:
  approval_policy:
    # Require approval before these tools execute
    require_approval:
      - write
      - edit
      - run_agent
    # Auto-approve these (observation only)
    auto_approve:
      - read
      - find
      - list_files
      - build
      - test
    # Require approval before entering these states
    gate_states:
      - ValidatingBuild    # "Agent thinks it's done — approve validation?"
      - RunningAgent       # "About to spawn child agent — approve?"
```

**Rollback from the cockpit.** The operator sees the full sentence in real
time. At any point they can select a previous step and say "go back to
here." Craft knows the sentence history (from LoopHooks), so it:

1. Identifies which steps to undo (from current back to target).
2. Sends a `RollbackRequested` signal with the target iteration.
3. The grammar enters a rollback state.
4. The `rollback` tool walks the sentence backward, calling `Undo` on each
   word and restoring the workspace to the target Git ref.
5. The agent resumes from the earlier state.

```
Craft UI:
┌─────────────────────────────────────────────────────────────┐
│ Sentence History                                             │
│                                                             │
│  ✓ #1  Idle → Composing         invoke_llm     3.2s        │
│  ✓ #2  Composing → Parsing      parse_response  0.01s      │
│  ✓ #3  Parsing → Composing      write           0.05s      │
│  ✓ #4  Composing → Composing    invoke_llm      2.8s       │
│  ✓ #5  Composing → Parsing      parse_response  0.01s      │
│  ✓ #6  Parsing → Composing      edit            0.03s  ← wrong edit
│  ● #7  Composing → Composing    invoke_llm      ...        │
│                                                             │
│  [↩ Rollback to #5]  [⏸ Pause]  [⏭ Step]                   │
└─────────────────────────────────────────────────────────────┘
```

Operator clicks "Rollback to #5" → craft sends `RollbackRequested{target:5}`
→ the rollback tool undoes steps #7, #6 (reverting the edit, truncating the
conversation) → agent resumes from step #5's state.

**Signal injection.** Sometimes the operator wants to force a transition —
override what the tool returned. Craft can inject a signal directly into
the engine, effectively saying "pretend the tool returned this signal
instead." This is useful for:

- Forcing a failing agent past a stuck state
- Skipping validation during debugging
- Testing specific grammar paths

```yaml
actions:
  - name: inject_signal
    signal: $dynamic    # special: the payload contains the actual signal
    description: "Inject an arbitrary signal into the grammar."
    payload_schema:
      type: object
      properties:
        signal: { type: string }
        reason: { type: string }
      required: [signal]
```

**State machine visualization with live position.** Craft renders the
machine.yaml as an interactive graph (using the state/signal/transition
data the engine already exposes). The current state is highlighted. As
transitions fire, edges animate. The operator can click on any state to
see: what signals are handled here, what tools get dispatched, what the
last N visits to this state produced.

### Implementation: approval hook

The approval gate doesn't require grammar changes because it operates at
the engine level via `LoopHooks.OnBeforeDispatch`:

```go
type LoopHooks struct {
    // ... existing hooks ...
    OnBeforeDispatch func(state State, signal Signal, cmd Command) error
}
```

If `OnBeforeDispatch` returns an error, the engine does not dispatch the
command. Instead it treats the error as a rejection — the hook can inject
an alternative signal (e.g., `Rejected`) or block until approval arrives.

Craft registers this hook at launch time:

```go
func (c *craftController) OnBeforeDispatch(state core.State, sig core.Signal, cmd core.Command) error {
    if !c.requiresApproval(cmd.Name()) {
        return nil // auto-approved
    }

    // Push approval request to the craft UI via SSE
    c.stream <- ApprovalRequest{
        State:   state,
        Signal:  sig,
        Tool:    cmd.Name(),
    }

    // Block until operator responds
    decision := <-c.approvalCh

    if decision.Approved {
        return nil
    }
    return fmt.Errorf("rejected by operator: %s", decision.Reason)
}
```

This is the same channel-blocking pattern as `serve_ui`, but at the hook
level rather than the tool level. The grammar continues to dispatch tools
normally — the hook silently gates them.

### How it fits the async pattern

Craft is a non-blocking launch word. It starts a streaming UI server and
returns immediately. The agent continues executing. Craft passively observes
through two channels:

1. **OTel stream** — craft registers as an in-process OTel exporter (or
   reads from the file exporter in real time). It receives every span and
   event the agent produces without the agent knowing.

2. **LoopHooks** — the engine's `OnResult` hook pushes state transitions
   and results to craft's event stream. No changes to the engine needed —
   hooks already exist.

3. **Inbox for intervention** — if the user clicks Pause/Rollback/Override,
   craft writes to the shared inbox. The grammar handles it like any other
   boundary signal.

```
Agent execution thread:             Craft UI (background):
────────────────────────            ───────────────────────
launch_craft.Execute()
  → starts craft server
  → registers OTel exporter         server starts streaming
  → wires LoopHooks → SSE
  → returns Launched

invoke_llm.Execute()                 shows: state=Composing, tool=invoke_llm
  → LLM responds                     shows: span completed, tokens used

parse_response.Execute()             shows: state=Parsing, tool=parse_response
  → yields tool call

$tool → write.Execute()              shows: state=Composing, tool=write
  → file written                     shows: workspace diff updated

                                     user clicks [Pause]
                                       → inbox <- PauseRequested

await_input.Execute()
  → receives PauseRequested
  → grammar handles pause
```

### Grammar integration

Craft runs alongside the agent's normal grammar. Two patterns:

**Pattern A: Craft as an always-on observer (no intervention)**

The grammar doesn't need to handle craft signals. Craft just observes via
hooks and OTel. It's launched once at startup and never interacts with the
grammar again.

```yaml
transitions:
  - state: Idle
    signal: Seed
    next: LaunchingCraft
    action: launch_craft

  - state: LaunchingCraft
    signal: Launched
    next: Composing
    action: invoke_llm

  # ... normal agent grammar continues ...
```

**Pattern B: Craft with intervention (debugging/approval mode)**

The grammar periodically checks for intervention signals by using
`await_input` with a timeout, or by handling intervention signals in every
state:

```yaml
transitions:
  # Normal flow
  - state: Composing
    signal: ToolDone
    next: Composing
    action: invoke_llm

  # Intervention: user paused from craft
  - state: Composing
    signal: PauseRequested
    next: Paused
    action: await_input

  - state: Paused
    signal: ResumeRequested
    next: Composing
    action: invoke_llm

  - state: Paused
    signal: StepRequested
    next: Stepping
    action: invoke_llm

  - state: Paused
    signal: RollbackRequested
    next: RollingBack
    action: rollback
```

### Tool declaration

```yaml
tools:
  - name: launch_craft
    type: builtin
    category: boundary
    boundary_actor: human
    init: launch_ui
    emits: [Launched]
    description: "Start the craft observatory and control plane (non-blocking)."
    config:
      addr: ":9000"
      assets: ./ui/craft/dist

      # Observation feeds (one-way, agent → UI)
      streams:
        - name: state_machine
          source: loop_hooks
          description: "State transitions and tool dispatch events."
        - name: traces
          source: otel_exporter
          description: "OpenTelemetry spans and events."
        - name: workspace
          source: git_workspace
          description: "File change diffs since baseline."
        - name: conversation
          source: llm_conversation
          description: "LLM message history as it grows."

      # Approval policy (gate dispatches at hook level)
      approval_policy:
        require_approval: [write, edit, run_agent]
        auto_approve: [read, find, list_files, build, test, lint]
        gate_states: [ValidatingBuild]

      # Intervention actions (two-way, UI → grammar via inbox)
      actions:
        - name: pause
          signal: PauseRequested
        - name: resume
          signal: ResumeRequested
        - name: step
          signal: StepRequested
        - name: rollback
          signal: RollbackRequested
        - name: inject_signal
          signal: $dynamic
        - name: kill
          signal: KillRequested

      data_mounts:
        - prefix: traces
          source: ./traces
          type: directory_listing
```

### Streaming protocol

Craft uses **Server-Sent Events (SSE)** for the observation feeds — the
server pushes events to the browser without polling:

```
GET /api/stream

event: state_transition
data: {"from":"Composing","to":"Parsing","signal":"LLMResponded","iteration":5}

event: tool_dispatch
data: {"tool":"parse_response","state":"Parsing"}

event: tool_result
data: {"tool":"parse_response","signal":"ToolDone","duration_ms":12,"output_preview":"..."}

event: span
data: {"name":"execute_tool parse_response","duration_ms":12,"attributes":{...}}

event: budget
data: {"iterations_used":5,"iterations_max":100,"tokens_used":4200,"tokens_max":50000}
```

The frontend renders these events in real time: animating the state machine
diagram, appending to the sentence log, updating metrics dashboards.

### Why this matters

Craft transforms the agent from a black box into an observable,
**controllable** system. The human operator can:

- Watch passively (pure observation, no grammar changes needed).
- Gate dangerous actions (approval policy, no grammar changes needed).
- Intervene on failure (rollback, signal injection).
- Debug interactively (pause, step, inspect).

All of this composes with the existing Grammar Machine pattern. The grammar
doesn't need approval states, rollback states, or debug states baked in.
Craft operates at the hook and inbox level — it wraps any grammar with
human control without modifying that grammar's definition.

The configurable UI principle applies here too: the craft frontend is just
`assets: ./ui/craft/dist`. A full debugging cockpit for development, a
simplified approval panel for production, a read-only dashboard for
stakeholders — same backend, different assets path.

