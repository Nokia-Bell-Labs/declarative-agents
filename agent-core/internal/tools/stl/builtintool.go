// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
)

// STLFactoryDeps are runtime ports needed by concrete STL builtin factories.
type STLFactoryDeps struct {
	Conversation    *llm.Conversation
	Registry        *core.Registry
	Parser          func() llm.ResponseParser
	Tracer          tracing.Tracer
	ProfilesDir     string
	Verbose         bool
	Ctx             context.Context
	Directory       string
	StateStore      core.StateStore
	Tracker         *ToolTracker
	ParseRetries    *ParseErrorRetryTracker
	OnModelResolved func(InvokeLLMResolvedConfig)
}

// RegisterFilesystemFactories registers filesystem builtin factories.
func RegisterFilesystemFactories(br *BuiltinRegistry, deps STLFactoryDeps) {
	fileFactories := []struct {
		init    string
		builder func(string) core.Builder
	}{
		{"file_read", func(root string) core.Builder { return &ReadBuilder{Root: root} }},
		{"file_write", func(root string) core.Builder { return &WriteBuilder{Root: root} }},
		{"file_edit", func(root string) core.Builder { return &EditBuilder{Root: root} }},
		{"file_find", func(root string) core.Builder { return &FindBuilder{Root: root} }},
		{"file_list", func(root string) core.Builder { return &ListFilesBuilder{Root: root} }},
	}
	for _, entry := range fileFactories {
		registerFileFactory(br, entry.init, entry.builder)
	}
}

func registerFileFactory(br *BuiltinRegistry, init string, builder func(string) core.Builder) {
	br.Register(init, func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return builder(vars["directory"]), nil
	})
}

// RegisterLLMFactories registers model-boundary builtin factories.
func RegisterLLMFactories(br *BuiltinRegistry, deps STLFactoryDeps) {
	br.Register("invoke_llm", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
			History:     deps.Conversation,
			Registry:    deps.Registry,
			Tracer:      deps.Tracer,
			ProfilesDir: deps.ProfilesDir,
			Verbose:     deps.Verbose,
			Ctx:         deps.Ctx,
			OnResolved:  deps.OnModelResolved,
		})
	})
	br.Register("parse_response", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ParseResponseBuilder{Registry: deps.Registry, Parser: currentParser(deps), Tracer: deps.Tracer, Verbose: deps.Verbose, Retry: deps.ParseRetries}, nil
	})
	br.Register("report_parse_error", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ReportParseErrorBuilder{Tracer: deps.Tracer, Retry: deps.ParseRetries}, nil
	})
	br.Register("reset_history", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ResetHistoryBuilder{History: deps.Conversation, Tracer: deps.Tracer}, nil
	})
	br.Register("nudge_reread", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &NudgeRereadBuilder{Tracer: deps.Tracer}, nil
	})
	br.Register("done", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return DoneBuilder{}, nil
	})
}

func currentParser(deps STLFactoryDeps) llm.ResponseParser {
	if deps.Parser == nil {
		return nil
	}
	return deps.Parser()
}

// RegisterLifecycleFactoryGroup registers lifecycle builtin factories.
func RegisterLifecycleFactoryGroup(br *BuiltinRegistry, deps STLFactoryDeps) {
	RegisterLifecycleFactories(br, LifecycleFactoryDeps{StateStore: deps.StateStore, Tracer: deps.Tracer})
	br.Register("checkpoint_history", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg CheckpointHistoryConfig
		if err := DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &CheckpointHistoryBuilder{Config: cfg, StateStore: deps.StateStore, Ctx: deps.Ctx}, nil
	})
	br.Register("checkpoint_rollback", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return checkpointRollbackBuilder(def, deps)
	})
}

func checkpointRollbackBuilder(def ToolDef, deps STLFactoryDeps) (core.Builder, error) {
	var cfg CheckpointRollbackConfig
	if err := DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	return &CheckpointRollbackBuilder{
		Config:     cfg,
		StateStore: deps.StateStore,
		Directory:  deps.Directory,
		Tracer:     deps.Tracer,
		Ctx:        deps.Ctx,
	}, nil
}

// RegisterValidationFactory registers the validate compatibility builtin.
func RegisterValidationFactory(br *BuiltinRegistry, deps STLFactoryDeps) {
	br.Register("validate", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ValidateBuilder{Tracker: deps.Tracker, Registry: deps.Registry, Tracer: deps.Tracer, Verbose: deps.Verbose}, nil
	})
}

// RegisterControlFactories registers child-agent and loop-control factories.
func RegisterControlFactories(br *BuiltinRegistry, deps STLFactoryDeps) {
	br.Register("self_invoke", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		parsed, err := decodeChildAgent(def)
		if err != nil {
			return nil, err
		}
		return &SelfInvokeBuilder{
			Config:    childExecuteConfig(parsed, vars["model"]),
			ExtraArgs: directoryArgs(vars["directory"]),
			Ctx:       deps.Ctx,
			Tracer:    deps.Tracer,
		}, nil
	})
}

func decodeChildAgent(def ToolDef) (ChildAgentConfig, error) {
	var parsed ChildAgentConfig
	if err := DecodeToolConfig(def, &parsed); err != nil {
		return ChildAgentConfig{}, err
	}
	if err := ValidateChildAgentConfig(def.Name, parsed); err != nil {
		return ChildAgentConfig{}, err
	}
	return parsed, nil
}

func childExecuteConfig(parsed ChildAgentConfig, model string) execute.Config {
	return execute.Config{
		Profile:          parsed.Profile,
		Machine:          parsed.Machine,
		Tools:            parsed.Tools,
		ToolDeclarations: parsed.ToolDeclarations,
		Model:            model,
	}
}

func directoryArgs(directory string) []string {
	if directory == "" {
		return nil
	}
	return []string{"--directory", directory}
}

// RegisterSpecValidationFactories registers specification validation factories.
func RegisterSpecValidationFactories(br *BuiltinRegistry, deps STLFactoryDeps) {
	RegisterValidateFactories(br, deps.Directory)
}
