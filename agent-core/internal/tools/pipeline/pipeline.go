// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package pipeline provides the pipeline builtin (srd049): one word that runs
// an ordered sequence of pure data-transform stages and emits one declared
// signal. The stage vocabulary is the existing pure inits -- a stage config
// is the config the standalone word takes -- and stages chain through the
// previous-Result contract, so the current-value selector inside a stage
// means the previous stage's output with its existing semantics (srd038).
// The machine keeps every state that decides; this word absorbs the states
// that only reshape.
package pipeline

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// stage pairs the built stage with the name failures and spans report.
type stage struct {
	name    string
	builder core.Builder
}

// Builder constructs pipeline commands from eagerly built stages (srd049
// R1.5): every stage already passed its own factory, so a config the
// standalone word would refuse never reaches a run.
type Builder struct {
	ToolName string
	Signal   core.Signal
	Stages   []stage
}

// Build returns a pipeline command. The engine injects the command-state
// view, tracer, and monitor recorder before dispatch; the command propagates
// each to the stages that accept it (srd049 R3.4).
func (b Builder) Build(prev core.Result) core.Command {
	return &pipelineCmd{name: b.ToolName, signal: b.Signal, stages: b.Stages, prev: prev}
}

type pipelineCmd struct {
	name     string
	signal   core.Signal
	stages   []stage
	prev     core.Result
	view     core.CommandStateView
	tracer   tracing.Tracer
	recorder monitor.ToolMetricsRecorder
}

func (c *pipelineCmd) Name() string { return c.name }

func (c *pipelineCmd) SetCommandState(view core.CommandStateView)       { c.view = view }
func (c *pipelineCmd) SetTracer(tracer tracing.Tracer)                  { c.tracer = tracer }
func (c *pipelineCmd) SetMonitorRecorder(r monitor.ToolMetricsRecorder) { c.recorder = r }

var (
	_ core.CommandStateAware    = (*pipelineCmd)(nil)
	_ core.TracerAware          = (*pipelineCmd)(nil)
	_ core.MonitorRecorderAware = (*pipelineCmd)(nil)
)

// Undo is a noop: every admissible stage is pure (srd049 R5.1).
func (c *pipelineCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

// Execute runs the stages in declared order. The first stage's previous
// Result is the pipeline's own, each later stage reads its predecessor
// (srd049 R3.1), and a stage that fails ends the run as CommandError naming
// the stage (R4.1). The final stage's output becomes the pipeline's output
// under the configured signal (R3.3).
func (c *pipelineCmd) Execute() core.Result {
	prev := c.prev
	for _, entry := range c.stages {
		command := entry.builder.Build(prev)
		c.inject(command)
		result := command.Execute()
		if result.Err != nil || result.Signal == core.CommandError {
			err := result.Err
			if err == nil {
				err = fmt.Errorf("stage emitted %s", core.CommandError)
			}
			return core.Result{
				CommandName: c.name,
				Signal:      core.CommandError,
				Output:      result.Output,
				Err:         fmt.Errorf("pipeline %s stage %s: %w", c.name, entry.name, err),
			}
		}
		prev = result
	}
	prev.CommandName = c.name
	prev.Signal = c.signal
	return prev
}

// inject hands the pipeline's injected dependencies to a stage that accepts
// them, so a stage records under the pipeline's dispatch span rather than
// escaping observability (srd049 R3.4).
func (c *pipelineCmd) inject(command core.Command) {
	if aware, ok := command.(core.CommandStateAware); ok && c.view != nil {
		aware.SetCommandState(c.view)
	}
	if aware, ok := command.(core.TracerAware); ok && c.tracer != nil {
		aware.SetTracer(c.tracer)
	}
	if aware, ok := command.(core.MonitorRecorderAware); ok && c.recorder != nil {
		aware.SetMonitorRecorder(c.recorder)
	}
}
