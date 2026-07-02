// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

// RegisterSpecFactories registers spec validation builtin tool factories.
func RegisterSpecFactories(br *toolregistry.BuiltinRegistry, directory string) {
	var vs *SpecState
	initVS := func() *SpecState {
		if vs == nil {
			vs = &SpecState{Directory: directory}
		}
		return vs
	}
	br.Register("load_corpus", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if dir := vars["directory"]; dir != "" {
			s.Directory = dir
		}
		return &LoadCorpusBuilder{VS: s}, nil
	})
	br.Register("validate_specs", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ValidateSpecsBuilder{VS: initVS()}, nil
	})
	br.Register("format_report", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &FormatReportBuilder{VS: initVS()}, nil
	})
}
