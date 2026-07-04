// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func launchRESTServer(t *testing.T, server Server, limits LimitProfile) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	_, baseURL := launchRESTServerWithState(t, state, server, limits)
	return state, baseURL
}

func launchRESTServerWithState(
	t *testing.T,
	state *ServerState,
	server Server,
	limits LimitProfile,
) (map[string]interface{}, string) {
	t.Helper()
	def := ServerDefinition{Name: serverName(server), Server: server, Limits: limits}
	return launchRESTServerDefinition(t, state, def)
}

func launchRESTServerDefinition(
	t *testing.T,
	state *ServerState,
	def ServerDefinition,
) (map[string]interface{}, string) {
	t.Helper()
	result := ServerBuilder{
		ToolName: "rest_server_launch", Init: InitServerLaunch, Server: def, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output, "http://" + output["address"].(string)
}

func decodedLaunchAddress(t *testing.T, output string) string {
	t.Helper()
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	return decoded["address"].(string)
}

func stopRESTServer(t *testing.T, state *ServerState, name string) map[string]interface{} {
	t.Helper()
	result := stopCommand(state, name).Execute()
	require.Equal(t, core.Signal("ServerStopped"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output
}

func requireAwaitSignal(t *testing.T, state *ServerState, name, signal string) {
	t.Helper()
	result := awaitCommand(state, name).Execute()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
}

func awaitCommand(state *ServerState, name string) core.Command {
	return ServerBuilder{
		ToolName: "rest_server_await", Init: InitServerAwait,
		Server: ServerDefinition{Name: name, Server: namedControlServer(name)}, State: state,
	}.Build(core.Result{})
}

func stopCommand(state *ServerState, name string) core.Command {
	return ServerBuilder{
		ToolName: "rest_server_stop", Init: InitServerStop,
		Server: ServerDefinition{Name: name, Server: namedControlServer(name)}, State: state,
	}.Build(core.Result{})
}

func awaitAnyResult(state *ServerState, source AwaitSource) core.Result {
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{source}, Timeout: time.Second,
	})
	output := map[string]interface{}{"source": event.Source}
	if err != nil {
		output["error"] = err.Error()
	}
	return core.Result{Signal: core.Signal(signal), Output: jsonOutput(output)}
}

func requireRESTToolDef(t *testing.T, init string) catalog.ToolDef {
	t.Helper()
	defs, err := catalog.LoadToolDefs(restDeclarationsPath(t))
	require.NoError(t, err)
	for _, def := range defs {
		if def.Init == init {
			return def
		}
	}
	require.Failf(t, "missing REST tool declaration", "init %q", init)
	return catalog.ToolDef{}
}

func requireRESTCommand(
	t *testing.T,
	def catalog.ToolDef,
	collection Collection,
	state *ServerState,
) core.Command {
	t.Helper()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection, ServerState: state})
	factory, ok := br.Resolve(def.Init)
	require.True(t, ok)
	builder, err := factory(def, nil)
	require.NoError(t, err)
	return builder.Build(core.Result{})
}

func launchRESTServerCommand(t *testing.T, collection Collection, state *ServerState, name string) string {
	t.Helper()
	def := requireRESTToolDef(t, InitServerLaunch)
	def.Name = "launch_" + name
	def.Config = map[string]interface{}{"rest_ref": name}
	result := requireRESTCommand(t, def, collection, state).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	return "http://" + decodedLaunchAddress(t, result.Output)
}

func awaitEventCommand(t *testing.T, collection Collection, state *ServerState, names ...string) core.Command {
	t.Helper()
	def := requireRESTToolDef(t, InitAwaitEvent)
	def.Config = map[string]interface{}{"sources": awaitEventSources(names)}
	return requireRESTCommand(t, def, collection, state)
}

func awaitEventSources(names []string) []interface{} {
	sources := make([]interface{}, 0, len(names))
	for _, name := range names {
		sources = append(sources, map[string]interface{}{"server": name})
	}
	return sources
}

func requireAwaitEventOutput(t *testing.T, result core.Result, source, signal string) {
	t.Helper()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
	require.Contains(t, result.Output, `"source":"`+source+`"`)
	require.Contains(t, result.Output, `"signal":"`+signal+`"`)
}

func postStatus(t *testing.T, url, body string, want int) {
	t.Helper()
	requestStatus(t, http.MethodPost, url, body, want)
}

func requestStatus(t *testing.T, method, url, body string, want int) {
	t.Helper()
	requestStatusWithHeaders(t, method, url, body, nil, want)
}

func requestStatusWithHeaders(
	t *testing.T,
	method string,
	url string,
	body string,
	headers map[string]string,
	want int,
) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(t, want, resp.StatusCode)
}

func requestBody(t *testing.T, method, url, body string, want int) string {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, want, resp.StatusCode, string(data))
	return string(data)
}

func postJSON(t *testing.T, url, body string, want int) map[string]interface{} {
	t.Helper()
	data := requestBody(t, http.MethodPost, url, body, want)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(data), &output))
	return output
}

