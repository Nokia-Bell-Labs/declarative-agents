// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
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
	mu          sync.Mutex
	runner      *ProfileWorkflowRunner
}

// NewLazyProfileWorkflowRunner creates a runner that uses documentation-curator profile config.
func NewLazyProfileWorkflowRunner(profilePath string) *LazyProfileWorkflowRunner {
	return &LazyProfileWorkflowRunner{profilePath: profilePath}
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
	runner, err := NewProfileWorkflowRunner(r.profilePath)
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
func NewProfileWorkflowRunner(profilePath string) (*ProfileWorkflowRunner, error) {
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
	return NewProfileWorkflowRunnerFromDefs(collection, selected)
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
