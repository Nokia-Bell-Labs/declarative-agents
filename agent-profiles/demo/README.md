<!-- Copyright (c) 2026 Nokia. All rights reserved. -->

# Knowledge Manager Demo

We drive the knowledge-manager documentation agent from a Go present deck,
[knowledge-manager.slide](knowledge-manager.slide). The deck has two steps: it
starts the documentation-curator agent, then posts a lifecycle-exit request to
its control server.

## Steps

The first slide runs [start_knowledge_manager.go](start_knowledge_manager.go),
which launches `agent-core/cmd/agent` against the documentation-curator profile
and serves the documentation, control, and monitor endpoints. The second slide
runs the lifecycle-exit agent through the same interpreter to post the exit
request and stop the curator.

## The lifecycle-exit agent

The exit request is a declarative agent under
[call-lifecycle-exit/](call-lifecycle-exit/), not a bespoke HTTP client. Its
machine has one boundary word, `post_exit`, that binds the rest tool to POST the
fixed `{"reason": "demo presentation"}` body to
`/api/lifecycle/exit`; the machine reaches a terminal `succeeded` state on HTTP
202 Accepted. The control-server URL is a declared REST client base
(`CURATOR_URL`, default `http://127.0.0.1:18082`), not runtime input, and the
endpoint carries no transport authority (`auth: none`). Run it with:

    agent --profile demo/call-lifecycle-exit/profile.yaml --directory .

Expressing the exit call as a machine rather than a Go binary makes the demo an
instance of the system's own thesis: runtime behavior lives in YAML and is run
by the interpreter. It replaces the former `call_lifecycle_exit/main.go`.

## Why start_knowledge_manager.go stays a Go binary

`start_knowledge_manager.go` is a launcher, not agent behavior. It resolves the
demo's repository paths, prepares a temporary profile overlay that points the
curator at a sibling `agent-core` checkout, and runs `agent-core/cmd/agent`. It
embeds no agent logic of its own; it is the harness that starts the interpreter,
the same role magefiles and `cmd/agent` wiring play elsewhere. Converting it to
an agent profile would have nothing to declare -- the agent it launches is the
documentation-curator profile it already runs. It is therefore exempt from the
executables-are-agents rule and remains a launcher. The recurring
declarative-decomposition audit cites this instead of re-flagging the launcher.
