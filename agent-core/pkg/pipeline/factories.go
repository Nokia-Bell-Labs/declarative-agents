// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
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

		var childCfg stl.ChildAgentConfig
		_ = stl.DecodeToolConfig(def, &childCfg)

		ps = &State{
			Directory: deps.Directory,
			Tracer:    deps.Tracer,
			Ctx:       deps.Ctx,
			TaskDeps:  make(map[string]string),
			ExecConfig: execute.Config{
				Machine:          childCfg.Machine,
				Tools:            childCfg.Tools,
				ToolDeclarations: childCfg.ToolDeclarations,
			},
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
		return &ExecuteTaskBuilder{PS: initPS(def)}, nil
	})
	br.Register("check_result", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &CheckResultBuilder{PS: initPS(def)}, nil
	})
}
