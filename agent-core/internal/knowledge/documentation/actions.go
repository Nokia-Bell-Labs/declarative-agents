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

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/filesystem"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/rest"
)

const defaultCuratorProfilePath = "agents/knowledge-manager/documentation-curator/profile.yaml"

var allowedWorkflowActions = map[string]bool{
	"doc_list":            true,
	"doc_get":             true,
	"doc_search":          true,
	"doc_validate":        true,
	"doc_suggest_changes": true,
	"doc_patch_approve":   true,
	"doc_patch_reject":    true,
	"doc_patch_reopen":    true,
}

// WorkflowRunner executes documentation-curator UI actions through tool words.
type WorkflowRunner interface {
	Run(r *http.Request) (ActionResponse, error)
}

// ActionResponse is the action API envelope returned to the UI.
type ActionResponse struct {
	Data   interface{} `json:"data,omitempty"`
	Tool   string      `json:"tool"`
	Signal string      `json:"signal"`
	Output interface{} `json:"output,omitempty"`
}

type actionRequest struct {
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// LazyProfileWorkflowRunner loads the profile-backed REST tool registry on first use.
type LazyProfileWorkflowRunner struct {
	profilePath string
	docsDir     string
	mu          sync.Mutex
	runner      *ProfileWorkflowRunner
}

// NewLazyProfileWorkflowRunner creates a runner that uses documentation-curator profile config.
func NewLazyProfileWorkflowRunner(profilePath, docsDir string) *LazyProfileWorkflowRunner {
	return &LazyProfileWorkflowRunner{profilePath: profilePath, docsDir: docsDir}
}

// Run executes one workflow action through the loaded REST tool registry.
func (r *LazyProfileWorkflowRunner) Run(req *http.Request) (ActionResponse, error) {
	runner, err := r.profileRunner()
	if err != nil {
		return ActionResponse{}, err
	}
	return runner.Run(req)
}

func (r *LazyProfileWorkflowRunner) profileRunner() (*ProfileWorkflowRunner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runner != nil {
		return r.runner, nil
	}
	runner, err := NewProfileWorkflowRunner(r.profilePath, r.docsDir)
	if err != nil {
		return nil, err
	}
	r.runner = runner
	return runner, nil
}

// ProfileWorkflowRunner dispatches action requests to selected REST client tools.
type ProfileWorkflowRunner struct {
	registry *core.Registry
}

// NewProfileWorkflowRunner loads profile tool declarations and REST definitions.
func NewProfileWorkflowRunner(profilePath, docsDir string) (*ProfileWorkflowRunner, error) {
	profile, err := catalog.LoadProfile(profilePath)
	if err != nil {
		return nil, err
	}
	collection, err := rest.LoadDefinitions(profile.RestDefinitions, profile.RestConfigDirs)
	if err != nil {
		return nil, err
	}
	declarations, err := catalog.LoadToolDeclarations(profile.ToolDeclarations)
	if err != nil {
		return nil, err
	}
	selection, err := catalog.LoadToolSelections(profile.Tools)
	if err != nil {
		return nil, err
	}
	selected, err := catalog.SelectTools(declarations, selection)
	if err != nil {
		return nil, err
	}
	runner, err := NewProfileWorkflowRunnerFromDefs(collection, selected)
	if err != nil {
		return nil, err
	}
	return runner, nil
}

// NewProfileWorkflowRunnerFromDefs creates a runner from already-loaded config.
func NewProfileWorkflowRunnerFromDefs(collection rest.Collection, defs []catalog.ToolDef) (*ProfileWorkflowRunner, error) {
	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	rest.RegisterFactories(builtins, rest.FactoryDeps{Definitions: collection})
	for _, def := range defs {
		if !allowedWorkflowActions[def.Name] {
			continue
		}
		if err := toolregistry.RegisterSingleBuiltin(reg, builtins, def, nil); err != nil {
			return nil, err
		}
	}
	return &ProfileWorkflowRunner{registry: reg}, nil
}

// Run decodes and dispatches one action request.
func (r *ProfileWorkflowRunner) Run(req *http.Request) (ActionResponse, error) {
	var action actionRequest
	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(&action); err != nil {
		return ActionResponse{}, fmt.Errorf("invalid action payload")
	}
	if !allowedWorkflowActions[action.Type] {
		return ActionResponse{}, fmt.Errorf("unsupported documentation action %q", action.Type)
	}
	builder, ok := r.registry.Resolve(action.Type)
	if !ok {
		return ActionResponse{}, fmt.Errorf("documentation action %q is not selected", action.Type)
	}
	output, err := json.Marshal(action.Params)
	if err != nil {
		return ActionResponse{}, fmt.Errorf("encode action params: %w", err)
	}
	result := builder.Build(core.Result{Output: string(output)}).Execute()
	return responseFromResult(action.Type, result)
}

func responseFromResult(tool string, result core.Result) (ActionResponse, error) {
	parsed := map[string]interface{}{}
	if strings.TrimSpace(result.Output) != "" {
		if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
			return ActionResponse{}, fmt.Errorf("decode %s result: %w", tool, err)
		}
	}
	response := ActionResponse{
		Data:   actionData(tool, parsed),
		Tool:   tool,
		Signal: string(result.Signal),
		Output: parsed,
	}
	if result.Signal == core.CommandError {
		return response, fmt.Errorf("%s failed: %s", tool, result.Output)
	}
	return response, nil
}

