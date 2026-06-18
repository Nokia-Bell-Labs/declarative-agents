// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
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

	var restCalled bool
	deps := toolregistry.StandardFactoryDeps{
		RegisterREST: func(*toolregistry.BuiltinRegistry) { restCalled = true },
	}

	toolregistry.RegisterStandardBuiltinFactories(toolregistry.NewBuiltinRegistry(), map[string]bool{"file_read": true}, deps)
	require.False(t, restCalled)

	toolregistry.RegisterStandardBuiltinFactories(toolregistry.NewBuiltinRegistry(), map[string]bool{InitClientGet: true}, deps)
	require.True(t, restCalled)
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
		Name: "get_issue",
		Config: map[string]interface{}{
			"rest_ref":  "github",
			"resource":  "issue",
			"operation": "get",
		},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, builder)
}

func TestDocumentationCuratorRESTDefinitionsLoad(t *testing.T) {
	t.Parallel()

	defs, err := catalog.LoadToolDefs(documentationCuratorDeclarationsPath(t))
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

	collection, err := LoadDefinitions([]string{documentationCuratorRestPath(t)}, nil)
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

func documentationCuratorDeclarationsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(profileRoot(t), "knowledge-manager", "documentation-curator", "declarations.yaml")
}

func documentationCuratorRestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(profileRoot(t), "knowledge-manager", "documentation-curator", "rest.yaml")
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
