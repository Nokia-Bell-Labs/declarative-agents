// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/profiles"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestMain(m *testing.M) {
	spec.SetAgentCoreInstallRoot(filepath.Clean(repoRootFromLifecycleRuntime()))
	os.Exit(m.Run())
}

func TestExitAgentStatusAndOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		config     ExitConfig
		previous   string
		wantOutput map[string]string
		wantErr    string
	}{
		{
			name: "omitted config defaults to success",
			wantOutput: map[string]string{
				"reason": "operator requested shutdown", "status": "success",
				"drain_policy": "", "signal": "AgentExited",
			},
		},
		{
			name:   "explicit success",
			config: ExitConfig{Reason: "operator", Status: "success", DrainPolicy: "stop_servers"},
			wantOutput: map[string]string{
				"reason": "operator", "status": "success",
				"drain_policy": "stop_servers", "signal": "AgentExited",
			},
		},
		{
			name:   "explicit failure",
			config: ExitConfig{Reason: "tool failed", Status: "failed"},
			wantOutput: map[string]string{
				"reason": "tool failed", "status": "failed",
				"drain_policy": "", "signal": "AgentExited",
			},
		},
		{
			name:     "unrelated runtime payload preserves config",
			config:   ExitConfig{Reason: "default reason", Status: "success", DrainPolicy: "stop_servers"},
			previous: `{"payload":{"operator":"tester"}}`,
			wantOutput: map[string]string{
				"reason": "default reason", "status": "success",
				"drain_policy": "stop_servers", "signal": "AgentExited",
			},
		},
		{
			name:     "runtime payload overrides config",
			config:   ExitConfig{Reason: "default reason", Status: "success", DrainPolicy: "stop_servers"},
			previous: `{"payload":{"reason":"runtime reason","status":"failed","drain_policy":"drain_then_stop","checkpoint_id":"cp-1"}}`,
			wantOutput: map[string]string{
				"reason": "runtime reason", "status": "failed", "drain_policy": "drain_then_stop",
				"checkpoint_id": "cp-1", "signal": "AgentExited",
			},
		},
		{
			name:     "malformed runtime status",
			previous: `{"payload":{"status":42}}`,
			wantErr:  `payload field "status" must be a string`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shutdownCalled := false
			res := (ExitBuilder{
				Config:   tt.config,
				Shutdown: func() { shutdownCalled = true },
			}).Build(core.Result{Output: tt.previous}).Execute()

			if tt.wantErr != "" {
				require.Equal(t, core.CommandError, res.Signal)
				require.ErrorContains(t, res.Err, tt.wantErr)
				assert.False(t, shutdownCalled)
				return
			}
			require.Equal(t, core.Signal("AgentExited"), res.Signal)
			require.Equal(t, "exit_agent", res.CommandName)
			assert.True(t, shutdownCalled)
			assert.Equal(t, tt.wantOutput, requireExitOutput(t, res))
		})
	}
}

func TestExitAgentRejectsMalformedPreviousResult(t *testing.T) {
	t.Parallel()
	cmd := (ExitBuilder{Shutdown: func() {}}).Build(core.Result{Output: `{"payload":`})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, "decode exit_agent previous result")
}

func TestExitAgentRequiresShutdownDependency(t *testing.T) {
	t.Parallel()
	res := (ExitBuilder{}).Build(core.Result{}).Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, "shutdown dependency")
}

func TestExitAgentUndoRequestsOperatorCompensation(t *testing.T) {
	t.Parallel()
	cmd := (ExitBuilder{Config: ExitConfig{Reason: "operator"}}).Build(core.Result{})

	res := cmd.Undo(core.Result{})

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "restart the agent or resume from a checkpoint")
}

