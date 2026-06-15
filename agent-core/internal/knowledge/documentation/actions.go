// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
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
	machine  *LazyMachineDocsRunner
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
	runner.machine = NewLazyMachineDocsRunner(profilePath, docsDir)
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
	if r.machine != nil && machineBackedAction(action.Type) {
		return r.machine.RunAction(req.Context(), action.Type, action.Params)
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
		Data:   actionData(parsed),
		Tool:   tool,
		Signal: string(result.Signal),
		Output: parsed,
	}
	if result.Signal == core.CommandError {
		return response, fmt.Errorf("%s failed: %s", tool, result.Output)
	}
	return response, nil
}

func actionData(output map[string]interface{}) interface{} {
	if body, ok := output["body"].(map[string]interface{}); ok {
		if data, ok := body["data"]; ok {
			return data
		}
	}
	if mapped, ok := output["mapped"].(map[string]interface{}); ok && len(mapped) > 0 {
		return mapped
	}
	return output
}

type MachineDocResponse struct {
	Status int
	Signal string
	Body   map[string]interface{}
}

type LazyMachineDocsRunner struct {
	profilePath string
	docsDir     string
	mu          sync.Mutex
	runner      *MachineDocsRunner
}

func NewLazyMachineDocsRunner(profilePath, docsDir string) *LazyMachineDocsRunner {
	return &LazyMachineDocsRunner{profilePath: profilePath, docsDir: docsDir}
}

func (r *LazyMachineDocsRunner) List(ctx context.Context) (MachineDocResponse, error) {
	runner, err := r.machineRunner()
	if err != nil {
		return MachineDocResponse{}, err
	}
	return runner.List(ctx)
}

func (r *LazyMachineDocsRunner) Get(ctx context.Context, path string) (MachineDocResponse, error) {
	runner, err := r.machineRunner()
	if err != nil {
		return MachineDocResponse{}, err
	}
	return runner.Get(ctx, path)
}

func (r *LazyMachineDocsRunner) RunAction(ctx context.Context, action string, params map[string]interface{}) (ActionResponse, error) {
	result, err := r.runActionResponse(ctx, action, params)
	if err != nil {
		return ActionResponse{}, err
	}
	return ActionResponse{Data: result.Body["data"], Tool: action, Signal: result.Signal, Output: result.Body}, nil
}

func (r *LazyMachineDocsRunner) runActionResponse(ctx context.Context, action string, params map[string]interface{}) (MachineDocResponse, error) {
	if action == "doc_list" {
		return r.List(ctx)
	}
	path, _ := params["path"].(string)
	return r.Get(ctx, path)
}

func (r *LazyMachineDocsRunner) machineRunner() (*MachineDocsRunner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runner != nil {
		return r.runner, nil
	}
	runner, err := NewMachineDocsRunner(r.profilePath, r.docsDir)
	if err != nil {
		return nil, err
	}
	r.runner = runner
	return runner, nil
}

type MachineDocsRunner struct {
	machine  core.MachineSpec
	registry *core.Registry
}

func NewMachineDocsRunner(profilePath, docsDir string) (*MachineDocsRunner, error) {
	profile, err := catalog.LoadProfile(profilePath)
	if err != nil {
		return nil, err
	}
	machine, err := core.LoadMachineSpec(requestMachinePath(profile.Machine))
	if err != nil {
		return nil, err
	}
	registry, err := requestMachineRegistry(profile.ToolDeclarations, docsResourceRoot(docsDir))
	if err != nil {
		return nil, err
	}
	return &MachineDocsRunner{machine: machine, registry: registry}, nil
}

func (r *MachineDocsRunner) List(ctx context.Context) (MachineDocResponse, error) {
	return r.run(ctx, map[string]interface{}{"resource": "docs"}, false)
}

func (r *MachineDocsRunner) Get(ctx context.Context, path string) (MachineDocResponse, error) {
	return r.run(ctx, map[string]interface{}{"resource": "docs", "path": path}, true)
}

func (r *MachineDocsRunner) run(ctx context.Context, params map[string]interface{}, read bool) (MachineDocResponse, error) {
	var last core.Result
	spec := requestSpecFor(r.machine, read)
	result, err := core.Loop(core.LoopParams{
		MachineSpec: &spec, Registry: r.registry, InitialSignal: core.Seed,
		InitialResult: parametersResult(params), Budget: requestMachineBudget(spec),
		Trace: tracing.NoopTracer{}, AgentName: "documentation_request",
		Hooks: core.LoopHooks{OnResult: func(rr core.RunResult, res core.Result) core.RunResult {
			last = res
			return rr
		}},
	}, ctx)
	if err != nil {
		return MachineDocResponse{}, err
	}
	return machineDocResponse(result, last), nil
}

