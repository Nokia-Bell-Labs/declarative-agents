// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

type specValidationConfig struct {
	SuitePaths    []string `json:"suite_paths"`
	CharterSuites []string `json:"charter_suites"`
	Charters      []string `json:"charters"`
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
	br.Register("validate_specs", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if err := applySpecValidationConfig(s, def, vars); err != nil {
			return nil, err
		}
		return &ValidateSpecsBuilder{VS: s}, nil
	})
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
	return nil
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
