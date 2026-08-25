// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package control

import (
	"context"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitSelfInvoke     = "self_invoke"
	InitValuePredicate = "value_predicate"
	InitPartition      = "partition"
	InitSelectSubset   = "select_subset"
)

// FactoryDeps names the process-local ports self_invoke captures at construction.
type FactoryDeps struct {
	Ctx              context.Context
	Tracer           tracing.Tracer
	CoreRoot         string
	ChildAgentBinary string
}

// RegisterFactories registers control builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	br.Register(InitSelfInvoke, selfInvokeFactory(deps))
	br.Register(InitValuePredicate, valuePredicateFactory())
	br.Register(InitPartition, partitionFactory())
	br.Register(InitSelectSubset, selectSubsetFactory())
}

func partitionFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.PartitionConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := ValidatePartitionConfig(
			def.Name, cfg.Items, cfg.Field, cfg.Op, cfg.Right, cfg.OperandType, cfg.Satisfied,
		); err != nil {
			return nil, err
		}
		return PartitionBuilder{
			ToolName: def.Name, Items: cfg.Items, Field: cfg.Field, Op: cfg.Op,
			Right: cfg.Right, OperandType: cfg.OperandType, Satisfied: core.Signal(cfg.Satisfied),
		}, nil
	}
}

func selectSubsetFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.SelectSubsetConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := ValidateSelectSubsetConfig(
			def.Name, cfg.Candidates, cfg.Vocabulary, cfg.MatchField,
			cfg.AllMatched, cfg.Partial, cfg.Empty,
		); err != nil {
			return nil, err
		}
		return SelectSubsetBuilder{
			ToolName: def.Name, Candidates: cfg.Candidates, Vocabulary: cfg.Vocabulary,
			MatchField: cfg.MatchField, AllMatched: core.Signal(cfg.AllMatched),
			Partial: core.Signal(cfg.Partial), Empty: core.Signal(cfg.Empty),
		}, nil
	}
}

// valuePredicateFactory builds the value predicate word (srd041). Config is
// validated here rather than at dispatch, so an unknown operator or a missing
// signal name fails registration before a run reaches the branch it names.
func valuePredicateFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.ValuePredicateConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := ValidateValuePredicateConfig(
			def.Name, cfg.Left, cfg.Op, cfg.Right, cfg.OperandType, cfg.Satisfied, cfg.Unsatisfied,
		); err != nil {
			return nil, err
		}
		return ValuePredicateBuilder{
			ToolName:    def.Name,
			Left:        cfg.Left,
			Op:          cfg.Op,
			Right:       cfg.Right,
			OperandType: cfg.OperandType,
			Satisfied:   core.Signal(cfg.Satisfied),
			Unsatisfied: core.Signal(cfg.Unsatisfied),
		}, nil
	}
}

func selfInvokeFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		parsed, err := decodeChildAgent(def)
		if err != nil {
			return nil, err
		}
		config := childExecuteConfig(parsed, deps.CoreRoot)
		config.Binary = deps.ChildAgentBinary
		return &SelfInvokeBuilder{
			ToolName:      def.Name,
			Config:        config,
			RequestFrom:   parsed.RequestFrom,
			OutputFrom:    parsed.OutputFrom,
			WorkspacePath: vars["directory"],
			ExtraArgs:     directoryArgs(vars["directory"]),
			Ctx:           deps.Ctx,
			Tracer:        deps.Tracer,
		}, nil
	}
}

func decodeChildAgent(def catalog.ToolDef) (catalog.ChildAgentConfig, error) {
	var parsed catalog.ChildAgentConfig
	if err := catalog.DecodeToolConfig(def, &parsed); err != nil {
		return catalog.ChildAgentConfig{}, err
	}
	if err := catalog.ValidateChildAgentConfig(def.Name, parsed); err != nil {
		return catalog.ChildAgentConfig{}, err
	}
	return parsed, nil
}

func childExecuteConfig(parsed catalog.ChildAgentConfig, coreRoot string) execute.Config {
	return execute.Config{
		Profile:  parsed.Profile,
		CoreRoot: coreRoot,
		Request:  parsed.Request,
		Output:   parsed.Output,
	}
}

func directoryArgs(directory string) []string {
	if directory == "" {
		return nil
	}
	return []string{"--directory", directory}
}
