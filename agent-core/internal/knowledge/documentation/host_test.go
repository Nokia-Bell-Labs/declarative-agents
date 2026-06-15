// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/lifecycle"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/rest"
)

func TestStandaloneServerServesDocsAPIAndSPA(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	server := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
			"asset.js":   &fstest.MapFile{Data: []byte("console.log('docs')")},
		},
	})
	handler := server.Handler()

	rec := getDocsRoute(t, handler, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"path":"VISION.yaml"`)
	require.Contains(t, rec.Body.String(), `"trace"`)

	rec = getDocsRoute(t, handler, "/api/v1/docs/VISION.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"raw":"title: Vision\n"`)
	require.Contains(t, rec.Body.String(), `"trace"`)

	rec = getDocsRoute(t, handler, "/docs/VISION.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "docs app")

	rec = getDocsRoute(t, handler, "/asset.js")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "console.log")
}

func TestStandaloneServerHealth(t *testing.T) {
	t.Parallel()

	handler := NewServer(HostConfig{Assets: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
	}}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestStandaloneServerStartServesDocsAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	running, err := NewServer(HostConfig{
		Addr: "127.0.0.1:0", DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, running.Close()) })

	body := getHTTPBody(t, "http://"+running.Addr+"/api/v1/docs")

	require.Contains(t, body, `"path":"VISION.yaml"`)
	require.Contains(t, body, `"trace"`)
}

func TestServeDocumentationUndoStopsOwnedListener(t *testing.T) {
	t.Parallel()
	host := NewDocumentationHostLifecycle()
	cmd := newServeDocumentationCommand(t, host).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.Signal("ServerLaunched"), res.Signal)
	addr := requireResultAddr(t, res)
	t.Cleanup(func() { _, _ = host.Stop() })

	undo := cmd.Undo()

	require.Equal(t, core.Signal("ServerStopped"), undo.Signal, undo.Output)
	requireAddressReleased(t, addr)
}

func TestCuratorMachineExitStopsDocumentationHost(t *testing.T) {
	t.Parallel()
	host := NewDocumentationHostLifecycle()
	var launchedAddr string
	result, err := core.Loop(curatorExitLoopParams(t, host, &launchedAddr), context.Background())
	require.NoError(t, err)

	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Done"), result.FinalState)
	require.NotEmpty(t, launchedAddr)
	requireDocsHostStoppedEvent(t, result)
	requireAddressReleased(t, launchedAddr)
}

func TestStandaloneServerServesContextFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "configs/sample.yaml", "name: sample\n")
	writeDocFixture(t, root, "pkg/demo/demo.go", "package demo\n")
	handler := NewServer(HostConfig{
		ConfigsDir: filepath.Join(root, "configs"),
		SourceDir:  root,
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler()

	config := getDocsRoute(t, handler, "/api/v1/configs/sample.yaml")
	require.Equal(t, http.StatusOK, config.Code)
	require.Contains(t, config.Body.String(), `"raw":"name: sample\n"`)

	source := getDocsRoute(t, handler, "/api/v1/source/pkg/demo/demo.go")
	require.Equal(t, http.StatusOK, source.Code)
	require.Contains(t, source.Body.String(), `"language":"go"`)
}

func newServeDocumentationCommand(t *testing.T, host *DocumentationHostLifecycle) ServeDocumentationBuilder {
	t.Helper()
	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	return ServeDocumentationBuilder{
		Config: ToolConfig{Addr: "127.0.0.1:0", DocsDir: root},
		Host:   host,
	}
}

func curatorExitLoopParams(t *testing.T, host *DocumentationHostLifecycle, launchedAddr *string) core.LoopParams {
	t.Helper()
	machine, err := core.LoadMachineSpec(filepath.Join(repoRootFromDocsTest(t), "agents", "knowledge-manager", "documentation-curator", "machine.yaml"))
	require.NoError(t, err)
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "serve_documentation"}, newServeDocumentationCommand(t, host))
	registerStaticDocsSignal(reg, "launch_curator_control", "ServerLaunched", "{}")
	registerStaticDocsSignal(reg, "await_curator_control", "ExitRequested", `{"payload":{"reason":"operator requested shutdown","status":"success"}}`)
	reg.Register(core.ToolSpec{Name: "exit_agent"}, lifecycle.ExitBuilder{
		Config: lifecycle.ExitConfig{Status: "success"}, Shutdown: func() {},
	})
	return core.LoopParams{MachineSpec: &machine, Registry: reg, Trace: tracing.NoopTracer{}, Hooks: core.LoopHooks{
		OnResult: captureLaunchAddr(t, launchedAddr),
	}}
}

func captureLaunchAddr(t *testing.T, launchedAddr *string) func(core.RunResult, core.Result) core.RunResult {
	t.Helper()
	return func(rr core.RunResult, res core.Result) core.RunResult {
		if res.CommandName == "serve_documentation" && res.Signal == core.Signal("ServerLaunched") {
			*launchedAddr = requireResultAddr(t, res)
		}
		return rr
	}
}

