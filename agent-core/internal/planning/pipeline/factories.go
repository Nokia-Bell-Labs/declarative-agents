// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// FactoryDeps holds the dependencies needed by pipeline tool factories.
type FactoryDeps struct {
	Directory        string
	ChildAgentBinary string
	CoreRoot         string
	Tracer           tracing.Tracer
	Ctx              context.Context
	ParseRetries     *toollm.ParseErrorRetryTracker
}

type passThroughPlanConfig struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// RegisterFactories registers all pipeline builtin tool factories
// (extract_task, select_all_ready, seed_passthrough_plan, mark_nodes_planning,
// assemble_prompt, parse_plan, issue state
// adapters, task formatting, graph lifecycle, mark_task_done, and
// remaining_work) into the provided
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
	br.Register("select_all_ready", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &SelectAllReadyBuilder{PS: initPS(def)}, nil
	})
	br.Register("seed_passthrough_plan", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg passThroughPlanConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.Title == "" || cfg.Summary == "" {
			return nil, fmt.Errorf("pipeline seed_passthrough_plan: title and summary are required")
		}
		return &SeedPassThroughPlanBuilder{PS: initPS(def), Title: cfg.Title, Summary: cfg.Summary}, nil
	})
	br.Register("mark_nodes_planning", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &MarkNodesPlanningBuilder{PS: initPS(def)}, nil
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
	br.Register("mark_nodes_executing", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &MarkNodesExecutingBuilder{PS: initPS(def)}, nil
	})
	br.Register("format_task_file", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &FormatTaskFileBuilder{PS: initPS(def)}, nil
	})
	br.Register("mark_task_done", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &MarkTaskDoneBuilder{PS: initPS(def)}, nil
	})
	br.Register("mark_task_failed", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &MarkTaskFailedBuilder{PS: initPS(def)}, nil
	})
	br.Register("remaining_work", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &RemainingWorkBuilder{PS: initPS(def)}, nil
	})
}
