// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"net/http"
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

func launchMachineRequestServer(
	t *testing.T,
	signal string,
	delay time.Duration,
	fail bool,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	server := machineRequestServer(machineRequestConfig(signal, delay, fail))
	def := ServerDefinition{Name: "machine", Server: server, MachineRequestRunner: nil}
	result := ServerBuilder{
		ToolName: "rest_server_launch", Init: InitServerLaunch, Server: def, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return state, "http://" + output["address"].(string)
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