type staticDocsSignalBuilder struct {
	name   string
	signal core.Signal
	output string
}

type staticDocsSignalCmd struct {
	name   string
	signal core.Signal
	output string
}

func registerStaticDocsSignal(reg *core.Registry, name string, signal core.Signal, output string) {
	reg.Register(core.ToolSpec{Name: name}, staticDocsSignalBuilder{name: name, signal: signal, output: output})
}

func (b staticDocsSignalBuilder) Build(_ core.Result) core.Command {
	return staticDocsSignalCmd(b)
}

func (c staticDocsSignalCmd) Name() string { return c.name }

func (c staticDocsSignalCmd) Execute() core.Result {
	return core.Result{Signal: c.signal, CommandName: c.name, Output: c.output}
}

func (c staticDocsSignalCmd) Undo() core.Result {
	return core.NoopUndo(c.name)
}

func requireResultAddr(t *testing.T, result core.Result) string {
	t.Helper()
	var output map[string]string
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.NotEmpty(t, output["addr"])
	return output["addr"]
}

func requireAddressReleased(t *testing.T, addr string) {
	t.Helper()
	require.Eventually(t, func() bool {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		_ = listener.Close()
		return true
	}, time.Second, 10*time.Millisecond)
}

func requireDocsHostStoppedEvent(t *testing.T, result core.RunResult) {
	t.Helper()
	require.NotEmpty(t, result.Events)
	last := result.Events[len(result.Events)-1]
	require.Equal(t, "serve_documentation", last.CommandName)
	require.Equal(t, core.Signal("ServerStopped"), last.Signal)
}

func TestStandaloneServerRunsActionsThroughWorkflowRunner(t *testing.T) {
	t.Parallel()

	handler := NewServer(HostConfig{
		Workflow: stubWorkflowRunner{},
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler()

	rec := postDocsJSON(t, handler, "/api/v1/actions", `{"type":"doc_validate","params":{"paths":["VISION.yaml"]}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"tool":"doc_validate"`)
	require.Contains(t, rec.Body.String(), `"signal":"RESTResponded"`)
}

func TestProfileWorkflowRunnerDispatchesConfiguredRESTTool(t *testing.T) {
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

	defs, err := catalog.LoadToolDefs(curatorDeclarationsPath(t))
	require.NoError(t, err)
	runner, err := NewProfileWorkflowRunnerFromDefs(collection, defs)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions", strings.NewReader(`{"type":"doc_get","params":{"path":"VISION.yaml"}}`))
	result, err := runner.Run(req)

	require.NoError(t, err)
	require.Equal(t, "doc_get", result.Tool)
	require.Equal(t, "RESTResourceRead", result.Signal)
	data := result.Data.(map[string]interface{})
	require.Equal(t, "title: Vision\n", data["raw"])
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
	require.Contains(t, names, "serve_documentation")
	require.Contains(t, names, "launch_curator_control")
	require.Contains(t, names, "await_curator_control")
	require.Contains(t, names, "exit_agent")
}

func TestCuratorControlRouteFeedsRestAwaitEvent(t *testing.T) {
	t.Parallel()
	collection, err := rest.LoadDefinitions([]string{curatorRestPath(t)}, nil)
	require.NoError(t, err)
	def := collection.Servers["documentation_curator_control"]
	def.Address = "127.0.0.1:0"
	collection.Servers["documentation_curator_control"] = def
	state, baseURL := launchCuratorControl(t, collection)
	postHTTPJSON(t, baseURL+"/api/lifecycle/exit", `{"reason":"operator requested shutdown"}`)

	event, signal, err := state.AwaitAny(rest.AwaitAnyOptions{
		Sources: []rest.AwaitSource{{Server: "documentation_curator_control", Routes: []string{"exit"}}},
		Timeout: time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "exit", event.Route)
	require.Equal(t, "operator requested shutdown", event.Payload["reason"])
}

type stubWorkflowRunner struct{}

func (stubWorkflowRunner) Run(r *http.Request) (ActionResponse, error) {
	defer r.Body.Close()
	return ActionResponse{
		Data: map[string]interface{}{"status": "valid"},
		Tool: "doc_validate", Signal: "RESTResponded",
	}, nil
}

func curatorDeclarationsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootFromDocsTest(t), "agents", "knowledge-manager", "documentation-curator", "declarations.yaml")
}

func curatorProfilePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootFromDocsTest(t), "agents", "knowledge-manager", "documentation-curator", "profile.yaml")
}

func curatorRestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootFromDocsTest(t), "agents", "knowledge-manager", "documentation-curator", "rest.yaml")
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
	server, err := collection.ResolveServer("documentation_curator_control")
	require.NoError(t, err)
	state := rest.NewServerState()
	output, err := state.Launch(server)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("documentation_curator_control") })
	return state, "http://" + output["address"].(string)
}

func getHTTPBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func toolNames(defs []catalog.ToolDef) map[string]bool {
	names := make(map[string]bool, len(defs))
	for _, def := range defs {
		names[def.Name] = true
	}
	return names
}

func repoRootFromDocsTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}
