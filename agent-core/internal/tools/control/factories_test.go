// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package control

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	want := []string{InitSelfInvoke, InitValuePredicate, InitPartition, InitSelectSubset}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{})
	require.ElementsMatch(t, want, br.Names())

	require.ElementsMatch(t, want, catalogInits(t, "control", toolregistry.StandardFactoryDeps{
		RegisterControl: func(br *toolregistry.BuiltinRegistry) { RegisterFactories(br, FactoryDeps{}) },
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
		{Name: "partition", Type: "builtin", Init: InitPartition, Config: map[string]interface{}{
			"items": "$.items", "field": "value", "op": "eq", "right": "x",
			"operand_type": "string", "satisfied": "Partitioned",
		}},
		{Name: "select_subset", Type: "builtin", Init: InitSelectSubset, Config: map[string]interface{}{
			"candidates": "$from(c).names", "vocabulary": "$from(v).names",
			"match_field": "name", "all_matched": "All", "partial": "Partial",
		}},
	}
	for _, def := range tests {
		t.Run(def.Name, func(t *testing.T) {
			br := toolregistry.NewBuiltinRegistry()
			RegisterFactories(br, FactoryDeps{})
			err := toolregistry.RegisterSingleBuiltin(core.NewRegistry(), br, def, nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), def.Name)
		})
	}
}

func TestRegisterFactoriesAcceptsValidConfig(t *testing.T) {
	t.Parallel()

	tests := []catalog.ToolDef{
		{Name: "partition", Type: "builtin", Init: InitPartition, Config: map[string]interface{}{
			"items": "$from(v).items", "field": "value", "op": "eq", "right": "x",
			"operand_type": "string", "satisfied": "Partitioned",
		}},
		{Name: "select_subset", Type: "builtin", Init: InitSelectSubset, Config: map[string]interface{}{
			"candidates": "$from(c).names", "vocabulary": "$from(v).names", "match_field": "name",
			"all_matched": "All", "partial": "Partial", "empty": "Empty",
		}},
	}
	for _, def := range tests {
		t.Run(def.Name, func(t *testing.T) {
			br := toolregistry.NewBuiltinRegistry()
			RegisterFactories(br, FactoryDeps{})
			reg := core.NewRegistry()
			require.NoError(t, toolregistry.RegisterSingleBuiltin(reg, br, def, nil))
			_, ok := reg.Resolve(def.Name)
			require.True(t, ok)
		})
	}
}
