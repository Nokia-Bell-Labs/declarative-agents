// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func curatorProfilePath(t *testing.T) string {
	t.Helper()
	return writeDocsRuntimeProfile(t)
}

func curatorRestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(docsRuntimeFixtureDir(t), "rest.yaml")
}

func curatorProfileAssetPath(t *testing.T, rel string) string {
	t.Helper()
	dir := filepath.Dir(writeDocsRuntimeProfile(t))
	return filepath.Join(dir, filepath.FromSlash(rel))
}

func writeDocsRuntimeProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restFixture := docsRuntimeFixtureDir(t)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeTestProfileFile(t, profilePath, fmt.Sprintf(`name: docs-runtime
machine: %q
tools:
  - %q
tool_declarations:
  - %q
  - %q
  - %q
  - %q
rest_definitions:
  - %q
`, filepath.Join(dir, "machine.yaml"), filepath.Join(dir, "tools.yaml"),
		filepath.Join(dir, "builtin.yaml"), filepath.Join(dir, "request-declarations.yaml"),
		filepath.Join(restFixture, "declarations.yaml"),
		filepath.Join(repoRootFromDocsTest(t), "tools", "builtin", "lifecycle", "exit-agent.yaml"),
		filepath.Join(restFixture, "rest.yaml")))
	writeTestProfileFile(t, filepath.Join(dir, "tools.yaml"), docsRuntimeToolsYAML)
	writeTestProfileFile(t, filepath.Join(dir, "builtin.yaml"), docsRuntimeBuiltinYAML)
	writeTestProfileFile(t, filepath.Join(dir, "request-declarations.yaml"), docsRuntimeRequestDeclarationsYAML)
	writeTestProfileFile(t, filepath.Join(dir, "request-machine.yaml"), docsRuntimeRequestMachineYAML)
	writeTestProfileFile(t, filepath.Join(dir, "machine.yaml"), docsRuntimeMachineYAML)
	writeTestProfileFile(t, filepath.Join(dir, "ui", "ux.yaml"), docsRuntimeUXYAML)
	return profilePath
}

func docsRuntimeFixtureDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootFromDocsTest(t), "internal", "tools", "rest", "testdata", "docs-runtime")
}

func writeTestProfileFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func loadCuratorProfileDefs(profile catalog.AgentProfile) ([]catalog.ToolDef, error) {
	explicit, err := catalog.LoadToolDeclarations(profile.ToolDeclarations)
	if err != nil {
		return nil, err
	}
	selection, err := catalog.LoadToolSelections(profile.Tools)
	if err != nil {
		return nil, err
	}
	return catalog.SelectTools(explicit, selection)
}

func launchCuratorControl(t *testing.T, collection rest.Collection) (*rest.ServerState, string) {
	t.Helper()
	server, err := collection.ResolveServer("docs_runtime_control")
	require.NoError(t, err)
	state := rest.NewServerState()
	output, err := state.Launch(server)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("docs_runtime_control") })
	return state, "http://" + output["address"].(string)
}

func getHTTPBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, err := json.Marshal(body)
	require.NoError(t, err)
	return string(data)
}

func postHTTPJSON(t *testing.T, url, body string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func requireUXRoutesMatchREST(t *testing.T, ux UXConfig, endpoints map[string]rest.Endpoint) {
	t.Helper()
	routes := uxRoutesByID(ux.Routes)
	require.Equal(t, restEndpointUIPath(endpoints["documents"].Path), routes["docs_index"].Path)
	require.Equal(t, endpoints["documents"].Binding, "machine_request")
	require.Equal(t, restEndpointUIPath(endpoints["document"].Path), routes["docs_detail"].Path)
	require.Equal(t, endpoints["document"].Binding, "machine_request")
}

func requireUXActionsSelected(t *testing.T, ux UXConfig, selected, machineActions map[string]bool) {
	t.Helper()
	for name, action := range ux.Actions {
		require.True(t, selected[action.UIAction], "UX action %s selects missing ToolDef %s", name, action.UIAction)
		if action.RequestMachineAction != "" {
			require.True(t, machineActions[action.RequestMachineAction], "UX action %s references missing machine action", name)
		}
	}
}

func restEndpointUIPath(path string) string {
	path = strings.TrimPrefix(path, "/api/v1")
	path = strings.ReplaceAll(path, "/{path...}", "/*")
	return strings.ReplaceAll(path, "/{path}", "/*")
}

func machineActionNames(machine core.MachineSpec) map[string]bool {
	names := map[string]bool{}
	for _, transition := range machine.Transitions {
		if transition.Action != "" {
			names[transition.Action] = true
		}
	}
	return names
}

func toolNames(defs []catalog.ToolDef) map[string]bool {
	names := make(map[string]bool, len(defs))
	for _, def := range defs {
		names[def.Name] = true
	}
	return names
}
