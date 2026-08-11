// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const selfInvokeTraceAttributeLimit = 500

// SelfInvokeBuilder constructs self-invocation commands.
type SelfInvokeBuilder struct {
	Config      execute.Config
	RequestFrom string
	OutputFrom  string
	ExtraArgs   []string
	Ctx         context.Context
	Tracer      tracing.Tracer
}

func (b *SelfInvokeBuilder) Build(res core.Result) core.Command {
	return &selfInvokeCmd{
		config: b.Config, requestFrom: b.RequestFrom, outputFrom: b.OutputFrom,
		extraArgs: b.ExtraArgs, ctx: b.Ctx, tracer: b.Tracer, previous: res,
		runID: toolexec.ExtractStringParam(res.Output, "run_id"),
	}
}

type selfInvokeCmd struct {
	config      execute.Config
	requestFrom string
	outputFrom  string
	extraArgs   []string
	ctx         context.Context
	tracer      tracing.Tracer
	previous    core.Result
	view        core.CommandStateView
	runID       string
	tracePath   string
}

func (c *selfInvokeCmd) Name() string                               { return "self_invoke" }
func (c *selfInvokeCmd) SetCommandState(view core.CommandStateView) { c.view = view }
func (c *selfInvokeCmd) Undo(_ core.Result) core.Result {
	return undo.BoundaryCompensationUndo(c.Name(), "restore child workspace/artifacts or compensate the child agent run")
}

var (
	_ core.CommandStateAware = (*selfInvokeCmd)(nil)
	_ core.ContextCommand    = (*selfInvokeCmd)(nil)
)

func (c *selfInvokeCmd) undoPayload() undo.BoundaryCompensationPayload {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: "child_agent_workspace_restore", Reason: "self-invocation runs a child agent process",
		Requires: []string{"child_workspace_ref", "child_trace"},
		Data:     map[string]interface{}{"child_profile": c.config.Profile, "child_run_id": c.runID},
	}}
	if c.tracePath != "" {
		payload.BoundaryCompensation.Data["artifact_paths"] = []string{c.tracePath}
	}
	return payload
}

func (c *selfInvokeCmd) Execute() core.Result {
	return c.ExecuteContext(c.ctx)
}

func (c *selfInvokeCmd) ExecuteContext(ctx context.Context) core.Result {
	cfg := c.config
	if cfg.Binary == "" {
		cfg.Binary = os.Args[0]
	}
	if err := c.resolveInvocation(&cfg); err != nil {
		wrapped := fmt.Errorf("%s: %w", c.Name(), err)
		return core.Result{
			Output: wrapped.Error(), Signal: core.CommandError,
			CommandName: c.Name(), Err: wrapped,
		}
	}
	cfg = c.configWithTrace(cfg)
	if ctx == nil {
		ctx = context.Background()
	}
	result := execute.RunAgent(ctx, cfg, c.extraArgs...)
	c.traceResult(cfg, result)
	return core.Result{
		Output: result.Stdout, Signal: selfInvokeSignal(result),
		Cost: core.Cost{Duration: result.Duration}, CommandName: c.Name(),
		Receipt: undo.EncodeBoundaryReceipt(c.undoPayload()),
	}
}

func (c *selfInvokeCmd) resolveInvocation(cfg *execute.Config) error {
	var err error
	if c.requestFrom != "" {
		cfg.Request, err = c.resolveString(c.requestFrom)
		if err != nil {
			return fmt.Errorf("request_from: %w", err)
		}
	}
	if c.outputFrom != "" {
		cfg.Output, err = c.resolveString(c.outputFrom)
		if err != nil {
			return fmt.Errorf("output_from: %w", err)
		}
	}
	return nil
}

func (c *selfInvokeCmd) resolveString(selector string) (string, error) {
	value, err := core.ResolveFromSelector(c.view, selector)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("selector %q resolved to %T, want string", selector, value)
	}
	if text == "" {
		return "", fmt.Errorf("selector %q resolved to an empty string", selector)
	}
	return text, nil
}

func (c *selfInvokeCmd) configWithTrace(cfg execute.Config) execute.Config {
	if cfg.OTelDir == "" {
		return cfg
	}
	c.tracePath = fmt.Sprintf("%s/child-%s.otel.json", cfg.OTelDir, c.runID)
	cfg.OTelLogFile = c.tracePath
	return cfg
}

func (c *selfInvokeCmd) traceResult(cfg execute.Config, result *execute.Result) {
	if c.tracer == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("self_invoke.binary", modelllm.Truncate(cfg.Binary, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.profile", modelllm.Truncate(cfg.Profile, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.run_id", modelllm.Truncate(c.runID, selfInvokeTraceAttributeLimit)),
		attribute.Int("self_invoke.exit_code", result.ExitCode),
		attribute.String("self_invoke.output", modelllm.Truncate(result.Stdout, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.stdout", modelllm.Truncate(result.Stdout, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.stderr", modelllm.Truncate(result.Stderr, selfInvokeTraceAttributeLimit)),
	}
	c.tracer.SetAttributes(attrs...)
	c.tracer.Event("self_invoke.result", attrs...)
}

func selfInvokeSignal(result *execute.Result) core.Signal {
	if result.Success() {
		return core.ToolDone
	}
	return core.ToolFailed
}
