// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

type (
	BuiltinFactory              = toolregistry.BuiltinFactory
	BuiltinRegistry             = toolregistry.BuiltinRegistry
	ExecBuilderFactory          = toolregistry.ExecBuilderFactory
	FactoryRegistrar            = toolregistry.FactoryRegistrar
	StandardFactoryDeps         = toolregistry.StandardFactoryDeps
	StandardFactoryCatalogEntry = toolregistry.StandardFactoryCatalogEntry
	DynamicToolActionDeps       = toolregistry.DynamicToolActionDeps
)

var (
	NewBuiltinRegistry               = toolregistry.NewBuiltinRegistry
	RegisterSingleBuiltin            = toolregistry.RegisterSingleBuiltin
	SelectedBuiltinInits             = toolregistry.SelectedBuiltinInits
	RegisterStandardBuiltinFactories = toolregistry.RegisterStandardBuiltinFactories
	StandardFactoryCatalog           = toolregistry.StandardFactoryCatalog
	BuildDynamicToolAction           = toolregistry.BuildDynamicToolAction
)

// RegisterUnifiedTools preserves the STL facade while exec builders still live here.
func RegisterUnifiedTools(reg *core.Registry, builtins *BuiltinRegistry, root string, defs []catalog.ToolDef, vars map[string]string) error {
	return toolregistry.RegisterUnifiedTools(reg, builtins, root, defs, vars, func(def catalog.ToolDef, root string) core.Builder {
		return &ExecBuilder{Def: def, Root: root}
	})
}

// RegisterUnifiedToolsForMachine preserves the STL facade while applying
// MachineSpec-derived dynamic manifest phases.
func RegisterUnifiedToolsForMachine(reg *core.Registry, builtins *BuiltinRegistry, root string, machine core.MachineSpec, defs []catalog.ToolDef, vars map[string]string) error {
	return toolregistry.RegisterUnifiedToolsForMachine(reg, builtins, root, machine, defs, vars, func(def catalog.ToolDef, root string) core.Builder {
		return &ExecBuilder{Def: def, Root: root}
	})
}