func getJSON(t *testing.T, url string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var output map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&output))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return output
}

func streamResponse(url string, bodyC chan<- string, errC chan<- error) {
	resp, err := http.Get(url)
	if err != nil {
		errC <- err
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		errC <- err
		return
	}
	bodyC <- string(data)
	errC <- nil
}

func requireActiveStreams(t *testing.T, state *ServerState, name string, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		runtime, err := state.runtime(name)
		if err != nil {
			return false
		}
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.activeStreams == want
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func clientCommand(def Definition, init, operation string, input map[string]interface{}) core.Command {
	return clientCommandWithCredentials(def, init, operation, input, nil)
}

func clientCommandWithCredentials(
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	credentials CredentialResolver,
) core.Command {
	return clientCommandWithMetricsAndCredentials(def, init, operation, input, restMetrics(), credentials)
}

func clientCommandWithMetrics(
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	metrics core.MetricConfig,
) core.Command {
	return clientCommandWithMetricsAndCredentials(def, init, operation, input, metrics, nil)
}

func clientCommandWithMetricsAndCredentials(
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	metrics core.MetricConfig,
	credentials CredentialResolver,
) core.Command {
	collection := NewCollection()
	_ = collection.Add(def)
	resolved, _ := collection.ResolveClientOperation(ClientToolConfig{
		RestRef: "github", Resource: "issue", Operation: operation,
	})
	params, _ := json.Marshal(map[string]interface{}{"tool": init, "parameters": input})
	return ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, Credentials: credentials, Metrics: metrics,
	}.Build(core.Result{Output: string(params)})
}

func restMetrics() core.MetricConfig {
	return core.MetricConfig{
		Instruments: []core.MetricInstrument{
			{Name: "rest.http_status_code", Kind: "gauge", Unit: "1", Description: "HTTP status.", ValueSource: "http_status_code", Attributes: []string{"operation"}},
			{Name: "rest.retry_count", Kind: "counter", Unit: "{retry}", Description: "Retry count.", ValueSource: "retry_count", Attributes: []string{"operation"}},
			{Name: "rest.request_bytes", Kind: "histogram", Unit: "By", Description: "Request bytes.", ValueSource: "request_bytes", Attributes: []string{"operation"}},
			{Name: "rest.response_bytes", Kind: "histogram", Unit: "By", Description: "Response bytes.", ValueSource: "response_bytes", Attributes: []string{"operation"}},
		},
		Attributes: []core.MetricAttribute{{Name: "operation", Source: "configured_operation", Cardinality: "bounded", AllowedValues: []string{"get"}, Redaction: "none"}},
	}
}

func requireClientSignal(t *testing.T, def Definition, init, operation string, input map[string]interface{}, signal string) {
	t.Helper()
	result := clientCommand(def, init, operation, input).Execute()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
	require.Contains(t, result.Output, `"operation":"`+operation+`"`)
}

func clientDefinition(t *testing.T, baseURL string, client Client) Definition {
	t.Helper()
	client.BaseURL = baseURL
	client.AuthRef = "none"
	def := Definition{
		Version: "v1",
		Auth: map[string]AuthProfile{
			"none": {Type: authNone},
		},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{"github": client},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

func resolvedClientOperation(t *testing.T, def Definition) ClientOperationDefinition {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(def))
	resolved, err := collection.ResolveClientOperation(ClientToolConfig{
		RestRef: "github", Resource: "issue", Operation: "get",
	})
	require.NoError(t, err)
	return resolved
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
	_, baseURL := launchRESTServerDefinition(t, state, def)
	return state, baseURL
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
	_, baseURL := launchRESTServerDefinition(t, state, def)
	return state, baseURL
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

func launchMonitorRESTServer(
	t *testing.T,
	name string,
	monitorState MonitorState,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	server := monitorServer(name)
	def := ServerDefinition{Name: name, Server: server, Monitor: monitorState}
	_, baseURL := launchRESTServerDefinition(t, state, def)
	return state, baseURL
}

func launchMonitorRESTServerFromFactory(
	t *testing.T,
	name string,
	monitorState MonitorState,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	collection := NewCollection()
	require.NoError(t, collection.Add(Definition{Servers: map[string]Server{name: monitorServer(name)}}))
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection, ServerState: state, Monitor: monitorState})
	factory, ok := br.Resolve(InitServerLaunch)
	require.True(t, ok)
	builder, err := factory(monitorLaunchTool(name), nil)
	require.NoError(t, err)
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	return state, "http://" + decodedLaunchAddress(t, result.Output)
}

func monitorLaunchTool(name string) catalog.ToolDef {
	return catalog.ToolDef{
		Name: "launch_monitor_rest", Type: "builtin", Init: InitServerLaunch,
		Description: "Launch monitor REST test server.",
		Config:      map[string]interface{}{"rest_ref": name},
	}
}