func TestRESTLifecycleControl_ExitAgentSignal(t *testing.T) {
	t.Parallel()
	state, baseURL := launchControlServer(t)
	defer func() { _, _ = state.Stop("agent_control") }()

	postControl(t, baseURL+"/api/lifecycle/exit", `{"reason":"operator requested shutdown"}`)
	event, signal, err := state.AwaitAny(rest.AwaitAnyOptions{
		Sources: []rest.AwaitSource{{Server: "agent_control", Routes: []string{"exit"}}},
		Timeout: time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "exit", event.Route)
	require.Equal(t, "operator requested shutdown", event.Payload["reason"])
	res := (ExitBuilder{
		Config:   ExitConfig{Status: "success"},
		Shutdown: func() {},
	}).Build(core.Result{Output: mustJSON(t, event)}).Execute()
	require.Equal(t, core.Signal("AgentExited"), res.Signal)
	require.Equal(t, "operator requested shutdown", requireExitOutput(t, res)["reason"])
}

func TestRESTLifecycleControl_ModeledEndpointRoutesToExitAgent(t *testing.T) {
	t.Parallel()
	state, baseURL := launchModeledControlServer(t)
	defer func() { _, _ = state.Stop("modeled_control") }()

	postControl(t, baseURL+"/api/lifecycle/exit", `{"reason":"modeled shutdown"}`)
	event, signal, err := state.AwaitAny(rest.AwaitAnyOptions{
		Sources: []rest.AwaitSource{{Server: "modeled_control", Routes: []string{"exit"}}},
		Timeout: time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "modeled shutdown", event.Payload["reason"])
	res := (ExitBuilder{Config: ExitConfig{Status: "success"}, Shutdown: func() {}}).
		Build(core.Result{Output: mustJSON(t, event)}).Execute()
	require.Equal(t, core.Signal("AgentExited"), res.Signal)
	require.Equal(t, "modeled shutdown", requireExitOutput(t, res)["reason"])
}

func TestRegisterLifecycleFactoriesRegistersExitAgent(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	var shutdownCalled bool
	RegisterFactories(br, FactoryDeps{Shutdown: func() { shutdownCalled = true }})
	factory, ok := br.Resolve("exit_agent")
	require.True(t, ok)

	builder, err := factory(catalog.ToolDef{Name: "exit_agent", Init: "exit_agent"}, nil)
	require.NoError(t, err)
	res := builder.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("AgentExited"), res.Signal)
	require.True(t, shutdownCalled)
}

func TestControlProfileSelectsExitAgentFlow(t *testing.T) {
	t.Parallel()
	profile, err := catalog.LoadProfile(controlProfilePath(t))
	require.NoError(t, err)
	dirDefs, err := catalog.LoadToolDeclarationsFromDirs(profile.ToolConfigDirs)
	require.NoError(t, err)
	localDefs, err := catalog.LoadToolDeclarations(profile.ToolDeclarations)
	require.NoError(t, err)
	selection, err := catalog.LoadToolSelections(profile.Tools)
	require.NoError(t, err)

	defs, err := catalog.SelectTools(catalog.MergeToolDefs(dirDefs, localDefs), selection)
	require.NoError(t, err)
	machine, err := core.LoadMachineSpec(profile.Machine)
	require.NoError(t, err)

	require.NoError(t, catalog.ValidateToolEmits(machine, defs))
	require.Contains(t, toolNames(defs), "exit_agent")
	require.Contains(t, toolNames(defs), "await_agent_control")
}

func launchControlServer(t *testing.T) (*rest.ServerState, string) {
	t.Helper()
	collection, err := rest.LoadDefinitions([]string{controlRestPath(t)}, nil)
	require.NoError(t, err)
	def, err := collection.ResolveServer("agent_control")
	require.NoError(t, err)
	state := rest.NewServerState()
	output, err := state.Launch(def)
	require.NoError(t, err)
	return state, "http://" + output["address"].(string)
}

func launchModeledControlServer(t *testing.T) (*rest.ServerState, string) {
	t.Helper()
	state := rest.NewServerState()
	output, err := state.Launch(rest.ServerDefinition{
		Name: "modeled_control",
		Server: rest.Server{
			Address:   "127.0.0.1:0",
			Queue:     rest.QueueConfig{Name: "modeled_control", Capacity: 8, Overflow: "reject", Timeout: "30s"},
			Shutdown:  rest.ShutdownConfig{Timeout: "5s", DrainPolicy: "drain"},
			Endpoints: map[string]rest.Endpoint{"exit": modeledExitEndpoint()},
		},
	})
	require.NoError(t, err)
	return state, "http://" + output["address"].(string)
}

func modeledExitEndpoint() rest.Endpoint {
	return rest.Endpoint{
		Method: "POST", Path: "/api/lifecycle/exit", Binding: "lifecycle_control",
		LifecycleControl: rest.LifecycleControl{
			Action: "exit", Signal: "ExitRequested", TargetSchema: stringBodySchema("reason"),
		},
		Request:  rest.RequestBinding{BodySchema: stringBodySchema("reason")},
		Response: rest.ResponseMapping{Output: map[string]string{"accepted": "true"}},
	}
}

func stringBodySchema(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object", "required": []interface{}{field},
		"properties": map[string]interface{}{field: map[string]interface{}{"type": "string"}},
	}
}

func postControl(t *testing.T, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func controlRestPath(t *testing.T) string {
	t.Helper()
	return controlProfileAssetPath(t, "rest.yaml")
}

func controlProfilePath(t *testing.T) string {
	t.Helper()
	return controlProfileAssetPath(t, "profile.yaml")
}

func controlProfileAssetPath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(lifecycleProfileRoot(t), "control", filepath.FromSlash(rel))
}

// lifecycleProfileRoot locates the profile-owned lifecycle conformance
// fixtures. The fixtures belong to agent-profiles, so a checkout of agent-core
// alone is a supported state: the run skips rather than failing, which keeps
// `go test ./...` hermetic here the same way it is in agent-profiles
// (srd034 R3.3, R3.4; GH-512).
func lifecycleProfileRoot(t *testing.T) string {
	t.Helper()
	res := profiles.ResolveFrom(repoRootFromLifecycleTest(t))
	switch res.Outcome {
	case profiles.Invalid:
		t.Fatalf("%s; unset it to use a discovered checkout", res.Reason())
	case profiles.Absent:
		t.Skipf("lifecycle conformance fixtures unavailable: %s", res.Reason())
	}
	return res.ConformanceRoot()
}

func repoRootFromLifecycleTest(t *testing.T) string {
	t.Helper()
	return repoRootFromLifecycleRuntime()
}

func repoRootFromLifecycleRuntime() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func toolNames(defs []catalog.ToolDef) map[string]bool {
	names := make(map[string]bool, len(defs))
	for _, def := range defs {
		names[def.Name] = true
	}
	return names
}

func requireExitOutput(t *testing.T, result core.Result) map[string]string {
	t.Helper()
	output := map[string]string{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output
}

func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
