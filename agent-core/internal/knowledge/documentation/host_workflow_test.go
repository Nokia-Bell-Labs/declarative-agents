// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestStandaloneServerRunsActionsThroughWorkflowRunner(t *testing.T) {
	t.Parallel()

	handler := NewServer(HostConfig{
		Workflow: fakeWorkflowRunner{},
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler()

	rec := postDocsJSON(t, handler, "/api/v1/actions", `{"type":"doc_validate","params":{"paths":["VISION.yaml"]}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"tool":"doc_validate"`)
	require.Contains(t, rec.Body.String(), `"signal":"RESTResponded"`)
}

func TestProfileWorkflowRunnerDispatchesConfiguredValidationAction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	apiServer := httptest.NewServer(NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler())
	defer apiServer.Close()

	collection, err := rest.LoadDefinitions([]string{curatorRestPath(t)}, nil)
	require.NoError(t, err)
	client := collection.Clients["documentation"]
	client.BaseURL = apiServer.URL
	collection.Clients["documentation"] = client
	collection.Limits["local_docs_api"] = rest.LimitProfile{}

	profile, err := catalog.LoadProfile(curatorProfilePath(t))
	require.NoError(t, err)
	defs, err := loadCuratorProfileDefs(profile)
	require.NoError(t, err)
	runner, err := NewProfileWorkflowRunnerFromDefs(collection, defs)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions", strings.NewReader(`{"type":"doc_validate","params":{"paths":["VISION.yaml"]}}`))
	result, err := runner.Run(req)

	require.NoError(t, err)
	require.Equal(t, "doc_validate", result.Tool)
	require.Equal(t, "RESTResponded", result.Signal)
	data := result.Data.(map[string]interface{})
	require.Contains(t, data, "findings")
	require.Contains(t, data, "checked_paths")
}

func TestCuratorProfileSelectsGenericControlExitFlow(t *testing.T) {
	t.Parallel()
	profile, err := catalog.LoadProfile(curatorProfilePath(t))
	require.NoError(t, err)
	defs, err := loadCuratorProfileDefs(profile)
	require.NoError(t, err)
	machine, err := core.LoadMachineSpec(profile.Machine)
	require.NoError(t, err)

	require.NoError(t, catalog.ValidateToolEmits(machine, defs))
	names := toolNames(defs)
	require.Contains(t, names, "launch_documentation")
	require.Contains(t, names, "stop_documentation")
	require.Contains(t, names, "launch_docs_control")
	require.Contains(t, names, "await_docs_control")
	require.Contains(t, names, "exit_agent")
}

func TestCuratorControlRouteFeedsRestAwaitEvent(t *testing.T) {
	t.Parallel()
	collection, err := rest.LoadDefinitions([]string{curatorRestPath(t)}, nil)
	require.NoError(t, err)
	def := collection.Servers["docs_runtime_control"]
	def.Address = "127.0.0.1:0"
	collection.Servers["docs_runtime_control"] = def
	state, baseURL := launchCuratorControl(t, collection)
	postHTTPJSON(t, baseURL+"/api/lifecycle/exit", `{"reason":"operator requested shutdown"}`)

	event, signal, err := state.AwaitAny(rest.AwaitAnyOptions{
		Sources: []rest.AwaitSource{{Server: "docs_runtime_control", Routes: []string{"exit"}}},
		Timeout: time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "exit", event.Route)
	require.Equal(t, "operator requested shutdown", event.Payload["reason"])
}
