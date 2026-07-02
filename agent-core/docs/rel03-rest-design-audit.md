# Release 03.0 REST Design Audit

Issue `agent-core-g60h` audits the REST runtime package and its release evidence. Main evidence comes from `internal/tools/rest/` and `tools/builtin/rest/all.yaml`. Profile evidence lives under `agents/rest/`; fixture evidence lives under `testdata/integration/rel03-rest-tools/`; release status evidence lives in `docs/SPECIFICATIONS.yaml`.

Method: I treated a REST behavior as supported only when a requirement, profile or config artifact, and Go test pointed to the same behavior. Partial support stays visible as follow-up work.

No code moved.

## Verdict

Client paths work. Async paths work. Inbound signal paths work. OpenAPI import paths work. Code follows the profile-first tool language model for those paths. The safety gaps from `agent-core-0l2i` are closed. Credential references resolve through `CredentialResolver`; redirect allowlists and request or response byte limits are enforced; server path, query, header, and body validation rejects unsafe requests; client and server redaction tests keep synthetic secrets out of Result output.

Release readiness is aligned.

A REST tool is not one feature. The release contract spans specs and profile assets, ToolDefs and builders, HTTP behavior, and machine transitions. Status changes are valid only when those artifacts agree. Client flows have that agreement. Inbound signal receipt has it too. Shutdown, OpenAPI import, handler invocation, event streaming, listener side effects, credential resolution, redirect policy, size limits, request validation, and redaction now have it.

## Design Constitution

### D1 Specification-Driven Development

D1 passes. Implementation follows the three REST SRDs and `test-rel03.0-rest-tools`. Go evidence exists in REST client tests, async tests, server tests, and OpenAPI tests under `internal/tools/rest/`, which gives the release suite executable proof across outbound calls, async state, inbound queues, and import compilation. Those tests cover the REST flows named by the release suite, including the server endpoint bindings completed by `agent-core-usbz.1`.

### D2 YAML-First Structured Docs

D2 passes. Sample YAML lives under `agents/rest/`. It covers profile setup and machine grammar. It also covers tool selection, REST config, and OpenAPI input. Fixture copies live in `testdata/integration/rel03-rest-tools/`. Markdown fits this report because the output is prose review material.

### D3 Traceability

D3 passes. `docs/SPECIFICATIONS.yaml` links release 03.0 to `test-rel03.0-rest-tools` and marks the release done. Named Go tests exist in `internal/tools/rest/`. The SRD029 binding gap is resolved by `agent-core-usbz.1`. Side-effect vocabulary alignment is resolved by `agent-core-usbz.2`.

### D4 Profile-First Runtime Docs

D4 passes. `agents/rest/profile.yaml` loads REST definitions through the profile contract implemented by `internal/tools/catalog/profile.go` and used by `cmd/agent/main.go`. Tool declarations reference named REST config through `rest_ref`, `resource`, and `operation`. No REST sample describes a separate REST binary.

### D5 Tool Language Boundary

D5 passes. ToolDefs in `tools/builtin/rest/all.yaml` and `agents/rest/declarations.yaml` declare the contract metadata required by the tool language. Sequencing for the sample payment flow lives in `agents/rest/machine.yaml`, not in Go command code. Shared REST ToolDefs can keep explicit listener effects after `agent-core-usbz.2`.

## Execution And Go Style

### Package Boundaries

Package boundaries pass. Implementation lives under `internal/tools/rest/`. Factory registration runs through `internal/tools/registry` from `internal/tools/rest/factories.go`. Profile loading remains in `internal/tools/catalog/profile.go`. Runtime wiring in `cmd/agent/main.go` loads definitions and registers factories without service-specific branches.

### Function And File Size

Function and file size checks pass through `mage lint`, `go vet ./...`, and `go test ./... -count=1`. Package files split client behavior from server behavior. Other files cover OpenAPI import, validation logic, and factory registration.

