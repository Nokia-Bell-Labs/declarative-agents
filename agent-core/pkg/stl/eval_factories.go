// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"io"
	"os"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
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
// (session-level: load_suite, next_point, run_point, report_session;
// per-point: prepare_workspace, run_agent, check_results, collect_metrics,
// dump_config) into the provided BuiltinRegistry. Session state is lazily
// initialized on first factory call.
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

	br.Register("load_suite", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := LoadSuiteFactory(es)
		return factory(def, vars)
	})
	br.Register("next_point", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := NextPointFactory(es)
		return factory(def, vars)
	})
	br.Register("run_point", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := RunPointFactory(es, deps.Registry)
		return factory(def, vars)
	})
	br.Register("report_session", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		es := initESS()
		factory := ReportSessionFactory(es)
		return factory(def, vars)
	})

	br.Register("prepare_workspace", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &PrepareWorkspaceBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("run_agent", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &RunAgentBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("check_results", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CheckResultsBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("collect_metrics", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &CollectMetricsBuilder{ES: &initESS().EvalState}, nil
	})
	br.Register("dump_config", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &DumpConfigBuilder{ES: &initESS().EvalState}, nil
	})
}
