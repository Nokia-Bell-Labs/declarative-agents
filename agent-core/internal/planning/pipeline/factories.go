// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/stl"
)

// FactoryDeps holds the dependencies needed by pipeline tool factories.
type FactoryDeps struct {
	Directory string
	Tracer    tracing.Tracer
	Ctx       context.Context
}

// RegisterFactories registers all pipeline builtin tool factories
// (extract_task, extract_all, assemble_prompt, parse_plan, create_issue,
// execute_task, check_result) into the provided BuiltinRegistry.
// Pipeline state is lazily initialized on first factory call.
func RegisterFactories(br *stl.BuiltinRegistry, deps FactoryDeps) {
	var ps *State

	initPS := func(def stl.ToolDef) *State {
		if ps != nil {
			return ps
		}

		ps = &State{
			Directory: deps.Directory,
			Tracer:    deps.Tracer,
			Ctx:       deps.Ctx,
			TaskDeps:  make(map[string]string),
		}
		return ps
	}

	br.Register("extract_task", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ExtractTaskBuilder{PS: initPS(def)}, nil
	})
	br.Register("extract_all", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ExtractAllBuilder{PS: initPS(def)}, nil
	})
	br.Register("assemble_prompt", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &AssemblePromptBuilder{PS: initPS(def)}, nil
	})
	br.Register("parse_plan", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ParsePlanBuilder{PS: initPS(def)}, nil
	})
	br.Register("create_issue", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &CreateIssueBuilder{PS: initPS(def)}, nil
	})
	br.Register("execute_task", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		var childCfg stl.ChildAgentConfig
		if err := stl.DecodeToolConfig(def, &childCfg); err != nil {
			return nil, err
		}
		if err := stl.ValidateChildAgentConfig(def.Name, childCfg); err != nil {
			return nil, fmt.Errorf("pipeline execute_task: %w", err)
		}
		ps := initPS(def)
		ps.ExecConfig = execute.Config{
			Profile:          childCfg.Profile,
			Machine:          childCfg.Machine,
			Tools:            childCfg.Tools,
			ToolDeclarations: childCfg.ToolDeclarations,
		}
		return &ExecuteTaskBuilder{PS: ps}, nil
	})
	br.Register("check_result", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &CheckResultBuilder{PS: initPS(def)}, nil
	})
}
