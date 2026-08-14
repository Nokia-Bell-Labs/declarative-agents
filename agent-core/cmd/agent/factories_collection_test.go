// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
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
			name: "compose",
			def: catalog.ToolDef{Name: "compose", Type: "builtin", Init: "compose", Config: map[string]interface{}{
				"template": "{{ value }}", "inputs": map[string]string{"value": "bad-selector"},
				"signal": "Composed",
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

func TestProfilePolicyReachesBuiltinBuilders(t *testing.T) {
	t.Parallel()

	filesystemFactories := toolregistry.NewBuiltinRegistry()
	registerFilesystemFactories()(filesystemFactories)
	findFactory, ok := filesystemFactories.Resolve("file_find")
	require.True(t, ok)
	findBuilder, err := findFactory(catalog.ToolDef{OutputCap: 17}, map[string]string{"directory": "/tmp"})
	require.NoError(t, err)
	require.Equal(t, 17, findBuilder.(*filesystem.FindBuilder).OutputLineCap)

	llmFactories := toolregistry.NewBuiltinRegistry()
	registerLLMFactories(&agentState{})(llmFactories)
	nudgeFactory, ok := llmFactories.Resolve("nudge_reread")
	require.True(t, ok)
	nudgeBuilder, err := nudgeFactory(catalog.ToolDef{
		Name: "nudge", Config: map[string]interface{}{"nudge_text": "custom reread"},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "custom reread", nudgeBuilder.(*control.NudgeRereadBuilder).Text)

	reportFactory, ok := llmFactories.Resolve("report_parse_error")
	require.True(t, ok)
	reportBuilder, err := reportFactory(catalog.ToolDef{
		Name: "report", Config: map[string]interface{}{"feedback_template": "fix {{error}}"},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "fix {{error}}", reportBuilder.(*toollm.ReportParseErrorBuilder).FeedbackTemplate)
}

func TestLLMParseFactoriesPreserveDeclarationNamesInReversers(t *testing.T) {
	st := &agentState{
		registry:     core.NewRegistry(),
		parseRetries: &toollm.ParseErrorRetryTracker{MaxConsecutive: 3},
	}
	builtins := toolregistry.NewBuiltinRegistry()
	registerLLMFactories(st)(builtins)

	defs := []catalog.ToolDef{
		{Name: "decode_response", Type: "builtin", Init: "parse_response"},
		{
			Name: "explain_parse_failure", Type: "builtin", Init: "report_parse_error",
			Config: map[string]interface{}{"feedback_template": "fix {{error}}"},
		},
	}
	reg := core.NewRegistry()
	for _, def := range defs {
		require.NoError(t, toolregistry.RegisterSingleBuiltin(reg, builtins, def, nil))

		builder, ok := reg.Resolve(def.Name)
		require.True(t, ok)
		require.Equal(t, def.Name, builder.Build(core.Result{}).Name())

		reverser, ok := builder.(core.Reverser)
		require.True(t, ok)
		require.Equal(t, def.Name, reverser.BuildReverser().Name())
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
		{Name: "compose", Type: "builtin", Init: "compose", Config: map[string]interface{}{
			"template": "{{ value }}", "inputs": map[string]string{"value": "$from(v).value"},
			"signal": "Composed",
		}},
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