### Hidden Workflow Sequencing

Workflow ownership passes. Client send and await are separate words in `client_command.go` and `client_async.go`. Launch, await, and stop are separate server words in `command.go` and `server_state.go`. HTTP handlers in `server_routes.go` validate requests and enqueue events; they do not select MachineSpec actions. Sample sequencing appears in `agents/rest/machine.yaml`.

### Validation And Safety

Validation passes. The runtime rejects undeclared params and runtime authority overrides. `TestRESTClient_ResolvesAuthCredentialRefs` proves `token_ref`, `username_ref`, and `password_ref` are names resolved by trusted runtime dependencies, not literal secrets. `TestRESTClient_MissingCredentialReferenceFailsAuthResolution` proves missing credential refs fail before the request leaves the process and report `auth_resolution`.

Redirect and size-limit gaps are closed. `TestRESTClient_RedirectAllowlistPolicy` proves allowlisted redirect hosts succeed and unlisted hosts fail as `network_io`. `TestRESTClient_RequestAndResponseSizeLimits` proves oversized request bodies fail before send and oversized response bodies produce a `size_limit` CommandError without returning the response content.

Server validation and redaction gaps are closed. `TestRESTServer_RejectsUndeclaredQueryAndHeader` proves undeclared query and header values, schema-invalid header values, and schema-invalid path values return `400` without enqueueing events. `TestRESTServer_RedactsAwaitAndStreamOutput` proves await and SSE output redact configured query, header, and body secrets. `TestRESTServer_RedactsHandlerResponses` proves handler responses redact configured body secrets before returning JSON.

## REST ToolDefs

Shared REST ToolDefs pass. `tools/builtin/rest/all.yaml` includes the expected boundary contract fields. `rest_server_launch` and `rest_server_stop` use `network_listen` and `network_listener_shutdown`. Those names match REST SRD language and are accepted by the contract audit after `agent-core-usbz.2`.

Sample REST ToolDefs pass. `agents/rest/declarations.yaml` loads through the normal profile path. `mage audit` validates the selected REST profile, including declared emits against `agents/rest/machine.yaml`.

## Quality Gates

Code and documentation gates passed through the standard Go and Mage checks. The release suite names the safety tests added by `agent-core-0l2i.1`, `agent-core-0l2i.2`, and `agent-core-0l2i.3`. Earlier endpoint work remains covered by `TestRESTServer_InvokeHandlerBindings` and `TestRESTServer_StreamEvents`; server tests also cover launch, await, stop, queue overflow, method rejection, body limits, and simple schema checks. The broader package evidence includes client sync tests, async send and await tests, OpenAPI import tests, contract loading tests, and tracing or redaction tests.

REST behavior crosses config loading, runtime validation, HTTP I/O, event queues, and release metadata. A green server-only test would not prove the release by itself. Here, `go build ./...`, `go vet ./...`, `mage lint`, `go test ./...`, and `mage audit` pass with the updated release suite count. The de-AI lexical checks are clean; structural warnings are recorded below.

De-AI exception: the structural checker reports list-heavy, tricolon-heavy, and exhaustive-list-heavy for `docs/specs/test-suites/test-rel03.0-rest-tools.yaml`. We keep that warning because the file is a formal YAML test suite. Its required fields are lists of inputs, expected facts, and traces, and rewriting those lists into prose would make the suite less useful to the spec graph. The checker also reports uniform paragraphs in this Markdown audit; lexical checks are clean, and the paragraphs stay short to preserve audit readability.

No drift remains.

## Follow-Up Issues

`agent-core-usbz.1` covers completed REST server endpoint binding work. `agent-core-usbz.2` covers completed REST side-effect vocabulary alignment with the contract audit. `agent-core-0l2i.1`, `agent-core-0l2i.2`, and `agent-core-0l2i.3` cover completed REST safety work for credential resolution, redirect and size policies, server validation, and redaction.

No further REST follow-up remains from this audit.
