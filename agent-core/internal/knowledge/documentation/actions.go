// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

var actionRESTClientInits = map[string]bool{
	rest.InitClientGet:    true,
	rest.InitClientSet:    true,
	rest.InitClientCreate: true,
	rest.InitClientDelete: true,
	rest.InitClientInvoke: true,
	rest.InitClientSend:   true,
	rest.InitClientAwait:  true,
}

type restDefinitionLoader func([]string, []string) (rest.Collection, error)

type restServerLifecycle interface {
	Launch(rest.ServerDefinition) (map[string]interface{}, error)
	Stop(string) (map[string]interface{}, error)
}

type restServerFactory func() restServerLifecycle

// LazyMachineRequestProxy forwards document and action API requests to the configured REST server.
type LazyMachineRequestProxy struct {
	profilePath string
	docsDir     string
	mu          sync.Mutex
	state       restServerLifecycle
	baseURL     string
	serverName  string
	loadDefs    restDefinitionLoader
	newServer   restServerFactory
}

// NewLazyMachineRequestProxy creates a same-origin proxy for document machine requests.
func NewLazyMachineRequestProxy(profilePath, docsDir string) *LazyMachineRequestProxy {
	return &LazyMachineRequestProxy{
		profilePath: profilePath, docsDir: docsDir,
		loadDefs:  loadRESTDefinitions,
		newServer: newRESTServerLifecycle,
	}
}

func (p *LazyMachineRequestProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	baseURL, err := p.backendBaseURL()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.forward(w, r, baseURL)
}

// Close stops the owned generic REST machine_request server.
func (p *LazyMachineRequestProxy) Close() error {
	p.mu.Lock()
	state := p.state
	serverName := p.serverName
	p.state = nil
	p.baseURL = ""
	p.serverName = ""
	p.mu.Unlock()
	if state == nil {
		return nil
	}
	_, err := state.Stop(serverName)
	return err
}

func (p *LazyMachineRequestProxy) backendBaseURL() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.baseURL != "" {
		return p.baseURL, nil
	}
	baseURL, state, err := p.launchBackend()
	if err != nil {
		return "", err
	}
	p.baseURL = baseURL
	p.state = state
	return p.baseURL, nil
}

func (p *LazyMachineRequestProxy) launchBackend() (string, restServerLifecycle, error) {
	profile, err := catalog.LoadProfile(p.profilePath)
	if err != nil {
		return "", nil, err
	}
	collection, err := p.loadDefs(profile.RestDefinitions, profile.RestConfigDirs)
	if err != nil {
		return "", nil, err
	}
	def, err := documentMachineRequestServer(collection)
	if err != nil {
		return "", nil, err
	}
	def.Server.Address = "127.0.0.1:0"
	def.MachineRequestRunner = p.requestRunner(collection)
	state := p.newServer()
	output, err := state.Launch(def)
	if err != nil {
		return "", nil, err
	}
	p.serverName = def.Name
	return "http://" + output["address"].(string), state, nil
}

func (p *LazyMachineRequestProxy) requestRunner(collection rest.Collection) rest.MachineRequestRunner {
	return rest.NewProfileMachineRequestRunner(rest.ProfileMachineRequestRunnerDeps{
		BaseDir:   filepath.Dir(p.profilePath),
		Directory: docsResourceRoot(p.docsDir),
		RegisterBuiltins: func(br *toolregistry.BuiltinRegistry, selected map[string]bool, reg *core.Registry) {
			registerMachineRequestFactories(br, selected, reg, collection)
		},
	})
}

func registerMachineRequestFactories(
	br *toolregistry.BuiltinRegistry,
	selected map[string]bool,
	_ *core.Registry,
	collection rest.Collection,
) {
	if selectedActionRESTClient(selected) {
		rest.RegisterFactories(br, rest.FactoryDeps{Definitions: collection})
	}
	if selectedBuiltinInit(selected, "list_resource") {
		registerResourceFactory(br, "list_resource", func(root string, cfg filesystem.ResourceConfig) core.Builder {
			return requestListResourceBuilder{root: root, resources: cfg}
		})
	}
	if selectedBuiltinInit(selected, "read_resource") {
		registerResourceFactory(br, "read_resource", func(root string, cfg filesystem.ResourceConfig) core.Builder {
			return requestReadResourceBuilder{root: root, resources: cfg}
		})
	}
	registerSelectedResponseFactory(br, selected, "doc_index_response", "DocumentIndexReady")
	registerSelectedResponseFactory(br, selected, "doc_detail_response", "DocumentDetailReady")
}

func selectedActionRESTClient(selected map[string]bool) bool {
	for init := range actionRESTClientInits {
		if selected[init] {
			return true
		}
	}
	return false
}

func selectedBuiltinInit(selected map[string]bool, init string) bool {
	return selected[init]
}

func registerSelectedResponseFactory(
	br *toolregistry.BuiltinRegistry,
	selected map[string]bool,
	name string,
	signal core.Signal,
) {
	if selectedBuiltinInit(selected, name) {
		br.Register(name, responseFactory(name, signal))
	}
}

type requestListResourceBuilder struct {
	root      string
	resources filesystem.ResourceConfig
}

func (b requestListResourceBuilder) Build(res core.Result) core.Command {
	return (&filesystem.ListResourceBuilder{
		Root: b.root, Resources: b.resources,
	}).Build(machineRequestParameterResult(res.Output))
}

type requestReadResourceBuilder struct {
	root      string
	resources filesystem.ResourceConfig
}

