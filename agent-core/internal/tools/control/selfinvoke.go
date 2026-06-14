// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"

	modelllm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	toolexec "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/exec"
)

// SelfInvokeBuilder constructs self-invocation commands.
type SelfInvokeBuilder struct {
	Config    execute.Config
	ExtraArgs []string
	Ctx       context.Context
	Tracer    tracing.Tracer
}

func (b *SelfInvokeBuilder) Build(res core.Result) core.Command {
	return &selfInvokeCmd{
		config: b.Config, extraArgs: b.ExtraArgs, ctx: b.Ctx,
		tracer: b.Tracer, runID: toolexec.ExtractStringParam(res.Output, "run_id"),
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
	return BoundaryCompensationUndo(c.Name(), "restore child workspace/artifacts or compensate the child agent run")
}

func (c *selfInvokeCmd) UndoMemento() (core.UndoMemento, error) {
	payload := c.undoPayload()
	return BoundaryCompensationMemento(c.Name(), payload, "restore child workspace/artifacts or compensate the child agent run")
}

func (c *selfInvokeCmd) undoPayload() BoundaryCompensationPayload {
	payload := BoundaryCompensationPayload{BoundaryCompensation: BoundaryCompensation{
		Strategy: "child_agent_workspace_restore", Reason: "self-invocation runs a child agent process",
		Requires: []string{"child_workspace_ref", "child_trace"}, ChildProfile: c.config.Profile,
		ChildMachine: c.config.Machine, ChildTools: c.config.Tools, ChildRunID: c.runID,
	}}
	if c.tracePath != "" {
		payload.BoundaryCompensation.ArtifactPaths = []string{c.tracePath}
	}
	return payload
}

func (c *selfInvokeCmd) Execute() core.Result {
	cfg := c.config
	if cfg.Binary == "" {
		cfg.Binary = os.Args[0]
	}
	extra := c.extraWithTrace(cfg)
	result := execute.RunAgent(c.ctx, cfg, extra...)
	c.traceResult(cfg, result)
	return core.Result{
		Output: result.Stdout, Signal: selfInvokeSignal(result),
		Cost: core.Cost{Duration: result.Duration}, CommandName: c.Name(),
	}
}

func (c *selfInvokeCmd) extraWithTrace(cfg execute.Config) []string {
	extra := append([]string{}, c.extraArgs...)
	if cfg.OTelDir == "" {
		return extra
	}
	c.tracePath = fmt.Sprintf("%s/child-%s.otel.json", cfg.OTelDir, c.runID)
	return append(extra, "--otel-log-file", c.tracePath)
}

func (c *selfInvokeCmd) traceResult(cfg execute.Config, result *execute.Result) {
	if c.tracer == nil {
		return
	}
	c.tracer.SetAttributes(
		attribute.String("self_invoke.binary", cfg.Binary),
		attribute.String("self_invoke.run_id", c.runID),
		attribute.Int("self_invoke.exit_code", result.ExitCode),
		attribute.String("self_invoke.output", modelllm.Truncate(result.Stdout, 500)),
	)
}

func selfInvokeSignal(result *execute.Result) core.Signal {
	if result.Success() {
		return core.ToolDone
	}
	return core.ToolFailed
}

// SelfInvokeToolSpec returns the ToolSpec for the self_invoke command.
func SelfInvokeToolSpec() core.ToolSpec {
	return core.ToolSpec{Name: "self_invoke", Visibility: core.Internal}
}
