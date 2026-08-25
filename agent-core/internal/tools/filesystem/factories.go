// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package filesystem

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitFileRead     = "file_read"
	InitFileWrite    = "file_write"
	InitFileEdit     = "file_edit"
	InitFileFind     = "file_find"
	InitListResource = "list_resource"
	InitReadResource = "read_resource"
)

// RegisterFactories registers filesystem builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry) {
	fileFactories := []struct {
		init    string
		builder func(string, core.MetricConfig, string) core.Builder
	}{
		{InitFileRead, func(root string, metrics core.MetricConfig, _ string) core.Builder {
			return &ReadBuilder{Root: root, Metrics: metrics}
		}},
		{InitFileWrite, func(root string, metrics core.MetricConfig, strategy string) core.Builder {
			return &WriteBuilder{Root: root, UndoStrategy: strategy, Metrics: metrics}
		}},
		{InitFileEdit, func(root string, metrics core.MetricConfig, strategy string) core.Builder {
			return &EditBuilder{Root: root, UndoStrategy: strategy, Metrics: metrics}
		}},
	}
	for _, entry := range fileFactories {
		registerFileFactory(br, entry.init, entry.builder)
	}
	br.Register(InitFileFind, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &FindBuilder{Root: vars["directory"], OutputLineCap: def.OutputCap}, nil
	})
	registerResourceFactories(br)
}

func registerFileFactory(br *toolregistry.BuiltinRegistry, init string, builder func(string, core.MetricConfig, string) core.Builder) {
	br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return builder(vars["directory"], def.Metrics, def.Undo.Strategy), nil
	})
}

func registerResourceFactories(br *toolregistry.BuiltinRegistry) {
	br.Register(InitListResource, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg, err := resourceConfig(def)
		if err != nil {
			return nil, err
		}
		return &ListResourceBuilder{Root: vars["directory"], Resources: cfg}, nil
	})
	br.Register(InitReadResource, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg, err := resourceConfig(def)
		if err != nil {
			return nil, err
		}
		return &ReadResourceBuilder{Root: vars["directory"], Resources: cfg}, nil
	})
}

func resourceConfig(def catalog.ToolDef) (ResourceConfig, error) {
	var cfg ResourceConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return ResourceConfig{}, err
	}
	return cfg, nil
}
