// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"io"
	"os"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
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

type evalFactoryState struct {
	deps    EvalFactoryDeps
	session *EvalSessionState
}

type evalSessionFactorySpec struct {
	name  string
	build func(*EvalSessionState) core.Builder
}

type evalConfiguredFactorySpec struct {
	name    string
	factory func(*EvalSessionState) toolregistry.BuiltinFactory
}

type evalPointFactorySpec struct {
	name  string
	build func(*EvalState) core.Builder
}

// RegisterEvalFactories registers all evaluator builtin tool factories
// (session-level: parse_suite_config, discover_suite_samples,
// expand_eval_grid, init_eval_session, report_suite_summary, next_point,
// run_point, report_session;
// per-point: create_point_dir, copy_sample_workspace, copy_sample_docs,
// init_workspace_repo, stage_workspace_baseline, commit_workspace_baseline,
// dump_config, run_agent, run_oracle_check, collect_trace_tokens,
// check_agent_version, summarize_point_results, collect_metrics) into the
// provided registry.BuiltinRegistry. Session state is lazily initialized on first
// factory call.
func RegisterEvalFactories(br *toolregistry.BuiltinRegistry, deps EvalFactoryDeps) {
	state := &evalFactoryState{deps: deps}
	registerEvalSessionFactories(br, state)
	registerEvalConfiguredFactories(br, state)
	registerEvalPointFactories(br, state)
}

func (s *evalFactoryState) init() *EvalSessionState {
	if s.session != nil {
		return s.session
	}
	stderr := s.deps.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	s.session = &EvalSessionState{
		EvalState: EvalState{Ctx: s.deps.Ctx},
		Stderr:    stderr,
		SuitePath: s.deps.SuitePath,
		OutputDir: s.deps.OutputDir,
		OllamaURL: s.deps.OllamaURL,
	}
	return s.session
}

func registerEvalSessionFactories(br *toolregistry.BuiltinRegistry, state *evalFactoryState) {
	for _, spec := range evalSessionFactorySpecs() {
		spec := spec
		br.Register(spec.name, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			es := state.init()
			factory := evaluatorSessionConfigFactory(es, spec.build)
			return factory(def, vars)
		})
	}
}

func registerEvalConfiguredFactories(br *toolregistry.BuiltinRegistry, state *evalFactoryState) {
	for _, spec := range evalConfiguredFactorySpecs() {
		spec := spec
		br.Register(spec.name, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			factory := spec.factory(state.init())
			return factory(def, vars)
		})
	}
}

func registerEvalPointFactories(br *toolregistry.BuiltinRegistry, state *evalFactoryState) {
	for _, spec := range evalPointFactorySpecs() {
		spec := spec
		br.Register(spec.name, func(catalog.ToolDef, map[string]string) (core.Builder, error) {
			return spec.build(&state.init().EvalState), nil
		})
	}
}

func evalSessionFactorySpecs() []evalSessionFactorySpec {
	return []evalSessionFactorySpec{
		{name: "parse_suite_config", build: func(es *EvalSessionState) core.Builder { return &ParseSuiteConfigBuilder{ES: es} }},
		{name: "discover_suite_samples", build: func(es *EvalSessionState) core.Builder { return &DiscoverSuiteSamplesBuilder{ES: es} }},
		{name: "expand_eval_grid", build: func(es *EvalSessionState) core.Builder { return &ExpandEvalGridBuilder{ES: es} }},
		{name: "init_eval_session", build: func(es *EvalSessionState) core.Builder { return &InitEvalSessionBuilder{ES: es} }},
		{name: "report_suite_summary", build: func(es *EvalSessionState) core.Builder { return &ReportSuiteSummaryBuilder{ES: es} }},
	}
}

func evalConfiguredFactorySpecs() []evalConfiguredFactorySpec {
	return []evalConfiguredFactorySpec{
		{name: "next_point", factory: NextPointFactory},
		{name: "run_point", factory: RunPointFactory},
		{name: "report_session", factory: ReportSessionFactory},
	}
}

func evalPointFactorySpecs() []evalPointFactorySpec {
	return []evalPointFactorySpec{
		{name: "create_point_dir", build: func(es *EvalState) core.Builder { return &CreatePointDirBuilder{ES: es} }},
		{name: "copy_sample_workspace", build: func(es *EvalState) core.Builder { return &CopySampleWorkspaceBuilder{ES: es} }},
		{name: "copy_sample_docs", build: func(es *EvalState) core.Builder { return &CopySampleDocsBuilder{ES: es} }},
		{name: "init_workspace_repo", build: func(es *EvalState) core.Builder { return &InitWorkspaceRepoBuilder{ES: es} }},
		{name: "stage_workspace_baseline", build: func(es *EvalState) core.Builder { return &StageWorkspaceBaselineBuilder{ES: es} }},
		{name: "commit_workspace_baseline", build: func(es *EvalState) core.Builder { return &CommitWorkspaceBaselineBuilder{ES: es} }},
		{name: "run_agent", build: func(es *EvalState) core.Builder { return &RunAgentBuilder{ES: es} }},
		{name: "run_oracle_check", build: func(es *EvalState) core.Builder { return &RunOracleCheckBuilder{ES: es} }},
		{name: "collect_trace_tokens", build: func(es *EvalState) core.Builder { return &CollectTraceTokensBuilder{ES: es} }},
		{name: "check_agent_version", build: func(es *EvalState) core.Builder { return &CheckAgentVersionBuilder{ES: es} }},
		{name: "summarize_point_results", build: func(es *EvalState) core.Builder { return &SummarizePointResultsBuilder{ES: es} }},
		{name: "collect_metrics", build: func(es *EvalState) core.Builder { return &CollectMetricsBuilder{ES: es} }},
		{name: "dump_config", build: func(es *EvalState) core.Builder { return &DumpConfigBuilder{ES: es} }},
	}
}
