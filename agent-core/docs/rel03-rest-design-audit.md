# Release 03.0 REST Design Audit

Issue `agent-core-g60h` audits the REST runtime package and its release evidence. Main evidence comes from `internal/tools/rest/` and `tools/builtin/rest/all.yaml`. Profile evidence lives under `agents/rest/`; fixture evidence lives under `testdata/integration/rel03-rest-tools/`; release status evidence lives in `docs/SPECIFICATIONS.yaml`.

Method: I treated a REST behavior as supported only when a requirement, profile or config artifact, and Go test pointed to the same behavior. Partial support stays visible as follow-up work.

No code moved.

## Verdict

Client paths work. Async paths work. Inbound signal paths work. OpenAPI import paths work. Code follows the profile-first tool language model for those paths. Two non-blocking gaps remain. `agent-core-usbz.1` covers `invoke_handler` and `stream_events` endpoint bindings from `srd029-rest-server-tools`. Listener side-effect names that do not yet fit the contract audit vocabulary are tracked by `agent-core-usbz.2`.

Release 03.0 should stay `in_progress` until those follow-ups close or the affected scope moves out of release 03.0.

No status is upgraded to `done` by this audit.

Release readiness is close. Care remains. A REST tool is not one feature; it is a contract among specs and profile assets, ToolDefs and builders, HTTP behavior and machine transitions. Status changes are valid only when those artifacts agree. Client flows have that agreement. Inbound signal receipt has it too. Shutdown and OpenAPI import also have it. For the remaining server binding names, code currently returns an unimplemented handler response instead of executing configured behavior. Listener effects use SRD language that is clearer than the current audit vocabulary. Those two mismatches are small enough to track separately, but they are real release criteria.

## Design Constitution

### D1 Specification-Driven Development

D1 passes. Implementation follows the three REST SRDs and `test-rel03.0-rest-tools`. Go evidence exists in REST client tests, async tests, server tests, and OpenAPI tests under `internal/tools/rest/`, which gives the release suite executable proof across outbound calls, async state, inbound queues, and import compilation. Those tests cover the core REST flows named by the release suite. Remaining endpoint binding work is tracked in `agent-core-usbz.1`.

### D2 YAML-First Structured Docs

D2 passes. Sample YAML lives under `agents/rest/`. It covers profile setup and machine grammar. It also covers tool selection, REST config, and OpenAPI input. Fixture copies live in `testdata/integration/rel03-rest-tools/`. Markdown fits this report because the output is prose review material.

### D3 Traceability

D3 passes with follow-ups. `docs/SPECIFICATIONS.yaml` links release 03.0 to `test-rel03.0-rest-tools` and leaves the release in progress. Named Go tests exist in `internal/tools/rest/`. The SRD029 binding gap is `agent-core-usbz.1`; side-effect vocabulary alignment is `agent-core-usbz.2`.

### D4 Profile-First Runtime Docs

D4 passes. `agents/rest/profile.yaml` loads REST definitions through the profile contract implemented by `internal/tools/catalog/profile.go` and used by `cmd/agent/main.go`. Tool declarations reference named REST config through `rest_ref`, `resource`, and `operation`. No REST sample describes a separate REST binary.

### D5 Tool Language Boundary

D5 passes with follow-ups. ToolDefs in `tools/builtin/rest/all.yaml` and `agents/rest/declarations.yaml` declare the contract metadata required by the tool language. Sequencing for the sample payment flow lives in `agents/rest/machine.yaml`, not in Go command code. Shared REST ToolDefs still need vocabulary alignment for listener effects, tracked by `agent-core-usbz.2`.

## Execution And Go Style

### Package Boundaries

Package boundaries pass. Implementation lives under `internal/tools/rest/`. Factory registration runs through `internal/tools/registry` from `internal/tools/rest/factories.go`. Profile loading remains in `internal/tools/catalog/profile.go`. Runtime wiring in `cmd/agent/main.go` loads definitions and registers factories without service-specific branches.

### Function And File Size

Function and file size checks pass through `mage lint`, `go vet ./...`, and `go test ./... -count=1`. Package files split client behavior from server behavior. Other files cover OpenAPI import, validation logic, and factory registration.

### Hidden Workflow Sequencing

Workflow ownership passes. Client send and await are separate words in `client_command.go` and `client_async.go`. Launch, await, and stop are separate server words in `command.go` and `server_state.go`. HTTP handlers in `server_routes.go` validate requests and enqueue events; they do not select MachineSpec actions. Sample sequencing appears in `agents/rest/machine.yaml`.

### Validation And Safety

Validation passes with one follow-up. The runtime rejects undeclared params and runtime authority overrides. Config-policy checks cover auth and redirects. They also cover public listener policy, async retry rules, and OpenAPI operation IDs. Route handling checks HTTP method and body limits. It also checks simple body schema types and queue capacity. `invoke_handler` and `stream_events` are not configured bindings yet; `agent-core-usbz.1` tracks that work.

## REST ToolDefs

Shared REST ToolDefs pass with one follow-up. `tools/builtin/rest/all.yaml` includes the expected boundary contract fields. `rest_server_launch` and `rest_server_stop` use `network_listen` and `network_listener_shutdown`. Those names match REST SRD language, but the current contract audit vocabulary does not accept them when the shared declarations are selected directly. `agent-core-usbz.2` tracks the fix.

Sample REST ToolDefs pass. `agents/rest/declarations.yaml` loads through the normal profile path. `mage audit` validates the selected REST profile, including declared emits against `agents/rest/machine.yaml`.

## Quality Gates

Code and documentation gates passed through the standard Go and Mage checks. The de-AI gate passed after wording and structure cleanup.

## Follow-Up Issues

`agent-core-usbz.1` covers remaining REST server endpoint bindings or release scope narrowing. `agent-core-usbz.2` covers REST side-effect vocabulary alignment with the contract audit.
