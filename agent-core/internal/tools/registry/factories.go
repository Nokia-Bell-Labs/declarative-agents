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
	RegisterREST           FactoryRegistrar
	RegisterDocumentation  FactoryRegistrar
	RegisterCompose        FactoryRegistrar
	RegisterService        FactoryRegistrar
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
		hookFactory("filesystem", []string{"file_read", "file_write", "file_edit", "file_find", "file_list", "list_resource", "read_resource"}, deps.RegisterFilesystem),
		hookFactory("llm", []string{"invoke_llm", "parse_response", "report_parse_error", "reset_history", "nudge_reread", "done"}, deps.RegisterLLM),
		hookFactory("lifecycle", []string{"suspend", "checkpoint_history", "checkpoint_rollback", "exit_agent"}, deps.RegisterLifecycle),
		hookFactory("validation", []string{"validate"}, deps.RegisterValidation),
		hookFactory("control", []string{"self_invoke"}, deps.RegisterControl),
		hookFactory("planning", []string{"load_graph", "extract_task", "extract_all", "assemble_prompt", "parse_plan", "create_issue", "execute_task", "check_result"}, deps.RegisterPlanning),
		hookFactory("evaluation", []string{"parse_suite_config", "discover_suite_samples", "expand_eval_grid", "init_eval_session", "report_suite_summary", "next_point", "run_point", "report_session", "run_agent", "run_oracle_check", "collect_trace_tokens", "check_agent_version", "summarize_point_results", "collect_metrics", "dump_config"}, deps.RegisterEvaluation),
		hookFactory("bench", []string{"serve_ui", "launch_eval"}, deps.RegisterBench),
		hookFactory("spec_validation", []string{"load_corpus", "validate_specs", "format_report"}, deps.RegisterSpecValidation),
		hookFactory("rest", []string{"rest_client_get", "rest_client_set", "rest_client_create", "rest_client_delete", "rest_client_invoke", "rest_client_send", "rest_client_await", "rest_server_launch", "rest_server_await", "rest_server_stop", "rest_await_event"}, deps.RegisterREST),
		hookFactory("documentation", []string{"launch_documentation", "stop_documentation"}, deps.RegisterDocumentation),
		hookFactory("compose", []string{"compose"}, deps.RegisterCompose),
		// The rig's service words. The init names are literal here because the
		// service package imports this one, so the list cannot be read from it.
		hookFactory("service", []string{
			"start_service", "await_healthy", "stop_service", "run_validators", "list_scenarios",
			"init_scenario_session", "next_scenario", "start_scenario_twins",
			"start_scenario_subject", "await_scenario_subject", "run_scenario_validators",
			"collect_scenario_verdict", "teardown_scenario", "report_scenario_session",
		}, deps.RegisterService),
	}
}

func hookFactory(name string, inits []string, hook FactoryRegistrar) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: name, Inits: inits, register: hook}
}
