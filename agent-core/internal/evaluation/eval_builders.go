// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// EvalState holds shared mutable state for eval tools, analogous to
// pipeline.State for pipeline tools. The run_point tool sets PC
// before each point's nested loop runs.
type EvalState struct {
	PC  *PointContext
	Ctx context.Context
}

// CreatePointDirBuilder creates createPointDirCmd instances.
type CreatePointDirBuilder struct {
	ES *EvalState
}

func (b *CreatePointDirBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "create_point_dir", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{
			inner: &createPointDirCmd{pc: pc}, point: pc,
			removePaths: func() []string { return []string{pc.PointDir} },
		}
	})
}

func (b *CreatePointDirBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "create_point_dir", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &createPointDirCmd{pc: pc}, point: pc}
	})
}

// SampleDocsBuilder exposes optional-docs presence and copy parameters.
type SampleDocsBuilder struct {
	ES *EvalState
}

func (b *SampleDocsBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "sample_docs", func(pc *PointContext) core.Command {
		return &sampleDocsCmd{pc: pc}
	})
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
	pc := b.ES.PC
	return &evaluatorReceiptCmd{
		inner: &runAgentCmd{pc: pc, ctx: b.ES.Ctx}, point: pc,
		removePaths: func() []string { return []string{pc.ResultPath} },
		boundary:    "harness child process and point workspace require compensation",
		boundaryMetadata: func() any {
			return map[string]any{
				"binary": pc.Harness.Binary, "profile": pc.ProfilePath,
				"point_dir": pc.PointDir, "trace_path": pc.TracePath, "result_path": pc.ResultPath,
				"exit_code": pc.ExitCode, "timed_out": pc.TimedOut,
			}
		},
	}
}

func (b *RunAgentBuilder) BuildReverser() core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("run_agent: EvalState.PC not initialized")}
	}
	return &evaluatorReceiptCmd{
		inner: &runAgentCmd{pc: b.ES.PC, ctx: b.ES.Ctx}, point: b.ES.PC,
		boundary: "harness child process and point workspace require compensation",
	}
}

// RecordOracleResultBuilder maps the configured oracle exec result into point state.
type RecordOracleResultBuilder struct {
	ES *EvalState
}

func (b *RecordOracleResultBuilder) Build(res core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("record_oracle_result: EvalState.PC not initialized")}
	}
	return &evaluatorReceiptCmd{inner: &recordOracleResultCmd{pc: b.ES.PC, prior: res}, point: b.ES.PC}
}

func (b *RecordOracleResultBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "record_oracle_result", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &recordOracleResultCmd{pc: pc}, point: pc}
	})
}

// CollectTraceTokensBuilder creates collectTraceTokensCmd instances.
type CollectTraceTokensBuilder struct {
	ES *EvalState
}

func (b *CollectTraceTokensBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("collect_trace_tokens: EvalState.PC not initialized")}
	}
	return &evaluatorReceiptCmd{inner: &collectTraceTokensCmd{pc: b.ES.PC}, point: b.ES.PC}
}

func (b *CollectTraceTokensBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "collect_trace_tokens", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &collectTraceTokensCmd{pc: pc}, point: pc}
	})
}

// CheckAgentVersionBuilder creates checkAgentVersionCmd instances.
type CheckAgentVersionBuilder struct {
	ES *EvalState
}

func (b *CheckAgentVersionBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("check_agent_version: EvalState.PC not initialized")}
	}
	return &evaluatorReceiptCmd{inner: &checkAgentVersionCmd{pc: b.ES.PC}, point: b.ES.PC}
}

func (b *CheckAgentVersionBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "check_agent_version", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &checkAgentVersionCmd{pc: pc}, point: pc}
	})
}

// SummarizePointResultsBuilder creates summarizePointResultsCmd instances.
type SummarizePointResultsBuilder struct {
	ES *EvalState
}

func (b *SummarizePointResultsBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("summarize_point_results: EvalState.PC not initialized")}
	}
	return &summarizePointResultsCmd{pc: b.ES.PC}
}

// RecordPointFailureBuilder projects a failed result into point state.
type RecordPointFailureBuilder struct {
	ES *EvalState
}

func (b *RecordPointFailureBuilder) Build(res core.Result) core.Command {
	return buildPointCommand(b.ES, "record_point_failure", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &recordPointFailureCmd{pc: pc, prior: res}, point: pc}
	})
}

func (b *RecordPointFailureBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "record_point_failure", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &recordPointFailureCmd{pc: pc}, point: pc}
	})
}

// CollectMetricsBuilder creates collectMetricsCmd instances.
type CollectMetricsBuilder struct {
	ES *EvalState
}

func (b *CollectMetricsBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("collect_metrics: EvalState.PC not initialized")}
	}
	pc := b.ES.PC
	return &evaluatorReceiptCmd{
		inner: &collectMetricsCmd{pc: pc}, point: pc,
		removePaths: func() []string { return []string{pc.PointDir + "/meta.json"} },
	}
}

func (b *CollectMetricsBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "collect_metrics", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &collectMetricsCmd{pc: pc}, point: pc}
	})
}

func buildPointCommand(es *EvalState, commandName string, build func(*PointContext) core.Command) core.Command {
	if es == nil || es.PC == nil {
		return &failCmd{err: fmt.Errorf("%s: EvalState.PC not initialized", commandName)}
	}
	return build(es.PC)
}

type failCmd struct {
	err error
}

func (f *failCmd) Name() string                   { return "fail" }
func (f *failCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(f.Name()) }

func (f *failCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		Err:         f.err,
		Output:      f.err.Error(),
		CommandName: "fail",
	}
}
