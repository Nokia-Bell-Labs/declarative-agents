// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRESTConfig_LoadsToolDefinitions(t *testing.T) {
	t.Parallel()

	defs, err := catalog.LoadToolDefs(restDeclarationsPath(t))
	require.NoError(t, err)
	require.Len(t, defs, len(StandardInits))
	for _, def := range defs {
		require.Equal(t, "builtin", def.Type)
		require.Equal(t, "boundary", def.Category)
		require.Contains(t, StandardInits, def.Init)
		require.NotEmpty(t, def.Emits)
		require.NotEmpty(t, def.Output.Schema)
		requireConfigUsesNamedRefs(t, def)
		requireNoAuthorityParameters(t, def.Parameters)
	}
}

func TestRESTFactoriesRegisterOnlyWhenSelected(t *testing.T) {
	t.Parallel()

	deps := toolregistry.StandardFactoryDeps{
		RegisterREST: func(registry *toolregistry.BuiltinRegistry) {
			registry.Register(InitClientGet, nil)
		},
	}

	unselected := toolregistry.NewBuiltinRegistry()
	toolregistry.RegisterStandardBuiltinFactories(unselected, map[string]bool{"file_read": true}, deps)
	require.Empty(t, unselected.Names())

	selected := toolregistry.NewBuiltinRegistry()
	toolregistry.RegisterStandardBuiltinFactories(selected, map[string]bool{InitClientGet: true}, deps)
	require.Equal(t, []string{InitClientGet}, selected.Names())
}

func TestRESTFactoriesResolveConfiguredDefinitions(t *testing.T) {
	t.Parallel()

	definition, err := ParseDefinition([]byte(validDefinitionYAML))
	require.NoError(t, err)
	collection := NewCollection()
	require.NoError(t, collection.Add(definition))
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection})

	factory, ok := br.Resolve(InitClientGet)
	require.True(t, ok)
	builder, err := factory(catalog.ToolDef{
		Name: "get_issue", Emits: []string{"RESTResourceRead", "CommandError"},
		Config: map[string]interface{}{
			"rest_ref":  "github",
			"resource":  "issue",
			"operation": "get",
		},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, builder)
}

func TestRESTDeclarationsMatchRuntimeOutputAndUndoKeys(t *testing.T) {
	t.Parallel()
	defs, err := catalog.LoadToolDefs(restDeclarationsPath(t))
	require.NoError(t, err)
	byName := make(map[string]catalog.ToolDef, len(defs))
	for _, def := range defs {
		byName[def.Name] = def
	}
	requireOutputProperties(t, byName["rest_client_send"], "request_id", "operation_id", "correlation")
	requireOutputProperties(t, byName["rest_client_await"], "request_id", "operation_id")
	requireOutputProperties(t, byName["rest_server_await"], "source", "route", "signal")
	launch := byName["rest_server_launch"]
	require.Equal(t, serverLaunchReceiptStrategy, launch.Undo.Strategy)
	require.Equal(t, "rest_server_launch", launch.Undo.Payload)
	require.Equal(t,
		[]string{"strategy", "declaration", "server", "address", "ownership"},
		launch.Undo.Captures,
	)
	require.Equal(t, []string{"receipt"}, launch.Undo.Requires)
	awaitAny := byName["rest_await_event"]
	require.Equal(t, "queue_event_restore", awaitAny.Undo.Strategy)
	require.Equal(t, []string{"source", "event"}, awaitAny.Undo.Captures)
}

func TestRESTDeclaredOutputPropertiesExistAtRuntime(t *testing.T) {
	t.Parallel()
	defs, err := catalog.LoadToolDefs(restDeclarationsPath(t))
	require.NoError(t, err)
	for _, def := range defs {
		properties, _ := def.Output.Schema["properties"].(map[string]interface{})
		actual := restRuntimeOutputProperties[def.Init]
		require.NotNil(t, actual, def.Init)
		for property := range properties {
			require.True(t, actual[property],
				"tool %s declares output property %s that init %s never emits",
				def.Name, property, def.Init)
		}
	}
}

var responseOutputProperties = map[string]bool{
	"rest_ref": true, "resource": true, "operation": true, "status": true,
	"headers": true, "body": true, "mapped": true, "resource_id": true,
	"request_id": true, "retry_count": true, "domain_error_code": true,
	"selected_authority": true,
}

