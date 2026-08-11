// Copyright (c) 2026 Nokia. All rights reserved.

package registry

// FactoryRegistrar registers a concrete builtin family into a BuiltinRegistry.
type FactoryRegistrar func(*BuiltinRegistry)

// StandardFactoryDeps holds concrete builtin family hooks.
type StandardFactoryDeps struct {
	RegisterFilesystem     FactoryRegistrar
	RegisterLLM            FactoryRegistrar
	RegisterLifecycle      FactoryRegistrar
	RegisterControl        FactoryRegistrar
	RegisterPlanning       FactoryRegistrar
	RegisterEvaluation     FactoryRegistrar
	RegisterSpecValidation FactoryRegistrar
	RegisterREST           FactoryRegistrar
	RegisterCompose        FactoryRegistrar
	RegisterService        FactoryRegistrar
	RegisterOTLP           FactoryRegistrar
}

// StandardFactoryCatalogEntry describes one selected-init-gated factory family.
type StandardFactoryCatalogEntry struct {
	Name     string
	Inits    []string
	register FactoryRegistrar
}

// SelectedBy reports whether any entry init is selected.
func (e StandardFactoryCatalogEntry) SelectedBy(selected map[string]bool) bool {
	for _, init := range e.Inits {
		if selected[init] {
			return true
		}
	}
	return false
}

// Register invokes the concrete registrar for this factory family.
func (e StandardFactoryCatalogEntry) Register(br *BuiltinRegistry) {
	if e.register != nil {
		e.register(br)
	}
}

// RegisterStandardBuiltinFactories registers only selected standard families.
func RegisterStandardBuiltinFactories(br *BuiltinRegistry, selected map[string]bool, deps StandardFactoryDeps) {
	for _, entry := range StandardFactoryCatalog(deps) {
		if entry.SelectedBy(selected) {
			entry.Register(br)
		}
	}
}

// StandardFactoryCatalog returns the standard selected-init factory families.
func StandardFactoryCatalog(deps StandardFactoryDeps) []StandardFactoryCatalogEntry {
	return []StandardFactoryCatalogEntry{
		hookFactory("filesystem", deps.RegisterFilesystem),
		hookFactory("llm", deps.RegisterLLM),
		hookFactory("lifecycle", deps.RegisterLifecycle),
		hookFactory("control", deps.RegisterControl),
		hookFactory("planning", deps.RegisterPlanning),
		hookFactory("evaluation", deps.RegisterEvaluation),
		hookFactory("spec_validation", deps.RegisterSpecValidation),
		hookFactory("rest", deps.RegisterREST),
		hookFactory("compose", deps.RegisterCompose),
		hookFactory("otlp", deps.RegisterOTLP),
		hookFactory("service", deps.RegisterService),
	}
}

func hookFactory(name string, hook FactoryRegistrar) StandardFactoryCatalogEntry {
	entry := StandardFactoryCatalogEntry{Name: name, register: hook}
	if hook == nil {
		return entry
	}

	probe := NewBuiltinRegistry()
	hook(probe)
	entry.Inits = probe.Names()
	return entry
}
