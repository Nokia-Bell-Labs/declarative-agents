// Copyright (c) 2026 Nokia. All rights reserved.

package dolt

import (
	"context"
	"fmt"
	"maps"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitProvision = "dolt_provision"
	InitQuery     = "dolt_query"
	InitWrite     = "dolt_write"
)

type ConnectionResolver interface {
	ResolveConnection(context.Context, string, map[string]string) (string, error)
}

type FactoryDeps struct {
	Opener      DatabaseOpener
	Connections ConnectionResolver
}
type builderConfig struct {
	toolName string
	config   *PreparedConfig
	vars     map[string]string
	opener   DatabaseOpener
	resolver ConnectionResolver
	undo     catalog.ToolUndoContract
	reverse  string
}
type ProvisionBuilder struct{ cfg builderConfig }
type QueryBuilder struct{ cfg builderConfig }
type WriteBuilder struct{ cfg builderConfig }

func ProvisionFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return operationFactory(KindProvision, deps)
}
func QueryFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return operationFactory(KindQuery, deps)
}
func WriteFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return operationFactory(KindWrite, deps)
}
func operationFactory(kind OperationKind, deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		prepared, err := DecodeConfig(def)
		if err != nil {
			return nil, err
		}
		if prepared.Kind != kind {
			return nil, fmt.Errorf(
				"tool %q init %q requires kind %q, got %q",
				def.Name, initForKind(kind), kind, prepared.Kind,
			)
		}
		cfg := builderConfig{
			toolName: def.Name, config: prepared, vars: maps.Clone(vars),
			opener: deps.Opener, resolver: deps.Connections,
			undo: def.Undo, reverse: def.Reversibility.Classification,
		}
		if cfg.opener == nil {
			cfg.opener = SQLDatabaseOpener{}
		}
		if cfg.resolver == nil {
			cfg.resolver = varsConnectionResolver{}
		}
		switch kind {
		case KindProvision:
			return ProvisionBuilder{cfg: cfg}, nil
		case KindQuery:
			return QueryBuilder{cfg: cfg}, nil
		case KindWrite:
			return WriteBuilder{cfg: cfg}, nil
		default:
			panic("unreachable Dolt operation kind")
		}
	}
}
func initForKind(kind OperationKind) string {
	switch kind {
	case KindProvision:
		return InitProvision
	case KindQuery:
		return InitQuery
	case KindWrite:
		return InitWrite
	default:
		return ""
	}
}

type varsConnectionResolver struct{}

func (varsConnectionResolver) ResolveConnection(_ context.Context, ref string, vars map[string]string) (string, error) {
	if value := vars[ref]; value != "" {
		return value, nil
	}
	return "", fmt.Errorf("configured connection reference is unavailable")
}
func buildCommand(cfg builderConfig, res core.Result) core.Command {
	params, err := decodeRuntimeParams(res.Output)
	return newCommand(cfg, params, err)
}
func (b ProvisionBuilder) Build(res core.Result) core.Command { return buildCommand(b.cfg, res) }
func (b ProvisionBuilder) BuildReverser() core.Command        { return newCommand(b.cfg, nil, nil) }
func (b QueryBuilder) Build(res core.Result) core.Command     { return buildCommand(b.cfg, res) }
func (b WriteBuilder) Build(res core.Result) core.Command     { return buildCommand(b.cfg, res) }
func (b WriteBuilder) BuildReverser() core.Command            { return newCommand(b.cfg, nil, nil) }

var (
	_ core.Builder  = ProvisionBuilder{}
	_ core.Reverser = ProvisionBuilder{}
	_ core.Builder  = QueryBuilder{}
	_ core.Builder  = WriteBuilder{}
	_ core.Reverser = WriteBuilder{}
)
