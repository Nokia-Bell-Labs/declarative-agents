// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	spec.SetAgentCoreInstallRoot(filepath.Clean(repoRootFromDocsRuntime()))
	os.Exit(m.Run())
}

type staticDocsSignalBuilder struct {
	name   string
	signal core.Signal
	output string
}

type staticDocsSignalCmd struct {
	name   string
	signal core.Signal
	output string
}

func (b staticDocsSignalBuilder) Build(_ core.Result) core.Command {
	return staticDocsSignalCmd(b)
}

func (c staticDocsSignalCmd) Name() string { return c.name }

func (c staticDocsSignalCmd) Execute() core.Result {
	return core.Result{Signal: c.signal, CommandName: c.name, Output: c.output}
}

func (c staticDocsSignalCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.name)
}

type fakeWorkflowRunner struct{}

func (fakeWorkflowRunner) Run(r *http.Request) (ActionResponse, error) {
	defer func() { _ = r.Body.Close() }()
	return ActionResponse{
		Data: map[string]interface{}{"status": "valid"},
		Tool: "doc_validate", Signal: "RESTResponded",
	}, nil
}

func repoRootFromDocsTest(t *testing.T) string {
	t.Helper()
	return repoRootFromDocsRuntime()
}

func repoRootFromDocsRuntime() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

const docsRuntimeToolsYAML = `tools:
  - launch_documentation
  - stop_documentation
  - launch_docs_control
  - await_docs_control
  - launch_monitor_rest
  - await_monitor_control
  - stop_monitor_rest
  - exit_agent
  - doc_list
  - doc_get
  - doc_search
  - doc_validate
  - doc_suggest_changes
  - doc_patch_approve
  - doc_patch_reject
  - doc_patch_reopen
  - doc_list_resource
  - doc_read_resource
  - doc_index_response
  - doc_detail_response
`

const docsRuntimeBuiltinYAML = `tools:
  - name: launch_documentation
    type: builtin
    init: launch_documentation
    emits: [ServerLaunched, CommandError]
  - name: stop_documentation
    type: builtin
    init: stop_documentation
    emits: [ServerStopped, CommandError]
  - name: launch_docs_control
    type: builtin
    init: rest_server_launch
    emits: [ServerLaunched, CommandError]
    config:
      rest_ref: docs_runtime_control
  - name: await_docs_control
    type: builtin
    init: rest_await_event
    emits: [ExitRequested, AwaitTimedOut, ServerStopped, CommandError]
    config:
      sources:
        - server: docs_runtime_control
          routes: [exit]
          signals: [ExitRequested]
      timeout: 30s
      stopped_behavior: emit_server_stopped
`

const docsRuntimeRequestDeclarationsYAML = `tools:
  - name: doc_list_resource
    type: builtin
    init: list_resource
    emits: [DocumentListReady, DocumentResourceDenied, CommandError]
    config:
      resources:
        docs:
          root: docs
          include: ["**/*.yaml", "**/*.yml", "**/*.md"]
          extensions: [yaml, yml, md]
          modes: [raw_yaml, parsed_yaml, raw_markdown]
          max_bytes: 1048576
  - name: doc_read_resource
    type: builtin
    init: read_resource
    emits: [DocumentReady, DocumentMissing, DocumentResourceDenied, DocumentParseFailed, CommandError]
    config:
      resources:
        docs:
          root: docs
          include: ["**/*.yaml", "**/*.yml", "**/*.md"]
          extensions: [yaml, yml, md]
          modes: [raw_yaml, parsed_yaml, raw_markdown]
          max_bytes: 1048576
  - name: doc_index_response
    type: builtin
    init: doc_index_response
    emits: [DocumentIndexReady, CommandError]
  - name: doc_detail_response
    type: builtin
    init: doc_detail_response
    emits: [DocumentDetailReady, CommandError]
`

