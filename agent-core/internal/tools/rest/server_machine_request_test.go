// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func TestRESTServerMachineRequestSuccess(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentationReady", 0, false)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"alice"}`, http.StatusOK)

	require.Equal(t, "hello alice", body["greeting"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
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

func TestRESTServerMachineRequestConformanceLoadsConfiguredMachineFile(t *testing.T) {
	t.Parallel()
	requireMachineRequestConformance(t)
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.MachineSpec = nil
	cfg.Profile = writeConformanceProfile(t)
	cfg.Machine = writeConformanceMachine(t, filepath.Dir(cfg.Profile))
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"profile"}`, http.StatusOK)

	require.Equal(t, "hello profile", body["greeting"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
}

func TestRESTServerMachineRequestConformanceMatchesCatchAllPath(t *testing.T) {
	t.Parallel()
	requireMachineRequestConformance(t)
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
		return nil
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
	defer stopRESTServer(t, state, "machine")

	raw := requestBody(t, http.MethodGet, baseURL+"/docs/specs/use-cases/uc007.yaml", "", http.StatusOK)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))

	require.Equal(t, "specs/use-cases/uc007.yaml", body["path"])
	require.Equal(t, "DocumentationReady", body["trace"].(map[string]interface{})["terminal_signal"])
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

func catchAllDocsEndpoint(cfg MachineRequest) map[string]Endpoint {
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

func requireMachineRequestConformance(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENT_CORE_MACHINE_REQUEST_CONFORMANCE") != "1" {
		t.Skip("set AGENT_CORE_MACHINE_REQUEST_CONFORMANCE=1 to run failing-first conformance tests")
	}
}

func writeConformanceProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte("name: conformance\nmachine: request-machine.yaml\n"), 0o644))
	return profile
}

func writeConformanceMachine(t *testing.T, dir string) string {
	t.Helper()
	machine := filepath.Join(dir, "request-machine.yaml")
	require.NoError(t, os.WriteFile(machine, []byte(conformanceMachineYAML), 0o644))
	return machine
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
