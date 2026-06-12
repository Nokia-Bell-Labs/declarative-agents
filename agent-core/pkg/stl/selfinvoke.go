// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// SelfInvokeBuilder constructs self-invocation commands as a builtin factory.
// It uses execute.Config for child agent invocation, eliminating
// duplicated argument construction.
type SelfInvokeBuilder struct {
	Config    execute.Config
	ExtraArgs []string
	Ctx       context.Context
	Tracer    tracing.Tracer
}

func (b *SelfInvokeBuilder) Build(res core.Result) core.Command {
	return &selfInvokeCmd{
		config:    b.Config,
		extraArgs: b.ExtraArgs,
		ctx:       b.Ctx,
		tracer:    b.Tracer,
		runID:     ExtractStringParam(res.Output, "run_id"),
	}
}

type selfInvokeCmd struct {
	config    execute.Config
	extraArgs []string
	ctx       context.Context
	tracer    tracing.Tracer
	runID     string
	tracePath string
}

func (c *selfInvokeCmd) Name() string { return "self_invoke" }
func (c *selfInvokeCmd) Undo() core.Result {
	return boundaryCompensationUndo(c.Name(), "restore child workspace/artifacts or compensate the child agent run")
}
func (c *selfInvokeCmd) UndoMemento() (core.UndoMemento, error) {
	payload := BoundaryCompensationPayload{BoundaryCompensation: BoundaryCompensation{
		Strategy:     "child_agent_workspace_restore",
		Reason:       "self-invocation runs a child agent process",
		Requires:     []string{"child_workspace_ref", "child_trace"},
		ChildMachine: c.config.Machine,
		ChildTools:   c.config.Tools,
		ChildRunID:   c.runID,
	}}
	if c.tracePath != "" {
		payload.BoundaryCompensation.ArtifactPaths = []string{c.tracePath}
	}
	return boundaryCompensationMemento(c.Name(), payload, "restore child workspace/artifacts or compensate the child agent run")
}

func (c *selfInvokeCmd) Execute() core.Result {
	cfg := c.config
	if cfg.Binary == "" {
		cfg.Binary = os.Args[0]
	}

	extra := append([]string{}, c.extraArgs...)
	if cfg.OTelDir != "" {
		tracePath := fmt.Sprintf("%s/child-%s.otel.json", cfg.OTelDir, c.runID)
		c.tracePath = tracePath
		extra = append(extra, "--otel-log-file", tracePath)
	}

	result := execute.RunAgent(c.ctx, cfg, extra...)

	if c.tracer != nil {
		c.tracer.SetAttributes(
			attribute.String("self_invoke.binary", cfg.Binary),
			attribute.String("self_invoke.run_id", c.runID),
			attribute.Int("self_invoke.exit_code", result.ExitCode),
			attribute.String("self_invoke.output", llm.Truncate(result.Stdout, 500)),
		)
	}

	signal := core.ToolDone
	if !result.Success() {
		signal = core.ToolFailed
	}

	return core.Result{
		Output:      result.Stdout,
		Signal:      signal,
		Cost:        core.Cost{Duration: result.Duration},
		CommandName: "self_invoke",
	}
}

// SelfInvokeToolSpec returns the ToolSpec for the self_invoke command.
func SelfInvokeToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:       "self_invoke",
		Visibility: core.Internal,
	}
}
