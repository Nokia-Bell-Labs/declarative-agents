// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRESTServerMachineRequestTerminalStatusMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		signal    string
		fail      bool
		status    int
		location  string
		bodyKey   string
		bodyValue string
		runStatus core.RunStatus
	}{
		{
			name: "2xx success", signal: "DocumentationReady", status: http.StatusCreated,
			bodyKey: "greeting", bodyValue: "hello alice", runStatus: core.StatusSucceeded,
		},
		{
			name: "3xx success without automatic redirect", signal: "DocumentationReady", status: http.StatusFound,
			location: "/not-followed", bodyKey: "greeting", bodyValue: "hello alice", runStatus: core.StatusSucceeded,
		},
		{
			name: "4xx failure", signal: "DocumentMissing", status: http.StatusNotFound,
			bodyKey: "error", bodyValue: "missing document", runStatus: core.StatusFailed,
		},
		{
			name: "5xx failure", signal: "DocumentationReady", fail: true, status: http.StatusInternalServerError,
			bodyKey: "error", bodyValue: "command failed", runStatus: core.StatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := machineRequestConfig(tt.signal, 0, tt.fail)
			terminalSignal := tt.signal
			if tt.fail {
				terminalSignal = string(core.CommandError)
			}
			cfg.MachineSpec = &core.MachineSpec{
				Name:           "status-mapping",
				InitialState:   "Start",
				States:         core.StateSpecsFromNames("Start", "Responding", terminalSignal),
				TerminalStates: []string{terminalSignal},
				Signals:        core.SignalSpecsFromNames(string(core.Seed), terminalSignal),
				Transitions: []core.TransitionSpec{
					{State: "Start", Signal: string(core.Seed), Next: "Responding", Action: "respond"},
					{State: "Responding", Signal: terminalSignal, Next: terminalSignal},
				},
			}
			mapping := cfg.Response.TerminalSignals[terminalSignal]
			mapping.Status = tt.status
			if tt.location != "" {
				mapping.Headers = map[string]string{"Location": tt.location}
			}
			cfg.Response.TerminalSignals[terminalSignal] = mapping
			state, baseURL := launchMachineRequestServerWithConfig(t, cfg)
			defer stopRESTServer(t, state, "machine")

			req, err := http.NewRequest(http.MethodPost, baseURL+"/docs", strings.NewReader(`{"name":"alice"}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

			require.Equal(t, tt.status, resp.StatusCode)
			require.Equal(t, tt.location, resp.Header.Get("Location"))
			require.Equal(t, tt.bodyValue, body[tt.bodyKey])
			trace := body["trace"].(map[string]interface{})
			require.Equal(t, terminalSignal, trace["terminal_signal"])
			require.Equal(t, string(tt.runStatus), trace["status"])
		})
	}
}

func TestRESTServerMachineRequestRecordsMonitorEvents(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore(monitor.Limits{Events: 32, Samples: 8})
	state := NewServerState()
	server := machineRequestServer(machineRequestConfig("DocumentationReady", 0, false))
	def := ServerDefinition{
		Name: "machine", Server: server, Limits: LimitProfile{},
		Monitor: MonitorState{Store: store},
	}
	_, baseURL := launchRESTServerDefinition(t, state, def)
	defer stopRESTServer(t, state, "machine")

	_ = postJSON(t, baseURL+"/docs", `{"name":"mon"}`, http.StatusOK)

	snap := store.Snapshot()
	var sawRespond bool
	for _, ev := range snap.RecentEvents {
		if ev.CommandName == "respond" {
			sawRespond = true
			break
		}
	}
	require.True(t, sawRespond, "expected request-machine dispatch in monitor events: %#v", snap.RecentEvents)
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

func TestRESTServerMachineRequestTimeout(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMachineRequestServer(t, "DocumentationReady", 50*time.Millisecond, false)
	defer stopRESTServer(t, state, "machine")

	body := requestBody(t, http.MethodPost, baseURL+"/docs", `{"name":"slow"}`, http.StatusGatewayTimeout)

	require.Contains(t, body, "machine_timeout")
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
	require.ErrorContains(t, err, "reserved target-format field")

	_, err = ParseDefinition([]byte(machineRequestDocumentResourcesYAML()))
	require.ErrorContains(t, err, "machine_request.document_resources")
	require.ErrorContains(t, err, "profile-selected filesystem resource ToolDefs")

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

func TestRESTServerMachineRequestOpenAPIBindKeepsExplicitCatchAllPath(t *testing.T) {
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
	endpoint := def.Servers["machine"].Endpoints["document"]
	endpoint.Method = "GET"
	endpoint.Path = "/docs/{path...}"
	endpoint.Request = RequestBinding{Path: map[string]interface{}{"path": map[string]interface{}{"type": "string"}}}
	def.Servers["machine"].Endpoints["document"] = endpoint
	require.NoError(t, CompileOpenAPIImports(&def, dir))
	endpoint = def.Servers["machine"].Endpoints["document"]
	require.Equal(t, "/docs/{path...}", endpoint.Path)
	state, baseURL := launchMachineRequestServerWithConfig(t, endpoint.MachineRequest, def.Servers["machine"].Endpoints)
	defer stopRESTServer(t, state, "machine")

	body := getJSON(t, baseURL+"/docs/specs/use-cases/uc007.yaml")

	require.Equal(t, "specs/use-cases/uc007.yaml", body["path"])
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
