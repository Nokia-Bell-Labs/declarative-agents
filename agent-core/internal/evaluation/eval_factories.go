// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// EvalFactoryDeps holds the dependencies needed by evaluator tool factories.
type EvalFactoryDeps struct {
	Ctx              context.Context
	Registry         *core.Registry
	Stderr           io.Writer
	SuitePath        string
	OutputDir        string
	OllamaURL        string
	ChildAgentBinary string
	CoreRoot         string
	Directory        string
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
// per-point: create_point_dir, sample_docs, record_agent_commit,
// dump_config, run_agent, record_oracle_result, collect_trace_tokens,
// check_agent_version, summarize_point_results, collect_metrics) into the
// provided registry.BuiltinRegistry. Session state is lazily initialized on first
// factory call.
func RegisterEvalFactories(br *toolregistry.BuiltinRegistry, deps EvalFactoryDeps) {
	state := &evalFactoryState{deps: deps}
	registerEvalSessionFactories(br, state)
	registerEvalConfiguredFactories(br, state)
	registerEvalPointFactories(br, state)
	registerEvalArtifactFactories(br, deps.Directory)
}

func registerEvalArtifactFactories(br *toolregistry.BuiltinRegistry, root string) {
	for _, operation := range []string{
		"list_evaluation_sessions",
		"analyze_evaluation_session",
		"list_evaluation_points",
		"read_evaluation_trace",
	} {
		operation := operation
		br.Register(operation, func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
			var cfg catalog.EvaluationArtifactsConfig
			if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
				return nil, err
			}
			if cfg.DataDir == "" {
				return nil, fmt.Errorf("tool %q config requires data_dir", def.Name)
			}
			dataDir := cfg.DataDir
			if !filepath.IsAbs(dataDir) {
				dataDir = filepath.Join(root, dataDir)
			}
			return &EvaluationArtifactBuilder{Name: def.Name, Operation: operation, DataDir: dataDir}, nil
		})
	}
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
		EvalState:        EvalState{Ctx: s.deps.Ctx},
		Stderr:           stderr,
		SuitePath:        s.deps.SuitePath,
		OutputDir:        s.deps.OutputDir,
		OllamaURL:        s.deps.OllamaURL,
		ChildAgentBinary: s.deps.ChildAgentBinary,
		CoreRoot:         s.deps.CoreRoot,
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
	RegisterEvalPointFactories(br, &state.init().EvalState)
}

// RegisterEvalPointFactories registers the stateful critic-point words against
// an existing EvalState. Nested point registries use this hook alongside the
// unified declaration registry rather than rebuilding a name switch.
func RegisterEvalPointFactories(br *toolregistry.BuiltinRegistry, es *EvalState) {
	for _, spec := range evalPointFactorySpecs() {
		spec := spec
		br.Register(spec.name, func(catalog.ToolDef, map[string]string) (core.Builder, error) {
			return spec.build(es), nil
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
		{name: "sample_docs", build: func(es *EvalState) core.Builder { return &SampleDocsBuilder{ES: es} }},
		{name: "run_agent", build: func(es *EvalState) core.Builder { return &RunAgentBuilder{ES: es} }},
		{name: "record_oracle_result", build: func(es *EvalState) core.Builder { return &RecordOracleResultBuilder{ES: es} }},
		{name: "collect_trace_tokens", build: func(es *EvalState) core.Builder { return &CollectTraceTokensBuilder{ES: es} }},
		{name: "check_agent_version", build: func(es *EvalState) core.Builder { return &CheckAgentVersionBuilder{ES: es} }},
		{name: "summarize_point_results", build: func(es *EvalState) core.Builder { return &SummarizePointResultsBuilder{ES: es} }},
		{name: "collect_metrics", build: func(es *EvalState) core.Builder { return &CollectMetricsBuilder{ES: es} }},
		{name: "record_agent_commit", build: func(es *EvalState) core.Builder { return &RecordAgentCommitBuilder{ES: es} }},
		{name: "dump_config", build: func(es *EvalState) core.Builder { return &DumpConfigBuilder{ES: es} }},
	}
}
