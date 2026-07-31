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
		hookFactory("filesystem", []string{"file_read", "file_write", "file_edit", "file_find", "list_resource", "read_resource"}, deps.RegisterFilesystem),
		hookFactory("llm", []string{"invoke_llm", "parse_response", "parse_structured", "report_parse_error", "reset_history", "nudge_reread", "done"}, deps.RegisterLLM),
		hookFactory("lifecycle", []string{"delay", "suspend", "checkpoint_history", "checkpoint_rollback", "exit_agent"}, deps.RegisterLifecycle),
		hookFactory("control", []string{"self_invoke", "value_predicate", "partition", "select_subset"}, deps.RegisterControl),
		hookFactory("planning", []string{"load_graph", "extract_task", "select_all_ready", "seed_passthrough_plan", "mark_nodes_planning", "project_planner_context", "capture_planner_failure", "parse_plan", "format_issue", "record_tracker_issue", "mark_nodes_executing", "format_task_file", "mark_task_done", "mark_task_failed", "remaining_work"}, deps.RegisterPlanning),
		hookFactory("evaluation", []string{"parse_suite_config", "discover_suite_samples", "expand_eval_grid", "init_eval_session", "report_suite_summary", "materialize_eval_points", "run_point", "report_session", "run_agent", "record_oracle_result", "collect_trace_tokens", "check_agent_version", "summarize_point_results", "collect_metrics", "record_agent_commit", "dump_config", "list_evaluation_sessions", "analyze_evaluation_session", "list_evaluation_points", "read_evaluation_trace"}, deps.RegisterEvaluation),
		hookFactory("spec_validation", []string{"load_corpus", "validate_specs", "format_report"}, deps.RegisterSpecValidation),
		hookFactory("rest", []string{"rest_client_get", "rest_client_set", "rest_client_create", "rest_client_delete", "rest_client_invoke", "rest_client_send", "rest_client_await", "rest_server_launch", "rest_server_await", "rest_server_stop", "rest_await_event"}, deps.RegisterREST),
		hookFactory("compose", []string{"compose", "render_each"}, deps.RegisterCompose),
		hookFactory("otlp", []string{
			"otlp_receiver_launch", "await_spans", "load_otlp_batch", "spool_spans", "relay_spans", "otlp_receiver_stop",
			"spool_list_traces", "spool_get_trace",
		}, deps.RegisterOTLP),
		// The rig's service words. The init names are literal here because the
		// service package imports this one, so the list cannot be read from it.
		hookFactory("service", []string{
			"start_service", "stop_service", "list_scenarios",
			"init_scenario_session", "next_scenario", "start_scenario_mock",
			"start_scenario_subject", "run_scenario_validator", "record_scenario_validators",
			"collect_scenario_verdict", "list_scenario_children", "report_scenario_session",
		}, deps.RegisterService),
	}
}

func hookFactory(name string, inits []string, hook FactoryRegistrar) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: name, Inits: inits, register: hook}
}
