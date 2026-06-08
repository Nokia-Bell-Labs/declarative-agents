// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// SelfInvokeConfig configures a child agent process invocation.
type SelfInvokeConfig struct {
	Binary     string        // defaults to os.Args[0]
	Machine    string        // --machine flag for child
	Tools      string        // --tools flag for child
	Directory  string        // --directory for child
	Model      string        // --model for child
	OllamaURL  string        // --ollama-url for child
	Timeout    time.Duration // child process timeout
	LLMTimeout time.Duration // --llm-timeout for child
	MaxTime    time.Duration // --max-time for child
	OTelDir    string        // directory for child trace file
}

// SelfInvokeResult holds the outcome of a child agent invocation.
type SelfInvokeResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

func (r *SelfInvokeResult) Success() bool { return r.ExitCode == 0 }

// SelfInvoke runs the agent binary with different configuration.
// It propagates OTel context to the child process.
func SelfInvoke(ctx context.Context, tracer tracing.Tracer, cfg SelfInvokeConfig, runID string) (*SelfInvokeResult, error) {
	binary := cfg.Binary
	if binary == "" {
		binary = os.Args[0]
	}

	args := buildSelfInvokeArgs(cfg, runID, ctx)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(childCtx, binary, args...)
	ProcGroupCmd(cmd)

	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := &SelfInvokeResult{
		Stdout:   string(output),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if childCtx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
		} else {
			return nil, fmt.Errorf("self-invoke %s: %w", binary, err)
		}
	}

	if tracer != nil {
		tracer.SetAttributes(
			attribute.String("self_invoke.binary", binary),
			attribute.String("self_invoke.run_id", runID),
			attribute.Int("self_invoke.exit_code", result.ExitCode),
			attribute.String("self_invoke.output", llm.Truncate(result.Stdout, 500)),
		)
	}

	return result, nil
}

// buildSelfInvokeArgs constructs the CLI argument list from config.
func buildSelfInvokeArgs(cfg SelfInvokeConfig, runID string, ctx context.Context) []string {
	var args []string

	if cfg.Machine != "" {
		args = append(args, "--machine", cfg.Machine)
	}
	if cfg.Tools != "" {
		args = append(args, "--tools", cfg.Tools)
	}
	if cfg.Directory != "" {
		args = append(args, "--directory", cfg.Directory)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.OllamaURL != "" {
		args = append(args, "--ollama-url", cfg.OllamaURL)
	}
	if cfg.LLMTimeout > 0 {
		args = append(args, "--llm-timeout", cfg.LLMTimeout.String())
	}
	if cfg.MaxTime > 0 {
		args = append(args, "--max-time", cfg.MaxTime.String())
	}

	if cfg.OTelDir != "" {
		tracePath := fmt.Sprintf("%s/child-%s.otel.json", cfg.OTelDir, runID)
		args = append(args, "--otel-log-file", tracePath)
	}

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		args = append(args, "--otel-parent-span", telemetry.FormatTraceparent(span.SpanContext()))
	}

	return args
}

// SelfInvokeBuilder constructs self-invocation commands as a builtin factory.
type SelfInvokeBuilder struct {
	Config SelfInvokeConfig
	Ctx    context.Context
	Tracer tracing.Tracer
}

func (b *SelfInvokeBuilder) Build(res core.Result) core.Command {
	return &selfInvokeCmd{
		config: b.Config,
		ctx:    b.Ctx,
		tracer: b.Tracer,
		runID:  ExtractStringParam(res.Output, "run_id"),
	}
}

type selfInvokeCmd struct {
	config SelfInvokeConfig
	ctx    context.Context
	tracer tracing.Tracer
	runID  string
}

func (c *selfInvokeCmd) Name() string { return "self_invoke" }

func (c *selfInvokeCmd) Execute() core.Result {
	result, err := SelfInvoke(c.ctx, c.tracer, c.config, c.runID)
	if err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.CommandError,
			Err:         err,
			CommandName: "self_invoke",
		}
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