func requestMachineRegistry(paths []string, root string) (*core.Registry, error) {
	defs, err := requestResourceDefs(paths)
	if err != nil {
		return nil, err
	}
	reg := core.NewRegistry()
	builtins := requestBuiltinRegistry()
	for _, def := range defs {
		if err := toolregistry.RegisterSingleBuiltin(reg, builtins, def, map[string]string{"directory": root}); err != nil {
			return nil, err
		}
	}
	registerResponseWords(reg)
	return reg, nil
}

func requestResourceDefs(paths []string) ([]catalog.ToolDef, error) {
	declarations, err := catalog.LoadToolDeclarations(paths)
	if err != nil {
		return nil, err
	}
	return catalog.SelectTools(declarations, []string{"doc_list_resource", "doc_read_resource"})
}

func requestBuiltinRegistry() *toolregistry.BuiltinRegistry {
	builtins := toolregistry.NewBuiltinRegistry()
	registerResourceFactory(builtins, "list_resource", func(root string, cfg filesystem.ResourceConfig) core.Builder {
		return &filesystem.ListResourceBuilder{Root: root, Resources: cfg}
	})
	registerResourceFactory(builtins, "read_resource", func(root string, cfg filesystem.ResourceConfig) core.Builder {
		return &filesystem.ReadResourceBuilder{Root: root, Resources: cfg}
	})
	return builtins
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

func registerResponseWords(reg *core.Registry) {
	reg.Register(core.ToolSpec{Name: "doc_index_response"}, responseBuilder{name: "doc_index_response", signal: "DocumentIndexReady"})
	reg.Register(core.ToolSpec{Name: "doc_detail_response"}, responseBuilder{name: "doc_detail_response", signal: "DocumentDetailReady"})
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

func requestSpecFor(machine core.MachineSpec, read bool) core.MachineSpec {
	if !read {
		return machine
	}
	for i, transition := range machine.Transitions {
		if transition.State == "AwaitingRequest" && transition.Signal == "Seed" {
			machine.Transitions[i].Next = "ReadingDocument"
			machine.Transitions[i].Action = "doc_read_resource"
		}
	}
	return machine
}

func requestMachineBudget(spec core.MachineSpec) core.Budget {
	return spec.BudgetSpec.ToBudget(core.Budget{MaxIterations: 10})
}

func parametersResult(params map[string]interface{}) core.Result {
	data, err := json.Marshal(map[string]interface{}{"parameters": params})
	if err != nil {
		return core.Result{Signal: core.Seed, Output: `{"parameters":{}}`}
	}
	return core.Result{Signal: core.Seed, Output: string(data)}
}

func machineDocResponse(run core.RunResult, last core.Result) MachineDocResponse {
	body := machineDocBody(last)
	return MachineDocResponse{Status: machineDocStatus(last.Signal), Signal: string(last.Signal), Body: bodyWithTrace(body, run)}
}

func machineDocBody(last core.Result) map[string]interface{} {
	if machineDocStatus(last.Signal) != http.StatusOK {
		return map[string]interface{}{"error": string(last.Signal)}
	}
	body := map[string]interface{}{}
	if err := json.Unmarshal([]byte(last.Output), &body); err != nil {
		return map[string]interface{}{"error": "response_invalid"}
	}
	return body
}

func bodyWithTrace(body map[string]interface{}, run core.RunResult) map[string]interface{} {
	body["trace"] = map[string]interface{}{"status": run.Status, "iterations": run.Iterations}
	return body
}

func machineDocStatus(signal core.Signal) int {
	switch signal {
	case "DocumentIndexReady", "DocumentDetailReady":
		return http.StatusOK
	case "DocumentMissing":
		return http.StatusNotFound
	case "DocumentResourceDenied":
		return http.StatusForbidden
	case "DocumentParseFailed":
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func writeMachineDocHTTP(w http.ResponseWriter, result MachineDocResponse, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result.Status, result.Body)
}

func machineBackedAction(action string) bool {
	return action == "doc_list" || action == "doc_get"
}

func requestMachinePath(machinePath string) string {
	return filepath.Join(filepath.Dir(machinePath), "request-machine.yaml")
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
