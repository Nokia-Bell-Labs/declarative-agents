// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requestMachineSpec() *core.MachineSpec {
	return &core.MachineSpec{
		Name:           "request",
		InitialState:   "Start",
		States:         core.StateSpecsFromNames("Start", "Responding", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("Seed", "DocumentationReady", "DocumentMissing", "CommandError"),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Seed", Next: "Responding", Action: "respond"},
			{State: "Responding", Signal: "DocumentationReady", Next: "Done"},
			{State: "Responding", Signal: "DocumentMissing", Next: "Done"},
			{State: "Responding", Signal: "CommandError", Next: "Failed"},
		},
	}
}

func requestReadMachineSpec() *core.MachineSpec {
	return &core.MachineSpec{
		Name:           "request",
		InitialState:   "Start",
		States:         core.StateSpecsFromNames("Start", "Responding", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("ReadRequested", "DocumentationReady"),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "ReadRequested", Next: "Responding", Action: "respond"},
			{State: "Responding", Signal: "DocumentationReady", Next: "Done"},
		},
	}
}

type responseBuilder struct {
	signal core.Signal
	delay  time.Duration
	fail   bool
}

func (b responseBuilder) Build(res core.Result) core.Command {
	return responseCommand{input: res.Output, signal: b.signal, delay: b.delay, fail: b.fail}
}

type responseCommand struct {
	input  string
	signal core.Signal
	delay  time.Duration
	fail   bool
}

func (c responseCommand) Name() string { return "respond" }

func (c responseCommand) Execute() core.Result {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	if c.fail {
		return core.Result{Signal: core.CommandError, Output: `{"message":"command failed"}`, Err: errCommandFailed{}}
	}
	name := requestName(c.input)
	if c.signal == "DocumentMissing" {
		return core.Result{Signal: c.signal, Output: `{"message":"missing document"}`}
	}
	return core.Result{Signal: c.signal, Output: `{"greeting":"hello ` + name + `"}`}
}

func (c responseCommand) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func requestName(input string) string {
	var seed struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(input), &seed); err != nil {
		return "unknown"
	}
	name, _ := seed.Parameters["name"].(string)
	return strings.TrimSpace(name)
}

type errCommandFailed struct{}

func (errCommandFailed) Error() string { return "command failed" }

func machineRequestDefinitionWithPath(path string) Definition {
	return Definition{
		Version: "v1",
		Servers: map[string]Server{"machine": {
			Address: "127.0.0.1:0",
			Endpoints: map[string]Endpoint{"document": {
				Method: "GET", Path: path, Binding: bindingMachineRequest,
				Request:        RequestBinding{Path: map[string]interface{}{"path": map[string]interface{}{"type": "string"}}},
				MachineRequest: machineRequestConfig("DocumentationReady", 0, false),
			}},
		}},
	}
}

func openAPIMachineRequestDefinition(cfg MachineRequest) Definition {
	return Definition{
		Version: "v1",
		OpenAPI: map[string]OpenAPIImport{"docs": {
			Path: "docs.yaml",
			Bind: map[string]string{"readDocument": "document"},
		}},
		Servers: map[string]Server{"machine": {
			Address: "127.0.0.1:0",
			Endpoints: map[string]Endpoint{"document": {
				Binding:        bindingMachineRequest,
				MachineRequest: cfg,
				Response:       ResponseMapping{Output: map[string]string{"path": "$.path"}},
			}},
		}},
	}
}

func docsOpenAPI() string {
	return `openapi: 3.0.3
info: {title: Docs, version: v1}
paths:
  /docs/{path}:
    get:
      operationId: readDocument
      parameters:
        - name: path
          in: path
          required: true
          schema: {type: string}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  path: {type: string}
`
}

type pathEchoBuilder struct{}

type pathEchoCommand struct{ input string }

func (pathEchoBuilder) Build(res core.Result) core.Command {
	return pathEchoCommand{input: res.Output}
}

