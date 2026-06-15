// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/rest"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

func TestExitAgentEmitsAgentExited(t *testing.T) {
	t.Parallel()
	var shutdownCalled bool
	cmd := (ExitBuilder{
		Config:   ExitConfig{Reason: "operator", Status: "success", DrainPolicy: "stop_servers"},
		Shutdown: func() { shutdownCalled = true },
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.Signal("AgentExited"), res.Signal)
	require.True(t, shutdownCalled)
	require.Contains(t, res.Output, "operator")
}

func TestExitAgentRequiresShutdownDependency(t *testing.T) {
	t.Parallel()
	res := (ExitBuilder{}).Build(core.Result{}).Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, "shutdown dependency")
}

func TestExitAgentUndoMementoIsOperatorCompensation(t *testing.T) {
	t.Parallel()
	cmd := (ExitBuilder{Config: ExitConfig{Reason: "operator"}}).Build(core.Result{})
	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)

	memento, err := provider.UndoMemento()

	require.NoError(t, err)
	require.NoError(t, core.ValidateUndoMemento(memento))
	require.Equal(t, core.UndoMementoCompensatable, memento.Kind)
	var payload undo.BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "operator_restart_or_checkpoint_resume", payload.BoundaryCompensation.Strategy)
}

func TestRESTLifecycleControlExitEventRoutesToExitAgent(t *testing.T) {
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

func postControl(t *testing.T, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func controlRestPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "agents", "control", "rest.yaml")
}

func controlProfilePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "agents", "control", "profile.yaml")
}

func toolNames(defs []catalog.ToolDef) map[string]bool {
	names := make(map[string]bool, len(defs))
	for _, def := range defs {
		names[def.Name] = true
	}
	return names
}
