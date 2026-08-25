// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	want := []string{
		InitInvokeLLM, InitParseResponse, InitParseStructured, InitReportParseError,
		InitResetHistory, InitNudgeReread, InitDone,
	}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{})
	require.ElementsMatch(t, want, br.Names())

	require.ElementsMatch(t, want, catalogInits(t, "llm", toolregistry.StandardFactoryDeps{
		RegisterLLM: func(br *toolregistry.BuiltinRegistry) { RegisterFactories(br, FactoryDeps{}) },
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
		{Name: "parse_structured", Type: "builtin", Init: InitParseStructured, Config: map[string]interface{}{
			"source": "$from(response).value", "schema": map[string]interface{}{"type": 7},
			"parsed": "Parsed", "unparsed": "Unparsed",
		}},
		{Name: "report_parse_error", Type: "builtin", Init: InitReportParseError, Config: map[string]interface{}{
			"response_contract": "unknown",
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
		{Name: "parse_structured", Type: "builtin", Init: InitParseStructured, Config: map[string]interface{}{
			"source": "$from(response).value",
			"schema": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"parsed": "Parsed", "unparsed": "Unparsed",
		}},
		{Name: "report_parse_error", Type: "builtin", Init: InitReportParseError, Config: map[string]interface{}{
			"feedback_template": "Correct {{error}} as YAML.",
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

func TestNudgeRereadAndReportParseErrorReachBuilders(t *testing.T) {
	t.Parallel()

	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{})

	nudgeFactory, ok := br.Resolve(InitNudgeReread)
	require.True(t, ok)
	nudgeBuilder, err := nudgeFactory(catalog.ToolDef{
		Name: "nudge", Config: map[string]interface{}{"nudge_text": "custom reread"},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "custom reread", nudgeBuilder.(*control.NudgeRereadBuilder).Text)

	reportFactory, ok := br.Resolve(InitReportParseError)
	require.True(t, ok)
	reportBuilder, err := reportFactory(catalog.ToolDef{
		Name: "report", Config: map[string]interface{}{"feedback_template": "fix {{error}}"},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "fix {{error}}", reportBuilder.(*ReportParseErrorBuilder).FeedbackTemplate)
}

func TestParseFactoriesPreserveDeclarationNamesInReversers(t *testing.T) {
	t.Parallel()

	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{
		Registry:     core.NewRegistry(),
		ParseRetries: &ParseErrorRetryTracker{MaxConsecutive: 3},
	})

	defs := []catalog.ToolDef{
		{Name: "decode_response", Type: "builtin", Init: InitParseResponse},
		{
			Name: "explain_parse_failure", Type: "builtin", Init: InitReportParseError,
			Config: map[string]interface{}{"feedback_template": "fix {{error}}"},
		},
	}
	reg := core.NewRegistry()
	for _, def := range defs {
		require.NoError(t, toolregistry.RegisterSingleBuiltin(reg, br, def, nil))

		builder, ok := reg.Resolve(def.Name)
		require.True(t, ok)
		require.Equal(t, def.Name, builder.Build(core.Result{}).Name())

		reverser, ok := builder.(core.Reverser)
		require.True(t, ok)
		require.Equal(t, def.Name, reverser.BuildReverser().Name())
	}
}

func TestParseResponseBeforeInvokeLLMKeepsZeroParserAndLiveManifestState(t *testing.T) {
	t.Parallel()

	resolved := &ResolvedModel{}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Resolved: resolved, Registry: core.NewRegistry()})

	parseFactory, ok := br.Resolve(InitParseResponse)
	require.True(t, ok)
	builder, err := parseFactory(catalog.ToolDef{Name: "parse"}, nil)
	require.NoError(t, err)
	parseBuilder := builder.(*ParseResponseBuilder)
	require.Nil(t, parseBuilder.Parser)
	require.Empty(t, parseBuilder.StateFunc())

	invokeFactory, ok := br.Resolve(InitInvokeLLM)
	require.True(t, ok)
	_, err = invokeFactory(catalog.ToolDef{
		Name: "chat", Type: "builtin", Init: InitInvokeLLM,
		Config: map[string]interface{}{
			"model": "mock-model", "manifest_state": "Calling",
		},
	}, nil)
	require.NoError(t, err)

	require.Equal(t, "mock-model", resolved.Model)
	require.Equal(t, "ollama", resolved.ProviderName)
	require.Equal(t, core.State("Calling"), resolved.ManifestState)
	require.NotNil(t, resolved.Parser)
	require.Nil(t, parseBuilder.Parser)
	require.Equal(t, core.State("Calling"), parseBuilder.StateFunc())
}

func TestRequestLocalResolvedModelDoesNotShareHostParser(t *testing.T) {
	t.Parallel()

	host := &ResolvedModel{Model: "host-model", ManifestState: "Hosting"}
	local := &ResolvedModel{}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Resolved: local, Registry: core.NewRegistry()})

	parseFactory, ok := br.Resolve(InitParseResponse)
	require.True(t, ok)
	builder, err := parseFactory(catalog.ToolDef{Name: "parse"}, nil)
	require.NoError(t, err)
	require.Empty(t, builder.(*ParseResponseBuilder).StateFunc())
	require.Equal(t, "host-model", host.Model)
}
