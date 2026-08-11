// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeToolConfigBasic(t *testing.T) {
	def := ToolDef{
		Name: "test_tool",
		Config: map[string]interface{}{
			"profile": "agents/gen/profile.yaml",
		},
	}
	var cfg ChildAgentConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "agents/gen/profile.yaml", cfg.Profile)
}

func TestDecodeToolConfigNilConfig(t *testing.T) {
	def := ToolDef{Name: "empty", Config: nil}
	var cfg ChildAgentConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Empty(t, cfg.Profile)
}

func TestDecodeToolConfigIntegers(t *testing.T) {
	def := ToolDef{
		Name: "llm",
		Config: map[string]interface{}{
			"model":          "qwen3:8b",
			"manifest_state": "Composing",
			"num_ctx":        4096,
			"max_time":       float64(600),
		},
	}
	var cfg LLMToolConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "qwen3:8b", cfg.Model)
	assert.Equal(t, "Composing", cfg.ManifestState)
	assert.Equal(t, 4096, cfg.NumCtx)
	assert.Equal(t, 600, cfg.MaxTime)
}

func TestDecodeToolConfigTypeMismatch(t *testing.T) {
	def := ToolDef{
		Name: "bad_tool",
		Config: map[string]interface{}{
			"num_ctx": "not-an-int",
		},
	}
	var cfg LLMToolConfig
	err := DecodeToolConfig(def, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tool "bad_tool" config`)
}

func TestDecodeToolConfigRejectsUnknownField(t *testing.T) {
	def := ToolDef{
		Name: "test",
		Config: map[string]interface{}{
			"profile":    "agents/gen/profile.yaml",
			"extra_junk": "ignored",
		},
	}
	var cfg ChildAgentConfig
	err := DecodeToolConfig(def, &cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tool "test" config`)
	assert.Contains(t, err.Error(), `unknown field "extra_junk"`)
}

func TestDecodeToolConfigLoadSuite(t *testing.T) {
	def := ToolDef{
		Name: "load_suite",
		Config: map[string]interface{}{
			"output_dir": "eval-results",
			"reps":       3,
			"timeout":    300,
			"ollama_url": "http://localhost:11434",
		},
	}
	var cfg LoadSuiteConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "eval-results", cfg.OutputDir)
	assert.Equal(t, 3, cfg.Reps)
	assert.Equal(t, 300, cfg.Timeout)
	assert.Equal(t, "http://localhost:11434", cfg.OllamaURL)
}

func TestDecodeToolConfigRunPoint(t *testing.T) {
	def := ToolDef{
		Name: "run_point",
		Config: map[string]interface{}{
			"point_machine":           "agents/critic/point.yaml",
			"point_tools":             "agents/critic/tools-point.yaml",
			"point_tool_declarations": []interface{}{"tools/builtin/critic-point/all.yaml", "tools/exec/all.yaml"},
		},
	}
	var cfg RunPointConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "agents/critic/point.yaml", cfg.PointMachine)
	assert.Equal(t, "agents/critic/tools-point.yaml", cfg.PointTools)
	assert.Equal(t, []string{"tools/builtin/critic-point/all.yaml", "tools/exec/all.yaml"}, cfg.PointToolDeclarations)
}

func TestDecodeToolConfigEvaluationArtifacts(t *testing.T) {
	def := ToolDef{
		Name: "list_evaluation_sessions",
		Config: map[string]interface{}{
			"data_dir": "eval-results",
		},
	}
	var cfg EvaluationArtifactsConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "eval-results", cfg.DataDir)
}