const docsRuntimeRequestMachineYAML = `name: docs-runtime-request
initial_state: AwaitingRequest
budget:
  max_iterations: 4
states:
  - name: AwaitingRequest
  - name: ListingDocuments
  - name: ReadingDocument
  - name: ShapingDocumentIndex
  - name: ShapingDocumentDetail
  - name: DocumentIndexReady
  - name: DocumentDetailReady
  - name: DocumentNotFound
  - name: RequestDenied
  - name: Failed
terminal_states: [DocumentIndexReady, DocumentDetailReady, DocumentNotFound, RequestDenied, Failed]
signals:
  - name: Seed
  - name: ReadRequested
  - name: DocumentListReady
  - name: DocumentReady
  - name: DocumentIndexReady
  - name: DocumentDetailReady
  - name: DocumentMissing
  - name: DocumentResourceDenied
  - name: DocumentParseFailed
  - name: CommandError
transitions:
  - state: AwaitingRequest
    signal: Seed
    next: ListingDocuments
    action: doc_list_resource
  - state: AwaitingRequest
    signal: ReadRequested
    next: ReadingDocument
    action: doc_read_resource
  - state: ListingDocuments
    signal: DocumentListReady
    next: ShapingDocumentIndex
    action: doc_index_response
  - state: ShapingDocumentIndex
    signal: DocumentIndexReady
    next: DocumentIndexReady
  - state: ShapingDocumentIndex
    signal: CommandError
    next: Failed
  - state: ListingDocuments
    signal: DocumentResourceDenied
    next: RequestDenied
  - state: ListingDocuments
    signal: CommandError
    next: Failed
  - state: ReadingDocument
    signal: DocumentReady
    next: ShapingDocumentDetail
    action: doc_detail_response
  - state: ShapingDocumentDetail
    signal: DocumentDetailReady
    next: DocumentDetailReady
  - state: ShapingDocumentDetail
    signal: CommandError
    next: Failed
  - state: ReadingDocument
    signal: DocumentMissing
    next: DocumentNotFound
  - state: ReadingDocument
    signal: DocumentResourceDenied
    next: RequestDenied
  - state: ReadingDocument
    signal: DocumentParseFailed
    next: Failed
  - state: ReadingDocument
    signal: CommandError
    next: Failed
`

const docsRuntimeMachineYAML = `name: docs-runtime
initial_state: Idle
budget:
  max_iterations: 10000
states:
  - name: Idle
  - name: LaunchingDocs
  - name: LaunchingControl
  - name: LaunchingMonitor
  - name: AwaitingControl
  - name: Exiting
  - name: StoppingMonitor
  - name: StoppingDocs
  - name: Done
  - name: Failed
terminal_states: [Done, Failed]
signals:
  - name: Seed
  - name: ServerLaunched
  - name: ExitRequested
  - name: AgentExited
  - name: ServerStopped
  - name: AwaitTimedOut
  - name: CommandError
transitions:
  - state: Idle
    signal: Seed
    next: LaunchingDocs
    action: launch_documentation
  - state: LaunchingDocs
    signal: ServerLaunched
    next: LaunchingControl
    action: launch_docs_control
  - state: LaunchingDocs
    signal: CommandError
    next: Failed
  - state: LaunchingDocs
    signal: ServerStopped
    next: Failed
  - state: LaunchingControl
    signal: ServerLaunched
    next: LaunchingMonitor
    action: launch_monitor_rest
  - state: LaunchingControl
    signal: CommandError
    next: Failed
  - state: LaunchingMonitor
    signal: ServerLaunched
    next: AwaitingControl
    action: await_docs_control
  - state: LaunchingMonitor
    signal: CommandError
    next: Failed
  - state: AwaitingControl
    signal: ExitRequested
    next: Exiting
    action: exit_agent
  - state: AwaitingControl
    signal: AwaitTimedOut
    next: Failed
  - state: AwaitingControl
    signal: ServerStopped
    next: Failed
  - state: AwaitingControl
    signal: CommandError
    next: Failed
  - state: Exiting
    signal: AgentExited
    next: StoppingMonitor
    action: stop_monitor_rest
  - state: Exiting
    signal: CommandError
    next: Failed
  - state: StoppingMonitor
    signal: ServerStopped
    next: StoppingDocs
    action: stop_documentation
  - state: StoppingMonitor
    signal: CommandError
    next: Failed
  - state: StoppingDocs
    signal: ServerStopped
    next: Done
  - state: StoppingDocs
    signal: ServerLaunched
    next: Failed
  - state: StoppingDocs
    signal: CommandError
    next: Failed
`

const docsRuntimeUXYAML = `id: docs-runtime-ui
title: Docs Runtime UI
source_owner: agent-core/internal/knowledge/documentation
routes:
  - id: docs_index
    path: /docs
    label: Documentation
    action: doc_list
    resource: docs
  - id: docs_detail
    path: /docs/*
    label: Document Detail
    action: doc_get
    resource: docs
sidebar:
  title: Documentation
  groups:
    overview:
      label: Overview
      order: 0
actions:
  list_documents:
    ui_action: doc_list
    request_machine_action: doc_list_resource
    route: docs_index
  read_document:
    ui_action: doc_get
    request_machine_action: doc_read_resource
    route: docs_detail
  validate_document:
    ui_action: doc_validate
    route: docs_detail
  suggest_changes:
    ui_action: doc_suggest_changes
    route: docs_detail
  approve_patch:
    ui_action: doc_patch_approve
    route: docs_detail
  reject_patch:
    ui_action: doc_patch_reject
    route: docs_detail
  reopen_patch:
    ui_action: doc_patch_reopen
    route: docs_detail
presentation:
  raw_yaml_toggle: true
  state_diagram: true
  config_viewer: true
  source_viewer: true
`
