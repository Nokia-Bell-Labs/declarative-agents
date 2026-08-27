// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitCompose        = "compose"
	InitRenderEach     = "render_each"
	InitProject        = "project"
	InitFlatMap        = "flat_map"
	InitReorderByIndex = "reorder_by_index"
)

// RegisterFactories registers compose builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry) {
	br.Register(InitCompose, composeFactory)
	br.Register(InitRenderEach, renderEachFactory)
	br.Register(InitProject, projectFactory)
	br.Register(InitFlatMap, flatMapFactory)
	br.Register(InitReorderByIndex, reorderByIndexFactory)
}

func composeFactory(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
	var cfg catalog.ComposeConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := ValidateConfig(def.Name, cfg.Inputs); err != nil {
		return nil, err
	}
	return Builder{
		ToolName: def.Name, Template: cfg.Template, Inputs: cfg.Inputs,
		Signal: core.Signal(cfg.Signal),
	}, nil
}

func renderEachFactory(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
	var cfg catalog.RenderEachConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := ValidateRenderEachConfig(def.Name, cfg.Items, cfg.ItemTemplate, cfg.Signal); err != nil {
		return nil, err
	}
	return RenderEachBuilder{
		ToolName: def.Name, Items: cfg.Items, ItemTemplate: cfg.ItemTemplate,
		Separator: cfg.Separator, Signal: core.Signal(cfg.Signal),
	}, nil
}

func projectFactory(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
	var cfg catalog.ProjectConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := ValidateProjectConfig(def.Name, cfg.Items, cfg.Field, cfg.Signal); err != nil {
		return nil, err
	}
	return ProjectBuilder{
		ToolName: def.Name, Items: cfg.Items, Field: cfg.Field, Signal: core.Signal(cfg.Signal),
	}, nil
}

func flatMapFactory(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
	var cfg catalog.FlatMapConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := ValidateFlatMapConfig(
		def.Name, cfg.Items, cfg.ElementFields, cfg.CarryFields, cfg.Signal,
	); err != nil {
		return nil, err
	}
	return FlatMapBuilder{
		ToolName: def.Name, Items: cfg.Items, ElementFields: cfg.ElementFields,
		CarryFields: cfg.CarryFields, Signal: core.Signal(cfg.Signal),
	}, nil
}

func reorderByIndexFactory(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
	var cfg catalog.ReorderByIndexConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := ValidateReorderByIndexConfig(
		def.Name, cfg.Items, cfg.Order, cfg.IndexField, cfg.Signal,
	); err != nil {
		return nil, err
	}
	return ReorderByIndexBuilder{
		ToolName: def.Name, Items: cfg.Items, Order: cfg.Order,
		IndexField: cfg.IndexField, Signal: core.Signal(cfg.Signal),
	}, nil
}
