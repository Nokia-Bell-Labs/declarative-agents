// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"io"
	"os"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// EvalFactoryDeps holds the dependencies needed by evaluator tool factories.
type EvalFactoryDeps struct {
	Ctx       context.Context
	Registry  *core.Registry
	Stderr    io.Writer
	SuitePath string
	OutputDir string
	OllamaURL string
}

// RegisterEvalFactories registers all evaluator builtin tool factories
// (session-level: parse_suite_config, discover_suite_samples,
// expand_eval_grid, init_eval_session, report_suite_summary, next_point,
// run_point, report_session;
// per-point: create_point_dir, copy_sample_workspace, copy_sample_docs,
// init_workspace_repo, stage_workspace_baseline, commit_workspace_baseline,
// dump_config, run_agent, run_oracle_check, collect_trace_tokens,
// check_agent_version, summarize_point_results, collect_metrics) into the
// provided BuiltinRegistry. Session state is lazily initialized on first
// factory call.
func RegisterEvalFactories(br *BuiltinRegistry, deps EvalFactoryDeps) {
	var ess *EvalSessionState

	initESS := func() *EvalSessionState {
		if ess != nil {
			return ess
		}
		stderr := deps.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		ess = &EvalSessionState{
			EvalState: EvalState{Ctx: deps.Ctx},
			Stderr:    stderr,
			SuitePath: deps.SuitePath,
			OutputDir: deps.OutputDir,
			OllamaURL: deps.OllamaURL,
		}
		return ess
	}

	br.Register("parse_suite_config", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := evaluatorSessionConfigFactory(es, func(es *EvalSessionState) core.Builder {
			return &ParseSuiteConfigBuilder{ES: es}
		})
		return factory(def, vars)
	})
	br.Register("discover_suite_samples", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := evaluatorSessionConfigFactory(es, func(es *EvalSessionState) core.Builder {
			return &DiscoverSuiteSamplesBuilder{ES: es}
		})
		return factory(def, vars)
	})
	br.Register("expand_eval_grid", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := evaluatorSessionConfigFactory(es, func(es *EvalSessionState) core.Builder {
			return &ExpandEvalGridBuilder{ES: es}
		})
		return factory(def, vars)
	})
	br.Register("init_eval_session", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := evaluatorSessionConfigFactory(es, func(es *EvalSessionState) core.Builder {
			return &InitEvalSessionBuilder{ES: es}
		})
		return factory(def, vars)
	})
	br.Register("report_suite_summary", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := evaluatorSessionConfigFactory(es, func(es *EvalSessionState) core.Builder {
			return &ReportSuiteSummaryBuilder{ES: es}
		})
		return factory(def, vars)
	})
	br.Register("next_point", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := NextPointFactory(es)
		return factory(def, vars)
	})
	br.Register("run_point", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := RunPointFactory(es)
		return factory(def, vars)
	})
	br.Register("report_session", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := ReportSessionFactory(es)
		return factory(def, vars)
	})

	br.Register("create_point_dir", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CreatePointDirBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("copy_sample_workspace", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CopySampleWorkspaceBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("copy_sample_docs", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CopySampleDocsBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("init_workspace_repo", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &InitWorkspaceRepoBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("stage_workspace_baseline", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &StageWorkspaceBaselineBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("commit_workspace_baseline", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CommitWorkspaceBaselineBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("run_agent", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &RunAgentBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("run_oracle_check", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &RunOracleCheckBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("collect_trace_tokens", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CollectTraceTokensBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("check_agent_version", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CheckAgentVersionBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("summarize_point_results", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &SummarizePointResultsBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("collect_metrics", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CollectMetricsBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("dump_config", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &DumpConfigBuilder{ES: &initESS().EvalState}, nil
	})
}
