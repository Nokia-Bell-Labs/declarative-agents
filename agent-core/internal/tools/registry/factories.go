// Copyright (c) 2026 Nokia. All rights reserved.

package registry

// FactoryRegistrar registers a concrete builtin family into a BuiltinRegistry.
type FactoryRegistrar func(*BuiltinRegistry)

// StandardFactoryDeps holds concrete builtin family hooks.
type StandardFactoryDeps struct {
	RegisterFilesystem     FactoryRegistrar
	RegisterLLM            FactoryRegistrar
	RegisterLifecycle      FactoryRegistrar
	RegisterValidation     FactoryRegistrar
	RegisterControl        FactoryRegistrar
	RegisterPlanning       FactoryRegistrar
	RegisterEvaluation     FactoryRegistrar
	RegisterBench          FactoryRegistrar
	RegisterSpecValidation FactoryRegistrar
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
		hookFactory("filesystem", []string{"file_read", "file_write", "file_edit", "file_find", "file_list"}, deps.RegisterFilesystem),
		hookFactory("llm", []string{"invoke_llm", "parse_response", "report_parse_error", "reset_history", "nudge_reread", "done"}, deps.RegisterLLM),
		hookFactory("lifecycle", []string{"suspend", "checkpoint_history", "checkpoint_rollback"}, deps.RegisterLifecycle),
		hookFactory("validation", []string{"validate"}, deps.RegisterValidation),
		hookFactory("control", []string{"self_invoke"}, deps.RegisterControl),
		hookFactory("planning", []string{"extract_task", "extract_all", "assemble_prompt", "parse_plan", "create_issue", "execute_task", "check_result"}, deps.RegisterPlanning),
		hookFactory("evaluation", []string{"parse_suite_config", "discover_suite_samples", "expand_eval_grid", "init_eval_session", "report_suite_summary", "next_point", "run_point", "report_session", "run_agent", "run_oracle_check", "collect_trace_tokens", "check_agent_version", "summarize_point_results", "collect_metrics", "dump_config"}, deps.RegisterEvaluation),
		hookFactory("bench", []string{"serve_ui", "launch_eval"}, deps.RegisterBench),
		hookFactory("spec_validation", []string{"load_corpus", "validate_specs", "format_report"}, deps.RegisterSpecValidation),
	}
}

func hookFactory(name string, inits []string, hook FactoryRegistrar) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: name, Inits: inits, register: hook}
}
