// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/subprocess"
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
// It propagates OTel context to the child process via subprocess.Run.
func SelfInvoke(ctx context.Context, tracer tracing.Tracer, cfg SelfInvokeConfig, runID string) (*SelfInvokeResult, error) {
	binary := cfg.Binary
	if binary == "" {
		binary = os.Args[0]
	}

	args, env := buildSelfInvokeArgs(cfg, runID)
	spec := subprocess.Spec{
		Binary:        binary,
		Args:          args,
		Env:           env,
		Timeout:       cfg.Timeout,
		PropagateOTel: true,
	}

	r := subprocess.Run(ctx, spec)
	if r.Err != nil {
		return nil, fmt.Errorf("self-invoke %s: %w", binary, r.Err)
	}

	result := &SelfInvokeResult{
		Stdout:   r.Stdout,
		ExitCode: r.ExitCode,
		Duration: r.Duration,
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

func buildSelfInvokeArgs(cfg SelfInvokeConfig, runID string) (args, env []string) {
	if cfg.Machine != "" {
		args = append(args, "--machine", cfg.Machine)
	}
	if cfg.Tools != "" {
		args = append(args, "--tools", cfg.Tools)
	}
	if cfg.Directory != "" {
		args = append(args, "--directory", cfg.Directory)
	}
	if cfg.OTelDir != "" {
		tracePath := fmt.Sprintf("%s/child-%s.otel.json", cfg.OTelDir, runID)
		args = append(args, "--otel-log-file", tracePath)
	}

	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.OllamaURL != "" {
		args = append(args, "--ollama-url", cfg.OllamaURL)
	}

	return args, nil
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
