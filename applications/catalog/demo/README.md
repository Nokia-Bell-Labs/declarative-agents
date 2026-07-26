<!-- Copyright (c) 2026 Nokia. All rights reserved. -->

# Knowledge Manager Demo

We drive the knowledge-manager documentation agent from
[knowledge-manager.slide](knowledge-manager.slide). The deck has two steps: it
starts the shipped documentation-curator profile, then posts a lifecycle-exit
request to its control server.

## Steps

The first slide launches the profile directly from the `agent-profiles` root:

    agent \
      --profile agents/knowledge-manager/documentation-curator/profile.yaml \
      --directory . \
      --core-root ../agent-core

`--directory` supplies the profile repository as the documentation workspace.
`--core-root` maps the profile's installed `/opt/agent-core` declaration path to
a development checkout. An installed runtime does not need `--core-root`.

For a source checkout, Mage can build the binary from the sibling
`agent-core` checkout and run the same command:

    mage demo:knowledgeManager

Set `AGENT_CORE_ROOT` when the core checkout is not the sibling
`../agent-core`. The second slide runs the lifecycle-exit agent through the
same interpreter to post the exit request and stop the curator.

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
