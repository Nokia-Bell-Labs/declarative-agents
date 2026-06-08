// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// BuiltinFactory creates a Builder from a tool definition's config map.
// Each builtin tool type registers a factory under a unique init name.
type BuiltinFactory func(def ToolDef, vars map[string]string) (core.Builder, error)

// BuiltinRegistry maps init names to their factory functions.
type BuiltinRegistry struct {
	factories map[string]BuiltinFactory
}

// NewBuiltinRegistry creates an empty builtin registry.
func NewBuiltinRegistry() *BuiltinRegistry {
	return &BuiltinRegistry{factories: make(map[string]BuiltinFactory)}
}

// Register adds a factory under the given init name.
func (br *BuiltinRegistry) Register(initName string, factory BuiltinFactory) {
	if _, exists := br.factories[initName]; exists {
		panic(fmt.Sprintf("builtin registry: duplicate init name %q", initName))
	}
	br.factories[initName] = factory
}

// Override replaces the factory for the given init name, or registers
// it if not present. Use this in tests to replace real factories with
// stubs.
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

// RegisterUnifiedTools loads a tools YAML file that may contain both
// exec and builtin tool definitions, and registers them all with the
// core registry.
//
// For exec tools (type: exec or no type): creates ExecBuilder as before.
// For builtin tools (type: builtin): looks up the init name in the
// BuiltinRegistry and calls the factory to create a Builder.
//
// The vars map provides template variable resolution (e.g. "model",
// "directory") for tool config values.
// RegisterSingleBuiltin registers a single builtin tool definition
// into the registry, resolving its factory from the BuiltinRegistry.
func RegisterSingleBuiltin(reg *core.Registry, builtins *BuiltinRegistry, td ToolDef, vars map[string]string) error {
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

func RegisterUnifiedTools(reg *core.Registry, builtins *BuiltinRegistry, root string, defs []ToolDef, vars map[string]string) error {
	for _, td := range defs {
		spec := td.ToToolSpec()

		switch td.Type {
		case "builtin":
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
			reg.Register(spec, builder)

		case "exec", "":
			if td.Binary == "" {
				return fmt.Errorf("exec tool %q has no binary", td.Name)
			}
			builder := &ExecBuilder{Def: td, Root: root}
			reg.Register(spec, builder)

		default:
			return fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
	}
	return nil
}
