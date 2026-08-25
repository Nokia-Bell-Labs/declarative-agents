// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
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

func standardCatalog(st *agentState) []toolregistry.StandardFactoryCatalogEntry {
	return toolregistry.StandardFactoryCatalog(standardFactoryDeps(st))
}

func TestFactoryRegistrarsProbeTwiceWithZeroValueDeps(t *testing.T) {
	t.Parallel()
	st := &agentState{}
	var first, second [][]string
	require.NotPanics(t, func() {
		first = catalogInitSets(standardCatalog(st))
		second = catalogInitSets(standardCatalog(st))
	})
	require.Equal(t, first, second)
}

func TestCatalogProbeDoesNotWriteServiceReap(t *testing.T) {
	t.Parallel()
	st := &agentState{}
	kept := false
	st.reapServices = func() { kept = true }
	_ = standardCatalog(st)
	registerServiceFactories(st)(toolregistry.NewBuiltinRegistry())
	st.reapServices()
	require.True(t, kept)
}

func TestNewAgentStateServiceStateSurvivesCatalogProbe(t *testing.T) {
	t.Parallel()
	st := newAgentState(runtimeConfig{}, agentStateDeps{Ctx: context.Background()})
	require.NotNil(t, st.services)
	require.NotNil(t, st.reapServices)
	svc := st.services
	_ = standardCatalog(st)
	registerServiceFactories(st)(toolregistry.NewBuiltinRegistry())
	require.Same(t, svc, st.services)
}

func TestRequestLocalStateAllocatesOwnServiceState(t *testing.T) {
	t.Parallel()
	host := newAgentState(runtimeConfig{}, agentStateDeps{Ctx: context.Background()})
	local := requestLocalState(host, core.NewRegistry())
	require.NotNil(t, local.services)
	require.NotSame(t, host.services, local.services)
}

func catalogInitSets(entries []toolregistry.StandardFactoryCatalogEntry) [][]string {
	out := make([][]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, append([]string(nil), entry.Inits...))
	}
	return out
}

func TestSpecValidationCatalogInitsMatchPackageRegistration(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	validation.RegisterSpecFactories(br, validation.FactoryDeps{})
	var catalogInits []string
	for _, entry := range standardCatalog(&agentState{}) {
		if entry.Name == "spec_validation" {
			catalogInits = entry.Inits
			break
		}
	}
	require.Equal(t, br.Names(), catalogInits)
}
