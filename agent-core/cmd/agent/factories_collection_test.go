// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestCollectionFactoriesRejectMalformedConfigAtRegistration(t *testing.T) {
	tests := []struct {
		name string
		def  catalog.ToolDef
	}{
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
		{Name: "parse_structured", Type: "builtin", Init: "parse_structured", Config: map[string]interface{}{
			"source": "$from(response).value",
			"schema": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"parsed": "Parsed", "unparsed": "Unparsed",
		}},
		{Name: "report_parse_error", Type: "builtin", Init: "report_parse_error", Config: map[string]interface{}{
			"feedback_template": "Correct {{error}} as YAML.",
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

func TestLLMDoneInitRegistersWholeFactoryFamily(t *testing.T) {
	t.Parallel()

	builtins := toolregistry.NewBuiltinRegistry()
	registerBuiltinFactories(builtins, &agentState{}, map[string]bool{toollm.InitDone: true})
	require.ElementsMatch(t, []string{
		toollm.InitInvokeLLM,
		toollm.InitParseResponse,
		toollm.InitParseStructured,
		toollm.InitReportParseError,
		toollm.InitResetHistory,
		toollm.InitNudgeReread,
		toollm.InitDone,
	}, builtins.Names())
}
