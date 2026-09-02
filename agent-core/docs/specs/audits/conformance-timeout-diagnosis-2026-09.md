<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Conformance timeout diagnosis -- 2026-09-02

This report records the evidence required by GH-1923 before changing agent
termination or conformance process management. GH-1904 reported that
`TestCorpusIngestNormalizesProviderEmbeddings` timed out after printing only
the telemetry exporter line when `TMPDIR` was absent.

The investigation separates the historical blocking operation from the
diagnostic defect exposed by forced termination. They are different problems.

## Scope

The inspected path is:

1. `applications/catalog/conformance.Run` starts the agent.
2. `agent-core/cmd/agent` loads and validates the profile.
3. The corpus-ingest machine reaches its first model-controlled iteration.
4. The harness deadline kills the child through `exec.CommandContext`.
5. Telemetry shutdown either publishes the final trace file or is skipped.

The raw Darwin SIGQUIT evidence is attached to GH-1925:
https://github.com/Nokia-Bell-Labs/declarative-agents/issues/1925#issuecomment-5509041715

Machine-specific runtime, GC, and binary-image frames remain in the local raw
capture and are not copied into this repository.

## Reproduction

The current tree includes GH-1922 / PR #1924, which pins corpus-ingest chat
inference to its protocol fixture. On macOS 26.5.2 (Darwin 25F84), this exact
command passes with no `TMPDIR` or `OLLAMA_URL`:

```text
env -i PATH="$PATH" HOME="$HOME" \
  go test -count=1 ./conformance \
  -run '^TestCorpusIngestNormalizesProviderEmbeddings$'
```

Result:

```text
ok  .../applications/catalog/conformance  4.746s
```

The released `v0.20260902.0` tree predates GH-1922. Under the same environment,
the valid Ollama-flat and Cohere-row children remained active while the invalid
multiple-row case completed. A SIGQUIT sent to the Cohere-row child three
seconds after launch produced the attached Go stack.

## Stack finding

Goroutine 1 had passed bootstrap and entered the runtime loop. It waited in:

```text
core.safeExecuteLegacy
core.SafeExecuteContext
core.dispatchWithMonitorContext
core.(*loopRunner).dispatch
core.coreLoop
core.Loop
main.runOrResume
main.runPrepared
```

The command worker was blocked in:

```text
net/http.(*persistConn).roundTrip
net/http.(*Client).Do
ollama.(*Adapter).Chat
llm.(*invokeLLMCmd).chat
llm.(*invokeLLMCmd).Execute
core.executeSafely
```

The block was therefore an `invoke_llm` HTTP round trip. The historical test
patched Chroma, embedding, and readiness REST clients to `httptest`, but left
the LLM declaration at `${OLLAMA_URL:-http://localhost:11434}`. GH-1922 fixed
that test defect and added exact fixture chat-call assertions.

The stack does not support a pre-loop startup hang, a telemetry setup block, or
a `/tmp` filesystem block. The original `TMPDIR` correlation changed timing
around an unintended live localhost model call; it did not identify the blocked
operation.

## Diagnostic finding

SIGQUIT terminated the child without running normal shutdown. The harness then
reported that `trace.otel.json` did not exist. This is consistent with the
file-exporter contract:

- spans are written to a temporary file beside the destination;
- shutdown force-flushes providers and ends the root span;
- shutdown closes and atomically renames the temporary file;
- SIGKILL or SIGQUIT skips those defers and leaves no final path.

`exec.CommandContext` also kills only the child leader by default. It does not
provide a SIGTERM grace period or an explicit process-group reap. That remains
a real harness defect even though the historical model call is now hermetic.

## Existing stage evidence

Additional stderr startup breadcrumbs are rejected.

The trace already records the distinctions GH-1923 needs:

- absence of `agent.run` means telemetry never published;
- `agent.run` without `init.registry_frozen` identifies pre-loop initialization;
- `init.registry_frozen` proves tool registration and machine preparation;
- `execute_tool <name>` identifies the active dispatch;
- cancellation and terminal events record how the loop ended.

The captured stack confirms the useful stage was `execute_tool invoke_llm`.
Publishing the existing trace on cooperative timeout is preferable to creating
a second stderr protocol. New diagnostics must not include prompts, request
bodies, environment values, URLs, or tool output.

## Selected design

GH-1926 should make SIGINT and SIGTERM cancel the agent runtime context after
bootstrap has enough state to close safely. Normal unwinding must call
`preparedRun.Close`, flush telemetry, and preserve the atomic rename contract.
Signal handling must not turn a pre-loop block into an indefinite wait.

GH-1927 should replace conformance leader-only deadline killing with one shared
Unix lifecycle:

1. start the child in its own process group;
2. send SIGTERM to the group at the test deadline;
3. wait for a short bounded grace period;
4. send SIGKILL to the group even if the leader exited;
5. wait until the child is reaped;
6. parse the normally published trace before using bounded fallback evidence.

Helper-process tests must cover cooperative exit and SIGTERM-ignoring
descendants in under five seconds. The tests must not reproduce the former
30-second model wait.

## Disposition

GH-1922 closes the historical corpus-ingest blocking cause. GH-1926 and GH-1927
remain justified as generic timeout-diagnostic and process-lifecycle fixes.
No new startup log taxonomy, synchronous span processor, periodic trace flush,
or change to `internal/support/subprocess.SetProcGroup` is approved by this
diagnosis.
