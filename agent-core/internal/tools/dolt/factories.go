// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"context"
	"fmt"
	"maps"
	"os"

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
	Opener                DatabaseOpener
	Connections           ConnectionResolver
	CheckpointIdentity    *DatabaseIdentity
	CheckpointIdentityErr error
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

// RegisterFactories registers Dolt builtin factories. CheckpointIdentityErr is
// resolved by the caller before constructing deps; a non-nil error fails at
// tool build time, not at registration.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	register := func(init string, factory toolregistry.BuiltinFactory) {
		br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			if deps.CheckpointIdentityErr != nil {
				return nil, deps.CheckpointIdentityErr
			}
			return factory(def, vars)
		})
	}
	register(InitProvision, ProvisionFactory(deps))
	register(InitQuery, QueryFactory(deps))
	register(InitWrite, WriteFactory(deps))
}

// EnvironmentConnections resolves a Dolt connection reference from the process
// environment. cmd/agent supplies this resolver; LookupEnv here predates the
// factory move and is not a new environment-variable contract.
type EnvironmentConnections struct{}

func (EnvironmentConnections) ResolveConnection(
	_ context.Context, ref string, _ map[string]string,
) (string, error) {
	//nolint:forbidigo // Trusted ToolDef config names the environment reference; DSN authority remains at cmd/agent.
	value, ok := os.LookupEnv(ref)
	if !ok || value == "" {
		return "", fmt.Errorf("configured Dolt connection reference %q is unavailable", ref)
	}
	return value, nil
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
		cfg, err := prepareBuilderConfig(def, prepared, vars, deps)
		if err != nil {
			return nil, err
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

func prepareBuilderConfig(
	def catalog.ToolDef,
	prepared *PreparedConfig,
	vars map[string]string,
	deps FactoryDeps,
) (builderConfig, error) {
	opener := deps.Opener
	if opener == nil {
		opener = SQLDatabaseOpener{}
	}
	resolver := deps.Connections
	if resolver == nil {
		resolver = varsConnectionResolver{}
	}
	if err := validateCheckpointSeparation(
		def.Name, prepared, vars, resolver, deps.CheckpointIdentity,
	); err != nil {
		return builderConfig{}, err
	}
	return builderConfig{
		toolName: def.Name, config: prepared, vars: maps.Clone(vars),
		opener: opener, resolver: resolver,
		undo: def.Undo, reverse: def.Reversibility.Classification,
	}, nil
}

func validateCheckpointSeparation(
	toolName string,
	config *PreparedConfig,
	vars map[string]string,
	resolver ConnectionResolver,
	checkpoint *DatabaseIdentity,
) error {
	if checkpoint == nil {
		return nil
	}
	dsn, err := resolver.ResolveConnection(context.Background(), config.ConnectionRef, vars)
	if err != nil {
		return fmt.Errorf("tool %q resolve connection for checkpoint separation: %w", toolName, err)
	}
	identity, err := IdentityFromDSN(dsn, config.Database)
	if err != nil {
		return fmt.Errorf("tool %q resolve database identity: %w", toolName, err)
	}
	if identity.SameDatabase(*checkpoint) {
		return fmt.Errorf(
			"tool %q database %q collides with the active Dolt checkpoint on server %q",
			toolName, identity.Database, identity.Server,
		)
	}
	return nil
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