var restRuntimeOutputProperties = map[string]map[string]bool{
	InitClientGet: responseOutputProperties, InitClientSet: responseOutputProperties,
	InitClientCreate: responseOutputProperties, InitClientDelete: responseOutputProperties,
	InitClientInvoke: responseOutputProperties,
	InitClientSend: {
		"request_id": true, "operation_id": true, "rest_ref": true, "resource": true,
		"idempotency_token": true, "correlation": true, "submitted_payload": true, "status": true,
	},
	InitClientAwait: {
		"request_id": true, "operation_id": true, "correlation": true,
		"rest_ref": true, "resource": true, "operation": true, "status": true,
		"headers": true, "body": true, "mapped": true, "resource_id": true,
		"retry_count": true, "domain_error_code": true, "selected_authority": true,
		"signal": true,
	},
	InitServerLaunch: {
		"server": true, "address": true, "route_count": true,
		"bindings": true, "owned": true, "active_streams": true,
	},
	InitServerAwait: inboundEventOutputProperties(),
	InitAwaitEvent:  inboundEventOutputProperties(),
	InitServerStop: {
		"server": true, "address": true, "drained_events": true,
		"dropped_events": true, "status": true, "drain_policy": true,
		"queue_outcome": true, "active_streams": true,
	},
}

func inboundEventOutputProperties() map[string]bool {
	return map[string]bool{
		"source": true, "queue": true, "route": true, "method": true,
		"signal": true, "payload": true, "request_id": true,
	}
}

func requireOutputProperties(t *testing.T, def catalog.ToolDef, names ...string) {
	t.Helper()
	properties, ok := def.Output.Schema["properties"].(map[string]interface{})
	require.True(t, ok, def.Name)
	for _, name := range names {
		require.Contains(t, properties, name, def.Name)
	}
}

func TestDocsRuntimeRESTDefinitionsLoad(t *testing.T) {
	t.Parallel()

	defs, err := catalog.LoadToolDefs(docsRuntimeDeclarationsPath(t))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"doc_list",
		"doc_get",
		"doc_search",
		"doc_validate",
		"doc_suggest_changes",
		"doc_patch_approve",
		"doc_patch_reject",
		"doc_patch_reopen",
		"launch_monitor_rest",
		"await_monitor_control",
		"stop_monitor_rest",
	}, toolDefNames(defs))

	collection, err := LoadDefinitions([]string{docsRuntimeRestPath(t)}, nil)
	require.NoError(t, err)
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection})

	for _, def := range defs {
		require.Equal(t, "builtin", def.Type)
		require.Equal(t, "boundary", def.Category)
		requireNoAuthorityParameters(t, def.Parameters)
		if def.Name != "await_monitor_control" {
			requireConfigUsesNamedRefs(t, def)
		}
		factory, ok := br.Resolve(def.Init)
		require.True(t, ok, "factory for init %q should be registered", def.Init)
		builder, err := factory(def, nil)
		require.NoError(t, err, "tool %q should resolve configured REST operation", def.Name)
		require.NotNil(t, builder)
	}
}

func restDeclarationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "tools", "builtin", "rest", "all.yaml")
}

func docsRuntimeDeclarationsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(docsRuntimeFixtureDir(t), "declarations.yaml")
}

func docsRuntimeRestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(docsRuntimeFixtureDir(t), "rest.yaml")
}

func docsRuntimeFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "testdata", "docs-runtime")
}

func toolDefNames(defs []catalog.ToolDef) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

func requireConfigUsesNamedRefs(t *testing.T, def catalog.ToolDef) {
	t.Helper()
	require.Contains(t, def.Config, "rest_ref")
	require.NotContains(t, def.Config, "url")
	require.NotContains(t, def.Config, "host")
	require.NotContains(t, def.Config, "method")
	require.NotContains(t, def.Config, "auth_ref")
	require.NotContains(t, def.Config, "redirect_policy")
}

func requireNoAuthorityParameters(t *testing.T, params map[string]interface{}) {
	t.Helper()
	properties, _ := params["properties"].(map[string]interface{})
	for _, forbidden := range []string{"url", "host", "method", "auth_ref", "redirect_policy"} {
		require.NotContains(t, properties, forbidden)
	}
}
