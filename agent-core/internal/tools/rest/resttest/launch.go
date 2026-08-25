// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package resttest holds shared REST test helpers for internal/tools/rest and
// the subpackages extracted from it. Tests compiled as package rest cannot
// import this package (import cycle); they keep same-package copies until they
// move into an extracted subpackage.
package resttest

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

// SkipIfShortRESTLaunch skips when the fast suite must not bind a listener.
func SkipIfShortRESTLaunch(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration-grade: production REST server launch binds a real loopback listener")
	}
}

// LaunchRESTServer launches a loopback REST server and returns its state and base URL.
func LaunchRESTServer(t *testing.T, server toolrest.Server, limits toolrest.LimitProfile) (*toolrest.ServerState, string) {
	t.Helper()
	state := toolrest.NewServerState()
	_, baseURL := LaunchRESTServerWithState(t, state, server, limits)
	return state, baseURL
}

// LaunchRESTServerWithState launches a server into an existing ServerState.
func LaunchRESTServerWithState(
	t *testing.T,
	state *toolrest.ServerState,
	server toolrest.Server,
	limits toolrest.LimitProfile,
) (map[string]interface{}, string) {
	t.Helper()
	def := toolrest.ServerDefinition{Name: serverName(server), Server: server, Limits: limits}
	return LaunchRESTServerDefinition(t, state, def)
}

// LaunchRESTServerDefinition launches a resolved server definition.
func LaunchRESTServerDefinition(
	t *testing.T,
	state *toolrest.ServerState,
	def toolrest.ServerDefinition,
) (map[string]interface{}, string) {
	t.Helper()
	SkipIfShortRESTLaunch(t)
	result := toolrest.ServerBuilder{
		ToolName: "rest_server_launch", Init: toolrest.InitServerLaunch, Server: def, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output, "http://" + output["address"].(string)
}

// StopRESTServer stops a launched server and returns its stop output.
func StopRESTServer(t *testing.T, state *toolrest.ServerState, name string) map[string]interface{} {
	t.Helper()
	result := toolrest.ServerBuilder{
		ToolName: "rest_server_stop", Init: toolrest.InitServerStop,
		Server: toolrest.ServerDefinition{Name: name, Server: NamedControlServer(name)}, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerStopped"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output
}

// RequireRESTCommand builds a REST builtin command from a ToolDef and collection.
func RequireRESTCommand(
	t *testing.T,
	def catalog.ToolDef,
	collection toolrest.Collection,
	state *toolrest.ServerState,
) core.Command {
	t.Helper()
	br := toolregistry.NewBuiltinRegistry()
	toolrest.RegisterFactories(br, toolrest.FactoryDeps{Definitions: collection, ServerState: state})
	factory, ok := br.Resolve(def.Init)
	require.True(t, ok)
	builder, err := factory(def, nil)
	require.NoError(t, err)
	return builder.Build(core.Result{})
}

// RequireActiveStreams waits until a launched server reports want in-flight streams.
func RequireActiveStreams(t *testing.T, state *toolrest.ServerState, name string, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, err := state.ActiveStreamCount(name)
		return err == nil && got == want
	}, 500*time.Millisecond, 10*time.Millisecond)
}

// RequireRESTToolDef loads the builtin REST declaration with the given init name.
func RequireRESTToolDef(t *testing.T, init string) catalog.ToolDef {
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

func restDeclarationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "tools", "builtin", "rest", "all.yaml")
}

func serverName(server toolrest.Server) string {
	if server.Queue.Name != "" {
		return server.Queue.Name
	}
	return "control"
}
