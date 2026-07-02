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
		return &createPointDirCmd{pc: pc}
	})
}

// CopySampleWorkspaceBuilder creates copySampleWorkspaceCmd instances.
type CopySampleWorkspaceBuilder struct {
	ES *EvalState
}

func (b *CopySampleWorkspaceBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "copy_sample_workspace", func(pc *PointContext) core.Command {
		return &copySampleWorkspaceCmd{pc: pc}
	})
}

// CopySampleDocsBuilder creates copySampleDocsCmd instances.
type CopySampleDocsBuilder struct {
	ES *EvalState
}

func (b *CopySampleDocsBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "copy_sample_docs", func(pc *PointContext) core.Command {
		return &copySampleDocsCmd{pc: pc}
	})
}

// InitWorkspaceRepoBuilder creates initWorkspaceRepoCmd instances.
type InitWorkspaceRepoBuilder struct {
	ES *EvalState
}

func (b *InitWorkspaceRepoBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "init_workspace_repo", func(pc *PointContext) core.Command {
		return &initWorkspaceRepoCmd{pc: pc}
	})
}

// StageWorkspaceBaselineBuilder creates stageWorkspaceBaselineCmd instances.
type StageWorkspaceBaselineBuilder struct {
	ES *EvalState
}

func (b *StageWorkspaceBaselineBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "stage_workspace_baseline", func(pc *PointContext) core.Command {
		return &stageWorkspaceBaselineCmd{pc: pc}
	})
}

// CommitWorkspaceBaselineBuilder creates commitWorkspaceBaselineCmd instances.
type CommitWorkspaceBaselineBuilder struct {
	ES *EvalState
}

func (b *CommitWorkspaceBaselineBuilder) Build(_ core.Result) core.Command {
	return buildPointCommand(b.ES, "commit_workspace_baseline", func(pc *PointContext) core.Command {
		return &commitWorkspaceBaselineCmd{pc: pc}
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
	return &runAgentCmd{
		pc:  b.ES.PC,
		ctx: b.ES.Ctx,
	}
}

// RunOracleCheckBuilder creates runOracleCheckCmd instances.
type RunOracleCheckBuilder struct {
	ES *EvalState
}

func (b *RunOracleCheckBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("run_oracle_check: EvalState.PC not initialized")}
	}
	return &runOracleCheckCmd{pc: b.ES.PC}
}

// CollectTraceTokensBuilder creates collectTraceTokensCmd instances.
type CollectTraceTokensBuilder struct {
	ES *EvalState
}

func (b *CollectTraceTokensBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("collect_trace_tokens: EvalState.PC not initialized")}
	}
	return &collectTraceTokensCmd{pc: b.ES.PC}
}

// CheckAgentVersionBuilder creates checkAgentVersionCmd instances.
type CheckAgentVersionBuilder struct {
	ES *EvalState
}

func (b *CheckAgentVersionBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("check_agent_version: EvalState.PC not initialized")}
	}
	return &checkAgentVersionCmd{pc: b.ES.PC}
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

func buildPointCommand(es *EvalState, commandName string, build func(*PointContext) core.Command) core.Command {
	if es == nil || es.PC == nil {
		return &failCmd{err: fmt.Errorf("%s: EvalState.PC not initialized", commandName)}
	}
	return build(es.PC)
}

type failCmd struct {
	err error
}

func (f *failCmd) Name() string      { return "fail" }
func (f *failCmd) Undo() core.Result { return core.NoopUndo(f.Name()) }

func (f *failCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		Err:         f.err,
		Output:      f.err.Error(),
		CommandName: "fail",
	}
}
