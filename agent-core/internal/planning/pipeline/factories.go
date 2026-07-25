// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// FactoryDeps holds the dependencies needed by pipeline tool factories.
type FactoryDeps struct {
	Directory        string
	ChildAgentBinary string
	Tracer           tracing.Tracer
	Ctx              context.Context
	ParseRetries     *toollm.ParseErrorRetryTracker
}

// RegisterFactories registers all pipeline builtin tool factories
// (extract_task, extract_all, assemble_prompt, parse_plan, issue state
// adapters, execute_task, mark_task_done, remaining_work) into the provided
// BuiltinRegistry.
// Pipeline state is lazily initialized on first factory call.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	var ps *State

	initPS := func(def catalog.ToolDef) *State {
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

	br.Register("load_graph", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &LoadGraphBuilder{PS: initPS(def)}, nil
	})
	br.Register("extract_task", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ExtractTaskBuilder{PS: initPS(def)}, nil
	})
	br.Register("extract_all", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ExtractAllBuilder{PS: initPS(def)}, nil
	})
	br.Register("assemble_prompt", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &AssemblePromptBuilder{PS: initPS(def)}, nil
	})
	br.Register("parse_plan", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ParsePlanBuilder{PS: initPS(def), Retry: deps.ParseRetries}, nil
	})
	br.Register("format_issue", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &FormatIssueBuilder{PS: initPS(def)}, nil
	})
	br.Register("record_tracker_issue", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &RecordTrackerIssueBuilder{PS: initPS(def)}, nil
	})
	br.Register("execute_task", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var childCfg catalog.ChildAgentConfig
		if err := catalog.DecodeToolConfig(def, &childCfg); err != nil {
			return nil, err
		}
		if err := catalog.ValidateChildAgentConfig(def.Name, childCfg); err != nil {
			return nil, fmt.Errorf("pipeline execute_task: %w", err)
		}
		ps := initPS(def)
		ps.ExecConfig = execute.Config{
			Binary:  deps.ChildAgentBinary,
			Profile: childCfg.Profile,
		}
		return &ExecuteTaskBuilder{PS: ps}, nil
	})
	br.Register("mark_task_done", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &MarkTaskDoneBuilder{PS: initPS(def)}, nil
	})
	br.Register("remaining_work", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &RemainingWorkBuilder{PS: initPS(def)}, nil
	})
}
