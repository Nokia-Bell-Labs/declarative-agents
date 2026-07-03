// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestMain(m *testing.M) {
	spec.SetAgentCoreInstallRoot(filepath.Clean(repoRootFromDocsRuntime()))
	os.Exit(m.Run())
}

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

func TestStandaloneServerConformanceUsesRESTMachineRequestRoutes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "SPECIFICATIONS.yaml", "id: specs\n")
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()

	rec := getDocsRoute(t, handler, "/api/v1/docs/SPECIFICATIONS.yaml")

	require.Equal(t, http.StatusOK, rec.Code)
	trace := responseTrace(t, rec.Body.Bytes())
	require.Equal(t, "documentation_curator_requests", trace["server"])
	require.Equal(t, "document", trace["route"])
	require.Equal(t, "documentation-curator-request", trace["machine"])
	require.Equal(t, "DocumentDetailReady", trace["terminal_signal"])
}

func TestStandaloneServerAcceptsBrowserHeadersForDocsGET(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "SPECIFICATIONS.yaml", "id: specs\n")
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "http://127.0.0.1:18081/docs")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"path":"SPECIFICATIONS.yaml"`)
	require.Contains(t, rec.Body.String(), `"trace"`)
}

func TestStandaloneServerServesProfileUXConfig(t *testing.T) {
	t.Parallel()
	handler := NewServer(HostConfig{
		ProfilePath: curatorProfilePath(t),
		Assets:      fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()

	rec := getDocsRoute(t, handler, "/api/v1/ux")

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]UXConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "Knowledge Manager Documentation UI", body["data"].Title)
	require.Equal(t, "doc_list", uxRoutesByID(body["data"].Routes)["docs_index"].Action)
	require.Equal(t, "doc_get", uxRoutesByID(body["data"].Routes)["docs_detail"].Action)
}

func TestLoadCuratorUXConfigRequiresProfileLocalConfig(t *testing.T) {
	t.Parallel()
	_, err := LoadCuratorUXConfig(filepath.Join(t.TempDir(), "profile.yaml"))

	require.ErrorContains(t, err, "ui/ux.yaml")
}

func TestCuratorUXConfigMatchesRouteAndActionContracts(t *testing.T) {
	t.Parallel()
	profile, err := catalog.LoadProfile(curatorProfilePath(t))
	require.NoError(t, err)
	ux, err := LoadCuratorUXConfig(curatorProfilePath(t))
	require.NoError(t, err)
	collection, err := rest.LoadDefinitions(profile.RestDefinitions, profile.RestConfigDirs)
	require.NoError(t, err)
	defs, err := loadCuratorProfileDefs(profile)
	require.NoError(t, err)
	machine, err := core.LoadMachineSpec(filepath.Join(filepath.Dir(curatorProfilePath(t)), "request-machine.yaml"))
	require.NoError(t, err)

	requireUXRoutesMatchREST(t, ux, collection.Servers["documentation_curator_requests"].Endpoints)
	requireUXActionsSelected(t, ux, toolNames(defs), machineActionNames(machine))
}

func TestLazyMachineRequestProxyOwnsBackendLifecycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	proxy := NewLazyMachineRequestProxy(curatorProfilePath(t), docsDir)
	t.Cleanup(func() { _ = proxy.Close() })

	rec := getDocsRoute(t, proxy, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	addr := proxyBackendAddr(t, proxy)

	require.NoError(t, proxy.Close())
	requireAddressReleased(t, addr)
}

func TestMachineRequestFactoriesUseSelectedInits(t *testing.T) {
	t.Parallel()
	builtins := toolregistry.NewBuiltinRegistry()
	registerMachineRequestFactories(builtins, map[string]bool{
		"list_resource":      true,
		"doc_index_response": true,
	})

	_, ok := builtins.Resolve("list_resource")
	require.True(t, ok)
	_, ok = builtins.Resolve("doc_index_response")
	require.True(t, ok)
	_, ok = builtins.Resolve("read_resource")
	require.False(t, ok)
	_, ok = builtins.Resolve("doc_detail_response")
	require.False(t, ok)
}

func TestServeDocumentationUndoStopsOwnedListener(t *testing.T) {
	t.Parallel()
	host := NewDocumentationHostLifecycle()
	cmd := newServeDocumentationCommand(t, host).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.Signal("ServerLaunched"), res.Signal)
	addr := requireResultAddr(t, res)
	t.Cleanup(func() { _, _ = host.Stop() })

	undo := cmd.Undo(core.Result{})

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
	machine, err := core.LoadMachineSpec(curatorProfileAssetPath(t, "machine.yaml"))
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

func (c staticDocsSignalCmd) Undo(_ core.Result) core.Result {
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

func responseTrace(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &body))
	trace, _ := body["trace"].(map[string]interface{})
	require.NotNil(t, trace)
	return trace
}

func proxyBackendAddr(t *testing.T, proxy *LazyMachineRequestProxy) string {
	t.Helper()
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	require.NotEmpty(t, proxy.baseURL)
	return strings.TrimPrefix(proxy.baseURL, "http://")
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

func curatorProfilePath(t *testing.T) string {
	t.Helper()
	return curatorProfileAssetPath(t, "profile.yaml")
}

func curatorRestPath(t *testing.T) string {
	t.Helper()
	return curatorProfileAssetPath(t, "rest.yaml")
}

func curatorProfileAssetPath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(docsProfileRoot(t), "knowledge-manager", "documentation-curator", filepath.FromSlash(rel))
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

func repoRootFromDocsTest(t *testing.T) string {
	t.Helper()
	return repoRootFromDocsRuntime()
}

func repoRootFromDocsRuntime() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func docsProfileRoot(t *testing.T) string {
	t.Helper()
	root := repoRootFromDocsTest(t)
	for _, candidate := range docsProfileRootCandidates(root) {
		if hasDocsProfile(candidate, "knowledge-manager/documentation-curator/profile.yaml") {
			return candidate
		}
		nested := filepath.Join(candidate, "agents")
		if hasDocsProfile(nested, "knowledge-manager/documentation-curator/profile.yaml") {
			return nested
		}
	}
	t.Fatalf("profile root not found; place agent-profiles next to agent-core or under ./agent-profiles")
	return ""
}

func docsProfileRootCandidates(root string) []string {
	return []string{
		filepath.Join(filepath.Dir(root), "agent-profiles"),
		filepath.Join(root, "agent-profiles"),
	}
}

func hasDocsProfile(root, rel string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}
