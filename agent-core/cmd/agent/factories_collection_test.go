// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestCollectionFactoriesRejectMalformedConfigAtRegistration(t *testing.T) {
	tests := []struct {
		name string
		def  catalog.ToolDef
	}{
		{
			name: "partition",
			def: catalog.ToolDef{Name: "partition", Type: "builtin", Init: "partition", Config: map[string]interface{}{
				"items": "$.items", "field": "value", "op": "eq", "right": "x",
				"operand_type": "string", "satisfied": "Partitioned",
			}},
		},
		{
			name: "select_subset",
			def: catalog.ToolDef{Name: "select_subset", Type: "builtin", Init: "select_subset", Config: map[string]interface{}{
				"candidates": "$from(c).names", "vocabulary": "$from(v).names",
				"match_field": "name", "all_matched": "All", "partial": "Partial",
			}},
		},
		{
			name: "render_each",
			def: catalog.ToolDef{Name: "render_each", Type: "builtin", Init: "render_each", Config: map[string]interface{}{
				"items": "$from(v).items", "item_template": "{{ bad path }}", "signal": "Rendered",
			}},
		},
		{
			name: "parse_structured",
			def: catalog.ToolDef{Name: "parse_structured", Type: "builtin", Init: "parse_structured", Config: map[string]interface{}{
				"source": "$from(response).value", "schema": map[string]interface{}{"type": 7},
				"parsed": "Parsed", "unparsed": "Unparsed",
			}},
		},
		{
			name: "report_parse_error",
			def: catalog.ToolDef{Name: "report_parse_error", Type: "builtin", Init: "report_parse_error", Config: map[string]interface{}{
				"response_contract": "unknown",
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builtins := toolregistry.NewBuiltinRegistry()
			registerBuiltinFactories(builtins, &agentState{}, map[string]bool{tc.def.Init: true})
			err := toolregistry.RegisterSingleBuiltin(core.NewRegistry(), builtins, tc.def, nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.def.Name)
		})
	}
}

func TestCollectionFactoriesRegisterValidConfig(t *testing.T) {
	defs := []catalog.ToolDef{
		{Name: "partition", Type: "builtin", Init: "partition", Config: map[string]interface{}{
			"items": "$from(v).items", "field": "value", "op": "eq", "right": "x",
			"operand_type": "string", "satisfied": "Partitioned",
		}},
		{Name: "select_subset", Type: "builtin", Init: "select_subset", Config: map[string]interface{}{
			"candidates": "$from(c).names", "vocabulary": "$from(v).names", "match_field": "name",
			"all_matched": "All", "partial": "Partial", "empty": "Empty",
		}},
		{Name: "render_each", Type: "builtin", Init: "render_each", Config: map[string]interface{}{
			"items": "$from(v).items", "item_template": "{{ name }}", "signal": "Rendered",
		}},
		{Name: "parse_structured", Type: "builtin", Init: "parse_structured", Config: map[string]interface{}{
			"source": "$from(response).value",
			"schema": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"parsed": "Parsed", "unparsed": "Unparsed",
		}},
		{Name: "report_parse_error", Type: "builtin", Init: "report_parse_error", Config: map[string]interface{}{
			"response_contract": "implementation_plan_yaml",
		}},
	}
	for _, def := range defs {
		t.Run(def.Name, func(t *testing.T) {
			builtins := toolregistry.NewBuiltinRegistry()
			registerBuiltinFactories(builtins, &agentState{}, map[string]bool{def.Init: true})
			reg := core.NewRegistry()
			require.NoError(t, toolregistry.RegisterSingleBuiltin(reg, builtins, def, nil))
			_, ok := reg.Resolve(def.Name)
			require.True(t, ok)
		})
	}
}