func actionData(tool string, output map[string]interface{}) interface{} {
	if tool == "doc_list" {
		return responseBodyData(output)
	}
	if tool == "doc_get" {
		return docGetActionData(output)
	}
	if mapped, ok := output["mapped"].(map[string]interface{}); ok && len(mapped) > 0 {
		return mapped
	}
	return responseBodyData(output)
}

func docGetActionData(output map[string]interface{}) interface{} {
	body, _ := output["body"].(map[string]interface{})
	if body == nil {
		return responseBodyData(output)
	}
	return map[string]interface{}{
		"path":    body["path"],
		"content": body["data"],
		"raw":     body["raw"],
	}
}

func responseBodyData(output map[string]interface{}) interface{} {
	if body, ok := output["body"].(map[string]interface{}); ok {
		if data, ok := body["data"]; ok {
			return data
		}
	}
	return output
}

// LazyMachineRequestProxy forwards document API requests to the configured REST server.
type LazyMachineRequestProxy struct {
	profilePath string
	docsDir     string
	mu          sync.Mutex
	state       *rest.ServerState
	baseURL     string
}

// NewLazyMachineRequestProxy creates a same-origin proxy for document machine requests.
func NewLazyMachineRequestProxy(profilePath, docsDir string) *LazyMachineRequestProxy {
	return &LazyMachineRequestProxy{profilePath: profilePath, docsDir: docsDir}
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
	p.state = nil
	p.baseURL = ""
	p.mu.Unlock()
	if state == nil {
		return nil
	}
	_, err := state.Stop("documentation_curator_requests")
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

func (p *LazyMachineRequestProxy) launchBackend() (string, *rest.ServerState, error) {
	profile, err := catalog.LoadProfile(p.profilePath)
	if err != nil {
		return "", nil, err
	}
	collection, err := rest.LoadDefinitions(profile.RestDefinitions, profile.RestConfigDirs)
	if err != nil {
		return "", nil, err
	}
	def, err := collection.ResolveServer("documentation_curator_requests")
	if err != nil {
		return "", nil, err
	}
	def.Server.Address = "127.0.0.1:0"
	def.MachineRequestRunner = p.requestRunner()
	state := rest.NewServerState()
	output, err := state.Launch(def)
	if err != nil {
		return "", nil, err
	}
	return "http://" + output["address"].(string), state, nil
}

func (p *LazyMachineRequestProxy) requestRunner() rest.MachineRequestRunner {
	return rest.NewProfileMachineRequestRunner(rest.ProfileMachineRequestRunnerDeps{
		BaseDir:          filepath.Dir(p.profilePath),
		Directory:        docsResourceRoot(p.docsDir),
		RegisterBuiltins: registerMachineRequestFactories,
	})
}

func registerMachineRequestFactories(br *toolregistry.BuiltinRegistry, selected map[string]bool) {
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
	var req struct {
		Payload map[string]interface{} `json:"payload"`
		Path    string                 `json:"path"`
	}
	_ = json.Unmarshal([]byte(output), &req)
	params := req.Payload
	if params == nil {
		params = map[string]interface{}{}
	}
	if _, ok := params["resource"]; !ok {
		params["resource"] = "docs"
	}
	if _, ok := params["path"].(string); !ok {
		if path := strings.TrimPrefix(req.Path, "/api/v1/docs/"); path != req.Path {
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
	req.Header = r.Header.Clone()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	writeProxiedDocumentResponse(w, resp)
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

func (c responseCmd) Name() string      { return c.name }
func (c responseCmd) Undo() core.Result { return core.NoopUndo(c.name) }

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
