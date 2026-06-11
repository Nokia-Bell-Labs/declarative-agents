// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// RegisterValidateFactories registers spec validation builtin tool
// factories (load_corpus, validate_specs, format_report) into the
// provided BuiltinRegistry. Validation state is lazily initialized on
// first factory call.
func RegisterValidateFactories(br *BuiltinRegistry, directory string) {
	var vs *ValidateSpecState

	initVS := func() *ValidateSpecState {
		if vs != nil {
			return vs
		}
		vs = &ValidateSpecState{Directory: directory}
		return vs
	}

	br.Register("load_corpus", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if dir := vars["directory"]; dir != "" {
			s.Directory = dir
		}
		return &LoadCorpusBuilder{VS: s}, nil
	})
	br.Register("validate_specs", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ValidateSpecsBuilder{VS: initVS()}, nil
	})
	br.Register("format_report", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &FormatReportBuilder{VS: initVS()}, nil
	})
}
