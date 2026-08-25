// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitCompose    = "compose"
	InitRenderEach = "render_each"
)

// RegisterFactories registers compose builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry) {
	br.Register(InitCompose, func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.ComposeConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := ValidateConfig(def.Name, cfg.Inputs); err != nil {
			return nil, err
		}
		return Builder{
			ToolName: def.Name,
			Template: cfg.Template,
			Inputs:   cfg.Inputs,
			Signal:   core.Signal(cfg.Signal),
		}, nil
	})
	br.Register(InitRenderEach, func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
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
	})
}