func (c pathEchoCommand) Name() string { return "respond" }

func (c pathEchoCommand) Execute() core.Result {
	return core.Result{Signal: "DocumentationReady", Output: `{"path":"` + requestPath(c.input) + `"}`}
}

func (c pathEchoCommand) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func requestPath(input string) string {
	var seed struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(input), &seed); err != nil {
		return ""
	}
	path, _ := seed.Parameters["path"].(string)
	return path
}

func conformanceProfileRunner(dir string) *ProfileMachineRequestRunner {
	return NewProfileMachineRequestRunner(ProfileMachineRequestRunnerDeps{
		BaseDir:   dir,
		Directory: dir,
		RegisterBuiltins: func(br *toolregistry.BuiltinRegistry, selected map[string]bool, _ *core.Registry) {
			br.Register("test_machine_request_respond", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
				return responseBuilder{signal: "DocumentationReady"}, nil
			})
		},
	})
}

func tempProfileDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func unresolvedResponseConfig() MachineRequest {
	cfg := MachineRequest{Profile: "profile.yaml"}
	cfg.Response.TerminalSignals = map[string]MachineResponseMapping{"UnknownReady": {Status: 200}}
	return cfg
}

func writeConformanceProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeProfileFiles(t, dir, "machine: request-machine.yaml\n")
	writeConformanceMachine(t, dir)
	writeConformanceDeclarations(t, dir, true)
	return dir
}

func writeProfileWithoutMachine(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeProfileFiles(t, dir, "")
	return dir
}

func writeProfileWithoutRespondTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeProfileFiles(t, dir, "machine: request-machine.yaml\n")
	writeConformanceMachine(t, dir)
	writeConformanceDeclarations(t, dir, false)
	return dir
}

func writeProfileFiles(t *testing.T, dir string, machineLine string) {
	t.Helper()
	data := "name: conformance\n" + machineLine + "tools:\n  - tools.yaml\ntool_declarations:\n  - declarations.yaml\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(data), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte("tools:\n  - respond\n"), 0o644))
}

func writeConformanceMachine(t *testing.T, dir string) {
	t.Helper()
	machine := filepath.Join(dir, "request-machine.yaml")
	require.NoError(t, os.WriteFile(machine, []byte(conformanceMachineYAML), 0o644))
}

func writeConformanceDeclarations(t *testing.T, dir string, includeRespond bool) {
	t.Helper()
	data := "tools: []\n"
	if includeRespond {
		data = conformanceDeclarationsYAML
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "declarations.yaml"), []byte(data), 0o644))
}

const conformanceMachineYAML = `name: request
initial_state: Start
states:
  - name: Start
  - name: Responding
  - name: Done
terminal_states:
  - Done
signals:
  - name: Seed
  - name: DocumentationReady
transitions:
  - state: Start
    signal: Seed
    next: Responding
    action: respond
  - state: Responding
    signal: DocumentationReady
    next: Done
`

const conformanceDeclarationsYAML = `tools:
  - name: respond
    type: builtin
    init: test_machine_request_respond
    emits: [DocumentationReady]
`

func restDocumentResourcesYAML() string {
	return `rest:
  version: v1
  document_resources:
    documentation_corpus:
      root: docs
      extensions: [.yaml]
      response_modes: [parsed_yaml]
      operations:
        get:
          type: get
          success_signal: DocumentReady
`
}

func machineRequestDocumentResourcesYAML() string {
	return `rest:
  version: v1
  servers:
    machine:
      address: 127.0.0.1:0
      endpoints:
        docs:
          method: POST
          path: /docs
          binding: machine_request
          machine_request:
            profile: profile.yaml
            timeout: 2s
            document_resources: [documentation_corpus]
            request:
              body:
                name: $.name
            response:
              terminal_signals:
                DocumentationReady:
                  status: 200
`
}
