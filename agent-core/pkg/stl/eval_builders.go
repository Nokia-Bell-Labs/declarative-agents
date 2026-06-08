// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// EvalState holds shared mutable state for eval tools, analogous to
// pipeline.State for pipeline tools. The eval session orchestrator
// sets PC before each point's loop runs.
type EvalState struct {
	PC  *PointContext
	Ctx context.Context
}

// RunAgentBuilder creates runAgentCmd instances using the PointContext
// and tool configuration from EvalState.
type RunAgentBuilder struct {
	ES *EvalState
}

func (b *RunAgentBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("run_agent: EvalState.PC not initialized")}
	}
	return &runAgentCmd{
		pc:  b.ES.PC,
		ctx: b.ES.Ctx,
	}
}

// CheckResultsBuilder creates checkResultsCmd instances.
type CheckResultsBuilder struct {
	ES *EvalState
}

func (b *CheckResultsBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("check_results: EvalState.PC not initialized")}
	}
	return &checkResultsCmd{pc: b.ES.PC}
}

// CollectMetricsBuilder creates collectMetricsCmd instances.
type CollectMetricsBuilder struct {
	ES *EvalState
}

func (b *CollectMetricsBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("collect_metrics: EvalState.PC not initialized")}
	}
	return &collectMetricsCmd{pc: b.ES.PC}
}

type failCmd struct {
	err error
}

func (f *failCmd) Name() string { return "fail" }
func (f *failCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		Err:         f.err,
		Output:      f.err.Error(),
		CommandName: "fail",
	}
}
