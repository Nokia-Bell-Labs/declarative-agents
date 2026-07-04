// Copyright (c) 2026 Nokia. All rights reserved.

package registry

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// BuiltinFactory creates a Builder from a tool definition's config map.
type BuiltinFactory func(def catalog.ToolDef, vars map[string]string) (core.Builder, error)

// BuiltinRegistry maps builtin init names to factory functions.
type BuiltinRegistry struct {
	factories map[string]BuiltinFactory
}

// NewBuiltinRegistry creates an empty builtin registry.
func NewBuiltinRegistry() *BuiltinRegistry {
	return &BuiltinRegistry{factories: make(map[string]BuiltinFactory)}
}

// Register adds a factory under an init name.
func (br *BuiltinRegistry) Register(initName string, factory BuiltinFactory) {
	if _, exists := br.factories[initName]; exists {
		panic(fmt.Sprintf("builtin registry: duplicate init name %q", initName))
	}
	br.factories[initName] = factory
}

// Override replaces the factory for an init name, or registers it if absent.
func (br *BuiltinRegistry) Override(initName string, factory BuiltinFactory) {
	br.factories[initName] = factory
}

// Resolve looks up a factory by init name.
func (br *BuiltinRegistry) Resolve(initName string) (BuiltinFactory, bool) {
	f, ok := br.factories[initName]
	return f, ok
}

// Names returns all registered init names sorted.
func (br *BuiltinRegistry) Names() []string {
	names := make([]string, 0, len(br.factories))
	for n := range br.factories {
		names = append(names, n)
	}
	return names
}

// ExecBuilderFactory creates a Builder for an exec declaration.
type ExecBuilderFactory func(def catalog.ToolDef, root string) core.Builder

// RegisterSingleBuiltin resolves and registers one builtin declaration.
func RegisterSingleBuiltin(reg *core.Registry, builtins *BuiltinRegistry, td catalog.ToolDef, vars map[string]string) error {
	if td.Init == "" {
		return fmt.Errorf("builtin tool %q has no init field", td.Name)
	}
	factory, ok := builtins.Resolve(td.Init)
	if !ok {
		return fmt.Errorf("builtin tool %q: unknown init %q", td.Name, td.Init)
	}
	builder, err := factory(td, vars)
	if err != nil {
		return fmt.Errorf("builtin tool %q init: %w", td.Name, err)
	}
	reg.Override(td.ToToolSpec(), builder)
	return nil
}

// RegisterUnifiedTools registers builtin and exec declarations.
func RegisterUnifiedTools(reg *core.Registry, builtins *BuiltinRegistry, root string, defs []catalog.ToolDef, vars map[string]string, execBuilder ExecBuilderFactory) error {
	for _, td := range defs {
		switch td.Type {
		case "builtin":
			if err := registerBuiltin(reg, builtins, td, vars); err != nil {
				return err
			}
		case "exec", "":
			if err := registerExec(reg, root, td, execBuilder); err != nil {
				return err
			}
		default:
			return fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
	}
	return nil
}

// RegisterUnifiedToolsForMachine registers selected declarations with dynamic
// manifest phases derived from the machine grammar.
func RegisterUnifiedToolsForMachine(reg *core.Registry, builtins *BuiltinRegistry, root string, machine core.MachineSpec, defs []catalog.ToolDef, vars map[string]string, execBuilder ExecBuilderFactory) error {
	return RegisterUnifiedTools(reg, builtins, root, catalog.ApplyDynamicToolPhases(machine, defs), vars, execBuilder)
}

func registerBuiltin(reg *core.Registry, builtins *BuiltinRegistry, td catalog.ToolDef, vars map[string]string) error {
	if td.Init == "" {
		return fmt.Errorf("builtin tool %q has no init field", td.Name)
	}
	factory, ok := builtins.Resolve(td.Init)
	if !ok {
		return fmt.Errorf("builtin tool %q: unknown init %q", td.Name, td.Init)
	}
	builder, err := factory(td, vars)
	if err != nil {
		return fmt.Errorf("builtin tool %q init: %w", td.Name, err)
	}
	reg.Register(td.ToToolSpec(), builder)
	return nil
}

func registerExec(reg *core.Registry, root string, td catalog.ToolDef, execBuilder ExecBuilderFactory) error {
	if td.Binary == "" {
		return fmt.Errorf("exec tool %q has no binary", td.Name)
	}
	if execBuilder == nil {
		return fmt.Errorf("exec tool %q: exec builder factory is required", td.Name)
	}
	reg.Register(td.ToToolSpec(), execBuilder(td, root))
	return nil
}

// SelectedBuiltinInits returns builtin init keys present in selected defs.
func SelectedBuiltinInits(defs []catalog.ToolDef) map[string]bool {
	selected := make(map[string]bool)
	for _, def := range defs {
		if def.Type == "builtin" && def.Init != "" {
			selected[def.Init] = true
		}
	}
	return selected
}
