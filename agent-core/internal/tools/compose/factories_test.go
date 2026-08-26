// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	want := []string{InitCompose, InitRenderEach, InitFlatMap, InitReorderByIndex}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br)
	require.ElementsMatch(t, want, br.Names())

	require.ElementsMatch(t, want, catalogInits(t, "compose", toolregistry.StandardFactoryDeps{
		RegisterCompose: RegisterFactories,
	}))
}

func catalogInits(t *testing.T, family string, deps toolregistry.StandardFactoryDeps) []string {
	t.Helper()
	for _, entry := range toolregistry.StandardFactoryCatalog(deps) {
		if entry.Name == family {
			return entry.Inits
		}
	}
	t.Fatalf("standard catalog missing family %q", family)
	return nil
}

func TestRegisterFactoriesRejectsMalformedConfig(t *testing.T) {
	t.Parallel()

	tests := []catalog.ToolDef{
		{Name: "compose", Type: "builtin", Init: InitCompose, Config: map[string]interface{}{
			"template": "{{ value }}", "inputs": map[string]string{"value": "bad-selector"},
			"signal": "Composed",
		}},
		{Name: "render_each", Type: "builtin", Init: InitRenderEach, Config: map[string]interface{}{
			"items": "$from(v).items", "item_template": "{{ bad path }}", "signal": "Rendered",
		}},
		{Name: "flat_map", Type: "builtin", Init: InitFlatMap, Config: map[string]interface{}{
			"items": "$from(v).items", "element_fields": map[string]string{}, "signal": "Flattened",
		}},
		{Name: "reorder_by_index", Type: "builtin", Init: InitReorderByIndex, Config: map[string]interface{}{
			"items": "$from(v).items", "order": "$from(v).order", "signal": "Reordered",
		}},
	}
	for _, def := range tests {
		t.Run(def.Name, func(t *testing.T) {
			br := toolregistry.NewBuiltinRegistry()
			RegisterFactories(br)
			err := toolregistry.RegisterSingleBuiltin(core.NewRegistry(), br, def, nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), def.Name)
		})
	}
}

func TestRegisterFactoriesAcceptsValidConfig(t *testing.T) {
	t.Parallel()

	tests := []catalog.ToolDef{
		{Name: "render_each", Type: "builtin", Init: InitRenderEach, Config: map[string]interface{}{
			"items": "$from(v).items", "item_template": "{{ name }}", "signal": "Rendered",
		}},
		{Name: "compose", Type: "builtin", Init: InitCompose, Config: map[string]interface{}{
			"template": "{{ value }}", "inputs": map[string]string{"value": "$from(v).value"},
			"signal": "Composed",
		}},
		{Name: "flat_map", Type: "builtin", Init: InitFlatMap, Config: map[string]interface{}{
			"items": "$from(v).items", "element_fields": map[string]string{"id": "ids.0"},
			"carry_fields": map[string]string{"source": "name"}, "signal": "Flattened",
		}},
		{Name: "reorder_by_index", Type: "builtin", Init: InitReorderByIndex, Config: map[string]interface{}{
			"items": "$from(v).items", "order": "$from(v).order",
			"index_field": "index", "signal": "Reordered",
		}},
	}
	for _, def := range tests {
		t.Run(def.Name, func(t *testing.T) {
			br := toolregistry.NewBuiltinRegistry()
			RegisterFactories(br)
			reg := core.NewRegistry()
			require.NoError(t, toolregistry.RegisterSingleBuiltin(reg, br, def, nil))
			_, ok := reg.Resolve(def.Name)
			require.True(t, ok)
		})
	}
}
