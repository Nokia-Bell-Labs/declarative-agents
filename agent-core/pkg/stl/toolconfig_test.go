// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeToolConfigBasic(t *testing.T) {
	def := ToolDef{
		Name: "test_tool",
		Config: map[string]interface{}{
			"machine": "configs/gen/machine.yaml",
			"tools":   "configs/gen/tools.yaml",
			"tools_declarations": []interface{}{
				"configs/tools/builtin.yaml",
				"configs/tools/exec.yaml",
			},
		},
	}
	var cfg ChildAgentConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "configs/gen/machine.yaml", cfg.Machine)
	assert.Equal(t, "configs/gen/tools.yaml", cfg.Tools)
	assert.Equal(t, []string{"configs/tools/builtin.yaml", "configs/tools/exec.yaml"}, cfg.ToolDeclarations)
}

func TestDecodeToolConfigNilConfig(t *testing.T) {
	def := ToolDef{Name: "empty", Config: nil}
	var cfg ChildAgentConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Empty(t, cfg.Machine)
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

func TestDecodeToolConfigExtraFieldsIgnored(t *testing.T) {
	def := ToolDef{
		Name: "test",
		Config: map[string]interface{}{
			"machine":    "m.yaml",
			"extra_junk": "ignored",
		},
	}
	var cfg ChildAgentConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "m.yaml", cfg.Machine)
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
			"point_machine": "configs/evaluator/point.yaml",
			"point_tools":   "configs/evaluator/tools-point.yaml",
		},
	}
	var cfg RunPointConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, "configs/evaluator/point.yaml", cfg.PointMachine)
	assert.Equal(t, "configs/evaluator/tools-point.yaml", cfg.PointTools)
}

func TestDecodeToolConfigServeUI(t *testing.T) {
	def := ToolDef{
		Name: "serve_ui",
		Config: map[string]interface{}{
			"addr":         ":8080",
			"data_dir":     "eval-results",
			"configs_dir":  "configs",
			"docs_dir":     "docs",
			"profiles_dir": "pkg/llm/profiles",
		},
	}
	var cfg ServeUIToolConfig
	require.NoError(t, DecodeToolConfig(def, &cfg))
	assert.Equal(t, ":8080", cfg.Addr)
	assert.Equal(t, "eval-results", cfg.DataDir)
	assert.Equal(t, "configs", cfg.ConfigsDir)
	assert.Equal(t, "docs", cfg.DocsDir)
	assert.Equal(t, "pkg/llm/profiles", cfg.ProfilesDir)
	assert.Empty(t, cfg.SourceDir)
}