func (b requestReadResourceBuilder) Build(res core.Result) core.Command {
	return (&filesystem.ReadResourceBuilder{
		Root: b.root, Resources: b.resources,
	}).Build(machineRequestParameterResult(res.Output))
}

func machineRequestParameterResult(output string) core.Result {
	params := machineRequestParameters(output)
	data, err := json.Marshal(map[string]interface{}{"parameters": params})
	if err != nil {
		return core.Result{Signal: core.Seed, Output: `{"parameters":{"resource":"docs"}}`}
	}
	return core.Result{Signal: core.Seed, Output: string(data)}
}

func machineRequestParameters(output string) map[string]interface{} {
	var seed struct {
		Parameters map[string]interface{} `json:"parameters"`
		Path       string                 `json:"path"`
	}
	_ = json.Unmarshal([]byte(output), &seed)
	params := seed.Parameters
	if params == nil {
		params = map[string]interface{}{}
	}
	if _, ok := params["resource"]; !ok {
		params["resource"] = "docs"
	}
	if _, ok := params["path"].(string); !ok {
		if path := strings.TrimPrefix(seed.Path, "/api/v1/docs/"); path != seed.Path {
			params["path"] = path
		}
	}
	return params
}

func (p *LazyMachineRequestProxy) forward(w http.ResponseWriter, r *http.Request, baseURL string) {
	target := proxyURL(baseURL, r.URL)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header = proxyRequestHeaders(r.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	writeProxiedDocumentResponse(w, resp)
}

func proxyRequestHeaders(headers http.Header) http.Header {
	forwarded := http.Header{}
	for _, name := range []string{"Accept", "Content-Type", "X-Request-ID"} {
		for _, value := range headers.Values(name) {
			forwarded.Add(name, value)
		}
	}
	return forwarded
}

func proxyURL(baseURL string, requestURL *url.URL) string {
	return baseURL + requestURL.RequestURI()
}

func writeProxiedDocumentResponse(w http.ResponseWriter, resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	parsed := map[string]interface{}{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		copyProxyHeaders(w, resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}
	writeJSON(w, resp.StatusCode, parsed)
}

func copyProxyHeaders(w http.ResponseWriter, headers http.Header) {
	for name, values := range headers {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
}

func registerResourceFactory(br *toolregistry.BuiltinRegistry, init string, factory func(string, filesystem.ResourceConfig) core.Builder) {
	br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg filesystem.ResourceConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return factory(vars["directory"], cfg), nil
	})
}

type responseBuilder struct {
	name   string
	signal core.Signal
}

type responseCmd struct {
	name   string
	signal core.Signal
	input  string
}

func (b responseBuilder) Build(res core.Result) core.Command {
	return responseCmd{name: b.name, signal: b.signal, input: res.Output}
}

func (c responseCmd) Name() string                   { return c.name }
func (c responseCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.name) }

func (c responseCmd) Execute() core.Result {
	output, err := c.output()
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.name, Err: err, Output: err.Error()}
	}
	data, err := json.Marshal(output)
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.name, Err: err, Output: err.Error()}
	}
	return core.Result{Signal: c.signal, CommandName: c.name, Output: string(data)}
}

func (c responseCmd) output() (map[string]interface{}, error) {
	if c.name == "doc_index_response" {
		return indexResponseOutput(c.input)
	}
	return detailResponseOutput(c.input)
}

func indexResponseOutput(input string) (map[string]interface{}, error) {
	var documents []interface{}
	if err := json.Unmarshal([]byte(input), &documents); err != nil {
		return nil, err
	}
	return map[string]interface{}{"data": documents, "documents": documents, "count": len(documents)}, nil
}

func detailResponseOutput(input string) (map[string]interface{}, error) {
	var detail map[string]interface{}
	if err := json.Unmarshal([]byte(input), &detail); err != nil {
		return nil, err
	}
	content := detail["parsed"]
	if content == nil {
		content = detail["raw"]
	}
	return map[string]interface{}{"data": map[string]interface{}{
		"path": detail["path"], "content": content, "raw": detail["raw"],
	}, "path": detail["path"], "content": content, "raw": detail["raw"], "content_type": detail["content_type"]}, nil
}

func docsResourceRoot(docsDir string) string {
	abs, err := filepath.Abs(docsDir)
	if err != nil {
		return docsDir
	}
	if filepath.Base(abs) == "docs" {
		return filepath.Dir(abs)
	}
	return abs
}

func documentMachineRequestServer(collection rest.Collection) (rest.ServerDefinition, error) {
	for name, server := range collection.Servers {
		if hasDocumentMachineRequestRoutes(server) {
			return collection.ResolveServer(name)
		}
	}
	return rest.ServerDefinition{}, fmt.Errorf("document machine_request server is not defined")
}

func hasDocumentMachineRequestRoutes(server rest.Server) bool {
	var hasIndex, hasDetail bool
	for _, endpoint := range server.Endpoints {
		if endpoint.Binding != "machine_request" {
			continue
		}
		if endpoint.Path == "/api/v1/docs" {
			hasIndex = true
		}
		if endpoint.Path == "/api/v1/docs/{path...}" {
			hasDetail = true
		}
	}
	return hasIndex && hasDetail
}

func loadRESTDefinitions(files []string, dirs []string) (rest.Collection, error) {
	return rest.LoadDefinitions(files, dirs)
}

func newRESTServerLifecycle() restServerLifecycle {
	return rest.NewServerState()
}
