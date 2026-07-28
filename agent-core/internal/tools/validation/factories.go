// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

type specValidationConfig struct {
	SuitePaths     []string `json:"suite_paths"`
	CharterSuites  []string `json:"charter_suites"`
	Charters       []string `json:"charters"`
	CorpusOptional bool     `json:"corpus_optional"`
	ResultsFrom    string   `json:"results_from"`
	ModuleFrom     string   `json:"module_from"`
	PackagesFrom   string   `json:"packages_from"`
	TestsFrom      string   `json:"tests_from"`
	RunFrom        string   `json:"run_from"`
}

// RegisterSpecFactories registers spec validation builtin tool factories.
func RegisterSpecFactories(br *toolregistry.BuiltinRegistry, directory string) {
	var vs *SpecState
	initVS := func() *SpecState {
		if vs == nil {
			vs = &SpecState{Directory: directory, TargetDirectory: directory}
		}
		return vs
	}
	registerLoadCorpusFactory(br, initVS)
	registerLoadTestClaimsFactory(br, initVS)
	registerValidateSpecsFactory(br, initVS)
	registerReduceConsistencyFactory(br, initVS)
	registerReduceRefFactory(br, initVS)
	registerReduceGrepFactory(br, initVS)
	registerResolveTestEvidenceFactory(br, initVS)
	registerReduceTestEvidenceRunFactory(br, initVS)
	registerFormatReportFactory(br, initVS)
}

func registerReduceConsistencyFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("reduce_consistency_checks", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg specValidationConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.ResultsFrom == "" {
			cfg.ResultsFrom = "$from(consistency_results).items"
		}
		return &ReduceConsistencyChecksBuilder{VS: initVS(), ResultsFrom: cfg.ResultsFrom}, nil
	})
}

func registerReduceRefFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("reduce_ref_checks", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg specValidationConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.ResultsFrom == "" {
			cfg.ResultsFrom = "$from(ref_results).items"
		}
		return &ReduceRefChecksBuilder{VS: initVS(), ResultsFrom: cfg.ResultsFrom}, nil
	})
}

func registerLoadTestClaimsFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("load_test_claims", func(_ catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if dir := vars["directory"]; dir != "" {
			s.Directory = dir
			s.TargetDirectory = dir
		}
		return &LoadTestClaimsBuilder{VS: s}, nil
	})
}

func registerResolveTestEvidenceFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("resolve_test_evidence", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg specValidationConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.ModuleFrom == "" {
			cfg.ModuleFrom = "$from(go_module).output"
		}
		if cfg.PackagesFrom == "" {
			cfg.PackagesFrom = "$from(go_packages).output"
		}
		if cfg.TestsFrom == "" {
			cfg.TestsFrom = "$from(go_test_inventory).output"
		}
		return &ResolveTestEvidenceBuilder{
			VS: initVS(), ModuleFrom: cfg.ModuleFrom,
			PackagesFrom: cfg.PackagesFrom, TestsFrom: cfg.TestsFrom,
		}, nil
	})
}

func registerReduceTestEvidenceRunFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("reduce_test_evidence_run", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg specValidationConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.RunFrom == "" {
			cfg.RunFrom = "$from(go_test_run).output"
		}
		return &ReduceTestEvidenceRunBuilder{VS: initVS(), RunFrom: cfg.RunFrom}, nil
	})
}

func registerLoadCorpusFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("load_corpus", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if dir := vars["directory"]; dir != "" {
			s.Directory = dir
			s.TargetDirectory = dir
		}
		if err := applySpecValidationConfig(s, def, vars); err != nil {
			return nil, err
		}
		return &LoadCorpusBuilder{VS: s}, nil
	})
}

func registerValidateSpecsFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("validate_specs", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if err := applySpecValidationConfig(s, def, vars); err != nil {
			return nil, err
		}
		return &ValidateSpecsBuilder{VS: s}, nil
	})
}

func registerReduceGrepFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("reduce_grep_checks", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		var cfg specValidationConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.ResultsFrom == "" {
			cfg.ResultsFrom = "$from(grep_results).items"
		}
		return &ReduceGrepChecksBuilder{VS: s, ResultsFrom: cfg.ResultsFrom}, nil
	})
}

func registerFormatReportFactory(br *toolregistry.BuiltinRegistry, initVS func() *SpecState) {
	br.Register("format_report", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if err := applySpecValidationConfig(s, def, vars); err != nil {
			return nil, err
		}
		return &FormatReportBuilder{VS: s}, nil
	})
}

func applySpecValidationConfig(s *SpecState, def catalog.ToolDef, vars map[string]string) error {
	var cfg specValidationConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return err
	}
	paths := append([]string(nil), cfg.SuitePaths...)
	paths = append(paths, cfg.CharterSuites...)
	paths = append(paths, cfg.Charters...)
	paths = append(paths, splitSuitePaths(vars["suite_paths"])...)
	paths = append(paths, splitSuitePaths(vars["charter_suites"])...)
	if len(paths) > 0 {
		s.SuitePaths = paths
	}
	if cfg.CorpusOptional || truthyVar(vars["corpus_optional"]) {
		s.CorpusOptional = true
	}
	return nil
}

func truthyVar(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func splitSuitePaths(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paths = append(paths, part)
		}
	}
	return paths
}
