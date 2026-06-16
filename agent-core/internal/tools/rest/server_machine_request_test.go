// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

func TestRESTServerMachineRequestSuccess(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentationReady", 0, false)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"alice"}`, http.StatusOK)

	require.Equal(t, "hello alice", body["greeting"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTServerMachineRequestTraceEnvelope(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentationReady", 0, false)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"trace"}`, http.StatusOK)

	trace := body["trace"].(map[string]interface{})
	require.Equal(t, "machine", trace["server"])
	require.Equal(t, "docs", trace["route"])
	require.Equal(t, "request", trace["machine"])
	require.Equal(t, "DocumentationReady", trace["terminal_signal"])
	require.NotZero(t, trace["iterations"])
	require.Equal(t, string(core.StatusSucceeded), trace["status"])
}

func TestRESTServerMachineRequestMissingResource(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentMissing", 0, false)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"missing"}`, http.StatusNotFound)

	require.Equal(t, "missing document", body["error"])
	require.Equal(t, "DocumentMissing", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTServerMachineRequestTimeout(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentationReady", 50*time.Millisecond, false)
	defer stopRESTServer(t, state, "machine")

	body := requestBody(t, http.MethodPost, baseURL+"/docs", `{"name":"slow"}`, http.StatusGatewayTimeout)

	require.Contains(t, body, "machine_timeout")
}

func TestRESTServerMachineRequestCommandError(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentationReady", 0, true)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"broken"}`, http.StatusInternalServerError)

	require.Equal(t, "command failed", body["error"])
	require.Equal(t, "CommandError", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTServerMachineRequestTerminalResponseSchema(t *testing.T) {
	t.Parallel()
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	mapping := cfg.Response.TerminalSignals["DocumentationReady"]
	mapping.Schema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"greeting": map[string]interface{}{"type": "integer"},
		},
		"required": []interface{}{"greeting"},
	}
	cfg.Response.TerminalSignals["DocumentationReady"] = mapping
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg)
	defer stopRESTServer(t, state, "machine")

	body := requestBody(t, http.MethodPost, baseURL+"/docs", `{"name":"schema"}`, http.StatusBadGateway)

	require.Contains(t, body, "response_invalid")
	require.Contains(t, body, "terminal response schema")
}

func TestRESTServerMachineRequestConformanceLoadsConfiguredMachineFile(t *testing.T) {
	t.Parallel()
	cfg := conformanceMachineRequestConfig()
	dir := writeConformanceProfile(t)
	runner := conformanceProfileRunner(dir)
	state, baseURL := launchMachineRequestServerWithRunner(t, cfg, runner)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"profile"}`, http.StatusOK)

	require.Equal(t, "hello profile", body["greeting"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTDocumentResourcesConfigConformance(t *testing.T) {
	t.Parallel()

	_, err := ParseDefinition([]byte(restDocumentResourcesYAML()))
	require.ErrorContains(t, err, "rest.document_resources")

	_, err = ParseDefinition([]byte(machineRequestDocumentResourcesYAML()))
	require.ErrorContains(t, err, "machine_request.document_resources")

	cfg := conformanceMachineRequestConfig()
	runner := conformanceProfileRunner(writeConformanceProfile(t))
	state, baseURL := launchMachineRequestServerWithRunner(t, cfg, runner)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"profile"}`, http.StatusOK)
	require.Equal(t, "hello profile", body["greeting"])
}

func TestProfileMachineRequestRunnerRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  MachineRequest
		dir  func(*testing.T) string
		want string
	}{
		{name: "missing profile", cfg: MachineRequest{}, dir: tempProfileDir, want: "profile is required"},
		{name: "missing machine", cfg: MachineRequest{Profile: "profile.yaml"}, dir: writeProfileWithoutMachine, want: "machine is required"},
		{name: "missing selected tool", cfg: MachineRequest{Profile: "profile.yaml"}, dir: writeProfileWithoutRespondTool, want: "respond"},
		{name: "unresolved response signal", cfg: unresolvedResponseConfig(), dir: writeConformanceProfile, want: "terminal signal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := conformanceProfileRunner(tc.dir(t))
			_, err := runner.RunMachineRequest(context.Background(), MachineRequestRun{Config: tc.cfg})
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestRESTServerMachineRequestConfiguredInitialSignal(t *testing.T) {
	t.Parallel()
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.InitialSignal = "ReadRequested"
	cfg.MachineSpec = requestReadMachineSpec()
	cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
		return nil
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
	defer stopRESTServer(t, state, "machine")

	body := getJSON(t, baseURL+"/docs/VISION.yaml")

	require.Equal(t, "VISION.yaml", body["path"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTServerMachineRequestMatchesCatchAllPath(t *testing.T) {
	t.Parallel()
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
		return nil
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
	defer stopRESTServer(t, state, "machine")

	body := getJSON(t, baseURL+"/docs/specs/use-cases/uc007.yaml")

	require.Equal(t, "specs/use-cases/uc007.yaml", body["path"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTServerMachineRequestOpenAPIBindPreservesConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "docs.yaml"), docsOpenAPI())
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.InitialSignal = "ReadRequested"
	cfg.Request = MachineRequestMapping{Path: map[string]string{"path": "$.path"}}
	cfg.MachineSpec = requestReadMachineSpec()
	cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
		return nil
	}
	def := openAPIMachineRequestDefinition(cfg)
	require.NoError(t, CompileOpenAPIImports(&def, dir))
	endpoint := def.Servers["machine"].Endpoints["document"]
	require.Equal(t, "GET", endpoint.Method)
	require.Equal(t, "/docs/{path}", endpoint.Path)
	require.Equal(t, bindingMachineRequest, endpoint.Binding)
	require.NotEmpty(t, endpoint.MachineRequest.Response.TerminalSignals)
	state, baseURL := launchMachineRequestServerWithConfig(t, endpoint.MachineRequest, def.Servers["machine"].Endpoints)
	defer stopRESTServer(t, state, "machine")

	body := getJSON(t, baseURL+"/docs/VISION.yaml")

	require.Equal(t, "VISION.yaml", body["path"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestValidateDefinitionRejectsCatchAllPathErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "malformed", path: "/docs/{path..}", want: "malformed path param"},
		{name: "not final", path: "/docs/{path...}/raw", want: "must be final"},
		{name: "ambiguous", path: "/docs/{path}/{path}", want: "ambiguous"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := machineRequestDefinitionWithPath(tc.path)
			require.ErrorContains(t, ValidateDefinition(def), tc.want)
		})
	}
}

func launchMachineRequestServer(
	t *testing.T,
	signal string,
	delay time.Duration,
	fail bool,
) (*ServerState, string) {
	t.Helper()
	return launchMachineRequestServerWithConfig(t, machineRequestConfig(signal, delay, fail))
}

func launchMachineRequestServerWithConfig(
	t *testing.T,
	cfg MachineRequest,
	endpoints ...map[string]Endpoint,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	server := machineRequestServer(cfg)
	if len(endpoints) > 0 {
		server.Endpoints = endpoints[0]
	}
	def := ServerDefinition{Name: "machine", Server: server, MachineRequestRunner: nil}
	result := ServerBuilder{
		ToolName: "rest_server_launch", Init: InitServerLaunch, Server: def, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return state, "http://" + output["address"].(string)
}

func launchMachineRequestServerWithRunner(
	t *testing.T,
	cfg MachineRequest,
	runner MachineRequestRunner,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	server := machineRequestServer(cfg)
	def := ServerDefinition{Name: "machine", Server: server, MachineRequestRunner: runner}
	result := ServerBuilder{
		ToolName: "rest_server_launch", Init: InitServerLaunch, Server: def, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return state, "http://" + output["address"].(string)
}

func catchAllDocsEndpoint(cfg MachineRequest) map[string]Endpoint {
	cfg.Request = MachineRequestMapping{Path: map[string]string{"path": "$.path"}}
	return map[string]Endpoint{
		"document": {
			Method: "GET", Path: "/docs/{path...}", Binding: bindingMachineRequest,
			Request:        RequestBinding{Path: map[string]interface{}{"path": map[string]interface{}{"type": "string"}}},
			MachineRequest: cfg,
		},
	}
}

func machineRequestServer(cfg MachineRequest) Server {
	return Server{
		Address:  "127.0.0.1:0",
		Queue:    QueueConfig{Name: "machine", Timeout: "20ms"},
		Shutdown: ShutdownConfig{Timeout: "200ms"},
		Endpoints: map[string]Endpoint{
			"docs": {
				Method: "POST", Path: "/docs", Binding: bindingMachineRequest,
				Request:        RequestBinding{BodySchema: bodySchemaWithRequired("name")},
				MachineRequest: cfg,
			},
		},
	}
}

func machineRequestConfig(signal string, delay time.Duration, fail bool) MachineRequest {
	return MachineRequest{
		Timeout: "10ms",
		Request: MachineRequestMapping{Body: map[string]string{
			"name": "$.name",
		}},
		Response: MachineRequestResponse{TerminalSignals: map[string]MachineResponseMapping{
			"DocumentationReady": {Status: 200, Body: map[string]string{"greeting": "$.greeting"}},
			"DocumentMissing":    {Status: 404, Body: map[string]string{"error": "$.message"}},
			"CommandError":       {Status: 500, Body: map[string]string{"error": "$.message"}},
		}},
		MachineSpec: requestMachineSpec(),
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: "respond"}, responseBuilder{signal: core.Signal(signal), delay: delay, fail: fail})
			return nil
		},
	}
}

func conformanceMachineRequestConfig() MachineRequest {
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.MachineSpec = nil
	cfg.InitFunc = nil
	cfg.Timeout = "2s"
	cfg.Profile = "profile.yaml"
	cfg.Machine = "request-machine.yaml"
	cfg.Response.TerminalSignals = map[string]MachineResponseMapping{
		"DocumentationReady": {Status: 200, Body: map[string]string{"greeting": "$.greeting"}},
	}
	return cfg
}

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

func (c responseCommand) Undo() core.Result { return core.NoopUndo(c.Name()) }

func requestName(input string) string {
	var req MachineRequestRun
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "unknown"
	}
	name, _ := req.Payload["name"].(string)
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

func (c pathEchoCommand) Undo() core.Result { return core.NoopUndo(c.Name()) }

func requestPath(input string) string {
	var req MachineRequestRun
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return ""
	}
	path, _ := req.Payload["path"].(string)
	return path
}

func conformanceProfileRunner(dir string) *ProfileMachineRequestRunner {
	return NewProfileMachineRequestRunner(ProfileMachineRequestRunnerDeps{
		BaseDir:   dir,
		Directory: dir,
		RegisterBuiltins: func(br *toolregistry.BuiltinRegistry, selected map[string]bool) {
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
